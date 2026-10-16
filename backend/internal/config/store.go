package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/repo"
)

// DefaultCacheTTL 是 config 表读取的默认缓存时长（开发方案 §6.1）。
const DefaultCacheTTL = 30 * time.Second

// Store 是 config 表的带缓存读取入口。缓存整表快照，按 TTL 惰性刷新。
type Store struct {
	pool *pgxpool.Pool
	ttl  time.Duration

	mu       sync.RWMutex
	snapshot map[string][]byte
	loadedAt time.Time
}

func NewStore(pool *pgxpool.Pool, ttl time.Duration) *Store {
	return &Store{pool: pool, ttl: ttl}
}

// Get 返回键的 JSON 原始值。键不存在或类型不符一律报错，由调用方决定如何呈现。
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	if err := s.ensureFresh(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.snapshot[key]
	if !ok {
		return nil, fmt.Errorf("配置项 %s 不存在", key)
	}
	return raw, nil
}

func (s *Store) GetInt(ctx context.Context, key string) (int64, error) {
	raw, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	var v int64
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, fmt.Errorf("配置项 %s 不是整数: %w", key, err)
	}
	return v, nil
}

func (s *Store) GetFloat(ctx context.Context, key string) (float64, error) {
	raw, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, fmt.Errorf("配置项 %s 不是数值: %w", key, err)
	}
	return v, nil
}

func (s *Store) GetString(ctx context.Context, key string) (string, error) {
	raw, err := s.Get(ctx, key)
	if err != nil {
		return "", err
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("配置项 %s 不是字符串: %w", key, err)
	}
	return v, nil
}

func (s *Store) GetBool(ctx context.Context, key string) (bool, error) {
	raw, err := s.Get(ctx, key)
	if err != nil {
		return false, err
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, fmt.Errorf("配置项 %s 不是布尔值: %w", key, err)
	}
	return v, nil
}

func (s *Store) GetStringSlice(ctx context.Context, key string) ([]string, error) {
	raw, err := s.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	var v []string
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("配置项 %s 不是字符串数组: %w", key, err)
	}
	return v, nil
}

// Set 写入非密钥配置项并立即失效本地缓存。
func (s *Store) Set(ctx context.Context, key string, value any, description string, updatedBy int64) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("编码配置项 %s: %w", key, err)
	}
	q := repo.New(s.pool)
	if err := q.UpsertConfig(ctx, repo.UpsertConfigParams{
		Key:         key,
		Value:       raw,
		Description: description,
		UpdatedBy:   repo.Int8OrNull(updatedBy),
	}); err != nil {
		return fmt.Errorf("写入配置项 %s: %w", key, err)
	}
	s.invalidate()
	return nil
}

// SetSecret 写入密钥类配置项。入参已是密文：加密由调用方在主密钥存在时完成，
// 本层不接触明文，也不接受空密文（config 表的 CHECK 要求两个字段同时存在）。
func (s *Store) SetSecret(ctx context.Context, key, ciphertext, description string, updatedBy int64) error {
	if ciphertext == "" {
		return fmt.Errorf("配置项 %s 的密文不可为空", key)
	}
	q := repo.New(s.pool)
	if err := q.UpsertSecretConfig(ctx, repo.UpsertSecretConfigParams{
		Key:              key,
		SecretCiphertext: repo.Text(ciphertext),
		Description:      description,
		UpdatedBy:        repo.Int8OrNull(updatedBy),
	}); err != nil {
		return fmt.Errorf("写入密钥配置项 %s: %w", key, err)
	}
	s.invalidate()
	return nil
}

// Refresh 强制刷新缓存，供管理端保存后立即生效。
func (s *Store) Refresh(ctx context.Context) error {
	rows, err := repo.New(s.pool).ListConfig(ctx)
	if err != nil {
		return fmt.Errorf("读取 config 表: %w", err)
	}
	snapshot := make(map[string][]byte, len(rows))
	for _, row := range rows {
		if row.IsSecret {
			continue
		}
		snapshot[row.Key] = row.Value
	}
	s.mu.Lock()
	s.snapshot = snapshot
	s.loadedAt = time.Now()
	s.mu.Unlock()
	return nil
}

func (s *Store) invalidate() {
	s.mu.Lock()
	s.loadedAt = time.Time{}
	s.mu.Unlock()
}

func (s *Store) ensureFresh(ctx context.Context) error {
	s.mu.RLock()
	fresh := !s.loadedAt.IsZero() && time.Since(s.loadedAt) < s.ttl
	s.mu.RUnlock()
	if fresh {
		return nil
	}
	return s.Refresh(ctx)
}
