package auth_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/mail/mailtest"
	"github.com/promptbench/backend/internal/oauth"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/auth"
)

const (
	testSecret = "0123456789abcdef0123456789abcdef"
	// testPassword 满足服务端的口令长度要求。
	testPassword = "TestPassw0rd!"
)

type harness struct {
	pool    *pgxpool.Pool
	service *auth.Service
	mail    *mailtest.Server
	config  *config.Store
	signer  *security.Signer
}

// options 允许用例替换邮件发送实现或注入 GitHub 客户端。
type options struct {
	mail   mail.Sender
	gitHub *oauth.Client
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, options{})
}

func newHarnessWith(t *testing.T, opts options) *harness {
	t.Helper()

	pool := pgtest.MigratedDatabase(t)
	mailServer := mailtest.New(t)

	sender := opts.mail
	if sender == nil {
		sender = mail.NewSMTPSender(mail.Config{
			Host: mailServer.Host(),
			Port: mailServer.Port(),
			From: "noreply@prompt-benchmark.local",
		})
	}

	cfgStore := config.NewStore(pool, config.DefaultCacheTTL)
	if err := cfgStore.Refresh(context.Background()); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	signer := security.NewSigner(testSecret, security.SessionTTL)
	service := auth.New(auth.Deps{
		Pool:          pool,
		Config:        cfgStore,
		Mail:          sender,
		Signer:        signer,
		Logger:        discardLogger(),
		GitHub:        opts.gitHub,
		StateSigner:   security.NewOAuthStateSigner(testSecret, security.OAuthStateTTL),
		PublicBaseURL: "http://localhost:5173",
	})

	return &harness{pool: pool, service: service, mail: mailServer, config: cfgStore, signer: signer}
}

// register 注册一个账户。
func (h *harness) register(t *testing.T, email, password string) (int64, error) {
	t.Helper()
	user, err := h.service.Register(context.Background(), auth.RegisterInput{Email: email, Password: password})
	if err != nil {
		return 0, err
	}
	return user.ID, nil
}

// registerAndVerify 注册并完成邮箱验证，得到一个正常态账户。
func (h *harness) registerAndVerify(t *testing.T, email, password string) int64 {
	t.Helper()

	id, err := h.register(t, email, password)
	if err != nil {
		t.Fatalf("注册 %s 失败: %v", email, err)
	}
	code := h.mail.LastCode(t)
	if _, err := h.service.VerifyEmail(context.Background(), email, code); err != nil {
		t.Fatalf("验证邮箱 %s 失败: %v", email, err)
	}
	return id
}

// login 登录并返回会话。
func (h *harness) login(t *testing.T, email, password string) auth.Session {
	t.Helper()
	session, err := h.service.Login(context.Background(), email, password, "127.0.0.1")
	if err != nil {
		t.Fatalf("登录 %s 失败: %v", email, err)
	}
	return session
}

func (h *harness) setConfig(t *testing.T, key string, value any) {
	t.Helper()
	if err := h.config.Set(context.Background(), key, value, "", 0); err != nil {
		t.Fatalf("写入配置 %s 失败: %v", key, err)
	}
}

func (h *harness) countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	return n
}

func (h *harness) userStatus(t *testing.T, email string) string {
	t.Helper()
	var status string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT status FROM users WHERE lower(email) = lower($1)`, email).Scan(&status); err != nil {
		t.Fatalf("查询账户状态失败: %v", err)
	}
	return status
}

func (h *harness) userRole(t *testing.T, email string) string {
	t.Helper()
	var role string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT role FROM users WHERE lower(email) = lower($1)`, email).Scan(&role); err != nil {
		t.Fatalf("查询身份组失败: %v", err)
	}
	return role
}

func (h *harness) tokenVersion(t *testing.T, email string) int32 {
	t.Helper()
	var version int32
	if err := h.pool.QueryRow(context.Background(),
		`SELECT token_version FROM users WHERE lower(email) = lower($1)`, email).Scan(&version); err != nil {
		t.Fatalf("查询令牌版本失败: %v", err)
	}
	return version
}

// seedInviteCode 直接写入邀请码，阶段 6 之前没有后台生成入口。
// expiresIn 为 0 表示不设有效期，为负表示已经过期。
func (h *harness) seedInviteCode(t *testing.T, code, kind string, maxUses int, expiresIn time.Duration) {
	t.Helper()
	var expires any
	if expiresIn > 0 {
		expires = time.Now().Add(expiresIn)
	} else if expiresIn < 0 {
		expires = time.Now().Add(expiresIn)
	}
	_, err := h.pool.Exec(context.Background(), `
		INSERT INTO invite_codes (code, kind, max_uses, expires_at)
		VALUES ($1, $2, $3, $4)`, code, kind, maxUses, expires)
	if err != nil {
		t.Fatalf("写入邀请码失败: %v", err)
	}
}

func (h *harness) disableInviteCode(t *testing.T, code string) {
	t.Helper()
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE invite_codes SET disabled = true WHERE code = $1`, code); err != nil {
		t.Fatalf("禁用邀请码失败: %v", err)
	}
}
