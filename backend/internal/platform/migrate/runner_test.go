package migrate_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/platform/migrate"
	"github.com/promptbench/backend/internal/platform/pgtest"
)

func countTables(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`).Scan(&n)
	if err != nil {
		t.Fatalf("统计表数量失败: %v", err)
	}
	return n
}

func countMigrated(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("统计迁移记录失败: %v", err)
	}
	return n
}

func TestApplyBuildsFullSchema(t *testing.T) {
	pool := pgtest.NewDatabase(t)
	if err := migrate.Apply(context.Background(), pool); err != nil {
		t.Fatalf("应用迁移失败: %v", err)
	}

	// 39 张业务表（开发方案 §4.1），另有引擎自建的 schema_migrations。
	if got := countTables(t, pool); got != 40 {
		t.Fatalf("表数量 = %d，期望 40（39 张业务表 + schema_migrations）", got)
	}
	if got := countMigrated(t, pool); got != 3 {
		t.Fatalf("迁移记录数 = %d，期望 3", got)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	pool := pgtest.NewDatabase(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatalf("首次应用迁移失败: %v", err)
	}
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatalf("重复应用迁移失败: %v", err)
	}
	if got := countMigrated(t, pool); got != 3 {
		t.Fatalf("重复应用后迁移记录数 = %d，期望仍为 3", got)
	}
	if got := countTables(t, pool); got != 40 {
		t.Fatalf("重复应用后表数量 = %d，期望仍为 40", got)
	}
}

func TestApplyDetectsChecksumDrift(t *testing.T) {
	pool := pgtest.NewDatabase(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatalf("应用迁移失败: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE schema_migrations SET checksum = 'deadbeef' WHERE version = '002_schema.sql'`); err != nil {
		t.Fatalf("篡改校验和失败: %v", err)
	}

	err := migrate.Apply(ctx, pool)
	if err == nil {
		t.Fatal("已发布的迁移被修改后应报错")
	}
	if !strings.Contains(err.Error(), "校验和") {
		t.Fatalf("错误信息未指出校验和不一致: %v", err)
	}
}

func TestSeedDataIsComplete(t *testing.T) {
	pool := pgtest.MigratedDatabase(t)
	ctx := context.Background()

	cases := []struct {
		table string
		want  int
	}{
		{"categories", 13},
		{"abilities", 8},
		{"difficulties", 3},
		{"config", 26},
	}
	for _, tc := range cases {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+tc.table).Scan(&n); err != nil {
			t.Fatalf("统计 %s 失败: %v", tc.table, err)
		}
		if n != tc.want {
			t.Errorf("%s 行数 = %d，期望 %d", tc.table, n, tc.want)
		}
	}

	var (
		username string
		isSystem bool
		status   string
	)
	err := pool.QueryRow(ctx,
		`SELECT username, is_system, status FROM users WHERE is_system`).
		Scan(&username, &isSystem, &status)
	if err != nil {
		t.Fatalf("查询系统账户失败: %v", err)
	}
	if username != "沧海遗粟" || !isSystem || status != "normal" {
		t.Fatalf("系统账户 = %s/%v/%s，期望 沧海遗粟/true/normal", username, isSystem, status)
	}

	// 越狱对抗类别是默认的高风险类别（C-19）。
	var highRisk bool
	if err := pool.QueryRow(ctx,
		`SELECT is_high_risk FROM categories WHERE slug = 'jailbreak'`).Scan(&highRisk); err != nil {
		t.Fatalf("查询越狱对抗类别失败: %v", err)
	}
	if !highRisk {
		t.Fatal("越狱对抗类别应默认标记为高风险")
	}
}

func TestDownScriptsDropEverything(t *testing.T) {
	pool := pgtest.MigratedDatabase(t)
	ctx := context.Background()

	// 直接执行仓库中的逆向脚本，验证建表与删表互相对称。
	downDir := filepath.Join("..", "..", "..", "migrations", "down")
	for _, name := range []string{"003_seed.sql", "002_schema.sql", "001_extensions.sql"} {
		content, err := os.ReadFile(filepath.Join(downDir, name))
		if err != nil {
			t.Fatalf("读取逆向脚本 %s 失败: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("执行逆向脚本 %s 失败: %v", name, err)
		}
	}

	if got := countTables(t, pool); got != 1 {
		t.Fatalf("逆向后表数量 = %d，期望仅剩 schema_migrations", got)
	}
}
