// Package migrate 实现内置迁移 runner：启动时按文件名顺序幂等应用迁移。
// 每个文件一个事务，失败即中止启动；已应用的文件记录在 schema_migrations 表。
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/migrations"
)

const createSchemaMigrations = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    checksum   TEXT        NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// Apply 应用全部未执行的迁移。已应用且校验和一致的文件跳过；校验和不一致说明
// 已发布的迁移被修改，属于禁止行为，直接报错中止启动。
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, createSchemaMigrations); err != nil {
		return fmt.Errorf("创建 schema_migrations 表: %w", err)
	}

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, name := range fileNames() {
		content, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("读取迁移 %s: %w", name, err)
		}
		sum := checksum(content)

		if recorded, ok := applied[name]; ok {
			if recorded != sum {
				return fmt.Errorf("迁移 %s 的校验和与已应用记录不一致：已应用 %s，当前 %s", name, recorded, sum)
			}
			continue
		}

		if err := applyOne(ctx, pool, name, string(content), sum); err != nil {
			return err
		}
	}
	return nil
}

// fileNames 返回按文件名升序排列的顶层迁移文件。
func fileNames() []string {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		panic(fmt.Errorf("读取内嵌迁移目录: %w", err))
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("读取 schema_migrations: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var version, sum string
		if err := rows.Scan(&version, &sum); err != nil {
			return nil, fmt.Errorf("扫描 schema_migrations: %w", err)
		}
		out[version] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 schema_migrations: %w", err)
	}
	return out, nil
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, name, sql, sum string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启迁移事务 %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("执行迁移 %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, name, sum); err != nil {
		return fmt.Errorf("记录迁移 %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交迁移 %s: %w", name, err)
	}
	return nil
}

func checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
