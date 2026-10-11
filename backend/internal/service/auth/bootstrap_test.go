package auth_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/service/auth"
)

const (
	adminEmail    = "admin@prompt-benchmark.local"
	adminPassword = "AdminInitPassw0rd!"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func countUsers(t *testing.T, pool *pgxpool.Pool, email string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM users WHERE lower(email) = lower($1)`, email).Scan(&n); err != nil {
		t.Fatalf("统计用户失败: %v", err)
	}
	return n
}

func TestEnsureInitialAdminCreatesVerifiedAdmin(t *testing.T) {
	pool := pgtest.MigratedDatabase(t)
	ctx := context.Background()

	if err := auth.EnsureInitialAdmin(ctx, pool, adminEmail, adminPassword, discardLogger()); err != nil {
		t.Fatalf("创建初始管理员失败: %v", err)
	}

	var (
		role     string
		status   string
		hash     string
		verified bool
	)
	err := pool.QueryRow(ctx,
		`SELECT role, status, password_hash, email_verified_at IS NOT NULL
		 FROM users WHERE lower(email) = lower($1)`, adminEmail).
		Scan(&role, &status, &hash, &verified)
	if err != nil {
		t.Fatalf("查询初始管理员失败: %v", err)
	}
	if role != "admin" || status != "normal" || !verified {
		t.Fatalf("管理员 = %s/%s/验证=%v，期望 admin/normal/true", role, status, verified)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(adminPassword)); err != nil {
		t.Fatalf("管理员口令哈希与初始口令不匹配: %v", err)
	}
}

func TestEnsureInitialAdminIsIdempotent(t *testing.T) {
	pool := pgtest.MigratedDatabase(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := auth.EnsureInitialAdmin(ctx, pool, adminEmail, adminPassword, discardLogger()); err != nil {
			t.Fatalf("第 %d 次创建失败: %v", i+1, err)
		}
	}
	if got := countUsers(t, pool, adminEmail); got != 1 {
		t.Fatalf("管理员数量 = %d，期望 1", got)
	}
}

func TestEnsureInitialAdminSkipsWithoutPassword(t *testing.T) {
	pool := pgtest.MigratedDatabase(t)

	if err := auth.EnsureInitialAdmin(context.Background(), pool, adminEmail, "", discardLogger()); err != nil {
		t.Fatalf("未设置口令时不应报错: %v", err)
	}
	if got := countUsers(t, pool, adminEmail); got != 0 {
		t.Fatalf("未设置口令时不应创建管理员，实际创建了 %d 个", got)
	}
}
