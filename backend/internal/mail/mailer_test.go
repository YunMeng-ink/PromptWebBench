package mail_test

import (
	"context"
	"strings"
	"testing"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/mail/mailtest"
)

func configuredSender(t *testing.T, server *mailtest.Server) *mail.SMTPSender {
	t.Helper()
	return mail.NewSMTPSender(mail.Config{
		Host: server.Host(),
		Port: server.Port(),
		From: "noreply@prompt-benchmark.local",
	})
}

func TestSendVerificationCodeReachesServer(t *testing.T) {
	server := mailtest.New(t)
	sender := configuredSender(t, server)

	err := sender.SendVerificationCode(context.Background(), "alice@example.com", "123456", domain.PurposeRegister)
	if err != nil {
		t.Fatalf("发送验证码失败: %v", err)
	}

	messages := server.Messages()
	if len(messages) != 1 {
		t.Fatalf("服务端收到 %d 封邮件，期望 1 封", len(messages))
	}
	msg := messages[0]
	if len(msg.To) != 1 || msg.To[0] != "alice@example.com" {
		t.Fatalf("收件人 = %v", msg.To)
	}
	if msg.From != "noreply@prompt-benchmark.local" {
		t.Fatalf("发件人 = %s", msg.From)
	}
	if !strings.Contains(msg.Data, "123456") {
		t.Fatalf("邮件正文缺少验证码: %s", msg.Data)
	}
	if !strings.Contains(msg.Data, "验证码") {
		t.Fatalf("邮件正文缺少中文说明: %s", msg.Data)
	}
	if code := server.LastCode(t); code != "123456" {
		t.Fatalf("解析出的验证码 = %s，期望 123456", code)
	}
}

func TestSendResetCodeUsesResetTemplate(t *testing.T) {
	server := mailtest.New(t)
	sender := configuredSender(t, server)

	if err := sender.SendVerificationCode(context.Background(), "bob@example.com", "654321", domain.PurposeResetPassword); err != nil {
		t.Fatalf("发送验证码失败: %v", err)
	}

	msg := server.Messages()[0]
	if !strings.Contains(msg.Data, "654321") {
		t.Fatalf("邮件正文缺少验证码: %s", msg.Data)
	}
	if !strings.Contains(msg.Data, "密码重置") {
		t.Fatalf("重置邮件应使用重置模板: %s", msg.Data)
	}
}

func TestUnconfiguredSenderFails(t *testing.T) {
	sender := mail.NewSMTPSender(mail.Config{})
	err := sender.SendVerificationCode(context.Background(), "carol@example.com", "111111", domain.PurposeRegister)
	if err == nil {
		t.Fatal("未配置 SMTP 时应返回错误而不是静默成功")
	}
	if !strings.Contains(err.Error(), "SMTP 未配置") {
		t.Fatalf("错误信息不明确: %v", err)
	}
}

func TestUnreachableServerFails(t *testing.T) {
	// 端口 1 上没有 SMTP 服务，发送必须明确失败，使注册流程能中止并不落库（G-14）。
	sender := mail.NewSMTPSender(mail.Config{Host: "127.0.0.1", Port: "1", From: "noreply@example.com"})
	err := sender.SendVerificationCode(context.Background(), "dave@example.com", "222222", domain.PurposeRegister)
	if err == nil {
		t.Fatal("SMTP 不可达时应返回错误")
	}
}

func TestConfiguredFlag(t *testing.T) {
	if (mail.Config{}).Configured() {
		t.Fatal("空配置不应视为已配置")
	}
	if !(mail.Config{Host: "smtp.example.com", From: "a@example.com"}).Configured() {
		t.Fatal("填写主机与发件人后应视为已配置")
	}
	if (mail.Config{Host: "smtp.example.com"}).Configured() {
		t.Fatal("缺少发件人不应视为已配置")
	}
}
