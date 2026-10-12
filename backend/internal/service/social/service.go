// Package social 承载社交与协作域的用例编排：收藏夹、关注、评论、举报、通知与公告。
//
// 三类约束决定本包的结构：
//   - 配额判定必须在事务内完成（C-21）：锁定 → 计数 → 判定 → 写入；
//   - 可见性判定只有 domain.Viewer 一处（开发方案 §3.1），收藏夹条目必须逐条复查，
//     因为夹内的提示词可能在上次访问后被改为私有或高风险；
//   - 公告采用共享条目 + 每用户水位线（C-33），不逐用户扇出。
package social

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/oplog"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Config *config.Store
	Logger *slog.Logger
	Oplog  *oplog.Recorder
}

type Service struct {
	pool   *pgxpool.Pool
	config *config.Store
	logger *slog.Logger
	oplog  *oplog.Recorder

	// stats 是统计缓存失效入口，注入后社交动作引起的提示词变化会让统计重算。
	stats StatsHook

	now func() time.Time
}

func New(deps Deps) *Service {
	recorder := deps.Oplog
	if recorder == nil {
		recorder = oplog.NewRecorder()
	}
	return &Service{
		pool:   deps.Pool,
		config: deps.Config,
		logger: deps.Logger,
		oplog:  recorder,
		now:    time.Now,
	}
}

// intOf 读取整型配置，缺项时用默认值：配置表被写坏时社交功能仍按需求默认值运行。
func (s *Service) intOf(ctx context.Context, key string, fallback int64) int64 {
	value, err := s.config.GetInt(ctx, key)
	if err != nil {
		s.logger.Warn("社交配置读取失败，按默认值运行",
			slog.String("key", key), slog.String("error", err.Error()))
		return fallback
	}
	return value
}

// boolOf 读取布尔型配置，缺项时用默认值。
func (s *Service) boolOf(ctx context.Context, key string, fallback bool) bool {
	value, err := s.config.GetBool(ctx, key)
	if err != nil {
		s.logger.Warn("社交配置读取失败，按默认值运行",
			slog.String("key", key), slog.String("error", err.Error()))
		return fallback
	}
	return value
}

// ensure 断言依赖齐备，避免空指针在运行期才暴露。
func (s *Service) ensure() error {
	if s.pool == nil || s.config == nil {
		return errDepsIncomplete
	}
	return nil
}

// statsHook 是社交服务对统计服务的依赖声明，只声明用到的方法，
// 使两个包之间不产生包级循环依赖。
type StatsHook interface {
	MarkStale(promptID int64)
}

// SetStatsHook 注入统计缓存失效入口。为空时社交功能仍可用，只是不刷新统计缓存。
func (s *Service) SetStatsHook(hook StatsHook) { s.stats = hook }
