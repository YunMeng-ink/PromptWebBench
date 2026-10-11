// Package pgtest 为集成测试提供真实 PostgreSQL 环境。
//
// 测试库通过 TEST_DATABASE_URL 指向一个可建库的维护库；每个用例在独立数据库上运行，
// 迁移真实执行，用例结束即销毁。这样集成测试覆盖的是与生产相同的建表与查询路径。
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/promptbench/backend/internal/platform/migrate"
)

// EnvVar 是维护库连接串所在的环境变量名。
const EnvVar = "TEST_DATABASE_URL"

// NewDatabase 创建一个空测试库并返回其连接池，用例结束时自动销毁。
func NewDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv(EnvVar)
	if dsn == "" {
		t.Fatalf("%s 未设置：集成测试需要真实 PostgreSQL，例如 postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable", EnvVar)
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("连接维护库失败: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("维护库不可达: %v", err)
	}

	name := "pb_test_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatalf("创建测试库 %s 失败: %v", name, err)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		admin.Close()
		t.Fatalf("解析连接串失败: %v", err)
	}
	cfg.ConnConfig.Database = name

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		admin.Close()
		t.Fatalf("连接测试库 %s 失败: %v", name, err)
	}

	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("销毁测试库 %s 失败: %v", name, err)
		}
		admin.Close()
	})
	return pool
}

// MigratedDatabase 创建空测试库并应用全部迁移。
func MigratedDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := NewDatabase(t)
	if err := migrate.Apply(context.Background(), pool); err != nil {
		t.Fatalf("应用迁移失败: %v", err)
	}
	return pool
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("生成测试库后缀失败: %v", err)
	}
	return fmt.Sprintf("%d_%s", os.Getpid(), hex.EncodeToString(buf))
}

// UserOptions 描述一个测试账户。
type UserOptions struct {
	Username string
	Email    string
	Password string
	Role     string
	Status   string
	Verified bool
}

// SeedUser 直接建号，用于把测试置于某个既定状态。密码按 bcrypt 存储，
// 因此用例可以用它真实登录。
func SeedUser(t *testing.T, pool *pgxpool.Pool, opts UserOptions) int64 {
	t.Helper()
	ctx := context.Background()

	role := opts.Role
	if role == "" {
		role = "user"
	}
	status := opts.Status
	if status == "" {
		status = "normal"
	}
	username := opts.Username
	if username == "" {
		username = "user_" + randomSuffix(t)
	}

	var passwordHash pgtype.Text
	if opts.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(opts.Password), 10)
		if err != nil {
			t.Fatalf("生成测试口令哈希失败: %v", err)
		}
		passwordHash = pgtype.Text{String: string(hash), Valid: true}
	}

	var verifiedAt pgtype.Timestamptz
	if opts.Verified {
		verifiedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}

	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash, role, status, email_verified_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		username, strings.ToLower(opts.Email), passwordHash, role, status, verifiedAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("创建测试账户失败: %v", err)
	}
	return id
}

// SeedPrompt 插入一条公开提示词及其首个版本，用于验证提示词转移等流程。
func SeedPrompt(t *testing.T, pool *pgxpool.Pool, uploaderID int64, title string) int64 {
	t.Helper()
	ctx := context.Background()

	var categoryID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM categories WHERE slug = 'reasoning'`).Scan(&categoryID); err != nil {
		t.Fatalf("读取类别失败: %v", err)
	}

	var promptID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO prompts (title, body, category_id, visibility, uploader_id)
		VALUES ($1, '测试正文', $2, 'public', $3)
		RETURNING id`, title, categoryID, uploaderID).Scan(&promptID); err != nil {
		t.Fatalf("创建测试提示词失败: %v", err)
	}

	var versionID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO prompt_versions (
			prompt_id, version_no, major, minor, title, body,
			category_id, visibility, sensitive_level, author_id
		) VALUES ($1, 'v1.0', 1, 0, $2, '测试正文', $3, 'public', 'low', $4)
		RETURNING id`, promptID, title, categoryID, uploaderID).Scan(&versionID); err != nil {
		t.Fatalf("创建测试提示词版本失败: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE prompts SET current_version_id = $1 WHERE id = $2`, versionID, promptID); err != nil {
		t.Fatalf("回填当前版本失败: %v", err)
	}
	return promptID
}
