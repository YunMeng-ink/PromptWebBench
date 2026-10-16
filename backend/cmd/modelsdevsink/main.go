// 命令 modelsdevsink 是本地开发与端到端测试用的 models.dev 替身：
// 按 models.dev 的真实响应结构返回一份固定的模型目录，使导入路径可以被稳定验证，
// 也让端到端测试不必访问外部网络。
// 用法：go run ./cmd/modelsdevsink -addr 127.0.0.1:2526
package main

import (
	"context"
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

// catalog 是 models.dev 响应的替身内容。三条条目覆盖三种形态：
// 字段齐全、缺少 modalities、缺少 limit（需求 §13.3）。
const catalog = `{
  "e2e-lab": {
    "id": "e2e-lab",
    "name": "端到端测试实验室",
    "models": {
      "e2e-import-one": {
        "id": "e2e-import-one",
        "name": "E2E Import One",
        "description": "字段齐全的导入条目",
        "release_date": "2025-02-01",
        "limit": {"context": 64000},
        "modalities": {"input": ["text", "image"]}
      },
      "e2e-import-two": {
        "id": "e2e-import-two",
        "name": "E2E Import Two",
        "limit": {"context": 8000}
      },
      "e2e-import-three": {
        "id": "e2e-import-three",
        "name": "E2E Import Three",
        "release_date": "2024-11-11",
        "modalities": {"input": ["text"]}
      }
    }
  }
}`

func main() {
	addr := flag.String("addr", "127.0.0.1:2526", "监听地址")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", *addr, err)
	}

	var hits atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/api.json", func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(catalog))
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Printf("models.dev 替身已启动: %s/api.json\n", *addr)

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
