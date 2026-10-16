package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrSecretUnavailable 表示未配置主密钥，密钥类配置项既不可保存也不可读取。
// 由服务层翻译为错误码 secret_unavailable。
var ErrSecretUnavailable = errors.New("未配置主密钥")

// SecretBox 以 AES-256-GCM 加解密 config 表的密钥类配置项（技术可行性报告 §3.3）。
//
// 主密钥来自环境变量 CONFIG_SECRET：取值先经 SHA-256 压缩为 32 字节，
// 因此运维可以用任意长度的口令，而不必要求 32 字节的十六进制串。
type SecretBox struct {
	key [32]byte

	// enabled 为假表示未配置主密钥。此时全部密钥类操作一律失败，
	// 而不是退化为明文存储。
	enabled bool
}

// NewSecretBox 构造加解密器。主密钥为空时返回的实例处于不可用状态。
func NewSecretBox(master string) *SecretBox {
	box := &SecretBox{}
	if master == "" {
		return box
	}
	box.key = sha256.Sum256([]byte(master))
	box.enabled = true
	return box
}

// Enabled 返回主密钥是否已配置。
func (b *SecretBox) Enabled() bool { return b.enabled }

// Seal 加密明文，输出 base64(nonce || 密文)。
func (b *SecretBox) Seal(plaintext string) (string, error) {
	gcm, err := b.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("生成加密随机数: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// Open 解密 Seal 的输出。
func (b *SecretBox) Open(ciphertext string) (string, error) {
	gcm, err := b.aead()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("解码密文: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("密文长度不足，无法包含随机数")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("解密配置项: %w", err)
	}
	return string(plain), nil
}

func (b *SecretBox) aead() (cipher.AEAD, error) {
	if !b.enabled {
		return nil, ErrSecretUnavailable
	}
	block, err := aes.NewCipher(b.key[:])
	if err != nil {
		return nil, fmt.Errorf("构造分组密码: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("构造 GCM: %w", err)
	}
	return gcm, nil
}
