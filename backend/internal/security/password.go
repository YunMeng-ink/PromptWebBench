// Package security 提供口令哈希、会话令牌、API Token 与验证码的生成与校验。
package security

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost 固定为 10（技术可行性报告 §3.3）。
const bcryptCost = 10

func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("生成口令哈希: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword 比对明文与哈希。哈希格式非法与口令不符都返回 false。
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
