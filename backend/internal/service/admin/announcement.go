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
)

// AnnouncementInput 是公告的写入字段（需求 §2.4、§17.8）。
type AnnouncementInput struct {
	Title string
	Body  string
}

// AnnouncementMaxRunes 是公告标题与正文的字数上限。
const (
	AnnouncementTitleMaxRunes = 200
	AnnouncementBodyMaxRunes  = 5000
)

// normalize 校验公告字段。
func (in AnnouncementInput) normalize() (AnnouncementInput, error) {
	out := AnnouncementInput{Title: trimSpace(in.Title), Body: trimSpace(in.Body)}
	if out.Title == "" {
		return AnnouncementInput{}, apperr.New(apperr.CodeValidationFailed, "公告标题不可为空")
	}
	if len([]rune(out.Title)) > AnnouncementTitleMaxRunes {
		return AnnouncementInput{}, apperr.New(apperr.CodeValidationFailed, "公告标题超出长度上限")
	}
	if len([]rune(out.Body)) > AnnouncementBodyMaxRunes {
		return AnnouncementInput{}, apperr.New(apperr.CodeValidationFailed, "公告正文超出长度上限")
	}
	return out, nil
}

// ListAnnouncements 返回公告列表，包含已撤回的条目（需求 §17.8「发布公告」的历史）。
// state 取 ""、"live" 或 "withdrawn"。
func (s *Service) ListAnnouncements(ctx context.Context, state string, page, pageSize int32) (Page[repo.AdminListAnnouncementsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListAnnouncementsRow]{}, err
	}
	if state != "" && state != "live" && state != "withdrawn" {
		return Page[repo.AdminListAnnouncementsRow]{}, apperr.New(apperr.CodeValidationFailed, "公告状态取值非法")
	}

	q := repo.New(s.pool)
	rows, err := q.AdminListAnnouncements(ctx, repo.AdminListAnnouncementsParams{
		State: state, LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	})
	if err != nil {
		return Page[repo.AdminListAnnouncementsRow]{}, fmt.Errorf("查询公告列表: %w", err)
	}
	total, err := q.AdminCountAnnouncements(ctx, state)
	if err != nil {
		return Page[repo.AdminListAnnouncementsRow]{}, fmt.Errorf("统计公告数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// CreateAnnouncement 发布一条公告（需求 §17.8「发布公告」）。
//
// 公告只存一份并带发布时间，用户侧的已读状态由水位线推出（C-33）：
// 撤回后公告立即从用户侧消失，不留下逐用户的通知副本。
func (s *Service) CreateAnnouncement(ctx context.Context, operator domain.User, in AnnouncementInput, ip string) (repo.Announcement, error) {
	if err := s.ensure(); err != nil {
		return repo.Announcement{}, err
	}
	normalized, err := in.normalize()
	if err != nil {
		return repo.Announcement{}, err
	}

	var created repo.Announcement
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		row, err := q.InsertAnnouncement(ctx, repo.InsertAnnouncementParams{
			Title:       normalized.Title,
			Body:        normalized.Body,
			PublishedBy: repo.Int8(operator.ID),
		})
		if err != nil {
			return wrap("写入公告", 0, err)
		}
		created = row
		return s.record(ctx, q, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionAnnouncementPublish,
			ObjectType: "announcement",
			ObjectID:   row.ID,
			After:      map[string]any{"title": normalized.Title},
			IP:         ip,
		})
	})
	if err != nil {
		return repo.Announcement{}, err
	}
	return created, nil
}

// UpdateAnnouncement 修改一条未撤回的公告（需求 §17.8 的公告管理）。
func (s *Service) UpdateAnnouncement(ctx context.Context, id int64, in AnnouncementInput) (repo.Announcement, error) {
	if err := s.ensure(); err != nil {
		return repo.Announcement{}, err
	}
	normalized, err := in.normalize()
	if err != nil {
		return repo.Announcement{}, err
	}
	row, err := repo.New(s.pool).UpdateAnnouncement(ctx, repo.UpdateAnnouncementParams{
		ID: id, Title: normalized.Title, Body: normalized.Body,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Announcement{}, invalidState("公告已撤回，不可修改")
		}
		return repo.Announcement{}, wrap("更新公告", id, err)
	}
	return row, nil
}

// WithdrawAnnouncement 撤回公告（需求 §2.4「公告管理」）。
// 撤回后公告立即从用户侧消失（C-33）。
func (s *Service) WithdrawAnnouncement(ctx context.Context, operator domain.User, id int64, ip string) (repo.Announcement, error) {
	if err := s.ensure(); err != nil {
		return repo.Announcement{}, err
	}

	var withdrawn repo.Announcement
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		current, err := q.AdminGetAnnouncement(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound("公告")
			}
			return wrap("查询公告", id, err)
		}
		row, err := q.WithdrawAnnouncement(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return invalidState("公告已撤回")
			}
			return wrap("撤回公告", id, err)
		}
		withdrawn = row
		return s.record(ctx, q, oplog.Entry{
			OperatorID: operator.ID,
			Action:     oplog.ActionAnnouncementWithdraw,
			ObjectType: "announcement",
			ObjectID:   id,
			Before:     map[string]any{"title": current.Title},
			IP:         ip,
		})
	})
	if err != nil {
		return repo.Announcement{}, err
	}
	return withdrawn, nil
}

// AnnouncementState 返回公告的展示状态：draft、live 或 withdrawn。
func AnnouncementState(row repo.Announcement) string {
	switch {
	case row.WithdrawnAt.Valid:
		return "withdrawn"
	case row.PublishedAt.Valid:
		return "live"
	default:
		return "draft"
	}
}
