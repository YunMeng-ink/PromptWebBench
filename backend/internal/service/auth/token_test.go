package auth_test

import (
	"context"
	"testing"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/security"
)

func TestGenerateAPITokenReplacesPreviousToken(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "token@example.com", testPassword)

	first, err := h.service.GenerateAPIToken(ctx, userID, "token", "127.0.0.1")
	if err != nil {
		t.Fatalf("生成令牌失败: %v", err)
	}
	if _, err := h.service.ResolveAPIToken(ctx, first.Plain); err != nil {
		t.Fatalf("新令牌应可用: %v", err)
	}

	second, err := h.service.GenerateAPIToken(ctx, userID, "token", "127.0.0.1")
	if err != nil {
		t.Fatalf("重新生成令牌失败: %v", err)
	}
	if second.Plain == first.Plain {
		t.Fatal("重新生成应产生不同的令牌")
	}

	if _, err := h.service.ResolveAPIToken(ctx, first.Plain); err == nil {
		t.Fatal("旧令牌应立即失效")
	}
	if _, err := h.service.ResolveAPIToken(ctx, second.Plain); err != nil {
		t.Fatalf("新令牌应可用: %v", err)
	}

	if h.countRows(t, `SELECT count(*) FROM api_tokens WHERE user_id = $1 AND revoked_at IS NULL`, userID) != 1 {
		t.Fatal("同一时间只应存在一个有效令牌")
	}
	if h.countRows(t, `SELECT count(*) FROM operation_logs WHERE action = 'api_token_reset'`) != 2 {
		t.Fatal("每次生成令牌都应写入操作日志")
	}
}

func TestGenerateAPITokenStoresHashOnly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "hash@example.com", testPassword)

	token, err := h.service.GenerateAPIToken(ctx, userID, "hash", "127.0.0.1")
	if err != nil {
		t.Fatalf("生成令牌失败: %v", err)
	}

	var stored string
	if err := h.pool.QueryRow(ctx,
		`SELECT token_hash FROM api_tokens WHERE user_id = $1 AND revoked_at IS NULL`, userID).Scan(&stored); err != nil {
		t.Fatalf("读取令牌哈希失败: %v", err)
	}
	if stored == token.Plain {
		t.Fatal("库中不应存储令牌明文")
	}
	if stored != security.HashAPIToken(token.Plain) {
		t.Fatal("库中应存储令牌的 SHA-256 哈希")
	}
}

func TestUnverifiedAccountCannotGenerateToken(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.register(t, "unverified@example.com", testPassword); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	var userID int64
	if err := h.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE lower(email) = 'unverified@example.com'`).Scan(&userID); err != nil {
		t.Fatalf("查询账户失败: %v", err)
	}

	_, err := h.service.GenerateAPIToken(ctx, userID, "unverified", "127.0.0.1")
	expectCode(t, err, apperr.CodeEmailNotVerified)
}

func TestRevokeAPIToken(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "revoke@example.com", testPassword)

	token, err := h.service.GenerateAPIToken(ctx, userID, "revoke", "127.0.0.1")
	if err != nil {
		t.Fatalf("生成令牌失败: %v", err)
	}
	if err := h.service.RevokeAPIToken(ctx, userID); err != nil {
		t.Fatalf("撤销令牌失败: %v", err)
	}
	if _, err := h.service.ResolveAPIToken(ctx, token.Plain); err == nil {
		t.Fatal("撤销后的令牌不应可用")
	}
}

func TestAPITokenStatus(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "status@example.com", testPassword)

	status, err := h.service.APITokenStatus(ctx, userID)
	if err != nil {
		t.Fatalf("查询令牌状态失败: %v", err)
	}
	if status.Present {
		t.Fatal("尚未生成令牌时应报告不存在")
	}

	token, err := h.service.GenerateAPIToken(ctx, userID, "status", "127.0.0.1")
	if err != nil {
		t.Fatalf("生成令牌失败: %v", err)
	}
	status, err = h.service.APITokenStatus(ctx, userID)
	if err != nil {
		t.Fatalf("查询令牌状态失败: %v", err)
	}
	if !status.Present || status.Prefix != token.Prefix {
		t.Fatalf("令牌状态 = %+v，期望存在且前缀为 %s", status, token.Prefix)
	}
	if status.Prefix == token.Plain {
		t.Fatal("状态查询不应返回明文")
	}
}

func TestResolveAPITokenRejectsBannedAccount(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "bannedtoken@example.com", testPassword)

	token, err := h.service.GenerateAPIToken(ctx, userID, "banned", "127.0.0.1")
	if err != nil {
		t.Fatalf("生成令牌失败: %v", err)
	}
	if _, err := h.pool.Exec(ctx, `UPDATE users SET status = 'banned' WHERE id = $1`, userID); err != nil {
		t.Fatalf("更新账户状态失败: %v", err)
	}

	_, err = h.service.ResolveAPIToken(ctx, token.Plain)
	expectCode(t, err, apperr.CodeBanned)
}

func TestResolveAPITokenRejectsUnknownToken(t *testing.T) {
	h := newHarness(t)

	_, err := h.service.ResolveAPIToken(context.Background(), "pb_deadbeef")
	expectCode(t, err, apperr.CodeUnauthorized)
}

// TestAPITokenExpiresAndStatusReports 覆盖令牌有效期：签发 90 天后到期，
// 到期后解析失败，状态接口带出到期时间与过期标记。
func TestAPITokenExpiresAndStatusReports(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "expiry@example.com", testPassword)

	token, err := h.service.GenerateAPIToken(ctx, userID, "expiry", "127.0.0.1")
	if err != nil {
		t.Fatalf("生成令牌失败: %v", err)
	}
	status, err := h.service.APITokenStatus(ctx, userID)
	if err != nil {
		t.Fatalf("查询令牌状态失败: %v", err)
	}
	if !status.Present || status.Expired || status.ExpiresAt.IsZero() {
		t.Fatalf("新令牌应处于有效状态并带出到期时间：%+v", status)
	}

	// 把到期时刻提前到过去：令牌失效。
	if _, err := h.pool.Exec(ctx,
		`UPDATE api_tokens SET expires_at = now() - interval '1 minute'
		 WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		t.Fatalf("调整到期时刻失败: %v", err)
	}
	if _, err := h.service.ResolveAPIToken(ctx, token.Plain); err == nil {
		t.Fatal("过期令牌应解析失败")
	}
	status, err = h.service.APITokenStatus(ctx, userID)
	if err != nil {
		t.Fatalf("查询令牌状态失败: %v", err)
	}
	if !status.Present || !status.Expired {
		t.Fatalf("过期令牌应带出过期标记：%+v", status)
	}
}
