package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// HotPrompt 是仪表盘榜单中的一条提示词。
type HotPrompt struct {
	ID            int64
	Title         string
	Visibility    domain.Visibility
	CommentCount  int32
	FavoriteCount int32
	CreatedAt     time.Time
}

// Dashboard 是统计仪表盘的全部内容：17 项计数与两个榜单（需求 §17.1）。
type Dashboard struct {
	Counts repo.AdminDashboardCountsRow
	Hot    []HotPrompt
	Recent []HotPrompt
}

// LoadDashboard 读取统计仪表盘。
//
// 计数以 UTC 自然日为「今日」的边界（C-03），与评分限额等其余按日统计口径一致。
func (s *Service) LoadDashboard(ctx context.Context) (Dashboard, error) {
	if err := s.ensure(); err != nil {
		return Dashboard{}, err
	}
	q := repo.New(s.pool)

	counts, err := q.AdminDashboardCounts(ctx, repo.Timestamptz(DayStart(s.now().UTC())))
	if err != nil {
		return Dashboard{}, fmt.Errorf("读取仪表盘计数: %w", err)
	}
	hot, err := q.AdminHotPrompts(ctx, domain.DashboardHotPromptLimit)
	if err != nil {
		return Dashboard{}, fmt.Errorf("读取热门提示词: %w", err)
	}
	recent, err := q.AdminRecentPrompts(ctx, domain.DashboardRecentPromptLimit)
	if err != nil {
		return Dashboard{}, fmt.Errorf("读取最近上传提示词: %w", err)
	}

	return Dashboard{
		Counts: counts,
		Hot:    hotPromptsFromRows(hot),
		Recent: recentPromptsFromRows(recent),
	}, nil
}

// DayStart 返回给定时刻所在 UTC 自然日的零点。全系统按日统计的窗口边界都由此产生。
func DayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// newDashboardPrompt 把两个榜单查询共有的列映射为榜单条目。两条查询由 sqlc 生成了
// 字段相同但类型不同的行结构体，取值规则在此收敛为一处。
func newDashboardPrompt(
	id int64, title, visibility string, commentCount, favoriteCount int32, createdAt pgtype.Timestamptz,
) HotPrompt {
	item := HotPrompt{
		ID:            id,
		Title:         title,
		CommentCount:  commentCount,
		FavoriteCount: favoriteCount,
	}
	if parsed, ok := domain.ParseVisibility(visibility); ok {
		item.Visibility = parsed
	}
	if createdAt.Valid {
		item.CreatedAt = createdAt.Time
	}
	return item
}

func hotPromptsFromRows(rows []repo.AdminHotPromptsRow) []HotPrompt {
	out := make([]HotPrompt, 0, len(rows))
	for _, row := range rows {
		out = append(out, newDashboardPrompt(
			row.ID, row.Title, row.Visibility, row.CommentCount, row.FavoriteCount, row.CreatedAt,
		))
	}
	return out
}

func recentPromptsFromRows(rows []repo.AdminRecentPromptsRow) []HotPrompt {
	out := make([]HotPrompt, 0, len(rows))
	for _, row := range rows {
		out = append(out, newDashboardPrompt(
			row.ID, row.Title, row.Visibility, row.CommentCount, row.FavoriteCount, row.CreatedAt,
		))
	}
	return out
}
