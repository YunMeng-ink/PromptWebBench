package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/domain"
)

// InTx 在单个事务内执行 fn；fn 返回错误即回滚，成功则提交。
// 配额判定、邀请码占用、统计快照写入等需要并发安全的操作都经由它。
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启事务: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交事务: %w", err)
	}
	return nil
}

// UserFromRow 把数据库行映射为账户实体。角色与状态的取值受 CHECK 约束保护，
// 出现未登记取值说明库被外部改动，直接返回错误而不静默降级。
func UserFromRow(row User) (domain.User, error) {
	role, err := domain.ParseRole(row.Role)
	if err != nil {
		return domain.User{}, fmt.Errorf("账户 %d: %w", row.ID, err)
	}
	status, err := domain.ParseStatus(row.Status)
	if err != nil {
		return domain.User{}, fmt.Errorf("账户 %d: %w", row.ID, err)
	}

	out := domain.User{
		ID:           row.ID,
		Username:     row.Username,
		Email:        row.Email,
		Role:         role,
		Status:       status,
		IsSystem:     row.IsSystem,
		Bio:          row.Bio,
		AvatarURL:    row.AvatarUrl,
		TokenVersion: row.TokenVersion,
		HasPassword:  row.PasswordHash.Valid && row.PasswordHash.String != "",
	}
	if out.HasPassword {
		out.PasswordHash = row.PasswordHash.String
	}
	out.EmailVerifiedAt = timePtr(row.EmailVerifiedAt)
	return out, nil
}

func timePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	value := ts.Time
	return &value
}

// 少量可空列在 None 语义上的转换辅助。0 与 "" 在本项目模型中不代表合法主键或文本，
// 因此统一作为“无值”处理。

func Int8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }

func Int8OrNull(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func Text(v string) pgtype.Text { return pgtype.Text{String: v, Valid: true} }

func TextOrNull(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func Timestamptz(v time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: v, Valid: true}
}

// TimestamptzOrNull 把可选时间转为可空列取值。
func TimestamptzOrNull(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}
