// Package stats 承载统计计算：双窗口聚合、区分度、置信度、统计快照与推荐标签。
//
// 全部数值计算委托给 internal/formula 的纯函数，本包只负责取数、编排与落库，
// 使算例可以在脱离数据库的单元测试里逐条复现（开发方案 §2.3）。
package stats

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/formula"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Config *config.Store
	Logger *slog.Logger
}

type Service struct {
	pool   *pgxpool.Pool
	config *config.Store
	logger *slog.Logger

	now func() time.Time

	// source 是 Bootstrap 的抽样源。生产取全局随机源，测试注入带种子的可复现源
	// （开发方案 §6.4 测试重点）。
	source formula.SamplingSource

	cache *cache

	// flight 合并同一提示词的并发重算，使一批并发读取只算一次。
	flight *flight
}

func New(deps Deps) *Service {
	return &Service{
		pool:   deps.Pool,
		config: deps.Config,
		logger: deps.Logger,
		now:    time.Now,
		cache:  newCache(),
		flight: newFlight(),
	}
}

// params 是从配置表读取的统计参数（可行性报告 §2.14）。
type params struct {
	Bayes        formula.BayesParams
	Disc         formula.DiscriminationParams
	Confidence   formula.ConfidenceParams
	DiscConf     formula.DiscriminationConfidenceParams
	Bootstrap    formula.BootstrapParams
	RecentWindow time.Duration
}

// floatOf 读取浮点型配置。缺项或取值非法时以默认值兜底并记录日志：配置表被管理员
// 写坏时，统计仍按需求给出的默认值运行，不会让详情页整体不可用。
func (s *Service) floatOf(ctx context.Context, key string, fallback float64) float64 {
	value, err := s.config.GetFloat(ctx, key)
	if err != nil {
		s.logger.Warn("统计配置读取失败，按默认值计算", slog.String("key", key), slog.String("error", err.Error()))
		return fallback
	}
	return value
}

// intOf 读取整型配置，语义与 floatOf 相同。
func (s *Service) intOf(ctx context.Context, key string, fallback int64) int64 {
	value, err := s.config.GetInt(ctx, key)
	if err != nil {
		s.logger.Warn("统计配置读取失败，按默认值计算", slog.String("key", key), slog.String("error", err.Error()))
		return fallback
	}
	return value
}

// load 读取全部统计参数。
func (s *Service) load(ctx context.Context) params {
	baseDisc := formula.DefaultDiscrimination()
	baseConf := formula.DefaultConfidence()

	out := params{
		Bayes: formula.BayesParams{
			Prior: s.floatOf(ctx, "bayes_prior", formula.DefaultBayes().Prior),
			K:     s.floatOf(ctx, "bayes_k", formula.DefaultBayes().K),
		},
		Disc: formula.DiscriminationParams{
			MinModels:           int(s.intOf(ctx, "discrim_min_models", int64(baseDisc.MinModels))),
			MinRatingsPerModel:  int(s.intOf(ctx, "discrim_min_ratings_per_model", int64(baseDisc.MinRatingsPerModel))),
			MinTotalRatings:     int(s.intOf(ctx, "discrim_min_total_ratings", int64(baseDisc.MinTotalRatings))),
			ModelCountTarget:    s.floatOf(ctx, "discrim_model_count_target", baseDisc.ModelCountTarget),
			RatingCountTarget:   s.floatOf(ctx, "discrim_rating_count_target", baseDisc.RatingCountTarget),
			ObsoleteThreshold:   s.floatOf(ctx, "obsolete_discrimination_threshold", baseDisc.ObsoleteThreshold),
			SaturationThreshold: s.floatOf(ctx, "saturation_threshold", baseDisc.SaturationThreshold),
		},
		// 完成度置信度的三个分母取需求 §5.1 的固定取值，不读配置：
		// 可行性报告 §2.14 的配置项总表里没有它们，说明它们不是可后台调整的参数。
		Confidence:   baseConf,
		DiscConf:     formula.DefaultDiscriminationConfidence(),
		Bootstrap:    formula.BootstrapParams{Iterations: int(s.intOf(ctx, "bootstrap_iterations", 500))},
		RecentWindow: time.Duration(s.intOf(ctx, "recent_window_days", 7)) * 24 * time.Hour,
	}
	return out
}

// windows 承载一次统计重算所需的全部窗口边界。
type windows struct {
	recentStart time.Time
	recentEnd   time.Time
}

// windowBounds 按 UTC 自然日切分近 N 日窗口：起点为当前自然日零点往前推 N-1 天，
// 终点为次日零点，左闭右开含当天（C-03）。
//
// 边界在应用层一次算好再作为参数传入，SQL 侧不出现任何时区表达式
// （开发方案 §3.4）。历史窗口不设边界：它按定义覆盖全部有效评分。
func (s *Service) windowBounds(now time.Time, recentWindow time.Duration) windows {
	dayStart := now.UTC().Truncate(24 * time.Hour)
	return windows{
		recentStart: dayStart.Add(-recentWindow + 24*time.Hour),
		recentEnd:   dayStart.Add(24 * time.Hour),
	}
}

func (w windows) recentRange() (*time.Time, *time.Time) {
	start, end := w.recentStart, w.recentEnd
	return &start, &end
}

// repoPool 断言 pool 非空，避免空指针在运行期才暴露。
func (s *Service) ensure() error {
	if s.pool == nil || s.config == nil {
		return fmt.Errorf("统计服务依赖不完整：Pool、Config 均不可为空")
	}
	return nil
}
