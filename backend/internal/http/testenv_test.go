package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/config"
	api "github.com/promptbench/backend/internal/http"
	"github.com/promptbench/backend/internal/mail"
	"github.com/promptbench/backend/internal/mail/mailtest"
	"github.com/promptbench/backend/internal/metrics"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/admin"
	"github.com/promptbench/backend/internal/service/auth"
	"github.com/promptbench/backend/internal/service/prompt"
	"github.com/promptbench/backend/internal/service/rating"
)

// testEnv 是一套完整可用的接口层测试环境：真实数据库、真实 SMTP 服务、真实 HTTP 服务。
type testEnv struct {
	Env    config.Env
	pool   *pgxpool.Pool
	config *config.Store
	mail   *mailtest.Server
	server *api.Server
	ts     *httptest.Server

	auth   *auth.Service
	admin  *admin.Service
	prompt *prompt.Service
	rating *rating.Service
	repo   *repo.Queries
	signer *security.Signer
}

// Deps 返回构造接口层所需的依赖，供需要自定义配置的用例复用。
func (e *testEnv) Deps(t *testing.T) api.Deps {
	t.Helper()
	return api.Deps{
		Env:     e.Env,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Pool:    e.pool,
		Config:  e.config,
		Metrics: metrics.New(),
		Repo:    e.repo,
		Auth:    e.auth,
		Admin:   e.admin,
		Prompt:  e.prompt,
		Rating:  e.rating,
		Signer:  e.signer,
	}
}

const testJWTSecret = "0123456789abcdef0123456789abcdef"

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := pgtest.MigratedDatabase(t)
	mailServer := mailtest.New(t)

	cfgStore := config.NewStore(pool, config.DefaultCacheTTL)
	if err := cfgStore.Refresh(t.Context()); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	signer := security.NewSigner(testJWTSecret, security.SessionTTL)
	authService := auth.New(auth.Deps{
		Pool:   pool,
		Config: cfgStore,
		Mail: mail.NewSMTPSender(mail.Config{
			Host: mailServer.Host(),
			Port: mailServer.Port(),
			From: "noreply@prompt-benchmark.local",
		}),
		Signer:        signer,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		StateSigner:   security.NewOAuthStateSigner(testJWTSecret, security.OAuthStateTTL),
		PublicBaseURL: "http://127.0.0.1",
	})
	adminService := admin.New(admin.Deps{Pool: pool, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	promptService := prompt.New(prompt.Deps{Pool: pool, Config: cfgStore, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ratingService := rating.New(rating.Deps{Pool: pool, Config: cfgStore, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	env := testEnv{
		Env:    testEnvConfig(),
		pool:   pool,
		config: cfgStore,
		mail:   mailServer,
		auth:   authService,
		admin:  adminService,
		prompt: promptService,
		rating: ratingService,
		repo:   repo.New(pool),
		signer: signer,
	}

	srv, err := api.New(api.Deps{
		Env:     env.Env,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Pool:    pool,
		Config:  cfgStore,
		Metrics: metrics.New(),
		Repo:    env.repo,
		Auth:    authService,
		Admin:   adminService,
		Prompt:  promptService,
		Rating:  ratingService,
		Signer:  signer,
	})
	if err != nil {
		t.Fatalf("构造接口层失败: %v", err)
	}
	env.server = srv
	env.ts = httptest.NewServer(srv.Handler())
	t.Cleanup(env.ts.Close)
	return &env
}

// apiClient 是带 Cookie 容器的测试客户端：写请求自动附带 CSRF 双提交头，
// 与前端封装的行为一致。
type apiClient struct {
	t      *testing.T
	base   string
	client *http.Client
}

func (e *testEnv) newClient(t *testing.T) *apiClient {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("创建 Cookie 容器失败: %v", err)
	}
	return &apiClient{t: t, base: e.ts.URL, client: &http.Client{Jar: jar, Timeout: 15 * time.Second}}
}

func (c *apiClient) csrfToken() string {
	parsed, err := url.Parse(c.base)
	if err != nil {
		c.t.Fatalf("解析基地址失败: %v", err)
	}
	for _, cookie := range c.client.Jar.Cookies(parsed) {
		if cookie.Name == api.CSRFCookieName {
			return cookie.Value
		}
	}
	return ""
}

func (c *apiClient) do(method, path string, body any, withCSRF bool) *http.Response {
	c.t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("序列化请求体失败: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatalf("构造请求失败: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if withCSRF {
		if token := c.csrfToken(); token != "" {
			req.Header.Set(api.CSRFHeaderName, token)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.t.Fatalf("请求 %s %s 失败: %v", method, path, err)
	}
	return resp
}

func (c *apiClient) get(path string) *http.Response {
	c.t.Helper()
	return c.do(http.MethodGet, path, nil, false)
}

func (c *apiClient) post(path string, body any) *http.Response {
	c.t.Helper()
	return c.do(http.MethodPost, path, body, true)
}

func (c *apiClient) patch(path string, body any) *http.Response {
	c.t.Helper()
	return c.do(http.MethodPatch, path, body, true)
}

func (c *apiClient) put(path string, body any) *http.Response {
	c.t.Helper()
	return c.do(http.MethodPut, path, body, true)
}

func (c *apiClient) delete(path string) *http.Response {
	c.t.Helper()
	return c.do(http.MethodDelete, path, nil, true)
}

// prime 先发一次读请求，取得 CSRF Cookie，模拟真实浏览器的首次导航。
func (c *apiClient) prime() {
	c.t.Helper()
	resp := c.get(api.APIPrefix + "/health")
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func decodeBody[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("解析响应体失败: %v，原文 %s", err, raw)
	}
	return out
}

func expectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode == want {
		return
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	t.Fatalf("状态码 = %d，期望 %d，响应体 %s", resp.StatusCode, want, raw)
}

func expectErrorCode(t *testing.T, resp *http.Response, want int, code string) {
	t.Helper()
	expectStatus(t, resp, want)
	envelope := decodeBody[api.ErrorEnvelope](t, resp)
	if envelope.Error.Code != code {
		t.Fatalf("错误码 = %s，期望 %s", envelope.Error.Code, code)
	}
}

func (e *testEnv) setConfig(t *testing.T, key string, value any) {
	t.Helper()
	if err := e.config.Set(t.Context(), key, value, "", 0); err != nil {
		t.Fatalf("写入配置 %s 失败: %v", key, err)
	}
}
