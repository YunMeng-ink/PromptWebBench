package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/service/auth"
)

func expectCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望返回错误码 %s，实际成功", code)
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("错误类型 = %T，期望 *apperr.Error（%v）", err, err)
	}
	if appErr.Code != code {
		t.Fatalf("错误码 = %s，期望 %s", appErr.Code, code)
	}
}

func TestRegisterCreatesUnverifiedAccountAndSendsCode(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	user, err := h.service.Register(ctx, auth.RegisterInput{Email: "Alice@Example.com", Password: testPassword})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	if user.Status != domain.StatusUnverified {
		t.Fatalf("注册后状态 = %s，期望 unverified", user.Status)
	}
	if user.Role != domain.RoleUser {
		t.Fatalf("注册后身份组 = %s，期望 user", user.Role)
	}
	if user.EmailVerifiedAt != nil {
		t.Fatal("注册后邮箱不应处于已验证状态")
	}
	if user.Email != "alice@example.com" {
		t.Fatalf("邮箱 = %s，期望小写归一", user.Email)
	}
	if user.Username == "" {
		t.Fatal("注册后应生成昵称")
	}

	code := h.mail.LastCode(t)
	if h.countRows(t, `SELECT count(*) FROM email_verifications WHERE lower(email) = 'alice@example.com' AND purpose = 'register' AND consumed_at IS NULL`) != 1 {
		t.Fatal("应存在一条未消费的注册验证码")
	}
	if _, err := h.service.VerifyEmail(ctx, "alice@example.com", code); err != nil {
		t.Fatalf("验证失败: %v", err)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t, "dup@example.com", testPassword)

	_, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "DUP@example.com", Password: testPassword})
	expectCode(t, err, apperr.CodeEmailTaken)
}

func TestRegisterHonoursEmailDomainWhitelist(t *testing.T) {
	h := newHarness(t)
	h.setConfig(t, "email_domain_whitelist", []string{"allowed.example.com", "@second.example.com"})

	_, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "a@blocked.example.com", Password: testPassword})
	expectCode(t, err, apperr.CodeEmailNotAllowed)

	if _, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "a@allowed.example.com", Password: testPassword}); err != nil {
		t.Fatalf("白名单内邮箱应可注册: %v", err)
	}
	if _, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "b@second.example.com", Password: testPassword}); err != nil {
		t.Fatalf("带 @ 前缀的白名单项应可匹配: %v", err)
	}
}

func TestRegistrationPolicyClosedBlocksPublicSignup(t *testing.T) {
	h := newHarness(t)
	h.setConfig(t, "registration_policy", string(domain.RegistrationClosed))

	_, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "closed@example.com", Password: testPassword})
	expectCode(t, err, apperr.CodeRegistrationClosed)
}

func TestClosedRegistrationStillAllowsInviteCode(t *testing.T) {
	h := newHarness(t)
	h.setConfig(t, "registration_policy", string(domain.RegistrationClosed))
	h.seedInviteCode(t, "NORMAL-INVITE", string(domain.InviteNormal), 1, time.Hour)

	user, err := h.service.Register(context.Background(), auth.RegisterInput{
		Email: "invited@example.com", Password: testPassword, InviteCode: "NORMAL-INVITE",
	})
	if err != nil {
		t.Fatalf("关闭公开注册时邀请码注册应可用: %v", err)
	}
	if user.Role != domain.RoleUser {
		t.Fatalf("普通邀请码注册后身份组 = %s，期望 user", user.Role)
	}
}

func TestInviteOnlyRequiresInviteCode(t *testing.T) {
	h := newHarness(t)
	h.setConfig(t, "registration_policy", string(domain.RegistrationInviteOnly))

	_, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "noinvite@example.com", Password: testPassword})
	expectCode(t, err, apperr.CodeRegistrationClosed)

	h.seedInviteCode(t, "DEV-INVITE", string(domain.InviteDeveloper), 1, time.Hour)
	user, err := h.service.Register(context.Background(), auth.RegisterInput{
		Email: "dev@example.com", Password: testPassword, InviteCode: "DEV-INVITE",
	})
	if err != nil {
		t.Fatalf("邀请码注册失败: %v", err)
	}
	if user.Role != domain.RoleDeveloper {
		t.Fatalf("开发者邀请码注册后身份组 = %s，期望 developer", user.Role)
	}
}

func TestInviteCodeIsConsumedOnce(t *testing.T) {
	h := newHarness(t)
	h.seedInviteCode(t, "ONE-SHOT", string(domain.InviteNormal), 1, time.Hour)

	if _, err := h.service.Register(context.Background(), auth.RegisterInput{
		Email: "first@example.com", Password: testPassword, InviteCode: "ONE-SHOT",
	}); err != nil {
		t.Fatalf("首次使用邀请码失败: %v", err)
	}

	_, err := h.service.Register(context.Background(), auth.RegisterInput{
		Email: "second@example.com", Password: testPassword, InviteCode: "ONE-SHOT",
	})
	expectCode(t, err, apperr.CodeInvalidInvite)

	var used int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT used_count FROM invite_codes WHERE code = 'ONE-SHOT'`).Scan(&used); err != nil {
		t.Fatalf("查询邀请码使用次数失败: %v", err)
	}
	if used != 1 {
		t.Fatalf("邀请码使用次数 = %d，期望 1", used)
	}
}

func TestInviteCodeRejectsUnknownExpiredAndDisabled(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	_, err := h.service.Register(ctx, auth.RegisterInput{Email: "unknown@example.com", Password: testPassword, InviteCode: "NOPE"})
	expectCode(t, err, apperr.CodeInvalidInvite)

	h.seedInviteCode(t, "EXPIRED", string(domain.InviteNormal), 1, -time.Minute)
	_, err = h.service.Register(ctx, auth.RegisterInput{Email: "expired@example.com", Password: testPassword, InviteCode: "EXPIRED"})
	expectCode(t, err, apperr.CodeInvalidInvite)

	h.seedInviteCode(t, "DISABLED", string(domain.InviteNormal), 1, time.Hour)
	h.disableInviteCode(t, "DISABLED")
	_, err = h.service.Register(ctx, auth.RegisterInput{Email: "disabled@example.com", Password: testPassword, InviteCode: "DISABLED"})
	expectCode(t, err, apperr.CodeInvalidInvite)
}

func TestRegisterDoesNotPersistWhenMailFails(t *testing.T) {
	// 指向一个没有 SMTP 服务的端口，发送必然失败（G-14：发送失败不落库）。
	h := newHarnessWith(t, options{mail: mail.NewSMTPSender(mail.Config{
		Host: "127.0.0.1", Port: "1", From: "noreply@example.com",
	})})

	_, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "mailfail@example.com", Password: testPassword})
	expectCode(t, err, apperr.CodeMailSendFailed)

	if h.countRows(t, `SELECT count(*) FROM users WHERE lower(email) = 'mailfail@example.com'`) != 0 {
		t.Fatal("邮件发送失败时不应创建账户")
	}
	if h.countRows(t, `SELECT count(*) FROM email_verifications WHERE lower(email) = 'mailfail@example.com'`) != 0 {
		t.Fatal("邮件发送失败时不应落库验证码")
	}
}

func TestVerifyEmailActivatesAccountAndConsumesCode(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.register(t, "verify@example.com", testPassword); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	code := h.mail.LastCode(t)

	user, err := h.service.VerifyEmail(ctx, "verify@example.com", code)
	if err != nil {
		t.Fatalf("验证失败: %v", err)
	}
	if user.Status != domain.StatusNormal {
		t.Fatalf("验证后状态 = %s，期望 normal", user.Status)
	}
	if user.EmailVerifiedAt == nil {
		t.Fatal("验证后应记录邮箱验证时间")
	}

	// 同一个验证码不能再用。
	_, err = h.service.VerifyEmail(ctx, "verify@example.com", code)
	expectCode(t, err, apperr.CodeInvalidCode)
}

func TestVerifyEmailRejectsWrongCode(t *testing.T) {
	h := newHarness(t)
	if _, err := h.register(t, "wrongcode@example.com", testPassword); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	_, err := h.service.VerifyEmail(context.Background(), "wrongcode@example.com", "000000")
	expectCode(t, err, apperr.CodeInvalidCode)
}

// TestVerificationCodeInvalidatedAfterRepeatedFailures 覆盖防穷举：
// 同一验证码连续输错达到上限后作废，此后输入正确验证码也无效，必须重新申请。
func TestVerificationCodeInvalidatedAfterRepeatedFailures(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.register(t, "lockout@example.com", testPassword); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	code := h.mail.LastCode(t)
	wrong := "000000"
	if code == wrong {
		wrong = "000001"
	}

	// 失败上限与实现中的 maxVerificationFailures 一致。
	const attempts = 5
	for i := 0; i < attempts; i++ {
		if _, err := h.service.VerifyEmail(ctx, "lockout@example.com", wrong); err == nil {
			t.Fatalf("第 %d 次错误验证码竟验证通过", i+1)
		}
	}

	// 失败达到上限后验证码已作废：正确验证码也无法通过。
	_, err := h.service.VerifyEmail(ctx, "lockout@example.com", code)
	expectCode(t, err, apperr.CodeInvalidCode)

	// 重新申请验证码后恢复正常。
	if err := h.service.ResendVerification(ctx, "lockout@example.com"); err != nil {
		t.Fatalf("重新申请验证码失败: %v", err)
	}
	fresh := h.mail.LastCode(t)
	if _, err := h.service.VerifyEmail(ctx, "lockout@example.com", fresh); err != nil {
		t.Fatalf("新验证码验证失败: %v", err)
	}
}

func TestVerifyEmailOnVerifiedAccountReportsAlreadyVerified(t *testing.T) {
	h := newHarness(t)
	h.registerAndVerify(t, "already@example.com", testPassword)

	// 直接写入一条有效验证码，模拟已验证账户仍持有验证码的情形。
	_, err := h.pool.Exec(context.Background(), `
		INSERT INTO email_verifications (email, code, purpose, expires_at)
		VALUES ('already@example.com', '123456', 'register', now() + interval '10 minutes')`)
	if err != nil {
		t.Fatalf("写入验证码失败: %v", err)
	}

	_, err = h.service.VerifyEmail(context.Background(), "already@example.com", "123456")
	expectCode(t, err, apperr.CodeAlreadyVerified)
}

func TestResendVerificationIsRateLimited(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.register(t, "resend@example.com", testPassword); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	// 注册已产生 1 条验证码，上限为每小时 5 条。
	for i := 0; i < 4; i++ {
		if err := h.service.ResendVerification(ctx, "resend@example.com"); err != nil {
			t.Fatalf("第 %d 次重发不应被限流: %v", i+1, err)
		}
	}
	expectCode(t, h.service.ResendVerification(ctx, "resend@example.com"), apperr.CodeRateLimited)
}

func TestResendVerificationForUnknownEmailIsSilent(t *testing.T) {
	h := newHarness(t)

	if err := h.service.ResendVerification(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("未知邮箱不应暴露错误: %v", err)
	}
	if len(h.mail.Messages()) != 0 {
		t.Fatal("未知邮箱不应发出邮件")
	}
}

func TestRegisterGeneratesUniqueUsername(t *testing.T) {
	h := newHarness(t)

	first, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "same@example.com", Password: testPassword})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if first.Username != "same" {
		t.Fatalf("首个昵称 = %s，期望 same", first.Username)
	}

	// 换个域名，本地部分相同，昵称需要自动避让。
	second, err := h.service.Register(context.Background(), auth.RegisterInput{Email: "same@other.com", Password: testPassword})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if second.Username == first.Username {
		t.Fatalf("昵称重复: %s", second.Username)
	}
	if !strings.HasPrefix(second.Username, "same") {
		t.Fatalf("昵称 = %s，期望以 same 开头", second.Username)
	}
}
