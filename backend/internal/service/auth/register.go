package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
)

type RegisterInput struct {
	Email      string
	Password   string
	InviteCode string
}

// Register 完成邮箱密码注册。注册结果一律为未验证态，需经邮箱验证码转正常（C-23）。
//
// 顺序为：只读预检 → 发送验证码 → 事务内建号、占用邀请码、落库验证码。
// 验证码发送失败时不产生任何写入（G-14）。
func (s *Service) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	email := normalizeEmail(in.Email)
	if err := s.checkEmailAllowed(ctx, email); err != nil {
		return domain.User{}, err
	}

	policy, err := s.registrationPolicy(ctx)
	if err != nil {
		return domain.User{}, err
	}

	inviteCode := normalizeInviteCode(in.InviteCode)
	if inviteCode == "" && !policy.AllowsPublicSignup() {
		return domain.User{}, apperr.New(apperr.CodeRegistrationClosed, "")
	}

	q := repo.New(s.pool)
	var invite *repo.InviteCode
	if inviteCode != "" {
		record, err := q.GetInviteCodeByCode(ctx, inviteCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.User{}, apperr.New(apperr.CodeInvalidInvite, "")
			}
			return domain.User{}, fmt.Errorf("查询邀请码: %w", err)
		}
		if !inviteUsable(record, s.now()) {
			return domain.User{}, apperr.New(apperr.CodeInvalidInvite, "")
		}
		invite = &record
	}

	if _, err := q.GetUserByEmail(ctx, email); err == nil {
		return domain.User{}, apperr.New(apperr.CodeEmailTaken, "")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, fmt.Errorf("检查邮箱占用: %w", err)
	}

	passwordHash, err := security.HashPassword(in.Password)
	if err != nil {
		return domain.User{}, err
	}
	code, err := security.GenerateVerificationCode()
	if err != nil {
		return domain.User{}, err
	}

	if err := s.mail.SendVerificationCode(ctx, email, code, domain.PurposeRegister); err != nil {
		s.logger.Error("注册验证码邮件发送失败",
			slog.String("email", maskEmail(email)),
			slog.String("error", err.Error()),
		)
		return domain.User{}, apperr.New(apperr.CodeMailSendFailed, "")
	}

	role := domain.RoleUser
	if invite != nil {
		kind, err := domain.ParseInviteKind(invite.Kind)
		if err != nil {
			return domain.User{}, fmt.Errorf("邀请码 %d: %w", invite.ID, err)
		}
		role = kind.RoleForInvite()
	}

	var created repo.User
	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		username, err := deriveUniqueUsername(ctx, q, usernameBase(email))
		if err != nil {
			return err
		}

		user, err := q.CreateUser(ctx, repo.CreateUserParams{
			Username:        username,
			Email:           email,
			PasswordHash:    repo.Text(passwordHash),
			Role:            string(role),
			Status:          string(domain.StatusUnverified),
			EmailVerifiedAt: repo.TimestamptzOrNull(nil),
		})
		if err != nil {
			return fmt.Errorf("创建账户: %w", err)
		}

		if invite != nil {
			if _, err := q.ConsumeInviteCode(ctx, invite.ID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return apperr.New(apperr.CodeInvalidInvite, "")
				}
				return fmt.Errorf("占用邀请码: %w", err)
			}
			if err := q.CreateInviteCodeUsage(ctx, repo.CreateInviteCodeUsageParams{
				InviteCodeID: invite.ID,
				UserID:       user.ID,
			}); err != nil {
				return fmt.Errorf("记录邀请码使用: %w", err)
			}
		}

		if err := q.SupersedeEmailVerifications(ctx, supersedeParams(email, domain.PurposeRegister)); err != nil {
			return fmt.Errorf("作废旧验证码: %w", err)
		}
		if _, err := q.CreateEmailVerification(ctx, repo.CreateEmailVerificationParams{
			UserID:    repo.Int8(user.ID),
			Email:     email,
			Code:      code,
			Purpose:   string(domain.PurposeRegister),
			ExpiresAt: repo.Timestamptz(s.now().Add(VerificationCodeTTL)),
		}); err != nil {
			return fmt.Errorf("保存验证码: %w", err)
		}

		created = user
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}

	return repo.UserFromRow(created)
}

// VerifyEmail 用验证码把账户从未验证态转为正常态。
func (s *Service) VerifyEmail(ctx context.Context, email, code string) (domain.User, error) {
	normalized := normalizeEmail(email)
	q := repo.New(s.pool)

	if err := s.consumeVerificationCode(ctx, q, normalized, domain.PurposeRegister, code); err != nil {
		return domain.User{}, err
	}

	row, err := q.GetUserByEmail(ctx, normalized)
	if err != nil {
		return domain.User{}, apperr.New(apperr.CodeInvalidCode, "")
	}
	if row.Status == string(domain.StatusNormal) {
		return domain.User{}, apperr.New(apperr.CodeAlreadyVerified, "")
	}
	if row.Status == string(domain.StatusBanned) {
		return domain.User{}, apperr.New(apperr.CodeBanned, "")
	}

	updated, err := q.VerifyUserEmail(ctx, row.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, apperr.New(apperr.CodeAlreadyVerified, "")
		}
		return domain.User{}, fmt.Errorf("更新邮箱验证状态: %w", err)
	}
	return repo.UserFromRow(updated)
}

// ResendVerification 重发注册验证码。邮箱不存在时静默返回，避免暴露注册情况。
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	normalized := normalizeEmail(email)
	q := repo.New(s.pool)

	row, err := q.GetUserByEmail(ctx, normalized)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("查询账户: %w", err)
	}
	switch domain.UserStatus(row.Status) {
	case domain.StatusNormal:
		return apperr.New(apperr.CodeAlreadyVerified, "")
	case domain.StatusBanned:
		return apperr.New(apperr.CodeBanned, "")
	}

	return s.issueVerificationCode(ctx, q, row.ID, normalized, domain.PurposeRegister)
}

func inviteUsable(record repo.InviteCode, now time.Time) bool {
	if record.Disabled || record.UsedCount >= record.MaxUses {
		return false
	}
	if record.ExpiresAt.Valid && !record.ExpiresAt.Time.After(now) {
		return false
	}
	return true
}

func normalizeInviteCode(raw string) string {
	return strings.TrimSpace(raw)
}
