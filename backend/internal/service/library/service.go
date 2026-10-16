// Package library 承载私有题库域的用例编排：题库、成员、题目、标签、排序、
// 题库内回收站与 Markdown 导出。
//
// 两类约束决定本包的结构：
//   - 权限判定取自需求 §10.4 与 §10.5 的两份清单，由 domain.LibraryActor 统一裁决（C-46），
//     服务层只声明「本操作需要哪项能力」，不再各写一套身份比较；
//   - 题目内容的可见性判定复用 domain.Viewer.CanView（开发方案 §3.1）。引用题读取来源
//     提示词的正文，若不复查来源可见性，普通用户协作者就能读到开发者专属内容的全文，
//     导出接口同样泄漏（开发方案 §6.6 专项回归）。
package library

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

type Deps struct {
	Pool     *pgxpool.Pool
	Config   *config.Store
	Logger   *slog.Logger
	Oplog    *oplog.Recorder
	Notifier Notifier
}

type Service struct {
	pool     *pgxpool.Pool
	config   *config.Store
	logger   *slog.Logger
	oplog    *oplog.Recorder
	notifier Notifier

	now func() time.Time
}

func New(deps Deps) *Service {
	recorder := deps.Oplog
	if recorder == nil {
		recorder = oplog.NewRecorder()
	}
	return &Service{
		pool:     deps.Pool,
		config:   deps.Config,
		logger:   deps.Logger,
		oplog:    recorder,
		notifier: deps.Notifier,
		now:      time.Now,
	}
}

// Notifier 是题库域对通知服务的依赖声明，只声明用到的方法，
// 使题库包与社交包之间不产生包级依赖。传入事务中的查询句柄，
// 保证通知与删除同事务：删了题却没通知的状态会让所有者永久不知情。
type Notifier interface {
	NotifyLibraryQuestionDeleted(
		ctx context.Context, q *repo.Queries, ownerID int64, libraryName, questionTitle, actorName string,
	) error
}

// intOf 读取整型配置，缺项时用默认值：配置表被写坏时题库功能仍按需求默认值运行。
func (s *Service) intOf(ctx context.Context, key string, fallback int64) int64 {
	value, err := s.config.GetInt(ctx, key)
	if err != nil {
		s.logger.Warn("题库配置读取失败，按默认值运行",
			slog.String("key", key), slog.String("error", err.Error()))
		return fallback
	}
	return value
}

// boolOf 读取布尔型配置，缺项时用默认值。
func (s *Service) boolOf(ctx context.Context, key string, fallback bool) bool {
	value, err := s.config.GetBool(ctx, key)
	if err != nil {
		s.logger.Warn("题库配置读取失败，按默认值运行",
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

// libraryQuota 返回当前身份最多可创建的私有题库数量（需求 §10.2）。
func (s *Service) libraryQuota(ctx context.Context, v domain.Viewer) int64 {
	key := "library_quota_user"
	fallback := int64(domain.LibraryQuotaUser)
	if v.IsDeveloper() {
		key = "library_quota_developer"
		fallback = int64(domain.LibraryQuotaDeveloper)
	}
	return s.intOf(ctx, key, fallback)
}

// questionLimit 返回单个题库的题目容量（需求 §10.2）。
func (s *Service) questionLimit(ctx context.Context) int32 {
	return int32(s.intOf(ctx, "library_question_limit", domain.LibraryQuestionLimit))
}

// retentionDays 返回回收站保留天数（需求 §11.3）。
func (s *Service) retentionDays(ctx context.Context) int64 {
	return s.intOf(ctx, "recycle_retention_days", domain.RecycleRetentionDays)
}
