package security

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("CorrectHorse1")
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	if hash == "CorrectHorse1" {
		t.Fatal("口令不能以明文存储")
	}
	if !VerifyPassword(hash, "CorrectHorse1") {
		t.Fatal("正确口令应校验通过")
	}
	if VerifyPassword(hash, "correcthorse1") {
		t.Fatal("错误口令不应通过")
	}
	if VerifyPassword("not-a-bcrypt-hash", "CorrectHorse1") {
		t.Fatal("非法哈希不应通过")
	}
}

func TestPasswordHashUsesSalt(t *testing.T) {
	first, err := HashPassword("SamePassword1")
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	second, err := HashPassword("SamePassword1")
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	if first == second {
		t.Fatal("同一口令的两次哈希应因随机盐而不同")
	}
}

func TestSessionSignAndVerify(t *testing.T) {
	signer := NewSigner("0123456789abcdef0123456789abcdef", time.Hour)

	token, err := signer.Sign(42, 3)
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	if strings.Count(token, ".") != 2 {
		t.Fatalf("令牌不是三段结构: %s", token)
	}

	claims, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("校验令牌失败: %v", err)
	}
	if claims.UserID != 42 || claims.TokenVersion != 3 {
		t.Fatalf("令牌内容 = %+v，期望 用户 42 版本 3", claims)
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt) != time.Hour {
		t.Fatalf("有效期 = %s，期望 1 小时", claims.ExpiresAt.Sub(claims.IssuedAt))
	}
}

func TestSessionRejectsTamperedToken(t *testing.T) {
	signer := NewSigner("0123456789abcdef0123456789abcdef", time.Hour)
	token, err := signer.Sign(7, 1)
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}

	tampered := token[:len(token)-2] + "xy"
	if _, err := signer.Verify(tampered); err == nil {
		t.Fatal("签名被改动后应校验失败")
	}
}

func TestSessionRejectsForeignKey(t *testing.T) {
	issuer := NewSigner("0123456789abcdef0123456789abcdef", time.Hour)
	other := NewSigner("ffffffffffffffffffffffffffffffff", time.Hour)

	token, err := issuer.Sign(7, 1)
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	if _, err := other.Verify(token); err == nil {
		t.Fatal("其他密钥签发的令牌应校验失败")
	}
}

func TestSessionRejectsExpiredToken(t *testing.T) {
	signer := NewSigner("0123456789abcdef0123456789abcdef", time.Hour)
	base := time.Now()
	signer.now = func() time.Time { return base }

	token, err := signer.Sign(7, 1)
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}

	signer.now = func() time.Time { return base.Add(2 * time.Hour) }
	if _, err := signer.Verify(token); err == nil {
		t.Fatal("过期令牌应校验失败")
	}
}

func TestSessionRejectsInvalidSubject(t *testing.T) {
	signer := NewSigner("0123456789abcdef0123456789abcdef", time.Hour)
	token, err := signer.Sign(0, 1)
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	if _, err := signer.Verify(token); err == nil {
		t.Fatal("非法用户标识应校验失败")
	}
}

func TestSessionRejectsEmptyToken(t *testing.T) {
	signer := NewSigner("0123456789abcdef0123456789abcdef", time.Hour)
	if _, err := signer.Verify(""); err == nil {
		t.Fatal("空令牌应校验失败")
	}
}
