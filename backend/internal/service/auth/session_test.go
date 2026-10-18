package auth_test

import (
	"context"
	"strings"
	"testing"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/platform/pgtest"
)

func TestLoginIssuesUsableSession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "login@example.com", testPassword)

	session := h.login(t, "LOGIN@example.com", testPassword)
	if session.User.ID != userID {
		t.Fatalf("会话用户 = %d，期望 %d", session.User.ID, userID)
	}
	if session.Token == "" {
		t.Fatal("会话令牌为空")
	}

	claims, err := h.signer.Verify(session.Token)
	if err != nil {
		t.Fatalf("校验会话令牌失败: %v", err)
	}
	resolved, err := h.service.ResolveSession(ctx, claims.UserID, claims.TokenVersion)
	if err != nil {
		t.Fatalf("解析会话失败: %v", err)
	}
	if resolved.ID != userID {
		t.Fatalf("解析出的用户 = %d，期望 %d", resolved.ID, userID)
	}
}

func TestLoginRejectsWrongCredentials(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t, "creds@example.com", testPassword)

	_, err := h.service.Login(context.Background(), "creds@example.com", "WrongPassw0rd!", "127.0.0.1")
	expectCode(t, err, apperr.CodeInvalidCredentials)

	_, err = h.service.Login(context.Background(), "nobody@example.com", testPassword, "127.0.0.1")
	expectCode(t, err, apperr.CodeInvalidCredentials)
}

func TestLoginRejectsBannedAccount(t *testing.T) {
	h := newHarness(t)
	userID := h.registerAndVerify(t, "banned@example.com", testPassword)
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE users SET status = 'banned' WHERE id = $1`, userID); err != nil {
		t.Fatalf("更新账户状态失败: %v", err)
	}

	_, err := h.service.Login(context.Background(), "banned@example.com", testPassword, "127.0.0.1")
	expectCode(t, err, apperr.CodeBanned)
}

func TestAdminLoginIsRecorded(t *testing.T) {
	h := newHarness(t)
	pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "admin@example.com", Password: testPassword, Role: "admin", Verified: true,
	})

	h.login(t, "admin@example.com", testPassword)

	if h.countRows(t, `SELECT count(*) FROM operation_logs WHERE action = 'admin_login'`) != 1 {
		t.Fatal("管理员登录应写入操作日志")
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.registerAndVerify(t, "logout@example.com", testPassword)

	session := h.login(t, "logout@example.com", testPassword)
	claims, err := h.signer.Verify(session.Token)
	if err != nil {
		t.Fatalf("校验会话令牌失败: %v", err)
	}
	if _, err := h.service.ResolveSession(ctx, claims.UserID, claims.TokenVersion); err != nil {
		t.Fatalf("登出前会话应有效: %v", err)
	}

	if err := h.service.Logout(ctx, session.User.ID); err != nil {
		t.Fatalf("登出失败: %v", err)
	}
	if _, err := h.service.ResolveSession(ctx, claims.UserID, claims.TokenVersion); err == nil {
		t.Fatal("登出后旧会话应失效")
	}
}

func TestChangePasswordInvalidatesOtherSessions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "rotate@example.com", testPassword)

	old := h.login(t, "rotate@example.com", testPassword)
	oldClaims, err := h.signer.Verify(old.Token)
	if err != nil {
		t.Fatalf("校验会话令牌失败: %v", err)
	}

	fresh, err := h.service.ChangePassword(ctx, userID, testPassword, "NewPassw0rd!")
	if err != nil {
		t.Fatalf("修改口令失败: %v", err)
	}

	if _, err := h.service.ResolveSession(ctx, oldClaims.UserID, oldClaims.TokenVersion); err == nil {
		t.Fatal("改密后旧会话应失效")
	}

	freshClaims, err := h.signer.Verify(fresh.Token)
	if err != nil {
		t.Fatalf("校验新会话令牌失败: %v", err)
	}
	if _, err := h.service.ResolveSession(ctx, freshClaims.UserID, freshClaims.TokenVersion); err != nil {
		t.Fatalf("改密后当前会话应继续可用: %v", err)
	}

	if _, err := h.service.Login(ctx, "rotate@example.com", testPassword, "127.0.0.1"); err == nil {
		t.Fatal("旧口令应不能再登录")
	}
	if _, err := h.service.Login(ctx, "rotate@example.com", "NewPassw0rd!", "127.0.0.1"); err != nil {
		t.Fatalf("新口令应可登录: %v", err)
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	h := newHarness(t)
	userID := h.registerAndVerify(t, "changepw@example.com", testPassword)

	_, err := h.service.ChangePassword(context.Background(), userID, "WrongPassw0rd!", "NewPassw0rd!")
	expectCode(t, err, apperr.CodeInvalidCredentials)
}

func TestPasswordResetFlow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.registerAndVerify(t, "reset@example.com", testPassword)
	versionBefore := h.tokenVersion(t, "reset@example.com")

	session := h.login(t, "reset@example.com", testPassword)
	claims, err := h.signer.Verify(session.Token)
	if err != nil {
		t.Fatalf("校验会话令牌失败: %v", err)
	}

	h.mail.Reset()
	if err := h.service.RequestPasswordReset(ctx, "reset@example.com"); err != nil {
		t.Fatalf("请求重置失败: %v", err)
	}
	code := h.mail.LastCode(t)

	if err := h.service.ResetPassword(ctx, "reset@example.com", code, "ResetPassw0rd!"); err != nil {
		t.Fatalf("重置口令失败: %v", err)
	}

	if _, err := h.service.ResolveSession(ctx, claims.UserID, claims.TokenVersion); err == nil {
		t.Fatal("重置口令后旧会话应失效")
	}
	if h.tokenVersion(t, "reset@example.com") <= versionBefore {
		t.Fatalf("令牌版本 = %d，重置口令后应高于 %d", h.tokenVersion(t, "reset@example.com"), versionBefore)
	}
	if _, err := h.service.Login(ctx, "reset@example.com", testPassword, "127.0.0.1"); err == nil {
		t.Fatal("旧口令应不能再登录")
	}
	if _, err := h.service.Login(ctx, "reset@example.com", "ResetPassw0rd!", "127.0.0.1"); err != nil {
		t.Fatalf("新口令应可登录: %v", err)
	}
}

func TestResetPasswordRejectsWrongCode(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.registerAndVerify(t, "resetcode@example.com", testPassword)

	if err := h.service.RequestPasswordReset(ctx, "resetcode@example.com"); err != nil {
		t.Fatalf("请求重置失败: %v", err)
	}
	expectCode(t, h.service.ResetPassword(ctx, "resetcode@example.com", "000000", "ResetPassw0rd!"), apperr.CodeInvalidCode)
}

func TestRequestPasswordResetForUnknownEmailIsSilent(t *testing.T) {
	h := newHarness(t)

	if err := h.service.RequestPasswordReset(context.Background(), "ghost@example.com"); err != nil {
		t.Fatalf("未知邮箱不应暴露错误: %v", err)
	}
	if len(h.mail.Messages()) != 0 {
		t.Fatal("未知邮箱不应发出邮件")
	}
}

func TestUpdateProfile(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "profile@example.com", testPassword)

	updated, err := h.service.UpdateProfile(ctx, userID, "新昵称", "这里是简介", "https://example.com/avatar.png")
	if err != nil {
		t.Fatalf("更新资料失败: %v", err)
	}
	if updated.Username != "新昵称" || updated.Bio != "这里是简介" {
		t.Fatalf("资料未生效: %+v", updated)
	}

	// 昵称留空表示保持原值。
	kept, err := h.service.UpdateProfile(ctx, userID, "", "只改简介", "")
	if err != nil {
		t.Fatalf("更新资料失败: %v", err)
	}
	if kept.Username != "新昵称" {
		t.Fatalf("昵称 = %s，期望保持原值", kept.Username)
	}

	other := h.registerAndVerify(t, "other@example.com", testPassword)
	_, err = h.service.UpdateProfile(ctx, other, "新昵称", "", "")
	expectCode(t, err, apperr.CodeConflict)

	// 昵称上限按字符数计，超出即拒绝。
	tooLong := strings.Repeat("长", domain.UsernameMaxRunes+1)
	_, err = h.service.UpdateProfile(ctx, userID, tooLong, "", "")
	expectCode(t, err, apperr.CodeValidationFailed)

	// 正好等于上限的中文昵称可以保存。
	atLimit := strings.Repeat("长", domain.UsernameMaxRunes)
	saved, err := h.service.UpdateProfile(ctx, userID, atLimit, "", "")
	if err != nil {
		t.Fatalf("保存上限长度昵称失败: %v", err)
	}
	if saved.Username != atLimit {
		t.Fatalf("昵称 = %q，期望 %q", saved.Username, atLimit)
	}
}
