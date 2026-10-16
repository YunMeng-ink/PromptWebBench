package library

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Library 是题库的服务层视图：领域实体加上展示所需的字段。
type Library struct {
	Entity domain.Library
	// Role 是当前身份在题库中的角色。
	Role domain.LibraryRole
	// MemberCount 是成员总数（含所有者）。
	MemberCount int32
	// LiveQuestionCount 是可见的题目数量（不含回收站中的题目）。
	LiveQuestionCount int64
	OwnerName         string
}

// Access 返回可见性判定要素，转发到领域实体。
func (l Library) Access() domain.Access { return l.Entity.Access() }

// LibraryPage 是题库列表的一页。
type LibraryPage struct {
	Items    []Library
	Page     int32
	PageSize int32
	Total    int64
}

// LibraryInput 是创建题库的输入。
type LibraryInput struct {
	Name        string
	Description string
}

// LibraryPatch 是修改题库的输入。字段为空表示本次不修改。
type LibraryPatch struct {
	Name        *string
	Description *string
}

// Member 是题库成员的服务层视图。
type Member struct {
	UserID    int64
	Username  string
	AvatarURL string
	Role      domain.LibraryRole
	InvitedBy *int64
	CreatedAt time.Time
}

// CreateLibrary 创建私有题库，创建者即所有者（需求 §10.3）。
//
// 数量配额在事务内判定（C-21）：先取该用户的咨询锁，再计数、判定、写入。
// 事务外计数后插入会让并发请求各自读到未超限的计数，双双写入成功而突破上限。
func (s *Service) CreateLibrary(ctx context.Context, v domain.Viewer, in LibraryInput) (Library, error) {
	if err := s.ensure(); err != nil {
		return Library{}, notFound("题库")
	}
	if !v.CanWrite() {
		return Library{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if err := domain.ValidateLibraryName(in.Name); err != nil {
		return Library{}, validateError(err)
	}
	if err := domain.ValidateLibraryDescription(in.Description); err != nil {
		return Library{}, validateError(err)
	}

	limit := s.libraryQuota(ctx, v)
	var created Library
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if err := q.LockLibraryOwner(ctx, v.UserID); err != nil {
			return wrap("获取题库配额锁", v.UserID, err)
		}
		count, err := q.CountLibraries(ctx, v.UserID)
		if err != nil {
			return wrap("统计题库数量", v.UserID, err)
		}
		if count >= limit {
			return apperr.Errorf(apperr.CodeQuotaExceeded, "私有题库数量已达上限 %d 个", limit)
		}

		row, err := q.InsertLibrary(ctx, repo.InsertLibraryParams{
			OwnerID:     v.UserID,
			Name:        in.Name,
			Description: in.Description,
		})
		if err != nil {
			return wrap("写入题库", v.UserID, err)
		}
		if _, err := q.InsertLibraryMember(ctx, repo.InsertLibraryMemberParams{
			LibraryID: row.ID,
			UserID:    v.UserID,
			Role:      string(domain.LibraryRoleOwner),
		}); err != nil {
			return wrap("写入题库所有者成员记录", row.ID, err)
		}

		created = Library{Entity: libraryFromRow(row), Role: domain.LibraryRoleOwner, MemberCount: 1}
		return nil
	})
	if err != nil {
		return Library{}, err
	}
	return created, nil
}

// ListMyLibraries 返回当前身份参与的题库，含自己创建的与受邀协作的。
func (s *Service) ListMyLibraries(ctx context.Context, v domain.Viewer, page, pageSize int32) (LibraryPage, error) {
	if err := s.ensure(); err != nil {
		return LibraryPage{}, notFound("题库")
	}
	if !v.Logged {
		return LibraryPage{}, apperr.New(apperr.CodeUnauthorized, "")
	}

	q := repo.New(s.pool)
	rows, err := q.ListLibrariesForUser(ctx, repo.ListLibrariesForUserParams{
		UserID:      v.UserID,
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return LibraryPage{}, wrap("查询题库列表", v.UserID, err)
	}
	total, err := q.CountLibrariesForUser(ctx, v.UserID)
	if err != nil {
		return LibraryPage{}, wrap("统计题库数量", v.UserID, err)
	}

	items := make([]Library, 0, len(rows))
	for _, row := range rows {
		role, ok := domain.ParseLibraryRole(row.MemberRole)
		if !ok {
			return LibraryPage{}, unexpectedValue("题库成员", row.ID, row.MemberRole)
		}
		live, err := q.CountLiveLibraryQuestions(ctx, row.ID)
		if err != nil {
			return LibraryPage{}, wrap("统计题库题目数量", row.ID, err)
		}
		items = append(items, Library{
			Entity: domain.Library{
				ID:            row.ID,
				OwnerID:       row.OwnerID,
				Name:          row.Name,
				Description:   row.Description,
				QuestionCount: row.QuestionCount,
			},
			Role:              role,
			OwnerName:         row.OwnerName,
			LiveQuestionCount: live,
		})
	}
	return LibraryPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// GetLibrary 返回题库详情。
func (s *Service) GetLibrary(ctx context.Context, v domain.Viewer, libraryID int64) (Library, error) {
	if err := s.ensure(); err != nil {
		return Library{}, notFound("题库")
	}

	q := repo.New(s.pool)
	lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
	if err != nil {
		return Library{}, err
	}
	members, err := q.ListLibraryMembers(ctx, libraryID)
	if err != nil {
		return Library{}, wrap("查询题库成员", libraryID, err)
	}
	live, err := q.CountLiveLibraryQuestions(ctx, libraryID)
	if err != nil {
		return Library{}, wrap("统计题库题目数量", libraryID, err)
	}
	ownerName, err := s.actorName(ctx, q, lib.OwnerID)
	if err != nil {
		return Library{}, err
	}
	return Library{
		Entity:            lib,
		Role:              actor.Role,
		MemberCount:       int32(len(members)),
		LiveQuestionCount: live,
		OwnerName:         ownerName,
	}, nil
}

// UpdateLibrary 修改题库名称与描述。
//
// 名称只允许所有者修改，描述所有者与协作者都可以修改（需求 §10.4、§10.5）。
// 因此两项能力分别判定：协作者提交改名请求时得到 403，而不是静默丢弃名称字段。
func (s *Service) UpdateLibrary(ctx context.Context, v domain.Viewer, libraryID int64, patch LibraryPatch) (Library, error) {
	if err := s.ensure(); err != nil {
		return Library{}, notFound("题库")
	}
	if !v.CanWrite() {
		return Library{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if patch.Name == nil && patch.Description == nil {
		return Library{}, apperr.New(apperr.CodeValidationFailed, "没有需要修改的字段")
	}
	if patch.Name != nil {
		if err := domain.ValidateLibraryName(*patch.Name); err != nil {
			return Library{}, validateError(err)
		}
	}
	if patch.Description != nil {
		if err := domain.ValidateLibraryDescription(*patch.Description); err != nil {
			return Library{}, validateError(err)
		}
	}

	var updated Library
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
		if err != nil {
			return err
		}
		if patch.Name != nil && *patch.Name != lib.Name {
			if !actor.Can(domain.CapUpdateName) {
				return forbidden("只有题库所有者可以修改题库名称")
			}
			row, err := q.UpdateLibraryName(ctx, repo.UpdateLibraryNameParams{ID: libraryID, Name: *patch.Name})
			if err != nil {
				return wrap("更新题库名称", libraryID, err)
			}
			lib = libraryFromRow(row)
		}
		if patch.Description != nil && *patch.Description != lib.Description {
			if !actor.Can(domain.CapUpdateDescription) {
				return forbidden("当前身份不可以修改题库描述")
			}
			row, err := q.UpdateLibraryDescription(ctx, repo.UpdateLibraryDescriptionParams{
				ID: libraryID, Description: *patch.Description,
			})
			if err != nil {
				return wrap("更新题库描述", libraryID, err)
			}
			lib = libraryFromRow(row)
		}
		updated = Library{Entity: lib, Role: actor.Role}
		return nil
	})
	if err != nil {
		return Library{}, err
	}
	return updated, nil
}

// DeleteLibrary 把题库移入回收站（需求 §11.2）。仅所有者可执行。
//
// 题库与题目一并隐藏：题目查询按题库成员身份前置过滤，题库本身不可见时
// 题目也无从读取，因此不需要逐题打删除标记。
// 删除本身由回收站记录留痕（删除人、删除时间、删除前快照），保留期结束后
// 才真正清除（需求 §11.3）。
func (s *Service) DeleteLibrary(ctx context.Context, v domain.Viewer, libraryID int64, ip string) error {
	if err := s.ensure(); err != nil {
		return notFound("题库")
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
		if err != nil {
			return err
		}
		if !actor.Can(domain.CapDeleteLibrary) {
			return forbidden("只有题库所有者可以删除题库")
		}
		if _, err := q.SoftDeleteLibrary(ctx, libraryID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("题库")
			}
			return wrap("删除题库", libraryID, err)
		}
		return s.entryToRecycleBin(ctx, q, recycleEntry{
			ObjectType: domain.RecycleObjectLibrary,
			ObjectID:   libraryID,
			OwnerID:    lib.OwnerID,
			LibraryID:  &libraryID,
			Snapshot:   librarySnapshot(lib),
			DeletedBy:  v.UserID,
		})
	})
}

// ListMembers 返回题库成员（需求 §10.3）。
func (s *Service) ListMembers(ctx context.Context, v domain.Viewer, libraryID int64) ([]Member, error) {
	if err := s.ensure(); err != nil {
		return nil, notFound("题库")
	}

	q := repo.New(s.pool)
	if _, _, err := s.loadVisibleLibrary(ctx, q, v, libraryID); err != nil {
		return nil, err
	}
	rows, err := q.ListLibraryMembers(ctx, libraryID)
	if err != nil {
		return nil, wrap("查询题库成员", libraryID, err)
	}
	out := make([]Member, 0, len(rows))
	for _, row := range rows {
		role, ok := domain.ParseLibraryRole(row.Role)
		if !ok {
			return nil, unexpectedValue("题库成员", row.ID, row.Role)
		}
		item := Member{
			UserID:    row.UserID,
			Username:  row.Username,
			AvatarURL: row.AvatarUrl,
			Role:      role,
			InvitedBy: int8Ptr(row.InvitedBy),
		}
		if row.CreatedAt.Valid {
			item.CreatedAt = row.CreatedAt.Time
		}
		out = append(out, item)
	}
	return out, nil
}

// InviteMember 邀请协作者。仅所有者可执行（需求 §10.5）。
func (s *Service) InviteMember(ctx context.Context, v domain.Viewer, libraryID int64, username string) (Member, error) {
	if err := s.ensure(); err != nil {
		return Member{}, notFound("题库")
	}
	if !v.CanWrite() {
		return Member{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if len([]rune(username)) == 0 {
		return Member{}, apperr.New(apperr.CodeValidationFailed, "请填写被邀请人的用户名")
	}

	var invited Member
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		lib, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
		if err != nil {
			return err
		}
		if !actor.Can(domain.CapInviteMember) {
			return forbidden("只有题库所有者可以邀请协作者")
		}

		target, err := q.GetUserForLibraryInvite(ctx, username)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("用户")
			}
			return wrap("查询被邀请用户", 0, err)
		}
		if target.Status == statusBanned {
			return apperr.New(apperr.CodeValidationFailed, "该账户已被封禁，无法加入题库")
		}

		inserted, err := q.InsertLibraryMember(ctx, repo.InsertLibraryMemberParams{
			LibraryID: libraryID,
			UserID:    target.ID,
			Role:      string(domain.LibraryRoleCollaborator),
			InvitedBy: int8OrNull(&v.UserID),
		})
		if err != nil {
			return wrap("写入题库成员", libraryID, err)
		}
		if inserted == 0 {
			// 唯一约束已存在该成员：所有者本人或已邀请过的协作者。
			return apperr.New(apperr.CodeInvalidState, "该用户已是题库成员")
		}

		if _, err := q.InsertNotification(ctx, repo.InsertNotificationParams{
			UserID:  target.ID,
			Kind:    string(domain.NotifySystem),
			Title:   "你被邀请加入私有题库",
			Body:    domain.NotificationLibraryInvitedBody(lib.Name),
			Link:    libraryLink(libraryID),
			Payload: invitedPayload(lib.Name, v.UserID),
		}); err != nil {
			return wrap("写入邀请通知", libraryID, err)
		}

		invited = Member{
			UserID:    target.ID,
			Username:  target.Username,
			Role:      domain.LibraryRoleCollaborator,
			InvitedBy: &v.UserID,
		}
		return nil
	})
	if err != nil {
		return Member{}, err
	}
	return invited, nil
}

// RemoveMember 移除协作者。仅所有者可执行（需求 §10.5）。
func (s *Service) RemoveMember(ctx context.Context, v domain.Viewer, libraryID, userID int64, ip string) error {
	if err := s.ensure(); err != nil {
		return notFound("题库")
	}
	if !v.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		_, actor, err := s.loadVisibleLibrary(ctx, q, v, libraryID)
		if err != nil {
			return err
		}
		if !actor.Can(domain.CapRemoveMember) {
			return forbidden("只有题库所有者可以移除协作者")
		}
		if _, err := q.DeleteLibraryMember(ctx, repo.DeleteLibraryMemberParams{
			LibraryID: libraryID,
			UserID:    userID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("题库协作者")
			}
			return wrap("移除题库协作者", libraryID, err)
		}
		return nil
	})
}

// loadVisibleLibrary 读取题库并构造判定主体。非成员与非管理员一律按不存在处理。
func (s *Service) loadVisibleLibrary(
	ctx context.Context, q *repo.Queries, v domain.Viewer, libraryID int64,
) (domain.Library, domain.LibraryActor, error) {
	row, err := q.GetLibrary(ctx, libraryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Library{}, domain.LibraryActor{}, notFound("题库")
		}
		return domain.Library{}, domain.LibraryActor{}, wrap("查询题库", libraryID, err)
	}
	lib := libraryFromRow(row)

	var member *domain.LibraryMember
	if v.Logged {
		memberRow, err := q.GetLibraryMember(ctx, repo.GetLibraryMemberParams{
			LibraryID: libraryID,
			UserID:    v.UserID,
		})
		switch {
		case err == nil:
			parsed, err := memberFromRow(memberRow)
			if err != nil {
				return domain.Library{}, domain.LibraryActor{}, err
			}
			member = &parsed
		case !errors.Is(err, pgx.ErrNoRows):
			return domain.Library{}, domain.LibraryActor{}, wrap("查询题库成员", libraryID, err)
		}
	}

	actor := domain.ActorFor(v, member)
	if !actor.Can(domain.CapViewLibrary) {
		return domain.Library{}, domain.LibraryActor{}, notFound("题库")
	}
	return lib, actor, nil
}

// requireCapability 断言主体具备某项能力。调用方已确认题库可见，
// 因此此处失败一律是角色权限不足。
func requireCapability(actor domain.LibraryActor, cap domain.LibraryCapability, message string) error {
	if !actor.Can(cap) {
		return forbidden(message)
	}
	return nil
}

// statusBanned 是用户处于封禁态时的状态取值（需求 §15）。
const statusBanned = "banned"

// libraryLink 返回题库页面的站内链接。
func libraryLink(libraryID int64) string {
	return "/libraries/" + itoa64(libraryID)
}

// invitedPayload 构造邀请通知的载荷。
func invitedPayload(libraryName string, inviterID int64) []byte {
	raw, err := json.Marshal(map[string]any{
		"library_name": libraryName,
		"inviter_id":   inviterID,
	})
	if err != nil {
		return []byte("{}")
	}
	return raw
}
