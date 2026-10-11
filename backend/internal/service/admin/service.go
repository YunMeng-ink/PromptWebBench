// Package admin 承载管理后台的用例编排。阶段 1 覆盖账户治理部分。
package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

type Service struct {
	pool   *pgxpool.Pool
	oplog  *oplog.Recorder
	logger *slog.Logger

	now func() time.Time
}

func New(deps Deps) *Service {
	return &Service{pool: deps.Pool, oplog: oplog.NewRecorder(), logger: deps.Logger, now: time.Now}
}

// BanResult 描述一次封禁的结果。
type BanResult struct {
	User                domain.User
	TransferredPrompts  int
	TransferredToSystem bool
}

// BanUser 封禁账户：提升令牌版本使既有会话失效（C-24），
// 并把其名下提示词转移到系统账户「沧海遗粟」，保留原上传者与转移日志（C-25、需求 §15.2）。
func (s *Service) BanUser(ctx context.Context, operator domain.User, targetID int64, reason, ip string) (BanResult, error) {
	if targetID == operator.ID {
		return BanResult{}, apperr.New(apperr.CodeInvalidState, "不能封禁自己")
	}

	var result BanResult
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		target, err := lockTarget(ctx, q, targetID)
		if err != nil {
			return err
		}
		if target.IsSystem {
			return apperr.New(apperr.CodeInvalidState, "系统账户不可封禁")
		}
		if target.IsBanned() {
			return apperr.New(apperr.CodeInvalidState, "账户已处于封禁状态")
		}

		system, err := q.GetSystemUser(ctx)
		if err != nil {
			return fmt.Errorf("读取系统账户: %w", err)
		}

		updated, err := q.SetUserStatus(ctx, repo.SetUserStatusParams{
			ID:           targetID,
			Status:       string(domain.StatusBanned),
			BannedAt:     repo.Timestamptz(s.now()),
			BannedReason: reason,
		})
		if err != nil {
			return fmt.Errorf("更新账户状态: %w", err)
		}

		moved, err := q.TransferPrompts(ctx, repo.TransferPromptsParams{
			FromUploader: targetID,
			ToUploader:   system.ID,
		})
		if err != nil {
			return fmt.Errorf("转移提示词: %w", err)
		}
		for _, promptID := range moved {
			if err := q.InsertTransferLog(ctx, repo.InsertTransferLogParams{
				PromptID:   promptID,
				FromUserID: targetID,
				ToUserID:   system.ID,
				Reason:     "user_banned",
				OperatorID: repo.Int8(operator.ID),
			}); err != nil {
				return fmt.Errorf("记录提示词转移: %w", err)
			}
		}

		if err := s.oplog.Record(ctx, q, oplog.Entry{
			OperatorID:   operator.ID,
			OperatorName: operator.Username,
			Action:       oplog.ActionUserBan,
			ObjectType:   "user",
			ObjectID:     targetID,
			Before:       map[string]any{"status": target.Status},
			After:        map[string]any{"status": domain.StatusBanned, "reason": reason},
			IP:           ip,
		}); err != nil {
			return err
		}
		if len(moved) > 0 {
			if err := s.oplog.Record(ctx, q, oplog.Entry{
				OperatorID:   operator.ID,
				OperatorName: operator.Username,
				Action:       oplog.ActionUserTransferPrompts,
				ObjectType:   "user",
				ObjectID:     targetID,
				After:        map[string]any{"to_user_id": system.ID, "prompt_count": len(moved)},
				IP:           ip,
			}); err != nil {
				return err
			}
		}

		user, err := repo.UserFromRow(updated)
		if err != nil {
			return err
		}
		result = BanResult{User: user, TransferredPrompts: len(moved), TransferredToSystem: len(moved) > 0}
		return nil
	})
	if err != nil {
		return BanResult{}, err
	}
	return result, nil
}

// UnbanUser 解封账户。转移过的提示词不自动转回原用户（C-25），
// 账户状态按邮箱是否已验证恢复到正常或未验证。
func (s *Service) UnbanUser(ctx context.Context, operator domain.User, targetID int64, ip string) (domain.User, error) {
	var result domain.User
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		target, err := lockTarget(ctx, q, targetID)
		if err != nil {
			return err
		}
		if !target.IsBanned() {
			return apperr.New(apperr.CodeInvalidState, "账户未处于封禁状态")
		}

		restored := domain.StatusUnverified
		if target.EmailVerifiedAt != nil {
			restored = domain.StatusNormal
		}

		updated, err := q.SetUserStatus(ctx, repo.SetUserStatusParams{
			ID:           targetID,
			Status:       string(restored),
			BannedAt:     repo.TimestamptzOrNull(nil),
			BannedReason: "",
		})
		if err != nil {
			return fmt.Errorf("更新账户状态: %w", err)
		}

		if err := s.oplog.Record(ctx, q, oplog.Entry{
			OperatorID:   operator.ID,
			OperatorName: operator.Username,
			Action:       oplog.ActionUserUnban,
			ObjectType:   "user",
			ObjectID:     targetID,
			Before:       map[string]any{"status": target.Status},
			After:        map[string]any{"status": restored},
			IP:           ip,
		}); err != nil {
			return err
		}

		user, err := repo.UserFromRow(updated)
		if err != nil {
			return err
		}
		result = user
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}
	return result, nil
}

// SetDeveloper 授予或撤销开发者身份（需求 §2.3、§17.4）。
func (s *Service) SetDeveloper(ctx context.Context, operator domain.User, targetID int64, enabled bool, ip string) (domain.User, error) {
	if targetID == operator.ID {
		return domain.User{}, apperr.New(apperr.CodeInvalidState, "不能变更自己的身份组")
	}

	var result domain.User
	err := repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		target, err := lockTarget(ctx, q, targetID)
		if err != nil {
			return err
		}
		if target.IsSystem {
			return apperr.New(apperr.CodeInvalidState, "系统账户不可变更身份组")
		}
		if target.IsBanned() {
			return apperr.New(apperr.CodeInvalidState, "封禁账户不可变更身份组")
		}
		if target.IsAdmin() {
			return apperr.New(apperr.CodeInvalidState, "管理员账户不可变更身份组")
		}

		next := domain.RoleUser
		action := oplog.ActionDeveloperRevoke
		if enabled {
			next = domain.RoleDeveloper
			action = oplog.ActionDeveloperGrant
		}
		if target.Role == next {
			return apperr.New(apperr.CodeInvalidState, "账户已处于目标身份组")
		}

		updated, err := q.SetUserRole(ctx, repo.SetUserRoleParams{ID: targetID, Role: string(next)})
		if err != nil {
			return fmt.Errorf("更新身份组: %w", err)
		}

		if err := s.oplog.Record(ctx, q, oplog.Entry{
			OperatorID:   operator.ID,
			OperatorName: operator.Username,
			Action:       action,
			ObjectType:   "user",
			ObjectID:     targetID,
			Before:       map[string]any{"role": target.Role},
			After:        map[string]any{"role": next},
			IP:           ip,
		}); err != nil {
			return err
		}

		user, err := repo.UserFromRow(updated)
		if err != nil {
			return err
		}
		result = user
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}
	return result, nil
}

// UserByID 查询账户，供后台查看。
func (s *Service) UserByID(ctx context.Context, id int64) (domain.User, error) {
	row, err := repo.New(s.pool).GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, apperr.New(apperr.CodeNotFound, "")
		}
		return domain.User{}, fmt.Errorf("查询账户: %w", err)
	}
	return repo.UserFromRow(row)
}

func lockTarget(ctx context.Context, q *repo.Queries, id int64) (domain.User, error) {
	row, err := q.LockUser(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, apperr.New(apperr.CodeNotFound, "")
		}
		return domain.User{}, fmt.Errorf("锁定账户: %w", err)
	}
	return repo.UserFromRow(row)
}
