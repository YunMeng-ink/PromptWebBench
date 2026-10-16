// Package admin 承载管理后台的用例编排：统计仪表盘、提示词管理、模型管理、
// 用户管理、标签管理、举报管理、注册与邀请码管理、操作日志、公告管理、
// 回收站管理与系统设置（需求第 17 章、C-42）。
//
// 结构约束：
//   - 后台可见全部对象，因此本包的读取不套用前台的可见性谓词；
//     准入由接口层的 requireAdmin 与限流规则共同把关。
//   - 一切会改变数据的动作都写入操作日志，且与业务变更处于同一事务（C-43）。
//   - 破坏性操作先做引用检查：模型被评分引用、提示词被题库引用时拒绝删除。
//   - 密钥类配置项经 security.SecretBox 加密；未配置主密钥时拒绝保存。
package admin

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/modelsdev"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/prompt"
)

type Deps struct {
	Pool    *pgxpool.Pool
	Config  *config.Store
	Logger  *slog.Logger
	Oplog   *oplog.Recorder
	Secrets *security.SecretBox

	// Mail 是环境变量提供的 SMTP 取值。系统设置中保存过同名配置项时以配置表为准。
	Mail mail.Config

	// ModelsDev 读取 models.dev 的模型目录（需求 §13.3）。
	ModelsDev *modelsdev.Client

	// Prompts 提供提示词写入能力。后台的「强制标记重大修改」复用前台的
	// 重大修改流程，删除与恢复也复用同一条路径，使评分归档、统计清空、
	// 回收站记录与推荐标签过期只有一处实现（需求 §7.3、§11）。
	Prompts PromptWriter

	// Moderation 提供被举报评论的删除能力，由社交服务实现。
	Moderation ContentModerator

	// Stats 是统计缓存失效入口，提示词被修改或彻底删除后调用。
	Stats StatsHook
}

// PromptWriter 是管理端用到的提示词写入能力声明，由 prompt.Service 实现。
type PromptWriter interface {
	Update(ctx context.Context, actor domain.User, id int64, in prompt.Input) (prompt.Result, error)
	Delete(ctx context.Context, actor domain.User, id int64, ip string) error
	Restore(ctx context.Context, actor domain.User, id int64, ip string) (domain.Prompt, error)
	Purge(ctx context.Context, actor domain.User, id int64, ip string) error
}

// ContentModerator 是内容处置能力的声明，由 social.Service 实现。
type ContentModerator interface {
	DeleteComment(ctx context.Context, v domain.Viewer, commentID int64, ip string) error
}

// StatsHook 是统计缓存失效入口的依赖声明。只声明用到的方法，
// 使 admin 与 stats 两个包之间不产生包级循环依赖。
type StatsHook interface {
	MarkStale(promptID int64)
}

type Service struct {
	pool      *pgxpool.Pool
	config    *config.Store
	logger    *slog.Logger
	oplog     *oplog.Recorder
	secrets   *security.SecretBox
	mail      mail.Config
	modelsDev *modelsdev.Client
	prompts   PromptWriter
	moderate  ContentModerator
	stats     StatsHook

	now func() time.Time
}

func New(deps Deps) *Service {
	recorder := deps.Oplog
	if recorder == nil {
		recorder = oplog.NewRecorder()
	}
	box := deps.Secrets
	if box == nil {
		box = security.NewSecretBox("")
	}
	client := deps.ModelsDev
	if client == nil {
		client = modelsdev.NewClient("")
	}
	return &Service{
		pool:      deps.Pool,
		config:    deps.Config,
		logger:    deps.Logger,
		oplog:     recorder,
		secrets:   box,
		mail:      deps.Mail,
		modelsDev: client,
		prompts:   deps.Prompts,
		moderate:  deps.Moderation,
		stats:     deps.Stats,
		now:       time.Now,
	}
}

// ensure 断言依赖齐备，避免空指针在运行期才暴露。
func (s *Service) ensure() error {
	if s.pool == nil || s.config == nil {
		return fmt.Errorf("管理后台依赖不完整：Pool 与 Config 不可为空")
	}
	return nil
}

// record 写入一条操作日志。写入与业务变更处于同一事务，避免有业务无日志。
func (s *Service) record(ctx context.Context, q *repo.Queries, entry oplog.Entry) error {
	if entry.OperatorName == "" {
		entry.OperatorName = s.operatorName(ctx, q, entry.OperatorID)
	}
	return s.oplog.Record(ctx, q, entry)
}

// operatorName 读取操作人昵称，供日志的冗余字段使用。
//
// 读取失败不阻断写入：日志的主体是操作类型与对象，昵称缺失时留空仍可追溯，
// 让一次读失败毁掉整个业务动作是更大的损失。
func (s *Service) operatorName(ctx context.Context, q *repo.Queries, operatorID int64) string {
	if operatorID == 0 {
		return ""
	}
	row, err := q.GetUserByID(ctx, operatorID)
	if err != nil {
		s.logger.Warn("读取操作人昵称失败",
			slog.Int64("user_id", operatorID), slog.String("error", err.Error()))
		return ""
	}
	return row.Username
}
