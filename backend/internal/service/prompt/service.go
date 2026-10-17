// Package prompt 承载提示词的用例编排：创建、修改、重大修改、删除与读取。
package prompt

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Config *config.Store
	Logger *slog.Logger

	// Oplog 写入操作日志。删除、恢复与彻底删除提示词都要留痕（需求 §17.8），
	// 无论动作来自作者本人还是管理后台。
	Oplog *oplog.Recorder

	// Stats 是统计缓存失效入口。提示词被修改、删除或彻底删除后调用，
	// 使下一次读取重新计算而不是返回旧快照。
	Stats StatsHook
}

// StatsHook 是统计缓存失效入口的依赖声明，只声明用到的方法，
// 使 prompt 与 stats 两个包之间不产生包级循环依赖。
type StatsHook interface {
	MarkStale(promptID int64)
}

type Service struct {
	pool   *pgxpool.Pool
	config *config.Store
	logger *slog.Logger
	oplog  *oplog.Recorder
	stats  StatsHook

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
		stats:  deps.Stats,
		now:    time.Now,
	}
}

// markStale 作废提示词的统计缓存。未注入统计服务时什么也不做。
func (s *Service) markStale(promptID int64) {
	if s.stats != nil {
		s.stats.MarkStale(promptID)
	}
}

// majorChangeTagKinds 是重大修改时需要转入待复核的推荐标签类型（C-12）：
// 过时与训练饱和两族标签都要由管理员重新决策。
func majorChangeTagKinds() []string {
	return domain.RecommendTagKindStrings()
}

// Input 是创建与更新共用的字段集合。两条路径共用同一套校验与权限判定函数，
// 使「只能由开发者使用」的约束在两个入口上同时生效（开发方案 §3.3）。
type Input struct {
	Title         string
	Body          string
	Description   string
	PurposeID     int64
	CapabilityID  int64
	DifficultyID  *int64
	Visibility    domain.Visibility
	Sensitive     domain.SensitiveLevel
	TagIDs        []int64
	ChangeNote    string
	IsMajorChange bool
}

// Result 是一次写入的结果。MajorChange 为真时 ArchivedRatings 给出被归档的评分条数，
// 前端据此展示重大修改提示（需求 §7.3 第 5 步）。
type Result struct {
	Prompt          domain.Prompt
	VersionNo       string
	MajorChange     bool
	ArchivedRatings int64
}

// Create 新建提示词：初始版本为 v1.0。公开提示词不占用私有额度（需求 §8.2、§8.3）。
func (s *Service) Create(ctx context.Context, actor domain.User, in Input) (Result, error) {
	viewer := domain.ViewerFor(actor)
	if !viewer.CanWrite() {
		return Result{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}

	var result Result
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		purpose, err := loadPurpose(ctx, q, in.PurposeID)
		if err != nil {
			return err
		}
		// 考察目的与可见性的权限判定在事务内以真实字典数据执行，与更新路径共用同一函数。
		if err := viewer.AuthorizePurpose(purpose.IsHighRisk, in.Visibility); err != nil {
			return translatePermission(err)
		}
		if err := s.validateReferences(ctx, q, in); err != nil {
			return err
		}
		if in.Visibility == domain.VisibilityPrivate {
			if err := s.checkPrivateQuota(ctx, q, viewer, actor.Role); err != nil {
				return err
			}
		}

		version := domain.FirstVersion()
		row, err := q.InsertPrompt(ctx, repo.InsertPromptParams{
			Title:           in.Title,
			Body:            in.Body,
			Description:     in.Description,
			PurposeID:       in.PurposeID,
			CapabilityID:    in.CapabilityID,
			DifficultyID:    repo.Int8OrNull(derefInt64(in.DifficultyID)),
			Visibility:      string(in.Visibility),
			SensitiveLevel:  string(in.Sensitive),
			UploaderID:      actor.ID,
			LatestVersionNo: version.String(),
			MajorModified:   false,
		})
		if err != nil {
			return fmt.Errorf("写入提示词: %w", err)
		}

		versionRow, err := q.InsertPromptVersion(ctx, repo.InsertPromptVersionParams{
			PromptID:       row.ID,
			VersionNo:      version.String(),
			Major:          int32(version.Major),
			Minor:          int32(version.Minor),
			Title:          in.Title,
			Body:           in.Body,
			Description:    in.Description,
			PurposeID:      in.PurposeID,
			CapabilityID:   in.CapabilityID,
			DifficultyID:   repo.Int8OrNull(derefInt64(in.DifficultyID)),
			Visibility:     string(in.Visibility),
			SensitiveLevel: string(in.Sensitive),
			IsMajor:        false,
			ChangeNote:     in.ChangeNote,
			AuthorID:       actor.ID,
		})
		if err != nil {
			return fmt.Errorf("写入初始版本: %w", err)
		}

		updated, err := q.SetPromptCurrentVersion(ctx, repo.SetPromptCurrentVersionParams{
			ID:               row.ID,
			CurrentVersionID: repo.Int8(versionRow.ID),
			LatestVersionNo:  version.String(),
			MajorModified:    false,
		})
		if err != nil {
			return fmt.Errorf("设置当前版本: %w", err)
		}

		if err := replaceTags(ctx, q, row.ID, in.TagIDs); err != nil {
			return err
		}

		fields := fieldsOfPrompt(updated)
		// 写入路径的返回行不联表取考察目的，高风险标记直接用事务内已校验的目的赋值。
		fields.IsHighRisk = purpose.IsHighRisk
		prompt, err := promptFromRow(fields)
		if err != nil {
			return err
		}
		result = Result{Prompt: prompt, VersionNo: version.String()}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// Update 修改提示词。作者可主动选择重大修改（需求 §7.1）；
// 管理员可强制标记重大修改（需求 §7.1）。
//
// 普通修改只生成新版本并切换前台展示版本，评分与历史版本全部保留（需求 §7.2）。
// 重大修改在版本切换之外，还要重置评分、归档历史评分、清空统计快照（需求 §7.3）。
func (s *Service) Update(ctx context.Context, actor domain.User, id int64, in Input) (Result, error) {
	viewer := domain.ViewerFor(actor)
	if !viewer.CanWrite() {
		return Result{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}

	var result Result
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		locked, err := lockPrompt(ctx, q, id)
		if err != nil {
			return err
		}
		if !viewer.CanManage(locked.Access()) {
			return apperr.New(apperr.CodeNotFound, "")
		}

		purpose, err := loadPurpose(ctx, q, in.PurposeID)
		if err != nil {
			return err
		}
		if err := viewer.AuthorizePurpose(purpose.IsHighRisk, in.Visibility); err != nil {
			return translatePermission(err)
		}
		if err := s.validateReferences(ctx, q, in); err != nil {
			return err
		}
		if in.Visibility == domain.VisibilityPrivate && locked.Visibility != domain.VisibilityPrivate {
			if err := s.checkPrivateQuota(ctx, q, viewer, actor.Role); err != nil {
				return err
			}
		}

		current := locked.LatestVersion
		// 重大修改由作者主动选择；管理员也走同一入口，勾选后强制按重大修改处理。
		major := in.IsMajorChange
		next := current.Next(major)

		// prompts 表冗余保存当前版本的字段，先同步内容再切换版本指针，
		// 使列表筛选与详情读到的始终是同一份取值。
		if _, err := q.UpdatePromptContent(ctx, repo.UpdatePromptContentParams{
			ID:             locked.ID,
			Title:          in.Title,
			Body:           in.Body,
			Description:    in.Description,
			PurposeID:      in.PurposeID,
			CapabilityID:   in.CapabilityID,
			DifficultyID:   repo.Int8OrNull(derefInt64(in.DifficultyID)),
			Visibility:     string(in.Visibility),
			SensitiveLevel: string(in.Sensitive),
		}); err != nil {
			return fmt.Errorf("同步提示词内容: %w", err)
		}

		versionRow, err := q.InsertPromptVersion(ctx, repo.InsertPromptVersionParams{
			PromptID:       locked.ID,
			VersionNo:      next.String(),
			Major:          int32(next.Major),
			Minor:          int32(next.Minor),
			Title:          in.Title,
			Body:           in.Body,
			Description:    in.Description,
			PurposeID:      in.PurposeID,
			CapabilityID:   in.CapabilityID,
			DifficultyID:   repo.Int8OrNull(derefInt64(in.DifficultyID)),
			Visibility:     string(in.Visibility),
			SensitiveLevel: string(in.Sensitive),
			IsMajor:        major,
			ChangeNote:     in.ChangeNote,
			AuthorID:       actor.ID,
		})
		if err != nil {
			return fmt.Errorf("写入新版本: %w", err)
		}

		var archived int64
		if major {
			// 需求 §7.3 第 4 步：历史评分归档后再清空当前评分。
			ids, err := q.ArchivePromptRatings(ctx, repo.ArchivePromptRatingsParams{
				Reason:   archiveReason,
				PromptID: locked.ID,
			})
			if err != nil {
				return fmt.Errorf("归档历史评分: %w", err)
			}
			if _, err := q.DeletePromptRatingsForMajorChange(ctx, locked.ID); err != nil {
				return fmt.Errorf("清空当前评分: %w", err)
			}
			// 需求 §7.3 第 6 步：区分度与完成度按新版本重新计算，阶段 3 消费新评分。
			if err := q.DeletePromptStatsForMajorChange(ctx, locked.ID); err != nil {
				return fmt.Errorf("清空统计快照: %w", err)
			}
			if err := q.DeletePromptModelStatsForMajorChange(ctx, locked.ID); err != nil {
				return fmt.Errorf("清空模型统计快照: %w", err)
			}
			// 需求 §7.3 第 7 步、C-12：已有的过时与训练饱和标签转待复核，
			// 由管理员决定是否继续展示。转待复核而不是删除，保留判定依据。
			if err := q.MoveTagsToReview(ctx, repo.MoveTagsToReviewParams{
				PromptID: locked.ID,
				Kinds:    majorChangeTagKinds(),
			}); err != nil {
				return fmt.Errorf("推荐标签转待复核: %w", err)
			}
			archived = int64(len(ids))
		}

		updated, err := q.SetPromptCurrentVersion(ctx, repo.SetPromptCurrentVersionParams{
			ID:               locked.ID,
			CurrentVersionID: repo.Int8(versionRow.ID),
			LatestVersionNo:  next.String(),
			MajorModified:    major,
		})
		if err != nil {
			return fmt.Errorf("切换当前版本: %w", err)
		}

		if err := replaceTags(ctx, q, locked.ID, in.TagIDs); err != nil {
			return err
		}

		fields := fieldsOfPrompt(updated)
		// 同创建路径：写入返回行不联表，高风险标记取自事务内已校验的考察目的。
		fields.IsHighRisk = purpose.IsHighRisk
		prompt, err := promptFromRow(fields)
		if err != nil {
			return err
		}
		result = Result{
			Prompt:          prompt,
			VersionNo:       next.String(),
			MajorChange:     major,
			ArchivedRatings: archived,
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	// 修改后快照与评分集合都已改变，作废进程内缓存，使下一次读取重新计算。
	s.markStale(id)
	return result, nil
}

// archiveReason 是评分归档原因，取值与 prompt_version_ratings.archive_reason 的约定一致。
const archiveReason = "major_modification"
