// Package auth 承载认证与账户用例。
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/promptbench/backend/internal/repo"
)

// initialAdminUsername 是初始管理员的登录名。
const initialAdminUsername = "admin"

// EnsureInitialAdmin 幂等地确保初始管理员存在（开发方案 §8.3）。
// 未提供 ADMIN_INIT_PASSWORD 时跳过创建，不生成任何默认口令。
func EnsureInitialAdmin(ctx context.Context, pool *pgxpool.Pool, email, password string, logger *slog.Logger) error {
	if password == "" {
		logger.Warn("未设置 ADMIN_INIT_PASSWORD，跳过初始管理员创建")
		return nil
	}

	q := repo.New(pool)
	existing, err := q.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		logger.Info("初始管理员已存在，跳过创建", slog.Int64("user_id", existing.ID))
		return nil
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("查询初始管理员: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("生成管理员口令哈希: %w", err)
	}

	created, err := q.CreateUser(ctx, repo.CreateUserParams{
		Username:        initialAdminUsername,
		Email:           email,
		PasswordHash:    repo.Text(string(hash)),
		Role:            "admin",
		Status:          "normal",
		EmailVerifiedAt: repo.Timestamptz(time.Now()),
	})
	if err != nil {
		return fmt.Errorf("创建初始管理员: %w", err)
	}

	logger.Info("已创建初始管理员，请尽快修改口令",
		slog.Int64("user_id", created.ID),
		slog.String("email", created.Email),
	)
	return nil
}
