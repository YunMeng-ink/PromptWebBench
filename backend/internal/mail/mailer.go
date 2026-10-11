// Package mail 通过 SMTP 发送验证码邮件（技术可行性报告 §3.3、G-14）。
package mail

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"time"

	"github.com/promptbench/backend/internal/domain"
)

// Config 是 SMTP 连接配置。
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

// Configured 判定 SMTP 是否可用。未配置时发送一律失败，注册与找回口会明确报错而不落库。
func (c Config) Configured() bool {
	return c.Host != "" && c.From != ""
}

// Sender 发送验证码邮件。
type Sender interface {
	SendVerificationCode(ctx context.Context, to, code string, purpose domain.VerificationPurpose) error
}

// SMTPSender 是标准库 SMTP 实现。
type SMTPSender struct {
	cfg Config
}

func NewSMTPSender(cfg Config) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

func (s *SMTPSender) SendVerificationCode(ctx context.Context, to, code string, purpose domain.VerificationPurpose) error {
	if !s.cfg.Configured() {
		return fmt.Errorf("SMTP 未配置，无法发送验证码邮件")
	}

	subject, body := composeVerificationCode(code, purpose)
	addr := net.JoinHostPort(s.cfg.Host, s.cfg.Port)

	var auth smtp.Auth
	if s.cfg.User != "" {
		auth = smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)
	}

	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, s.cfg.From, []string{to}, buildMessage(s.cfg.From, to, subject, body))
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("发送验证码邮件到 %s: %w", to, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("发送验证码邮件到 %s: %w", to, ctx.Err())
	}
}

func composeVerificationCode(code string, purpose domain.VerificationPurpose) (subject, body string) {
	switch purpose {
	case domain.PurposeRegister:
		return "Prompt Benchmark 邮箱验证码",
			fmt.Sprintf("你的邮箱验证码是 %s。\r\n\r\nYour email verification code is %s.\r\n\r\n验证码有效期 15 分钟，请勿转告他人。", code, code)
	default:
		return "Prompt Benchmark 密码重置验证码",
			fmt.Sprintf("你的密码重置验证码是 %s。\r\n\r\nYour password reset code is %s.\r\n\r\n验证码有效期 15 分钟，请勿转告他人。", code, code)
	}
}

func buildMessage(from, to, subject, body string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.BEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(&buf, "Content-Transfer-Encoding: 8bit\r\n")
	fmt.Fprintf(&buf, "\r\n%s\r\n", body)
	return buf.Bytes()
}
