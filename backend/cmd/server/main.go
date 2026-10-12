// 命令 server 是 Prompt Benchmark 后端的装配与启动入口。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"

	"github.com/promptbench/backend/internal/config"
	api "github.com/promptbench/backend/internal/http"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/metrics"
	"github.com/promptbench/backend/internal/oauth"
	"github.com/promptbench/backend/internal/platform"
	"github.com/promptbench/backend/internal/platform/migrate"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/admin"
	"github.com/promptbench/backend/internal/service/auth"
	"github.com/promptbench/backend/internal/service/prompt"
	"github.com/promptbench/backend/internal/service/rating"
	"github.com/promptbench/backend/internal/service/stats"
)

func main() {
	envFile := flag.String("env-file", "", "可选：从文件加载环境变量（KEY=VALUE，仅补齐未设置的变量）")
	flag.Parse()

	if err := run(*envFile); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run(envFile string) error {
	if envFile != "" {
		// 已存在的环境变量优先，便于在 shell 中临时覆盖文件取值。
		if err := godotenv.Load(envFile); err != nil {
			return fmt.Errorf("加载环境变量文件 %s: %w", envFile, err)
		}
	}

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

	mailConfig := mail.Config{
		Host:     env.SMTPHost,
		Port:     env.SMTPPort,
		User:     env.SMTPUser,
		Password: env.SMTPPassword,
		From:     env.SMTPFrom,
	}
	if !mailConfig.Configured() {
		logger.Warn("未配置 SMTP，注册与密码找回将无法发送验证码")
	}
	mailer := mail.NewSMTPSender(mailConfig)

	signer := security.NewSigner(env.JWTSecret, security.SessionTTL)
	gitHub := oauth.NewGitHubClient(env.GitHubClientID, env.GitHubClientSecret)
	if !gitHub.Configured() {
		logger.Warn("未配置 GitHub OAuth，第三方登录不可用")
	}

	authService := auth.New(auth.Deps{
		Pool:          pool,
		Config:        cfgStore,
		Mail:          mailer,
		Signer:        signer,
		Logger:        logger,
		GitHub:        gitHub,
		StateSigner:   security.NewOAuthStateSigner(env.JWTSecret, security.OAuthStateTTL),
		PublicBaseURL: env.PublicBaseURL,
	})
	adminService := admin.New(admin.Deps{Pool: pool, Logger: logger})
	promptService := prompt.New(prompt.Deps{Pool: pool, Config: cfgStore, Logger: logger})
	statsService := stats.New(stats.Deps{Pool: pool, Config: cfgStore, Logger: logger})
	ratingService := rating.New(rating.Deps{
		Pool: pool, Config: cfgStore, Logger: logger, Stats: statsService,
	})

	srv, err := api.New(api.Deps{
		Env:     env,
		Logger:  logger,
		Pool:    pool,
		Config:  cfgStore,
		Metrics: metrics.New(),
		Repo:    repo.New(pool),
		Auth:    authService,
		Admin:   adminService,
		Prompt:  promptService,
		Rating:  ratingService,
		Stats:   statsService,
		Signer:  signer,
	})
	if err != nil {
		return err
	}

	go runBackgroundTasks(ctx, logger, srv.Limiter())

	sc := echo.StartConfig{Address: ":" + env.Port, GracefulTimeout: 10 * time.Second, HideBanner: true, HidePort: true}
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
