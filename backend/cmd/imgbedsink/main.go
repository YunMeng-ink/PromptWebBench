// 命令 imgbedsink 是本地开发与端到端测试用的图床替身：
// 接收 multipart 上传并返回与 img.scdn.io 相同形态的成功响应，
// 使评分附图上传路径可以被稳定验证，也让端到端测试不必把测试图片送到外部图床。
// 用法：go run ./cmd/imgbedsink -addr 127.0.0.1:2527
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2527", "监听地址")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", *addr, err)
	}

	var hits atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1.php", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"success":false,"error":"缺少图片","message":"缺少图片"}`))
			return
		}
		if _, _, err := r.FormFile("image"); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"success":false,"error":"缺少图片","message":"缺少图片"}`))
			return
		}
		seq := hits.Add(1)
		name := fmt.Sprintf("e2e-%d.webp", seq)
		payload := map[string]any{
			"success": true,
			"url":     fmt.Sprintf("http://%s/i/%s", *addr, name),
			"data": map[string]any{
				"url":      fmt.Sprintf("http://%s/i/%s", *addr, name),
				"filename": name,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Printf("图床替身已启动: %s/api/v1.php\n", *addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP 服务退出: %v", err)
	}
}
