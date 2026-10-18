package stats

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/promptbench/backend/internal/formula"
	"github.com/promptbench/backend/internal/repo"
)

// LeaderboardRefresh 是排行榜快照的固定刷新周期。
// 前端按同一周期轮询，服务端在这段时间内直接返回同一份结果。
const LeaderboardRefresh = 60 * time.Second

const leaderboardMaxLimit = 50

// 排行榜的时间窗口取值。
const (
	LeaderboardWindowOverall = "overall"
	LeaderboardWindowRecent  = "recent"
	LeaderboardWindowHistory = "history"
)

// LeaderboardEntry 是榜单中的一条提示词。
type LeaderboardEntry struct {
	Rank             int
	PromptID         int64
	Title            string
	Discrimination   float64
	Confidence       formula.ConfidenceLevel
	ValidModelCount  int
	TotalRatingCount int
	ComputedAt       time.Time
}

// Leaderboard 是一份排行榜快照。
type Leaderboard struct {
	Window         string
	RefreshedAt    time.Time
	RefreshSeconds int
	Items          []LeaderboardEntry
}

type leaderboardCacheEntry struct {
	items       []LeaderboardEntry
	refreshedAt time.Time
}

// Leaderboard 返回按区分度排序的公开提示词。
//
// 榜单只读取已落库的统计快照，不在请求路径上重算；快照的刷新由统计读取路径负责。
// 服务端按固定周期缓存结果，前端用返回的 refresh_seconds 决定轮询间隔。
func (s *Service) Leaderboard(ctx context.Context, window string, limit int32) (Leaderboard, error) {
	if err := s.ensure(); err != nil {
		return Leaderboard{}, err
	}
	key := normalizeLeaderboardWindow(window)
	if limit < 1 {
		limit = 20
	}
	if limit > leaderboardMaxLimit {
		limit = leaderboardMaxLimit
	}

	s.leaderboardMu.Lock()
	cached, ok := s.leaderboard[key]
	if ok && s.now().Sub(cached.refreshedAt) < LeaderboardRefresh {
		s.leaderboardMu.Unlock()
		return leaderboardView(key, cached, limit), nil
	}
	s.leaderboardMu.Unlock()

	rows, err := repo.New(s.pool).ListDiscriminationLeaderboard(ctx,
		repo.ListDiscriminationLeaderboardParams{WindowKey: key, LimitCount: leaderboardMaxLimit})
	if err != nil {
		return Leaderboard{}, fmt.Errorf("查询区分度排行榜: %w", err)
	}

	items := make([]LeaderboardEntry, 0, len(rows))
	for index, row := range rows {
		entry := LeaderboardEntry{
			Rank:             index + 1,
			PromptID:         row.ID,
			Title:            row.Title,
			Discrimination:   row.Discrimination,
			Confidence:       formula.ConfidenceLevel(row.Confidence),
			ValidModelCount:  int(row.ValidModelCount),
			TotalRatingCount: int(row.TotalRatingCount),
		}
		if row.ComputedAt.Valid {
			entry.ComputedAt = row.ComputedAt.Time
		}
		items = append(items, entry)
	}

	snapshot := leaderboardCacheEntry{items: items, refreshedAt: s.now()}
	s.leaderboardMu.Lock()
	s.leaderboard[key] = snapshot
	s.leaderboardMu.Unlock()
	return leaderboardView(key, snapshot, limit), nil
}

// normalizeLeaderboardWindow 把查询参数收敛到三个已登记取值。
func normalizeLeaderboardWindow(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case LeaderboardWindowRecent:
		return LeaderboardWindowRecent
	case LeaderboardWindowHistory:
		return LeaderboardWindowHistory
	default:
		return LeaderboardWindowOverall
	}
}

// leaderboardView 按请求条数截断缓存中的榜单，不修改缓存本身。
func leaderboardView(window string, snapshot leaderboardCacheEntry, limit int32) Leaderboard {
	items := snapshot.items
	if int64(len(items)) > int64(limit) {
		items = items[:limit]
	}
	out := Leaderboard{
		Window:         window,
		RefreshedAt:    snapshot.refreshedAt,
		RefreshSeconds: int(LeaderboardRefresh.Seconds()),
		Items:          make([]LeaderboardEntry, len(items)),
	}
	copy(out.Items, items)
	return out
}
