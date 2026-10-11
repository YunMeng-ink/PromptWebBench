// 命令 server 是 Prompt Benchmark 后端的装配与启动入口。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/http"
	"github.com/promptbench/backend/internal/metrics"
	"github.com/promptbench/backend/internal/platform"
	"github.com/promptbench/backend/internal/platform/migrate"
	"github.com/promptbench/backend/internal/service/auth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	env, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := platform.NewLogger(env.LogLevel)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := platform.NewPool(ctx, env.DSN(), 10)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := migrate.Apply(ctx, pool); err != nil {
		return err
	}

	cfgStore := config.NewStore(pool, config.DefaultCacheTTL)
	if err := cfgStore.Refresh(ctx); err != nil {
		return err
	}

	if err := auth.EnsureInitialAdmin(ctx, pool, env.AdminInitEmail, env.AdminInitPassword, logger); err != nil {
		return err
	}

	srv, err := api.New(api.Deps{
		Env:     env,
		Logger:  logger,
		Pool:    pool,
		Config:  cfgStore,
		Metrics: metrics.New(),
	})
	if err != nil {
		return err
	}

	go runBackgroundTasks(ctx, logger, srv.Limiter())

	sc := echo.StartConfig{Address: ":" + env.Port, GracefulTimeout: 10 * time.Second}
	if err := sc.Start(ctx, srv.Handler()); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("启动 HTTP 服务: %w", err)
	}
	logger.Info("服务已停止")
	return nil
}

// runBackgroundTasks 承载低频定时任务：限流窗口清理，以及后续阶段的
// 回收站到期清理与推荐标签重算。
func runBackgroundTasks(ctx context.Context, logger *slog.Logger, limiter *api.Limiter) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			limiter.Cleanup()
			logger.Debug("已完成限流窗口清理")
		}
	}
}
