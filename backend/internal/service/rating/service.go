// Package rating 承载「提示词 × 模型」完成度评分的用例编排。
//
// 三类限制都在事务内判定（需求 §3.4）：
//   - 单条评分最多修改 3 次：由条件 UPDATE 保证，并发修改不会突破上限；
//   - 每用户每日最多修改 10 次；
//   - 每用户每日最多删除 5 次。
//
// 每日窗口以 UTC 自然日切分（C-03），边界由 domain.DayRange 在应用层计算后作为参数传入，
// SQL 中不出现 date_trunc 与任何依赖会话时区的表达式（开发方案 §3.4）。
package rating

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Config *config.Store
	Logger *slog.Logger
	// Stats 用于评分写入后作废统计缓存，并取统计重算锁与重算事务互斥。
	// 为空时评分功能仍可用，只是不参与统计刷新。
	Stats StatsHook
}

// StatsHook 是评分服务对统计服务的依赖声明。
// 只声明用到的两个方法，使两个包之间不产生包级循环依赖。
type StatsHook interface {
	// MarkStale 作废某提示词的统计缓存。
	MarkStale(promptID int64)
	// LockRecompute 在当前事务上取该提示词的统计重算锁。
	LockRecompute(ctx context.Context, q *repo.Queries, promptID int64) error
}

type Service struct {
	pool   *pgxpool.Pool
	config *config.Store
	logger *slog.Logger
	stats  StatsHook

	now func() time.Time
}

func New(deps Deps) *Service {
	return &Service{
		pool:   deps.Pool,
		config: deps.Config,
		logger: deps.Logger,
		stats:  deps.Stats,
		now:    time.Now,
	}
}

// syncStats 作废统计缓存。统计重算由读取路径按需触发（读放大），
// 评分提交只负责让下一次读取不再返回旧值。
func (s *Service) syncStats(promptID int64) {
	if s.stats != nil {
		s.stats.MarkStale(promptID)
	}
}

// lockStats 在当前事务上取统计重算锁，使重算读到的评分集合与本事务的写入互斥。
func (s *Service) lockStats(ctx context.Context, q *repo.Queries, promptID int64) error {
	if s.stats == nil {
		return nil
	}
	if err := s.stats.LockRecompute(ctx, q, promptID); err != nil {
		return fmt.Errorf("获取提示词 %d 的统计重算锁: %w", promptID, err)
	}
	return nil
}

// SubmitInput 是提交与修改评分的输入。
type SubmitInput struct {
	PromptID int64
	ModelID  int64
	Score    int
}

// Submit 提交评分。评分只接受 1-5 整数，空值不构成评分（C-01、需求 §3.2）。
func (s *Service) Submit(ctx context.Context, actor domain.User, in SubmitInput, ip string) (domain.Rating, error) {
	if !actor.CanWriteContent() {
		return domain.Rating{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if !domain.ValidScore(in.Score) {
		return domain.Rating{}, apperr.New(apperr.CodeValidationFailed, "评分须为 1 至 5 的整数")
	}

	var created domain.Rating
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		target, err := loadRateablePrompt(ctx, q, domain.ViewerFor(actor), in.PromptID)
		if err != nil {
			return err
		}
		if target.CurrentVersionID == nil {
			return apperr.New(apperr.CodeInvalidState, "提示词缺少当前版本")
		}
		if _, err := q.GetModel(ctx, in.ModelID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "模型不存在")
			}
			return fmt.Errorf("查询模型: %w", err)
		}

		// 与统计重算互斥：重算期间不写入新评分，快照才不会与评分错位。
		if err := s.lockStats(ctx, q, in.PromptID); err != nil {
			return err
		}

		row, err := q.CreateRating(ctx, repo.CreateRatingParams{
			UserID:          actor.ID,
			PromptID:        in.PromptID,
			PromptVersionID: *target.CurrentVersionID,
			ModelID:         in.ModelID,
			Score:           int16(in.Score),
		})
		if err != nil {
			// ON CONFLICT DO NOTHING 在冲突时不返回行。这是「已评过分」的判定依据，
			// 不是错误：并发提交同一「提示词 × 模型」时恰好一个事务写入成功，
			// 其余读到空行（开发方案 §3.5）。
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeRatingExists, "")
			}
			return fmt.Errorf("写入评分: %w", err)
		}

		if err := s.logChange(ctx, q, logEntry{
			RatingID:   row.ID,
			Actor:      actor,
			PromptID:   in.PromptID,
			ModelID:    in.ModelID,
			Action:     actionCreate,
			AfterScore: int16(in.Score),
			IP:         ip,
		}); err != nil {
			return err
		}

		rating, err := ratingFromRow(row)
		if err != nil {
			return err
		}
		created = rating
		return nil
	})
	if err != nil {
		return domain.Rating{}, err
	}
	s.syncStats(in.PromptID)
	return created, nil
}

// Update 修改已有评分。单条修改次数上限与每日修改限额都在事务内判定。
func (s *Service) Update(ctx context.Context, actor domain.User, in SubmitInput, ip string) (domain.Rating, error) {
	if !actor.CanWriteContent() {
		return domain.Rating{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	if !domain.ValidScore(in.Score) {
		return domain.Rating{}, apperr.New(apperr.CodeValidationFailed, "评分须为 1 至 5 的整数")
	}

	maxEdits, err := s.config.GetInt(ctx, "rating_max_edits_per_record")
	if err != nil {
		return domain.Rating{}, fmt.Errorf("读取单条评分修改次数上限: %w", err)
	}
	dailyLimit, err := s.config.GetInt(ctx, "rating_daily_edit_limit")
	if err != nil {
		return domain.Rating{}, fmt.Errorf("读取每日评分修改限额: %w", err)
	}

	var updated domain.Rating
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		existing, err := q.GetRating(ctx, repo.GetRatingParams{
			UserID:   actor.ID,
			PromptID: in.PromptID,
			ModelID:  in.ModelID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "评分不存在")
			}
			return fmt.Errorf("查询评分: %w", err)
		}

		// 先取用户锁再计数：并发修改各自读到未超限后都写入会突破每日限额。
		if err := q.LockRatingUser(ctx, actor.ID); err != nil {
			return fmt.Errorf("锁定每日评分修改限额: %w", err)
		}
		if err := s.checkDailyLimit(ctx, q, actor.ID, actionUpdate, dailyLimit); err != nil {
			return err
		}

		if err := s.lockStats(ctx, q, in.PromptID); err != nil {
			return err
		}

		// 单条上限由条件 UPDATE 保证：edit_count 已达上限时该行不会被更新。
		row, err := q.UpdateRatingScore(ctx, repo.UpdateRatingScoreParams{
			ID:       existing.ID,
			Score:    int16(in.Score),
			MaxEdits: int32(maxEdits),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeRatingEditLimitExceeded, "")
			}
			return fmt.Errorf("更新评分: %w", err)
		}

		if err := s.logChange(ctx, q, logEntry{
			RatingID:        existing.ID,
			Actor:           actor,
			PromptID:        in.PromptID,
			ModelID:         in.ModelID,
			Action:          actionUpdate,
			BeforeScore:     int16(existing.Score),
			AfterScore:      int16(in.Score),
			BeforeEditCount: existing.EditCount,
			AfterEditCount:  row.EditCount,
			IP:              ip,
		}); err != nil {
			return err
		}

		rating, err := ratingFromRow(row)
		if err != nil {
			return err
		}
		updated = rating
		return nil
	})
	if err != nil {
		return domain.Rating{}, err
	}
	s.syncStats(in.PromptID)
	return updated, nil
}

// Delete 删除评分。用户删除后重新评分视为新评分，按新的创建时间计入时间窗口
// （需求 §3.3）。
func (s *Service) Delete(ctx context.Context, actor domain.User, promptID, modelID int64, ip string) error {
	if !actor.CanWriteContent() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}
	dailyLimit, err := s.config.GetInt(ctx, "rating_daily_delete_limit")
	if err != nil {
		return fmt.Errorf("读取每日评分删除限额: %w", err)
	}

	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		existing, err := q.GetRating(ctx, repo.GetRatingParams{
			UserID:   actor.ID,
			PromptID: promptID,
			ModelID:  modelID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeNotFound, "评分不存在")
			}
			return fmt.Errorf("查询评分: %w", err)
		}

		// 先取用户锁再计数：并发删除各自读到未超限后都写入会突破每日限额。
		if err := q.LockRatingUser(ctx, actor.ID); err != nil {
			return fmt.Errorf("锁定每日评分删除限额: %w", err)
		}
		if err := s.checkDailyLimit(ctx, q, actor.ID, actionDelete, dailyLimit); err != nil {
			return err
		}

		if err := s.lockStats(ctx, q, promptID); err != nil {
			return err
		}

		if _, err := q.SoftDeleteRating(ctx, existing.ID); err != nil {
			return fmt.Errorf("软删除评分: %w", err)
		}

		if err := s.logChange(ctx, q, logEntry{
			RatingID:        existing.ID,
			Actor:           actor,
			PromptID:        promptID,
			ModelID:         modelID,
			Action:          actionDelete,
			BeforeScore:     int16(existing.Score),
			BeforeEditCount: existing.EditCount,
			IP:              ip,
		}); err != nil {
			return err
		}
		s.syncStats(promptID)
		return nil
	})
}

// checkDailyLimit 判定当日限额。窗口左闭右开，UTC 自然日零点为起点（C-03）。
//
// 错误码与单条限额分开：开发方案 §5.4 把 rating_edit_limit_exceeded 与
// rating_delete_limit_exceeded 定为「单条」修改与删除超限，当日累计属于配额，
// 用 quota_exceeded 表达，前端因此能给出「明日零点重试」而非「这条不能再改」。
func (s *Service) checkDailyLimit(ctx context.Context, q *repo.Queries, userID int64, action string, limit int64) error {
	start, end := domain.DayRange(s.now())
	count, err := q.CountRatingChangesInDay(ctx, repo.CountRatingChangesInDayParams{
		UserID:   userID,
		Action:   action,
		DayStart: repo.Timestamptz(start),
		DayEnd:   repo.Timestamptz(end),
	})
	if err != nil {
		return fmt.Errorf("统计当日评分操作次数: %w", err)
	}
	if count >= limit {
		return apperr.New(apperr.CodeQuotaExceeded, "当日评分操作次数已达上限，UTC 零点后重置")
	}
	return nil
}
