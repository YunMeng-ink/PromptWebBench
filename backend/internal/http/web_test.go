package api_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api "github.com/promptbench/backend/internal/http"
)

// newWebServer 以 STATIC_DIR 模式构造接口层，静态资源来自测试临时目录。
func newWebServer(t *testing.T, dir string) *api.Server {
	t.Helper()
	e := newTestEnv(t)
	deps := e.Deps(t)
	deps.Env.StaticDir = dir
	srv, err := api.New(deps)
	if err != nil {
		t.Fatalf("构造接口层失败: %v", err)
	}
	return srv
}

func writeWebFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}

func webGet(t *testing.T, base, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(base + path)
	if err != nil {
		t.Fatalf("GET %s 失败: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func webBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	return string(raw)
}

// TestWebStaticDirServing 验证 STATIC_DIR 模式：首页与 SPA 回落、资源缓存策略、
// 未命中资源 404，接口路由不受静态兜底影响。
func TestWebStaticDirServing(t *testing.T) {
	dir := t.TempDir()
	writeWebFile(t, filepath.Join(dir, "index.html"), "<!doctype html><title>Prompt Benchmark</title>")
	writeWebFile(t, filepath.Join(dir, "assets", "app.js"), "console.log('app')")
	writeWebFile(t, filepath.Join(dir, "robots.txt"), "User-agent: *")

	srv := newWebServer(t, dir)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// 首页、目录入口与未命中文件的 SPA 路由都返回 index.html。
	for _, p := range []string{"/", "/index.html", "/prompts/123/edit"} {
		resp := webGet(t, ts.URL, p)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s 状态码 = %d，期望 200", p, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("GET %s Content-Type = %q，期望 text/html", p, ct)
		}
		if got := webBody(t, resp); got != "<!doctype html><title>Prompt Benchmark</title>" {
			t.Fatalf("GET %s 响应体 = %q", p, got)
		}
	}

	// 页面入口不缓存，带哈希的资源长缓存。
	if cc := webGet(t, ts.URL, "/").Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("GET / Cache-Control = %q，期望 no-cache", cc)
	}
	resp := webGet(t, ts.URL, "/assets/app.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /assets/app.js 状态码 = %d，期望 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("GET /assets/app.js Cache-Control = %q", cc)
	}
	if got := webBody(t, resp); got != "console.log('app')" {
		t.Fatalf("GET /assets/app.js 响应体 = %q", got)
	}

	// 带扩展名且未命中的资源直接 404，不回落页面。
	expectErrorCode(t, webGet(t, ts.URL, "/missing.png"), http.StatusNotFound, "not_found")

	// 路径穿越被净化，无法越出静态根目录。
	expectErrorCode(t, webGet(t, ts.URL, "/../server.go"), http.StatusNotFound, "not_found")

	// 接口路由不受静态兜底影响。
	expectStatus(t, webGet(t, ts.URL, api.APIPrefix+"/health"), http.StatusOK)
	expectErrorCode(t, webGet(t, ts.URL, api.APIPrefix+"/nonexistent"), http.StatusNotFound, "not_found")
}

// TestWebStaticCacheHeaders 验证静态资源与 SPA 回落的缓存头与协商缓存：
// 页面入口按请求路径拿到 no-cache 与 ETag，条件请求返回 304；
// 非哈希的根目录文件使用短缓存；静态响应不携带 CSRF Cookie 与 Vary: Cookie。
func TestWebStaticCacheHeaders(t *testing.T) {
	dir := t.TempDir()
	writeWebFile(t, filepath.Join(dir, "index.html"), "<!doctype html><title>Prompt Benchmark</title>")
	writeWebFile(t, filepath.Join(dir, "assets", "app.js"), "console.log('app')")
	writeWebFile(t, filepath.Join(dir, "robots.txt"), "User-agent: *")

	srv := newWebServer(t, dir)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// SPA 回落拿到的是 index.html，缓存策略必须按实际返回的文件判定。
	spa := webGet(t, ts.URL, "/prompts/123/edit")
	if cc := spa.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("SPA 回落 Cache-Control = %q，期望 no-cache", cc)
	}
	etag := spa.Header.Get("ETag")
	if etag == "" {
		t.Fatal("SPA 回落缺少 ETag")
	}
	if cookies := spa.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("静态响应不应签发 Cookie，实际 %v", cookies)
	}
	if vary := spa.Header.Get("Vary"); strings.Contains(vary, "Cookie") {
		t.Fatalf("静态响应不应带 Vary: Cookie，实际 %q", vary)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/prompts/123/edit", nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("If-None-Match", etag)
	conditional, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("条件请求失败: %v", err)
	}
	t.Cleanup(func() { conditional.Body.Close() })
	if conditional.StatusCode != http.StatusNotModified {
		t.Fatalf("条件请求状态码 = %d，期望 304", conditional.StatusCode)
	}

	asset := webGet(t, ts.URL, "/assets/app.js")
	if etag := asset.Header.Get("ETag"); etag == "" {
		t.Fatal("带哈希资源缺少 ETag")
	}
	if cc := webGet(t, ts.URL, "/robots.txt").Header.Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Fatalf("robots.txt Cache-Control = %q，期望 public, max-age=3600", cc)
	}
}

// TestAPICacheHeaders 验证接口缓存策略：只读字典与公开聚合可共享缓存，
// 与身份相关的接口与全部写方法一律 no-store，且不签发 CSRF Cookie。
func TestAPICacheHeaders(t *testing.T) {
	e := newTestEnv(t)
	c := e.newClient(t)

	if cc := c.get(api.APIPrefix + "/capabilities").Header.Get("Cache-Control"); cc != "public, max-age=300, stale-while-revalidate=3600" {
		t.Fatalf("/capabilities Cache-Control = %q", cc)
	}
	if cc := c.get(api.APIPrefix + "/leaderboard").Header.Get("Cache-Control"); cc != "public, max-age=60, stale-while-revalidate=300" {
		t.Fatalf("/leaderboard Cache-Control = %q", cc)
	}
	if cc := c.get(api.APIPrefix + "/prompts").Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("/prompts Cache-Control = %q，期望 no-store", cc)
	}
	if cc := c.get(api.APIPrefix + "/purposes").Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("/purposes Cache-Control = %q，期望 no-store", cc)
	}
	if cc := c.get(api.APIPrefix + "/auth/session").Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("/auth/session Cache-Control = %q，期望 no-store", cc)
	}

	// 登录态探测签发 CSRF Cookie，写请求才能通过双提交校验。
	if token := c.csrfToken(); token == "" {
		t.Fatal("登录态探测应签发 CSRF Cookie")
	}

	// 可共享缓存的只读接口不重新签发 Cookie，否则 CDN 无法缓存。
	clean := e.newClient(t)
	if cc := clean.get(api.APIPrefix + "/capabilities").Header.Get("Cache-Control"); cc == "" {
		t.Fatal("可缓存接口缺少 Cache-Control")
	}
	if token := clean.csrfToken(); token != "" {
		t.Fatalf("可缓存的只读接口不应签发 CSRF Cookie，实际 %q", token)
	}
}

// TestWebWithoutStatic 验证常规构建（未开启 embedweb 标签、未设置 STATIC_DIR）
// 不注册静态路由：非接口路径保持 404，前端由 Vite 开发服务器服务。
func TestWebWithoutStatic(t *testing.T) {
	e := newTestEnv(t)
	c := e.newClient(t)
	for _, p := range []string{"/", "/prompts/123/edit"} {
		expectErrorCode(t, c.get(p), http.StatusNotFound, "not_found")
	}
}

// TestWebRequestLoggerStatus 验证返回错误的处理器在访问日志中记录真实状态码：
// 4xx 记 info 级别，只有 5xx 才记「请求失败」。
func TestWebRequestLoggerStatus(t *testing.T) {
	dir := t.TempDir()
	writeWebFile(t, filepath.Join(dir, "index.html"), "<!doctype html>")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	e := newTestEnv(t)
	deps := e.Deps(t)
	deps.Env.StaticDir = dir
	deps.Logger = logger
	srv, err := api.New(deps)
	if err != nil {
		t.Fatalf("构造接口层失败: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	expectErrorCode(t, webGet(t, ts.URL, "/missing.png"), http.StatusNotFound, "not_found")

	logText := buf.String()
	if strings.Contains(logText, "请求失败") {
		t.Fatalf("4xx 响应被记成请求失败：%s", logText)
	}
	if !strings.Contains(logText, "status=404") {
		t.Fatalf("访问日志未记录 404 状态码：%s", logText)
	}
}
