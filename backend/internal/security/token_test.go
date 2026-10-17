package security

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateAPITokenShape(t *testing.T) {
	plain, hash, prefix, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("生成令牌失败: %v", err)
	}
	if !strings.HasPrefix(plain, apiTokenPrefix) {
		t.Fatalf("令牌缺少前缀: %s", plain)
	}
	if len(plain) != len(apiTokenPrefix)+apiTokenRandomBytes*2 {
		t.Fatalf("令牌长度 = %d，期望 %d", len(plain), len(apiTokenPrefix)+apiTokenRandomBytes*2)
	}
	if prefix != plain[:len(apiTokenPrefix)+8] {
		t.Fatalf("前缀 = %s，与令牌不匹配", prefix)
	}
	if hash != HashAPIToken(plain) {
		t.Fatal("返回的哈希与明文不一致")
	}
	if strings.Contains(hash, plain) {
		t.Fatal("哈希不应包含明文")
	}
}

func TestGenerateAPITokenIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		plain, _, _, err := GenerateAPIToken()
		if err != nil {
			t.Fatalf("生成令牌失败: %v", err)
		}
		if seen[plain] {
			t.Fatal("令牌重复")
		}
		seen[plain] = true
	}
}

func TestGenerateVerificationCodeIsSixDigits(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := GenerateVerificationCode()
		if err != nil {
			t.Fatalf("生成验证码失败: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("验证码长度 = %d，期望 6（值 %s）", len(code), code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("验证码含非数字字符: %s", code)
			}
		}
		seen[code] = true
	}
	if len(seen) < 100 {
		t.Fatalf("200 次生成的验证码只有 %d 个不同取值，随机性不足", len(seen))
	}
}

func TestOAuthStateSignAndVerify(t *testing.T) {
	signer := NewOAuthStateSigner("secret-key", 10*time.Minute)
	nonce, err := GenerateOAuthState()
	if err != nil {
		t.Fatalf("生成 nonce 失败: %v", err)
	}

	state := signer.Sign(nonce)
	if err := signer.Verify(state); err != nil {
		t.Fatalf("校验 state 失败: %v", err)
	}
	if err := signer.Verify(state + "x"); err == nil {
		t.Fatal("被篡改的 state 应校验失败")
	}
	if err := signer.Verify("only.two"); err == nil {
		t.Fatal("结构不完整的 state 应校验失败")
	}
	if err := signer.Verify(""); err == nil {
		t.Fatal("空 state 应校验失败")
	}
}

func TestOAuthStateRejectsExpiredState(t *testing.T) {
	signer := NewOAuthStateSigner("secret-key", 10*time.Minute)
	base := time.Now()
	signer.now = func() time.Time { return base }

	state := signer.Sign("nonce-value")
	signer.now = func() time.Time { return base.Add(11 * time.Minute) }
	if err := signer.Verify(state); err == nil {
		t.Fatal("过期的 state 应校验失败")
	}
}

func TestOAuthStateRejectsForeignKey(t *testing.T) {
	issuer := NewOAuthStateSigner("key-a", 10*time.Minute)
	other := NewOAuthStateSigner("key-b", 10*time.Minute)

	if err := other.Verify(issuer.Sign("nonce")); err == nil {
		t.Fatal("其他密钥签发的 state 应校验失败")
	}
}

func TestOAuthStateNonceOfExtractsNonce(t *testing.T) {
	signer := NewOAuthStateSigner("secret-key", 10*time.Minute)

	got, err := signer.NonceOf(signer.Sign("nonce-abc"))
	if err != nil || got != "nonce-abc" {
		t.Fatalf("NonceOf = %q, %v，期望取出 nonce-abc", got, err)
	}
	if _, err := signer.NonceOf("tampered.123.abc"); err == nil {
		t.Fatal("篡改签名的 state 应被拒绝")
	}
	if _, err := signer.NonceOf(""); err == nil {
		t.Fatal("空 state 应被拒绝")
	}
}
