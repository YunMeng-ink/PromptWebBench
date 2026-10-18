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

	"github.com/promptbench/backend/internal/buildinfo"
	"github.com/promptbench/backend/internal/config"
	api "github.com/promptbench/backend/internal/http"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/metrics"
	"github.com/promptbench/backend/internal/modelsdev"
	"github.com/promptbench/backend/internal/oauth"
	"github.com/promptbench/backend/internal/platform"
	"github.com/promptbench/backend/internal/platform/migrate"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/admin"
	"github.com/promptbench/backend/internal/service/auth"
	"github.com/promptbench/backend/internal/service/library"
	"github.com/promptbench/backend/internal/service/prompt"
	"github.com/promptbench/backend/internal/service/rating"
	"github.com/promptbench/backend/internal/service/social"
	"github.com/promptbench/backend/internal/service/stats"
)

func main() {
	envFile := flag.String("env-file", "", "可选：从文件加载环境变量（KEY=VALUE，仅补齐未设置的变量）")
	showVersion := flag.Bool("version", false, "打印版本号后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}

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
	defer cfgStore.Close()
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
	statsService := stats.New(stats.Deps{Pool: pool, Config: cfgStore, Logger: logger})
	// 提示词域的写入会改变统计快照与评分集合，因此注入统计缓存失效入口；
	// 删除、恢复与彻底删除还须留痕，因此同时注入操作日志写入器。
	promptService := prompt.New(prompt.Deps{
		Pool: pool, Config: cfgStore, Logger: logger, Stats: statsService,
	})
	socialService := social.New(social.Deps{Pool: pool, Config: cfgStore, Logger: logger})
	// 社交动作会改变提示词的派生数据（评论数、收藏数），注入统计缓存失效入口。
	socialService.SetStatsHook(statsService)
	// 协作者删题须通知题库所有者（需求 §10.6、C-34），通知由社交域发出。
	libraryService := library.New(library.Deps{
		Pool: pool, Config: cfgStore, Logger: logger, Notifier: socialService,
	})
	ratingService := rating.New(rating.Deps{
		Pool: pool, Config: cfgStore, Logger: logger, Stats: statsService,
	})
	// 管理后台复用提示词域与社交域的写入路径：强制标记重大修改、删除与恢复提示词、
	// 删除被举报评论都在各自的域内实现一次，后台只做编排与留痕。
	adminService := admin.New(admin.Deps{
		Pool:       pool,
		Config:     cfgStore,
		Logger:     logger,
		Secrets:    security.NewSecretBox(env.ConfigSecret),
		Mail:       mailConfig,
		ModelsDev:  modelsdev.NewClient(env.ModelsDevURL),
		Prompts:    promptService,
		Moderation: socialService,
		Stats:      statsService,
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
		Social:  socialService,
		Library: libraryService,
		Signer:  signer,
	})
	if err != nil {
		return err
	}

	go runBackgroundTasks(ctx, logger, srv.Limiter(), libraryService, adminService)

	sc := echo.StartConfig{Address: ":" + env.Port, GracefulTimeout: 10 * time.Second, HideBanner: true, HidePort: true}
	if err := sc.Start(ctx, srv.Handler()); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("启动 HTTP 服务: %w", err)
	}
	logger.Info("服务已停止")
	return nil
}

// runBackgroundTasks 承载低频定时任务：限流窗口清理与回收站到期清理。
//
// 回收站清理按对象类型分工：题库与题库题目由题库域处理，提示词由管理后台的服务处理
// （提示词的彻底删除还要做题库引用检查）。两者都是幂等的，重复执行不会重复删除。
func runBackgroundTasks(
	ctx context.Context,
	logger *slog.Logger,
	limiter *api.Limiter,
	libraryService *library.Service,
	adminService *admin.Service,
) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			limiter.Cleanup()
			purgeExpiredRecycleBin(ctx, logger, libraryService, adminService)
		}
	}
}

// purgeExpiredRecycleBin 清理到期的回收站记录（需求 §11.3：到期自动彻底删除）。
func purgeExpiredRecycleBin(
	ctx context.Context,
	logger *slog.Logger,
	libraryService *library.Service,
	adminService *admin.Service,
) {
	const batch = 100
	libraryPurged, err := libraryService.PurgeExpired(ctx, batch)
	if err != nil {
		logger.Error("清理到期题库回收站记录失败", slog.String("error", err.Error()))
	}
	promptPurged, err := adminService.PurgeExpiredPrompts(ctx, batch)
	if err != nil {
		logger.Error("清理到期提示词回收站记录失败", slog.String("error", err.Error()))
	}
	if libraryPurged > 0 || promptPurged > 0 {
		logger.Info("已清理到期回收站记录",
			slog.Int("library_entries", libraryPurged), slog.Int("prompt_entries", promptPurged))
	}
}
