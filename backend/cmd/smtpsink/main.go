// 命令 smtpsink 是本地开发与端到端测试用的 SMTP 接收器：
// 接收邮件并逐封写入目录，便于人工查看验证码，也便于测试读取。
// 用法：go run ./cmd/smtpsink -addr 127.0.0.1:2525 -dir .maildrop
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/emersion/go-smtp"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2525", "监听地址")
	dir := flag.String("dir", ".maildrop", "邮件落盘目录")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatalf("创建邮件目录失败: %v", err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", *addr, err)
	}

	sink := &sink{dir: *dir}
	server := smtp.NewServer(sink)
	server.Domain = "localhost"
	server.AllowInsecureAuth = true

	fmt.Printf("SMTP 接收器已启动: %s，邮件写入 %s\n", *addr, *dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	if err := server.Serve(listener); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
		log.Fatalf("SMTP 服务退出: %v", err)
	}
}

type sink struct {
	dir     string
	counter atomic.Int64
}

func (s *sink) NewSession(*smtp.Conn) (smtp.Session, error) {
	return &session{sink: s}, nil
}

type session struct {
	sink *sink
	from string
	to   []string
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

	name := fmt.Sprintf("%d-%03d.eml", time.Now().UnixNano(), s.sink.counter.Add(1)%1000)
	path := filepath.Join(s.sink.dir, name)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("写入邮件文件: %w", err)
	}

	fmt.Printf("收到邮件 -> %s (收件人 %s)\n", path, strings.Join(s.to, ", "))
	return nil
}
