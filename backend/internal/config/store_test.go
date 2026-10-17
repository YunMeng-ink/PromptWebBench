package config_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/platform/pgtest"
)

func newStore(t *testing.T) *config.Store {
	t.Helper()
	pool := pgtest.MigratedDatabase(t)
	store := config.NewStore(pool, config.DefaultCacheTTL)
	t.Cleanup(store.Close)
	if err := store.Refresh(context.Background()); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}
	return store
}

func TestStoreReadsSeededTypedValues(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if got, err := store.GetInt(ctx, "bayes_prior"); err != nil || got != 3 {
		t.Fatalf("bayes_prior = %d, err = %v，期望 3", got, err)
	}
	if got, err := store.GetFloat(ctx, "saturation_threshold"); err != nil || got != 4.5 {
		t.Fatalf("saturation_threshold = %v, err = %v，期望 4.5", got, err)
	}
	if got, err := store.GetInt(ctx, "recent_window_days"); err != nil || got != 7 {
		t.Fatalf("recent_window_days = %d, err = %v，期望 7", got, err)
	}
	if got, err := store.GetString(ctx, "registration_policy"); err != nil || got != "open" {
		t.Fatalf("registration_policy = %q, err = %v，期望 open", got, err)
	}
	if got, err := store.GetBool(ctx, "collaborator_restore_allowed"); err != nil || !got {
		t.Fatalf("collaborator_restore_allowed = %v, err = %v，期望 true", got, err)
	}
	whitelist, err := store.GetStringSlice(ctx, "email_domain_whitelist")
	if err != nil {
		t.Fatalf("读取 email_domain_whitelist 失败: %v", err)
	}
	if len(whitelist) != 0 {
		t.Fatalf("email_domain_whitelist = %v，期望空表示不限制", whitelist)
	}
}

func TestStoreReportsMissingKey(t *testing.T) {
	store := newStore(t)
	_, err := store.GetInt(context.Background(), "no_such_key")
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("读取不存在的配置项应报错，实际 %v", err)
	}
}

func TestStoreReportsTypeMismatch(t *testing.T) {
	store := newStore(t)
	// registration_policy 是字符串，按整数读取必须报错而不是静默取零值。
	if _, err := store.GetInt(context.Background(), "registration_policy"); err == nil {
		t.Fatal("类型不符应报错")
	}
}

func TestSetInvalidatesCacheImmediately(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "bayes_k", 9, "测试覆盖", 0); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	got, err := store.GetInt(ctx, "bayes_k")
	if err != nil {
		t.Fatalf("写入后读取失败: %v", err)
	}
	if got != 9 {
		t.Fatalf("写入后 bayes_k = %d，期望 9（缓存应已失效）", got)
	}
}

func TestCacheExpiresAfterTTL(t *testing.T) {
	pool := pgtest.MigratedDatabase(t)
	const ttl = time.Second
	store := config.NewStore(pool, ttl)
	t.Cleanup(store.Close)
	ctx := context.Background()

	if err := store.Refresh(ctx); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}
	// 绕过 Store 直接改库，模拟另一个进程写入；缓存应在一个 TTL 后被替换。
	if _, err := pool.Exec(ctx, `UPDATE config SET value = '11'::jsonb WHERE key = 'bayes_k'`); err != nil {
		t.Fatalf("直接改库失败: %v", err)
	}

	if got, err := store.GetInt(ctx, "bayes_k"); err != nil || got != 5 {
		t.Fatalf("TTL 内读取 = %d, err = %v，期望命中缓存返回 5", got, err)
	}
	time.Sleep(ttl + 200*time.Millisecond)
	if got, err := store.GetInt(ctx, "bayes_k"); err != nil || got != 11 {
		t.Fatalf("TTL 后读取 = %d, err = %v，期望 11", got, err)
	}
}

func TestSecretValuesAreNotExposedAsPlainValues(t *testing.T) {
	pool := pgtest.MigratedDatabase(t)
	store := config.NewStore(pool, config.DefaultCacheTTL)
	t.Cleanup(store.Close)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO config (key, secret_ciphertext, is_secret, description)
		 VALUES ('smtp_password', 'ciphertext-placeholder', true, '测试密钥')`); err != nil {
		t.Fatalf("写入密钥类配置失败: %v", err)
	}
	if err := store.Refresh(ctx); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}
	if _, err := store.Get(ctx, "smtp_password"); err == nil {
		t.Fatal("密钥类配置不应出现在明文缓存中")
	}
}
