package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
)

// Login 校验口令并签发会话。被封禁账户不能登录（C-24）。
func (s *Service) Login(ctx context.Context, email, password, ip string) (Session, error) {
	normalized := normalizeEmail(email)

	row, err := repo.New(s.pool).GetUserByEmail(ctx, normalized)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, apperr.New(apperr.CodeInvalidCredentials, "")
		}
		return Session{}, fmt.Errorf("查询账户: %w", err)
	}

	user, err := repo.UserFromRow(row)
	if err != nil {
		return Session{}, err
	}
	if user.IsBanned() {
		return Session{}, apperr.New(apperr.CodeBanned, "")
	}
	if !user.HasPassword || !security.VerifyPassword(user.PasswordHash, password) {
		return Session{}, apperr.New(apperr.CodeInvalidCredentials, "")
	}

	session, err := s.issueSession(user)
	if err != nil {
		return Session{}, err
	}

	if user.IsAdmin() {
		if err := s.recordAdminLogin(ctx, user, ip); err != nil {
			return Session{}, err
		}
	}
	return session, nil
}

// recordAdminLogin 记录后台登录（需求 §17.8）。
func (s *Service) recordAdminLogin(ctx context.Context, user domain.User, ip string) error {
	q := repo.New(s.pool)
	return s.oplog.Record(ctx, q, oplog.Entry{
		OperatorID:   user.ID,
		OperatorName: user.Username,
		Action:       oplog.ActionAdminLogin,
		ObjectType:   "user",
		ObjectID:     user.ID,
		IP:           ip,
	})
}

// ChangePassword 修改口令并返回新会话令牌：口令变更提升令牌版本使其他设备的旧会话失效，
// 当前这次操作换发新令牌以保持本会话可用（G-13）。
func (s *Service) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) (Session, error) {
	current, err := s.userByID(ctx, userID)
	if err != nil {
		return Session{}, err
	}
	if !current.HasPassword || !security.VerifyPassword(current.PasswordHash, oldPassword) {
		return Session{}, apperr.New(apperr.CodeInvalidCredentials, "原口令不正确")
	}

	hash, err := security.HashPassword(newPassword)
	if err != nil {
		return Session{}, err
	}

	updated, err := repo.New(s.pool).UpdateUserPassword(ctx, repo.UpdateUserPasswordParams{
		ID:           userID,
		PasswordHash: repo.Text(hash),
	})
	if err != nil {
		return Session{}, fmt.Errorf("更新口令: %w", err)
	}

	user, err := repo.UserFromRow(updated)
	if err != nil {
		return Session{}, err
	}
	return s.issueSession(user)
}

// RequestPasswordReset 发送密码重置验证码。邮箱不存在或无口令时静默返回。
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	normalized := normalizeEmail(email)
	q := repo.New(s.pool)

	row, err := q.GetUserByEmail(ctx, normalized)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("查询账户: %w", err)
	}
	if !row.PasswordHash.Valid || row.PasswordHash.String == "" {
		return nil
	}
	if row.Status == string(domain.StatusBanned) {
		return apperr.New(apperr.CodeBanned, "")
	}
	return s.issueVerificationCode(ctx, q, row.ID, normalized, domain.PurposeResetPassword)
}

// ResetPassword 用验证码设置新口令，并使全部既有会话失效。
func (s *Service) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	normalized := normalizeEmail(email)
	q := repo.New(s.pool)

	if err := s.consumeVerificationCode(ctx, q, normalized, domain.PurposeResetPassword, code); err != nil {
		return err
	}

	row, err := q.GetUserByEmail(ctx, normalized)
	if err != nil {
		return apperr.New(apperr.CodeInvalidCode, "")
	}
	if row.Status == string(domain.StatusBanned) {
		return apperr.New(apperr.CodeBanned, "")
	}

	hash, err := security.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if _, err := q.UpdateUserPassword(ctx, repo.UpdateUserPasswordParams{
		ID:           row.ID,
		PasswordHash: repo.Text(hash),
	}); err != nil {
		return fmt.Errorf("更新口令: %w", err)
	}
	return nil
}

// UpdateProfile 修改昵称、简介与头像地址。昵称唯一；
// 头像地址可以为空，非空时必须是 http(s) 绝对地址。
func (s *Service) UpdateProfile(ctx context.Context, userID int64, username, bio, avatarURL string) (domain.User, error) {
	if avatarURL != "" {
		parsed, err := url.ParseRequestURI(avatarURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return domain.User{}, apperr.New(apperr.CodeValidationFailed, "头像地址必须是 http 或 https 绝对地址")
		}
	}

	q := repo.New(s.pool)

	if username != "" {
		exists, err := q.UsernameExists(ctx, username)
		if err != nil {
			return domain.User{}, fmt.Errorf("检查用户名: %w", err)
		}
		if exists {
			current, err := s.userByID(ctx, userID)
			if err != nil {
				return domain.User{}, err
			}
			if !strings.EqualFold(current.Username, username) {
				return domain.User{}, apperr.New(apperr.CodeConflict, "该昵称已被使用")
			}
		}
	} else {
		current, err := s.userByID(ctx, userID)
		if err != nil {
			return domain.User{}, err
		}
		username = current.Username
	}

	updated, err := q.UpdateUserProfile(ctx, repo.UpdateUserProfileParams{
		ID:        userID,
		Username:  username,
		Bio:       bio,
		AvatarUrl: avatarURL,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("更新资料: %w", err)
	}
	return repo.UserFromRow(updated)
}
