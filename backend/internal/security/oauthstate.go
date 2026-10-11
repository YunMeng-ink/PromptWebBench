package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// OAuthStateTTL 是 OAuth 授权跳转的有效期。
const OAuthStateTTL = 10 * time.Minute

// ErrInvalidOAuthState 表示 OAuth 回调携带的 state 缺失、过期或被篡改。
var ErrInvalidOAuthState = errors.New("OAuth state 无效")

// OAuthStateSigner 用会话密钥对 OAuth state 做 HMAC 签名，
// 使回调可以校验来源而不必在库中保存一次性随机数。
type OAuthStateSigner struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

func NewOAuthStateSigner(secret string, ttl time.Duration) *OAuthStateSigner {
	return &OAuthStateSigner{key: []byte(secret), ttl: ttl, now: time.Now}
}

// Sign 返回 "<nonce>.<签发秒>.<签名>" 形式的 state。
func (s *OAuthStateSigner) Sign(nonce string) string {
	issued := strconv.FormatInt(s.now().Unix(), 10)
	payload := nonce + "." + issued
	return payload + "." + s.mac(payload)
}

// Verify 校验 state 的签名与签发时间。
func (s *OAuthStateSigner) Verify(state string) error {
	parts := strings.Split(state, ".")
	if len(parts) != 3 {
		return ErrInvalidOAuthState
	}
	nonce, issuedRaw, signature := parts[0], parts[1], parts[2]
	payload := nonce + "." + issuedRaw
	if !hmac.Equal([]byte(signature), []byte(s.mac(payload))) {
		return ErrInvalidOAuthState
	}
	issued, err := strconv.ParseInt(issuedRaw, 10, 64)
	if err != nil {
		return ErrInvalidOAuthState
	}
	if s.now().Sub(time.Unix(issued, 0)) > s.ttl {
		return ErrInvalidOAuthState
	}
	return nil
}

func (s *OAuthStateSigner) mac(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
