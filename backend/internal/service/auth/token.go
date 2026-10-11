package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
)

// APIToken 是一次生成的令牌信息。明文只在生成时返回一次。
type APIToken struct {
	Plain  string
	Prefix string
}

// APITokenStatus 是当前令牌的展示信息，不含明文。
type APITokenStatus struct {
	Present   bool
	Prefix    string
	CreatedAt time.Time
}

// GenerateAPIToken 生成唯一有效令牌：先撤销既有令牌，再签发新令牌（需求 §16.3）。
// 未验证账户不可生成令牌（C-23）。
func (s *Service) GenerateAPIToken(ctx context.Context, userID int64, operatorName, ip string) (APIToken, error) {
	user, err := s.userByID(ctx, userID)
	if err != nil {
		return APIToken{}, err
	}
	if !user.CanWriteContent() {
		return APIToken{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}

	plain, hash, prefix, err := security.GenerateAPIToken()
	if err != nil {
		return APIToken{}, err
	}

	err = repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		if err := q.RevokeActiveAPITokens(ctx, userID); err != nil {
			return fmt.Errorf("撤销既有令牌: %w", err)
		}
		if _, err := q.CreateAPIToken(ctx, repo.CreateAPITokenParams{
			UserID:      userID,
			TokenHash:   hash,
			TokenPrefix: prefix,
		}); err != nil {
			return fmt.Errorf("创建令牌: %w", err)
		}
		return s.oplog.Record(ctx, q, oplog.Entry{
			OperatorID:   userID,
			OperatorName: operatorName,
			Action:       oplog.ActionAPITokenReset,
			ObjectType:   "user",
			ObjectID:     userID,
			IP:           ip,
		})
	})
	if err != nil {
		return APIToken{}, err
	}
	return APIToken{Plain: plain, Prefix: prefix}, nil
}

// RevokeAPIToken 撤销当前有效令牌。
func (s *Service) RevokeAPIToken(ctx context.Context, userID int64) error {
	if err := repo.New(s.pool).RevokeActiveAPITokens(ctx, userID); err != nil {
		return fmt.Errorf("撤销令牌: %w", err)
	}
	return nil
}

// APITokenStatus 返回当前令牌的展示信息。
func (s *Service) APITokenStatus(ctx context.Context, userID int64) (APITokenStatus, error) {
	record, err := repo.New(s.pool).GetActiveAPIToken(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APITokenStatus{Present: false}, nil
		}
		return APITokenStatus{}, fmt.Errorf("查询令牌: %w", err)
	}
	return APITokenStatus{
		Present:   true,
		Prefix:    record.TokenPrefix,
		CreatedAt: record.CreatedAt.Time,
	}, nil
}
