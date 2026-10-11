package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// apiTokenPrefix 是 API Token 的可读前缀，便于用户区分令牌与其他凭据。
const apiTokenPrefix = "pb_"

// apiTokenRandomBytes 决定令牌的熵：32 字节随机数。
const apiTokenRandomBytes = 32

// GenerateAPIToken 生成 API Token。明文只在创建时返回一次，
// 库中仅存 SHA-256 哈希（技术可行性报告 §3.3）。
func GenerateAPIToken() (plain, hash, prefix string, err error) {
	buf := make([]byte, apiTokenRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("生成 API Token 随机数: %w", err)
	}
	plain = apiTokenPrefix + hex.EncodeToString(buf)
	return plain, HashAPIToken(plain), plain[:len(apiTokenPrefix)+8], nil
}

// HashAPIToken 计算令牌哈希，用于按哈希检索令牌。
func HashAPIToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// GenerateVerificationCode 生成 6 位数字验证码。
func GenerateVerificationCode() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成验证码随机数: %w", err)
	}
	value := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	return fmt.Sprintf("%06d", value%1_000_000), nil
}

// GenerateOAuthState 生成 OAuth 流程的随机 state。
func GenerateOAuthState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成 OAuth state: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
