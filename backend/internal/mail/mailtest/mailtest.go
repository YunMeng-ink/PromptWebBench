// Package mailtest 提供一个真实的 SMTP 服务端实现，供集成测试读取发出的邮件。
// 测试走完整 SMTP 会话，不替换邮件发送实现。
package mailtest

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"github.com/emersion/go-smtp"
)

// Message 是一封被捕获的邮件。
type Message struct {
	From string
	To   []string
	Data string
}

// Server 是本地 SMTP 服务端。
type Server struct {
	host string
	port string

	mu       sync.Mutex
	messages []Message
}

// New 启动一个监听回环地址的 SMTP 服务端，测试结束自动关闭。
func New(t *testing.T) *Server {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听 SMTP 端口失败: %v", err)
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("监听地址类型异常: %T", listener.Addr())
	}

	s := &Server{host: "127.0.0.1", port: strconv.Itoa(addr.Port)}
	smtpServer := smtp.NewServer(&backend{server: s})
	smtpServer.Domain = "localhost"
	smtpServer.AllowInsecureAuth = true

	go func() { _ = smtpServer.Serve(listener) }()

	t.Cleanup(func() {
		_ = smtpServer.Shutdown(context.Background())
		_ = listener.Close()
	})
	return s
}

func (s *Server) Host() string { return s.host }
func (s *Server) Port() string { return s.port }

// Messages 返回已捕获的邮件副本。
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// Reset 清空已捕获的邮件。
func (s *Server) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}

var codePattern = regexp.MustCompile(`\b(\d{6})\b`)

// LastCode 返回最近一封邮件中的 6 位验证码。
func (s *Server) LastCode(t *testing.T) string {
	t.Helper()

	messages := s.Messages()
	if len(messages) == 0 {
		t.Fatal("尚未收到任何邮件")
	}
	match := codePattern.FindStringSubmatch(messages[len(messages)-1].Data)
	if match == nil {
		t.Fatalf("邮件正文中未找到验证码: %s", messages[len(messages)-1].Data)
	}
	return match[1]
}

func (s *Server) record(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

type backend struct {
	server *Server
}

func (b *backend) NewSession(*smtp.Conn) (smtp.Session, error) {
	return &session{server: b.server}, nil
}

type session struct {
	server *Server
	from   string
	to     []string
}

func (s *session) Reset() {
	s.from = ""
	s.to = nil
}

func (s *session) Logout() error { return nil }

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("读取邮件内容: %w", err)
	}
	s.server.record(Message{From: s.from, To: append([]string(nil), s.to...), Data: string(raw)})
	return nil
}
