package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/service/prompt"
)

// PromptFilter 是后台提示词列表的筛选条件。空串与 nil 表示该维度不参与筛选。
type PromptFilter struct {
	Keyword    string
	Visibility string
	// Deleted 取 ""（不限）、"true"（仅回收站）或 "false"（仅在库）。
	Deleted    string
	CategoryID *int64
	UploaderID *int64
}

// ListPrompts 返回提示词列表（需求 §17.2「查看所有提示词、搜索、筛选」）。
func (s *Service) ListPrompts(ctx context.Context, f PromptFilter, page, pageSize int32) (Page[repo.AdminListPromptsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListPromptsRow]{}, err
	}
	if f.Visibility != "" {
		if _, ok := domain.ParseVisibility(f.Visibility); !ok {
			return Page[repo.AdminListPromptsRow]{}, apperr.New(apperr.CodeValidationFailed, "可见性取值非法")
		}
	}
	if f.Deleted != "" && f.Deleted != "true" && f.Deleted != "false" {
		return Page[repo.AdminListPromptsRow]{}, apperr.New(apperr.CodeValidationFailed, "删除状态取值非法")
	}

	q := repo.New(s.pool)
	params := repo.AdminListPromptsParams{
		Keyword:     f.Keyword,
		Visibility:  f.Visibility,
		Deleted:     f.Deleted,
		CategoryID:  repo.Int8OrNull(deref(f.CategoryID)),
		UploaderID:  repo.Int8OrNull(deref(f.UploaderID)),
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	}
	rows, err := q.AdminListPrompts(ctx, params)
	if err != nil {
		return Page[repo.AdminListPromptsRow]{}, fmt.Errorf("查询提示词列表: %w", err)
	}
	total, err := q.AdminCountPrompts(ctx, repo.AdminCountPromptsParams{
		Keyword:    f.Keyword,
		Visibility: f.Visibility,
		Deleted:    f.Deleted,
		CategoryID: params.CategoryID,
		UploaderID: params.UploaderID,
	})
	if err != nil {
		return Page[repo.AdminListPromptsRow]{}, fmt.Errorf("统计提示词数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// PromptDetail 是后台提示词详情：提示词本体、标签与全部版本。
type PromptDetail struct {
	Prompt   repo.GetPromptIncludingDeletedRow
	Tags     []repo.Tag
	Versions []repo.PromptVersion
}

// GetPrompt 读取提示词详情，包含回收站中的条目（需求 §17.2「查看提示词详情、查看提示词版本」）。
func (s *Service) GetPrompt(ctx context.Context, id int64) (PromptDetail, error) {
	if err := s.ensure(); err != nil {
		return PromptDetail{}, err
	}
	q := repo.New(s.pool)
	row, err := q.GetPromptIncludingDeleted(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PromptDetail{}, apperr.New(apperr.CodeNotFound, "")
		}
		return PromptDetail{}, fmt.Errorf("查询提示词: %w", err)
	}
	tags, err := q.ListPromptTags(ctx, id)
	if err != nil {
		return PromptDetail{}, fmt.Errorf("查询提示词标签: %w", err)
	}
	versions, err := q.ListPromptVersions(ctx, id)
	if err != nil {
		return PromptDetail{}, fmt.Errorf("查询提示词版本: %w", err)
	}
	return PromptDetail{Prompt: row, Tags: tags, Versions: versions}, nil
}

// ListPromptRatings 返回提示词的评分记录（需求 §17.2「查看评分记录」）。
func (s *Service) ListPromptRatings(ctx context.Context, promptID int64, page, pageSize int32) (Page[repo.AdminListPromptRatingsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListPromptRatingsRow]{}, err
	}
	q := repo.New(s.pool)
	rows, err := q.AdminListPromptRatings(ctx, repo.AdminListPromptRatingsParams{
		PromptID: promptID, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListPromptRatingsRow]{}, fmt.Errorf("查询评分记录: %w", err)
	}
	total, err := q.AdminCountPromptRatings(ctx, promptID)
	if err != nil {
		return Page[repo.AdminListPromptRatingsRow]{}, fmt.Errorf("统计评分记录: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// ListPromptComments 返回提示词的评论（需求 §17.2「查看评论」），包含已删除的评论：
// 管理员需要看到被举报内容在被删除前的原文。
func (s *Service) ListPromptComments(ctx context.Context, promptID int64, page, pageSize int32) (Page[repo.AdminListPromptCommentsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListPromptCommentsRow]{}, err
	}
	q := repo.New(s.pool)
	rows, err := q.AdminListPromptComments(ctx, repo.AdminListPromptCommentsParams{
		PromptID: promptID, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListPromptCommentsRow]{}, fmt.Errorf("查询评论: %w", err)
	}
	total, err := q.AdminCountPromptComments(ctx, promptID)
	if err != nil {
		return Page[repo.AdminListPromptCommentsRow]{}, fmt.Errorf("统计评论数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// AttachPromptTag 给提示词添加标签（需求 §17.2「添加或移除标签」）。
func (s *Service) AttachPromptTag(ctx context.Context, operator domain.User, promptID, tagID int64, ip string) error {
	return s.mutatePromptTags(ctx, operator, promptID, ip, "添加标签", func(q *repo.Queries) error {
		if _, err := q.AdminGetTag(ctx, tagID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "标签不存在")
			}
			return fmt.Errorf("查询标签: %w", err)
		}
		return q.AttachPromptTag(ctx, repo.AttachPromptTagParams{PromptID: promptID, TagID: tagID})
	})
}

// DetachPromptTag 移除提示词的标签（需求 §17.2）。
func (s *Service) DetachPromptTag(ctx context.Context, operator domain.User, promptID, tagID int64, ip string) error {
	return s.mutatePromptTags(ctx, operator, promptID, ip, "移除标签", func(q *repo.Queries) error {
		return q.DetachPromptTag(ctx, repo.DetachPromptTagParams{PromptID: promptID, TagID: tagID})
	})
}

// mutatePromptTags 收敛标签增删的公共部分：提示词必须存在，变更后写一条
// prompt_tag_update 日志，日志里带上变更前后的标签集合。
func (s *Service) mutatePromptTags(
	ctx context.Context, operator domain.User, promptID int64, ip, note string,
	mutate func(q *repo.Queries) error,
) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if _, err := q.GetPromptIncludingDeleted(ctx, promptID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "")
			}
			return fmt.Errorf("查询提示词: %w", err)
		}
		before, err := q.ListPromptTagIDs(ctx, promptID)
		if err != nil {
			return fmt.Errorf("查询提示词标签: %w", err)
		}
		if err := mutate(q); err != nil {
			return err
		}
		after, err := q.ListPromptTagIDs(ctx, promptID)
		if err != nil {
			return fmt.Errorf("查询提示词标签: %w", err)
		}
		return s.record(ctx, q, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionPromptTagUpdate,
			ObjectType: "prompt",
			ObjectID:   promptID,
			Before:     map[string]any{"tag_ids": before},
			After:      map[string]any{"tag_ids": after},
			Note:       note,
			IP:         ip,
		})
	})
}

// ForceMajorMark 强制标记重大修改（需求 §7.1、§17.2）。
//
// 它复用前台的重大修改流程：生成新的主版本、归档并清空当前评分、清空统计快照、
// 推荐标签转待复核（需求 §7.3 第 1 至 7 步）。管理员只提供变更说明，
// 内容本身不变——这是「标记」而非「编辑」。
func (s *Service) ForceMajorMark(ctx context.Context, operator domain.User, promptID int64, note, ip string) (prompt.Result, error) {
	if err := s.ensure(); err != nil {
		return prompt.Result{}, err
	}
	if s.prompts == nil {
		return prompt.Result{}, apperr.New(apperr.CodeInternal, "提示词写入能力未注入")
	}

	q := repo.New(s.pool)
	row, err := q.GetPromptIncludingDeleted(ctx, promptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return prompt.Result{}, apperr.New(apperr.CodeNotFound, "")
		}
		return prompt.Result{}, fmt.Errorf("查询提示词: %w", err)
	}
	if row.DeletedAt.Valid {
		return prompt.Result{}, apperr.New(apperr.CodeInvalidState, "回收站中的提示词不可标记重大修改")
	}
	tagIDs, err := q.ListPromptTagIDs(ctx, promptID)
	if err != nil {
		return prompt.Result{}, fmt.Errorf("查询提示词标签: %w", err)
	}

	result, err := s.prompts.Update(ctx, operator, promptID, prompt.Input{
		Title:         row.Title,
		Body:          row.Body,
		Description:   row.Description,
		CategoryID:    row.CategoryID,
		AbilityID:     int8Ptr(row.AbilityID),
		DifficultyID:  int8Ptr(row.DifficultyID),
		Visibility:    domain.Visibility(row.Visibility),
		Sensitive:     domain.SensitiveLevel(row.SensitiveLevel),
		TagIDs:        tagIDs,
		ChangeNote:    note,
		IsMajorChange: true,
	})
	if err != nil {
		return prompt.Result{}, err
	}

	// 日志在业务事务之后单独写入：主流程由 prompt.Service 拥有自己的事务，
	// 而标记动作已成功，日志写入失败不应让调用方以为标记失败。
	if err := repo.InTx(ctx, s.pool, func(tx *repo.Queries) error {
		return s.record(ctx, tx, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionPromptMajorMark,
			ObjectType: "prompt",
			ObjectID:   promptID,
			Before:     map[string]any{"version_no": row.LatestVersionNo},
			After:      map[string]any{"version_no": result.VersionNo, "archived_ratings": result.ArchivedRatings},
			Note:       note,
			IP:         ip,
		})
	}); err != nil {
		return prompt.Result{}, err
	}
	return result, nil
}

// ListRecommendTags 返回推荐标签列表，可按状态与类型筛选（需求 §17.5、§6.3）。
func (s *Service) ListRecommendTags(ctx context.Context, states []string, kind string, page, pageSize int32) (Page[repo.AdminListRecommendTagsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListRecommendTagsRow]{}, err
	}
	for _, state := range states {
		if _, ok := domain.ParseRecommendTagState(state); !ok {
			return Page[repo.AdminListRecommendTagsRow]{}, apperr.New(apperr.CodeValidationFailed, "推荐标签状态取值非法")
		}
	}
	if kind != "" && !isKnownRecommendKind(kind) {
		return Page[repo.AdminListRecommendTagsRow]{}, apperr.New(apperr.CodeValidationFailed, "推荐标签类型取值非法")
	}

	q := repo.New(s.pool)
	rows, err := q.AdminListRecommendTags(ctx, repo.AdminListRecommendTagsParams{
		States: states, Kind: kind, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListRecommendTagsRow]{}, fmt.Errorf("查询推荐标签: %w", err)
	}
	total, err := q.AdminCountRecommendTags(ctx, repo.AdminCountRecommendTagsParams{States: states, Kind: kind})
	if err != nil {
		return Page[repo.AdminListRecommendTagsRow]{}, fmt.Errorf("统计推荐标签数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// RecommendTagReview 是一次推荐标签处置的输入。
type RecommendTagReview struct {
	// State 只能是 confirmed 或 ignored（C-13）。
	State domain.RecommendTagState
	// LabelZh 与 LabelEn 为空时保留原有文案。
	LabelZh string
	LabelEn string
}

// ReviewRecommendTags 批量处置推荐标签（需求 §17.5「批量处理推荐标签」）。
//
// 每条标签单独校验与记日志：批量动作在审计上仍是逐条可追溯的操作，
// 任何一条状态不合法时整体回滚，不会出现「处理了一半」的结果。
func (s *Service) ReviewRecommendTags(ctx context.Context, operator domain.User, ids []int64, review RecommendTagReview, ip string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if review.State != domain.RecommendConfirmed && review.State != domain.RecommendIgnored {
		return apperr.New(apperr.CodeValidationFailed, "推荐标签只能被确认或忽略")
	}
	if len(ids) == 0 {
		return apperr.New(apperr.CodeValidationFailed, "请至少选择一条推荐标签")
	}
	if len([]rune(review.LabelZh)) > domain.RecommendTagLabelMaxRunes ||
		len([]rune(review.LabelEn)) > domain.RecommendTagLabelMaxRunes {
		return apperr.New(apperr.CodeValidationFailed, "标签文案超出长度上限")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		for _, id := range ids {
			current, err := q.GetRecommendTag(ctx, id)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return apperr.New(apperr.CodeNotFound, "")
				}
				return fmt.Errorf("查询推荐标签 %d: %w", id, err)
			}
			state, ok := domain.ParseRecommendTagState(current.State)
			if !ok {
				return fmt.Errorf("推荐标签 %d 的状态取值 %q 未登记", id, current.State)
			}
			// 待确认与待复核都可以被处置；已确认、已忽略与已过期是终态。
			if state != domain.RecommendPending && state != domain.RecommendReview {
				return apperr.New(apperr.CodeInvalidState, "该推荐标签已处置过")
			}

			labelZh := current.LabelZh
			if review.LabelZh != "" {
				labelZh = review.LabelZh
			}
			labelEn := current.LabelEn
			if review.LabelEn != "" {
				labelEn = review.LabelEn
			}
			if _, err := q.UpdateRecommendTag(ctx, repo.UpdateRecommendTagParams{
				ID:         id,
				State:      string(review.State),
				LabelZh:    labelZh,
				LabelEn:    labelEn,
				ReviewedBy: operator.ID,
				ReviewedAt: repo.Timestamptz(s.now().UTC()),
			}); err != nil {
				return fmt.Errorf("更新推荐标签 %d: %w", id, err)
			}

			action := oplog.ActionRecommendTagIgnore
			if review.State == domain.RecommendConfirmed {
				action = oplog.ActionRecommendTagConfirm
			}
			if err := s.record(ctx, q, oplog.Entry{
				OperatorID: operator.ID,
				Action:     action,
				ObjectType: "recommend_tag",
				ObjectID:   id,
				Before:     map[string]any{"state": current.State, "kind": current.Kind},
				After:      map[string]any{"state": string(review.State), "label_zh": labelZh, "label_en": labelEn},
				IP:         ip,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func isKnownRecommendKind(kind string) bool {
	for _, known := range domain.AllRecommendTagKinds() {
		if domain.RecommendTagKind(kind) == known {
			return true
		}
	}
	return false
}
