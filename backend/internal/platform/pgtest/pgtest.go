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
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

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
