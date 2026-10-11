package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SessionTTL 是会话有效期（技术可行性报告 §3.3：JWT HS256，24 小时）。
const SessionTTL = 24 * time.Hour

// ErrInvalidSession 表示会话令牌缺失、格式错误、签名不符或已过期。
var ErrInvalidSession = errors.New("会话令牌无效")

// SessionClaims 是会话令牌携带的信息。角色与账户状态不写入令牌，
// 每次请求都从数据库读取，使封禁与身份变更立即生效。
type SessionClaims struct {
	UserID       int64
	TokenVersion int32
	IssuedAt     time.Time
	ExpiresAt    time.Time
}

type jwtClaims struct {
	TokenVersion int32 `json:"tv"`
	jwt.RegisteredClaims
}

// Signer 签发与校验会话令牌。
type Signer struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

func NewSigner(secret string, ttl time.Duration) *Signer {
	return &Signer{key: []byte(secret), ttl: ttl, now: time.Now}
}

func (s *Signer) Sign(userID int64, tokenVersion int32) (string, error) {
	now := s.now()
	claims := jwtClaims{
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
	if err != nil {
		return "", fmt.Errorf("签发会话令牌: %w", err)
	}
	return signed, nil
}

// Verify 校验签名与有效期，并返回令牌内容。
func (s *Signer) Verify(token string) (SessionClaims, error) {
	var claims jwtClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return s.key, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil || !parsed.Valid {
		return SessionClaims{}, ErrInvalidSession
	}

	userID, err := claims.GetSubject()
	if err != nil {
		return SessionClaims{}, ErrInvalidSession
	}
	id, err := parseUserID(userID)
	if err != nil {
		return SessionClaims{}, ErrInvalidSession
	}

	out := SessionClaims{UserID: id, TokenVersion: claims.TokenVersion}
	if claims.IssuedAt != nil {
		out.IssuedAt = claims.IssuedAt.Time
	}
	if claims.ExpiresAt != nil {
		out.ExpiresAt = claims.ExpiresAt.Time
	}
	return out, nil
}

func parseUserID(subject string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(subject, "%d", &id); err != nil || id <= 0 {
		return 0, ErrInvalidSession
	}
	return id, nil
}
