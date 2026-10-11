package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/config"
	api "github.com/promptbench/backend/internal/http"
)

func testEnvConfig() config.Env {
	return config.Env{
		Port:             "0",
		JWTSecret:        "0123456789abcdef0123456789abcdef",
		CORSAllowOrigins: []string{"http://localhost:5173"},
		LogLevel:         "error",
	}
}

func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	return newTestEnv(t).server
}

func serve(t *testing.T, srv *api.Server) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func registerProbeGet(srv *api.Server, path string) {
	type out struct {
		Body struct {
			OK bool `json:"ok"`
		}
	}
	api.Register(srv, "user_read", huma.Operation{
		Method:      http.MethodGet,
		Path:        api.APIPrefix + path,
		OperationID: "probeGet" + strings.ReplaceAll(path, "/", "_"),
	}, func(_ context.Context, _ *struct{}) (*out, error) {
		result := &out{}
		result.Body.OK = true
		return result, nil
	})
}

func registerProbePost(srv *api.Server, path string) {
	type out struct {
		Body struct {
			OK bool `json:"ok"`
		}
	}
	api.Register(srv, "prompt_write", huma.Operation{
		Method:      http.MethodPost,
		Path:        api.APIPrefix + path,
		OperationID: "probePost" + strings.ReplaceAll(path, "/", "_"),
	}, func(_ context.Context, _ *struct{}) (*out, error) {
		result := &out{}
		result.Body.OK = true
		return result, nil
	})
}

func decodeEnvelope(t *testing.T, body []byte) api.ErrorEnvelope {
	t.Helper()
	var envelope api.ErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("响应不是错误信封: %v，响应体 %s", err, body)
	}
	return envelope
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)
	ts := serve(t, srv)

	resp, err := http.Get(ts.URL + api.APIPrefix + "/health")
	if err != nil {
		t.Fatalf("请求健康检查失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("健康检查状态码 = %d，期望 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("解析健康检查响应失败: %v", err)
	}
	if payload.Status != "ok" {
		t.Fatalf("健康检查状态 = %q，期望 ok", payload.Status)
	}
	if resp.Header.Get("X-Request-Id") == "" {
		t.Error("响应缺少 X-Request-Id")
	}
}

func TestUnknownRouteReturnsNotFoundEnvelope(t *testing.T) {
	srv := newTestServer(t)
	ts := serve(t, srv)

	resp, err := http.Get(ts.URL + api.APIPrefix + "/no-such-endpoint")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := decodeEnvelope(t, body).Error.Code; code != string(apperr.CodeNotFound) {
		t.Fatalf("错误码 = %s，期望 %s", code, apperr.CodeNotFound)
	}
}

func TestMetricsReachableFromLoopbackAndHiddenFromForwardedClients(t *testing.T) {
	srv := newTestServer(t)
	ts := serve(t, srv)

	// 先制造一次请求，使计数器产生样本。
	if _, err := http.Get(ts.URL + api.APIPrefix + "/health"); err != nil {
		t.Fatalf("预热请求失败: %v", err)
	}

	resp, err := http.Get(ts.URL + api.APIPrefix + "/metrics")
	if err != nil {
		t.Fatalf("抓取指标失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("回环抓取状态码 = %d，期望 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "prompt_benchmark_http_requests_total") {
		t.Fatalf("指标输出缺少自定义计数器，实际内容片段: %.200s", body)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+api.APIPrefix+"/metrics", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	forwarded, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("带转发头的请求失败: %v", err)
	}
	defer forwarded.Body.Close()
	if forwarded.StatusCode != http.StatusNotFound {
		t.Fatalf("非回环来源抓取状态码 = %d，期望 404", forwarded.StatusCode)
	}
	forwardedBody, _ := io.ReadAll(forwarded.Body)
	if code := decodeEnvelope(t, forwardedBody).Error.Code; code != string(apperr.CodeNotFound) {
		t.Fatalf("错误码 = %s，期望 %s", code, apperr.CodeNotFound)
	}
}

func TestOpenAPISpecDescribesErrorContract(t *testing.T) {
	srv := newTestServer(t)
	registerProbeGet(srv, "/probe")
	ts := serve(t, srv)

	resp, err := http.Get(ts.URL + api.APIPrefix + "/openapi.json")
	if err != nil {
		t.Fatalf("获取 OpenAPI 规范失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "openapi+json") {
		t.Fatalf("内容类型 = %s，期望 application/openapi+json", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"openapi":"3.1.0"`) {
		t.Fatalf("规范版本不是 3.1.0: %.200s", body)
	}

	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("规范不是合法 JSON: %v", err)
	}

	// 错误模型由 Huma 依据 huma.NewError 的返回类型生成：信封为 APIError，
	// 内含 ErrorBody，字段为 code 与 message。
	envelopeProps := dig(t, spec, "components", "schemas", "APIError", "properties")
	if _, ok := envelopeProps.(map[string]any)["error"]; !ok {
		t.Fatalf("APIError 组件缺少 error 属性: %#v", envelopeProps)
	}
	bodyProps := dig(t, spec, "components", "schemas", "ErrorBody", "properties")
	for _, field := range []string{"code", "message"} {
		if _, ok := bodyProps.(map[string]any)[field]; !ok {
			t.Fatalf("ErrorBody 组件缺少 %s 属性: %#v", field, bodyProps)
		}
	}

	defaultResp := dig(t, spec, "paths", api.APIPrefix+"/probe", "get", "responses", "default")
	ref := dig(t, defaultResp.(map[string]any), "content", "application/json", "schema", "$ref")
	if ref != "#/components/schemas/APIError" {
		t.Fatalf("默认错误响应引用 = %v，期望 #/components/schemas/APIError", ref)
	}
}

func dig(t *testing.T, node map[string]any, path ...string) any {
	t.Helper()
	var current any = node
	for _, key := range path {
		asMap, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("路径 %v 在 %q 处不是对象", path, key)
		}
		next, ok := asMap[key]
		if !ok {
			t.Fatalf("路径 %v 缺少键 %q", path, key)
		}
		current = next
	}
	return current
}

func TestCSRFBlocksWriteWithoutToken(t *testing.T) {
	srv := newTestServer(t)
	registerProbePost(srv, "/probe")
	ts := serve(t, srv)

	resp, err := http.Post(ts.URL+api.APIPrefix+"/probe", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("状态码 = %d，期望 403", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := decodeEnvelope(t, body).Error.Code; code != string(apperr.CodeInvalidCSRF) {
		t.Fatalf("错误码 = %s，期望 %s", code, apperr.CodeInvalidCSRF)
	}
}

func TestCSRFPassesWithDoubleSubmitToken(t *testing.T) {
	srv := newTestServer(t)
	registerProbePost(srv, "/probe")
	ts := serve(t, srv)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("创建 Cookie 容器失败: %v", err)
	}
	client := &http.Client{Jar: jar}

	if _, err := client.Get(ts.URL + api.APIPrefix + "/health"); err != nil {
		t.Fatalf("获取 CSRF Cookie 失败: %v", err)
	}
	parsed, _ := url.Parse(ts.URL)
	var token string
	for _, c := range jar.Cookies(parsed) {
		if c.Name == "pb_csrf" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("未拿到 pb_csrf Cookie")
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+api.APIPrefix+"/probe", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("双提交校验通过时状态码 = %d，期望 200，响应体 %s", resp.StatusCode, body)
	}
}

func TestCSRFBearerChannelIsExempt(t *testing.T) {
	srv := newTestServer(t)
	registerProbePost(srv, "/probe")
	ts := serve(t, srv)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+api.APIPrefix+"/probe", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer dummy-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Bearer 通道不应被 CSRF 拦截，响应体 %s", body)
	}
}

func TestWriteRouteWithoutRulePanics(t *testing.T) {
	srv := newTestServer(t)

	defer func() {
		if recover() == nil {
			t.Fatal("未声明限流规则的写路由应当 panic")
		}
	}()
	api.Register(srv, "", huma.Operation{
		Method:      http.MethodPost,
		Path:        api.APIPrefix + "/no-rule",
		OperationID: "probeNoRule",
	}, func(_ context.Context, _ *struct{}) (*struct{}, error) { return nil, nil })
}

func TestWriteRouteWithUnknownRulePanics(t *testing.T) {
	srv := newTestServer(t)

	defer func() {
		if recover() == nil {
			t.Fatal("引用未登记限流规则的路由应当 panic")
		}
	}()
	api.Register(srv, "not_a_rule", huma.Operation{
		Method:      http.MethodPost,
		Path:        api.APIPrefix + "/bad-rule",
		OperationID: "probeBadRule",
	}, func(_ context.Context, _ *struct{}) (*struct{}, error) { return nil, nil })
}

func TestReadRouteMayOmitRule(t *testing.T) {
	srv := newTestServer(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("读方法未声明限流规则不应 panic: %v", r)
		}
	}()
	api.Register(srv, "", huma.Operation{
		Method:      http.MethodGet,
		Path:        api.APIPrefix + "/read-no-rule",
		OperationID: "probeReadNoRule",
	}, func(_ context.Context, _ *struct{}) (*struct{}, error) { return nil, nil })

	var found *api.RouteRecord
	for _, record := range srv.RegisteredRoutes() {
		if record.Path == api.APIPrefix+"/read-no-rule" {
			route := record
			found = &route
		}
	}
	if found == nil {
		t.Fatal("未登记的读路由应出现在路由清单中")
	}
	if found.Rule != "" {
		t.Fatalf("读路由的限流规则 = %s，期望为空", found.Rule)
	}
}

func TestRequestBodyLimitIsEnforced(t *testing.T) {
	srv := newTestServer(t)
	registerProbePost(srv, "/probe")
	ts := serve(t, srv)

	oversized := strings.NewReader(strings.Repeat("a", int(api.BodyLimitBytes)+1024))
	req, _ := http.NewRequest(http.MethodPost, ts.URL+api.APIPrefix+"/probe", oversized)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer dummy-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if code := decodeEnvelope(t, body).Error.Code; code != string(apperr.CodeRequestEntityTooLarge) {
		t.Fatalf("错误码 = %s，期望 %s", code, apperr.CodeRequestEntityTooLarge)
	}
}

func TestServerRejectsIncompleteDependencies(t *testing.T) {
	_, err := api.New(api.Deps{Env: testEnvConfig()})
	if err == nil {
		t.Fatal("依赖不完整时构造接口层应报错")
	}
	if !strings.Contains(err.Error(), "依赖不完整") {
		t.Fatalf("错误信息不明确: %v", err)
	}
}

func TestServerRejectsEmptyCORSOrigins(t *testing.T) {
	env := newTestEnv(t)
	deps := env.Deps(t)
	deps.Env.CORSAllowOrigins = nil

	_, err := api.New(deps)
	if err == nil {
		t.Fatal("缺少 CORS 来源时应报错")
	}
	if !strings.Contains(err.Error(), "CORS_ALLOW_ORIGINS") {
		t.Fatalf("错误信息未指出缺少 CORS 来源: %v", err)
	}
}
