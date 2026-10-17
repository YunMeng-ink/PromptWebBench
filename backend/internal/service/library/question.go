package library

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Question 是题库题目的服务层视图。
//
// 引用题的标题与正文取自来源提示词的最新版本（需求 §7.4），派生题取自自身存储
// （C-28）。两者都带出 SourceAccess：内容源自哪条提示词，就按那条提示词的可见性
// 判定，否则派生副本会成为绕开 C-20 的通道。
type Question struct {
	ID               int64
	LibraryID        int64
	Library          domain.Library
	Kind             domain.LibraryQuestionKind
	Title            string
	Body             string
	Description      string
	PurposeID        *int64
	PurposeNameZh    string
	PurposeNameEn    string
	CapabilityID     *int64
	CapabilityNameZh string
	CapabilityNameEn string
	DifficultyID     *int64
	Sensitive        domain.SensitiveLevel
	SortOrder        int32
	Tags             []string

	SourcePromptID      *int64
	SourceVersionNo     string
	SourceAvailable     bool
	SourceMajorModified bool

	// Deleted 为真表示题目在题库回收站中。
	Deleted   bool
	DeletedAt time.Time
	DeletedBy *int64
	ExpiresAt time.Time

	CreatedAt time.Time
	UpdatedAt time.Time

	// SourceAccess 是题目内容所属提示词的可见性要素；为空表示来源行不存在
	// （无来源的派生题，或彻底删除被绕过后残留的异常行）。
	SourceAccess *domain.Access
}

// canView 判定当前身份能否看到这道题目。
//
// 两层判定：题库成员身份（非成员一律不可见）与内容所属提示词的可见性。
// 后者复用 domain.Viewer.CanView，与提示词详情、收藏夹条目同出一条规则（C-20）。
// 有来源但来源行缺失时按不可见处理（fail-closed）：彻底删除被引用检查阻止，
// 此分支只在数据异常时兜底；无来源的派生题不在此列。
func (q Question) canView(actor domain.LibraryActor) bool {
	if !actor.Can(domain.CapViewLibrary) {
		return false
	}
	if q.SourceAccess == nil {
		return q.SourcePromptID == nil
	}
	return actor.Viewer.CanView(*q.SourceAccess)
}

// QuestionPage 是题目列表的一页。
type QuestionPage struct {
	Items    []Question
	Page     int32
	PageSize int32
	Total    int64
}

// AddQuestionInput 是添加题目的输入。
//
// PromptID 非空表示添加引用题，否则用 Title 与 Body 添加派生题（需求 §10.7）。
type AddQuestionInput struct {
	PromptID     *int64
	Title        string
	Body         string
	Description  string
	PurposeID    *int64
	CapabilityID *int64
	DifficultyID *int64
	Sensitive    domain.SensitiveLevel
	Tags         []string
}

// UpdateQuestionInput 是修改题目内容的输入。
type UpdateQuestionInput struct {
	Title        string
	Body         string
	Description  string
	PurposeID    *int64
	CapabilityID *int64
	DifficultyID *int64
	Sensitive    domain.SensitiveLevel
	Note         string
}

// AddQuestion 向题库添加题目。
//
// 引用题的容量判定在事务内完成（C-21）：插入后条件递增题库计数，
// 计数已满时递增不命中，返回错误让整个事务回滚。
func (s *Service) AddQuestion(ctx context.Context, v domain.Viewer, libraryID int64, in AddQuestionInput) (Question, error) {
	if err := s.ensure(); err != nil {
		return Question{}, notFound("题库")
	}
	if !v.CanWrite() {
		return Question{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := validateAddQuestion(in); err != nil {
		return Question{}, err
	}

	var created Question
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, domain.CapAddQuestion, "当前身份不可以向题库添加题目"); err != nil {
			return err
		}

		maxOrder, err := q.MaxLibraryQuestionSortOrder(ctx, libraryID)
		if err != nil {
			return wrap("读取题库题目顺序", libraryID, err)
		}

		var questionID int64
		if in.PromptID != nil {
			questionID, err = s.insertReferenceQuestion(ctx, q, v, lib, *in.PromptID, maxOrder+1, in.Tags)
		} else {
			questionID, err = s.insertDerivedQuestion(ctx, q, lib, in, maxOrder+1)
		}
		if err != nil {
			return err
		}

		question, err := s.loadQuestion(ctx, q, actor, lib, questionID, false)
		if err != nil {
			return err
		}
		created = question
		return nil
	})
	if err != nil {
		return Question{}, err
	}
	return created, nil
}

// insertReferenceQuestion 写入引用题并占用容量。
func (s *Service) insertReferenceQuestion(
	ctx context.Context, q *repo.Queries, v domain.Viewer, lib domain.Library,
	promptID int64, sortOrder int32, tags []string,
) (int64, error) {
	// 锁定来源提示词行：与彻底删除流程互斥。清理方先锁行并复查引用计数，
	// 若本事务先取得锁，清理方会看到新插入的引用而放弃删除；若清理方先删除，
	// 此处锁定的读取不命中（行已不在或已删除），引用写入随之失败。
	prompt, err := q.LockPrompt(ctx, promptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, notFound("提示词")
		}
		return 0, wrap("查询来源提示词", promptID, err)
	}
	access, err := promptAccessFromColumns(prompt.ID, prompt.Visibility, prompt.IsHighRisk, prompt.UploaderID)
	if err != nil {
		return 0, err
	}
	// 只能引用自己看得到的提示词：普通用户无法把开发者可见或高风险提示词
	// 加入题库，否则引用题会成为读取受限内容的入口。
	if !v.CanView(access) {
		return 0, notFound("提示词")
	}

	row, err := q.InsertLibraryQuestion(ctx, repo.InsertLibraryQuestionParams{
		LibraryID:             lib.ID,
		Kind:                  string(domain.QuestionReference),
		SourcePromptID:        int8OrNull(&promptID),
		SourcePromptVersionID: prompt.CurrentVersionID,
		SortOrder:             sortOrder,
	})
	if err != nil {
		return 0, wrap("写入引用题", lib.ID, err)
	}

	limit := s.questionLimit(ctx)
	if _, err := q.IncrementLibraryQuestionCount(ctx, repo.IncrementLibraryQuestionCountParams{
		ID:         lib.ID,
		LimitCount: limit,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, apperr.Errorf(apperr.CodeQuotaExceeded, "题库最多容纳 %d 道引用题", limit)
		}
		return 0, wrap("更新题库题目计数", lib.ID, err)
	}
	if err := s.attachTags(ctx, q, row.ID, tags); err != nil {
		return 0, err
	}
	return row.ID, nil
}

// insertDerivedQuestion 写入派生题。
//
// 派生题不占用题库容量（C-22）：它是题库内的独立副本，不计入 150 题上限。
func (s *Service) insertDerivedQuestion(
	ctx context.Context, q *repo.Queries, lib domain.Library, in AddQuestionInput, sortOrder int32,
) (int64, error) {
	sensitive := in.Sensitive
	if sensitive == "" {
		sensitive = domain.SensitiveLow
	}
	row, err := q.InsertLibraryQuestion(ctx, repo.InsertLibraryQuestionParams{
		LibraryID:      lib.ID,
		Kind:           string(domain.QuestionDerived),
		Title:          textOrNull(in.Title),
		Body:           textOrNull(in.Body),
		Description:    in.Description,
		PurposeID:      int8OrNull(in.PurposeID),
		CapabilityID:   int8OrNull(in.CapabilityID),
		DifficultyID:   int8OrNull(in.DifficultyID),
		SensitiveLevel: textOrNull(string(sensitive)),
		SortOrder:      sortOrder,
	})
	if err != nil {
		return 0, wrap("写入派生题", lib.ID, err)
	}
	if err := s.attachTags(ctx, q, row.ID, in.Tags); err != nil {
		return 0, err
	}
	return row.ID, nil
}

// ListQuestions 返回题库内当前身份可见的题目。
//
// 先逐条复查可见性再分页：不可见的题目要从结果里剔除而不是占一个位置，
// 否则分页会出现空页，总数与列表口径也不一致（与收藏夹条目同一处理）。
func (s *Service) ListQuestions(
	ctx context.Context, v domain.Viewer, libraryID int64, page, pageSize int32,
) (QuestionPage, error) {
	if err := s.ensure(); err != nil {
		return QuestionPage{}, notFound("题库")
	}

	q := repo.New(s.pool)
	lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
	if err != nil {
		return QuestionPage{}, err
	}
	visible, err := s.visibleQuestions(ctx, q, actor, lib, false)
	if err != nil {
		return QuestionPage{}, err
	}

	offset := int((page - 1) * pageSize)
	limit := int(pageSize)
	if offset > len(visible) {
		offset = len(visible)
	}
	end := offset + limit
	if end > len(visible) {
		end = len(visible)
	}
	return QuestionPage{
		Items:    visible[offset:end],
		Page:     page,
		PageSize: pageSize,
		Total:    int64(len(visible)),
	}, nil
}

// GetQuestion 返回单道题目。不可见与不存在同样返回 404（开发方案 §5.3）。
func (s *Service) GetQuestion(ctx context.Context, v domain.Viewer, questionID int64) (Question, error) {
	if err := s.ensure(); err != nil {
		return Question{}, notFound("题目")
	}

	q := repo.New(s.pool)
	ref, err := q.GetLibraryQuestionRef(ctx, repo.GetLibraryQuestionRefParams{
		ID:             questionID,
		IncludeDeleted: false,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Question{}, notFound("题目")
		}
		return Question{}, wrap("查询题目", questionID, err)
	}
	lib, actor, err := s.loadVisibleLibrary(ctx, q, v, ref.LibraryID)
	if err != nil {
		return Question{}, err
	}
	return s.loadQuestion(ctx, q, actor, lib, questionID, false)
}

// UpdateQuestion 修改题目内容。
//
// 引用题被修改时原地转为派生题（需求 §10.7）：来源提示词不受影响，
// 修改历史与修改人记录在题目修订表中（C-28：派生题不参与版本机制）。
func (s *Service) UpdateQuestion(
	ctx context.Context, v domain.Viewer, questionID int64, in UpdateQuestionInput,
) (Question, error) {
	if err := s.ensure(); err != nil {
		return Question{}, notFound("题目")
	}
	if !v.CanWrite() {
		return Question{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := validateUpdateQuestion(in); err != nil {
		return Question{}, err
	}

	var updated Question
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		ref, err := q.GetLibraryQuestionRef(ctx, repo.GetLibraryQuestionRefParams{
			ID:             questionID,
			IncludeDeleted: false,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("题目")
			}
			return wrap("查询题目", questionID, err)
		}
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, ref.LibraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, domain.CapUpdateQuestion, "当前身份不可以修改题目内容"); err != nil {
			return err
		}
		before, err := s.loadQuestion(ctx, q, actor, lib, questionID, false)
		if err != nil {
			return err
		}

		sensitive := in.Sensitive
		if sensitive == "" {
			sensitive = before.Sensitive
		}
		params := repo.DeriveLibraryQuestionParams{
			ID:             questionID,
			Title:          textOrNull(in.Title),
			Body:           textOrNull(in.Body),
			Description:    in.Description,
			PurposeID:      int8OrNull(in.PurposeID),
			CapabilityID:   int8OrNull(in.CapabilityID),
			DifficultyID:   int8OrNull(in.DifficultyID),
			SensitiveLevel: textOrNull(string(sensitive)),
		}
		var after repo.LibraryQuestion
		if before.Kind == domain.QuestionReference {
			// 引用题内容被修改即成为派生题：保留来源提示词标识供追溯。
			after, err = q.DeriveLibraryQuestion(ctx, params)
			if err == nil {
				// 容量只计引用题（C-22）：引用转派生后立即归还容量，
				// 否则容量被永久占用、用户在未达上限时被提前拒绝。
				err = q.DecrementLibraryQuestionCount(ctx, ref.LibraryID)
				if err != nil {
					return wrap("归还题库题目计数", ref.LibraryID, err)
				}
			}
		} else {
			after, err = q.UpdateLibraryQuestionContent(ctx, repo.UpdateLibraryQuestionContentParams(params))
		}
		if err != nil {
			return wrap("更新题目内容", questionID, err)
		}

		if err := s.writeRevision(ctx, q, v.UserID, questionID, before, in); err != nil {
			return err
		}
		question, err := s.loadQuestion(ctx, q, actor, lib, after.ID, false)
		if err != nil {
			return err
		}
		updated = question
		return nil
	})
	if err != nil {
		return Question{}, err
	}
	return updated, nil
}

// ReorderQuestions 调整题目顺序（需求 §10.4、§10.5）。
//
// 传入的标识序列即目标顺序，序号从 1 开始；不属于本库的标识被忽略，
// 未出现在序列中的题目保持原有顺序并排在最后。
func (s *Service) ReorderQuestions(ctx context.Context, v domain.Viewer, libraryID int64, ids []int64) error {
	if err := s.ensure(); err != nil {
		return notFound("题库")
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if len(ids) == 0 {
		return apperr.New(apperr.CodeValidationFailed, "题目顺序不能为空")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, domain.CapReorderQuestion, "当前身份不可以调整题目顺序"); err != nil {
			return err
		}

		visible, err := s.visibleQuestions(ctx, q, actor, lib, false)
		if err != nil {
			return err
		}
		known := make(map[int64]bool, len(visible))
		for _, item := range visible {
			known[item.ID] = true
		}

		orders := make([]int32, 0, len(ids))
		targets := make([]int64, 0, len(ids))
		next := int32(1)
		for _, id := range ids {
			if !known[id] {
				continue
			}
			targets = append(targets, id)
			orders = append(orders, next)
			next++
		}
		if len(targets) == 0 {
			return apperr.New(apperr.CodeValidationFailed, "没有可调整顺序的题目")
		}
		if err := q.ReorderLibraryQuestions(ctx, repo.ReorderLibraryQuestionsParams{
			LibraryID: libraryID,
			Ids:       targets,
			Orders:    orders,
		}); err != nil {
			return wrap("调整题目顺序", libraryID, err)
		}
		return nil
	})
}

// AddTag 给题目贴标签（需求 §10.4、§10.5）。
func (s *Service) AddTag(ctx context.Context, v domain.Viewer, questionID int64, tag string) error {
	return s.mutateTags(ctx, v, questionID, domain.CapTagQuestion, "当前身份不可以给题目贴标签", func(q *repo.Queries) error {
		if len([]rune(tag)) == 0 || len([]rune(tag)) > domain.LibraryQuestionTagMaxRunes {
			return validateError(errTagLength)
		}
		return q.InsertLibraryQuestionTag(ctx, repo.InsertLibraryQuestionTagParams{
			QuestionID: questionID,
			Tag:        tag,
		})
	})
}

// RemoveTag 移除题目标签（需求 §10.4、§10.5）。
func (s *Service) RemoveTag(ctx context.Context, v domain.Viewer, questionID int64, tag string) error {
	return s.mutateTags(ctx, v, questionID, domain.CapUntagQuestion, "当前身份不可以移除题目标签", func(q *repo.Queries) error {
		return q.DeleteLibraryQuestionTag(ctx, repo.DeleteLibraryQuestionTagParams{
			QuestionID: questionID,
			Tag:        tag,
		})
	})
}

// mutateTags 是标签增删的公共流程：定位题库、判定角色、确认题目可见后执行动作。
func (s *Service) mutateTags(
	ctx context.Context, v domain.Viewer, questionID int64,
	cap domain.LibraryCapability, denyMessage string, action func(*repo.Queries) error,
) error {
	if err := s.ensure(); err != nil {
		return notFound("题目")
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		ref, err := q.GetLibraryQuestionRef(ctx, repo.GetLibraryQuestionRefParams{
			ID:             questionID,
			IncludeDeleted: false,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("题目")
			}
			return wrap("查询题目", questionID, err)
		}
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, ref.LibraryID)
		if err != nil {
			return err
		}
		if err := requireCapability(actor, cap, denyMessage); err != nil {
			return err
		}
		if _, err := s.loadQuestion(ctx, q, actor, lib, questionID, false); err != nil {
			return err
		}
		if err := action(q); err != nil {
			return wrap("更新题目标签", questionID, err)
		}
		return nil
	})
}

// ListRevisions 返回题目的修改记录（需求 §10.7 保留修改记录与修改人）。
func (s *Service) ListRevisions(ctx context.Context, v domain.Viewer, questionID int64) ([]Revision, error) {
	if err := s.ensure(); err != nil {
		return nil, notFound("题目")
	}

	q := repo.New(s.pool)
	ref, err := q.GetLibraryQuestionRef(ctx, repo.GetLibraryQuestionRefParams{
		ID:             questionID,
		IncludeDeleted: true,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFound("题目")
		}
		return nil, wrap("查询题目", questionID, err)
	}
	lib, actor, err := s.loadVisibleLibrary(ctx, q, v, ref.LibraryID)
	if err != nil {
		return nil, err
	}
	if _, err := s.loadQuestion(ctx, q, actor, lib, questionID, true); err != nil {
		return nil, err
	}

	rows, err := q.ListLibraryQuestionRevisions(ctx, questionID)
	if err != nil {
		return nil, wrap("查询题目修订", questionID, err)
	}
	out := make([]Revision, 0, len(rows))
	for _, row := range rows {
		item := Revision{
			ID:         row.ID,
			EditorID:   row.EditorID,
			EditorName: row.EditorName,
			Note:       row.Note,
			Before:     row.Before,
			After:      row.After,
		}
		if row.CreatedAt.Valid {
			item.CreatedAt = row.CreatedAt.Time
		}
		out = append(out, item)
	}
	return out, nil
}

// Revision 是一条题目修改记录。
type Revision struct {
	ID         int64
	EditorID   int64
	EditorName string
	Note       string
	Before     []byte
	After      []byte
	CreatedAt  time.Time
}

// writeRevision 记录一次题目修改（需求 §10.7）。
func (s *Service) writeRevision(
	ctx context.Context, q *repo.Queries, editorID, questionID int64, before Question, in UpdateQuestionInput,
) error {
	beforeData, err := encodeJSON(map[string]any{
		"title":       before.Title,
		"body":        before.Body,
		"description": before.Description,
		"kind":        string(before.Kind),
	})
	if err != nil {
		return err
	}
	afterData, err := encodeJSON(map[string]any{
		"title":       in.Title,
		"body":        in.Body,
		"description": in.Description,
		"kind":        string(domain.QuestionDerived),
	})
	if err != nil {
		return err
	}
	if err := q.InsertLibraryQuestionRevision(ctx, repo.InsertLibraryQuestionRevisionParams{
		QuestionID: questionID,
		EditorID:   editorID,
		Before:     beforeData,
		After:      afterData,
		Note:       in.Note,
	}); err != nil {
		return wrap("写入题目修订记录", questionID, err)
	}
	return nil
}

// visibleQuestions 返回题库内对当前身份可见的题目。
//
// 可见性谓词只存在于 domain（题库成员身份与内容所属提示词的可见性），
// 因此这里逐条复查而不是在 SQL 里另写一份（开发方案 §3.1）。
func (s *Service) visibleQuestions(
	ctx context.Context, q *repo.Queries, actor domain.LibraryActor, lib domain.Library, includeDeleted bool,
) ([]Question, error) {
	rows, err := q.ListLibraryQuestions(ctx, repo.ListLibraryQuestionsParams{
		LibraryID:      lib.ID,
		IncludeDeleted: includeDeleted,
	})
	if err != nil {
		return nil, wrap("查询题库题目", lib.ID, err)
	}

	tags, err := s.tagsByQuestion(ctx, q, rows)
	if err != nil {
		return nil, err
	}

	out := make([]Question, 0, len(rows))
	for _, row := range rows {
		question, err := questionFromRow(row, lib)
		if err != nil {
			return nil, err
		}
		if !question.canView(actor) {
			continue
		}
		// 没有标签的题目也要给空列表而不是空值：接口约定列表一律为 []（C-40），
		// 空值会让前端在读长度时崩掉。
		question.Tags = tags[row.ID]
		if question.Tags == nil {
			question.Tags = []string{}
		}
		if row.DeletedAt.Valid {
			question.Deleted = true
			question.DeletedAt = row.DeletedAt.Time
		}
		out = append(out, question)
	}
	return out, nil
}

// loadQuestion 读取单道题目。不可见与不存在一样返回 404。
func (s *Service) loadQuestion(
	ctx context.Context, q *repo.Queries, actor domain.LibraryActor, lib domain.Library,
	questionID int64, includeDeleted bool,
) (Question, error) {
	items, err := s.visibleQuestions(ctx, q, actor, lib, includeDeleted)
	if err != nil {
		return Question{}, err
	}
	for _, item := range items {
		if item.ID == questionID {
			return item, nil
		}
	}
	return Question{}, notFound("题目")
}

// tagsByQuestion 批量读取题目标签，避免逐题查询。
func (s *Service) tagsByQuestion(
	ctx context.Context, q *repo.Queries, rows []repo.ListLibraryQuestionsRow,
) (map[int64][]string, error) {
	out := make(map[int64][]string, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	tagRows, err := q.ListLibraryQuestionTagsByQuestions(ctx, ids)
	if err != nil {
		return nil, wrap("查询题目标签", 0, err)
	}
	for _, row := range tagRows {
		out[row.QuestionID] = append(out[row.QuestionID], row.Tag)
	}
	return out, nil
}

// attachTags 写入题目标签。重复标签由唯一约束吸收，超过数量上限直接拒绝。
func (s *Service) attachTags(ctx context.Context, q *repo.Queries, questionID int64, tags []string) error {
	normalized, err := normalizeTags(tags)
	if err != nil {
		return err
	}
	for _, tag := range normalized {
		if err := q.InsertLibraryQuestionTag(ctx, repo.InsertLibraryQuestionTagParams{
			QuestionID: questionID,
			Tag:        tag,
		}); err != nil {
			return wrap("写入题目标签", questionID, err)
		}
	}
	return nil
}

// normalizeTags 去重并校验标签。空标签被丢弃，同类标签只保留一个。
func normalizeTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if len([]rune(tag)) == 0 {
			continue
		}
		if len([]rune(tag)) > domain.LibraryQuestionTagMaxRunes {
			return nil, validateError(errTagLength)
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	if len(out) > domain.LibraryQuestionTagCountLimit {
		return nil, apperr.Errorf(apperr.CodeValidationFailed,
			"单题标签不能超过 %d 个", domain.LibraryQuestionTagCountLimit)
	}
	return out, nil
}

// validateAddQuestion 校验添加题目的输入。
func validateAddQuestion(in AddQuestionInput) error {
	if in.PromptID != nil {
		if len([]rune(in.Title)) != 0 || len([]rune(in.Body)) != 0 {
			return apperr.New(apperr.CodeValidationFailed, "引用题不接受正文，正文取自来源提示词")
		}
		if _, err := normalizeTags(in.Tags); err != nil {
			return err
		}
		return nil
	}
	if err := domain.ValidateLibraryQuestionTitle(in.Title); err != nil {
		return validateError(err)
	}
	if err := domain.ValidateLibraryQuestionBody(in.Body); err != nil {
		return validateError(err)
	}
	if err := domain.ValidateLibraryQuestionDescription(in.Description); err != nil {
		return validateError(err)
	}
	if _, err := normalizeTags(in.Tags); err != nil {
		return err
	}
	return nil
}

// validateUpdateQuestion 校验修改题目的输入。
func validateUpdateQuestion(in UpdateQuestionInput) error {
	if err := domain.ValidateLibraryQuestionTitle(in.Title); err != nil {
		return validateError(err)
	}
	if err := domain.ValidateLibraryQuestionBody(in.Body); err != nil {
		return validateError(err)
	}
	if err := domain.ValidateLibraryQuestionDescription(in.Description); err != nil {
		return validateError(err)
	}
	if len([]rune(in.Note)) > revisionNoteMaxRunes {
		return apperr.Errorf(apperr.CodeValidationFailed, "修改说明不能超过 %d 个字", revisionNoteMaxRunes)
	}
	return nil
}

// revisionNoteMaxRunes 是题目修改说明的字数上限。
const revisionNoteMaxRunes = 500
