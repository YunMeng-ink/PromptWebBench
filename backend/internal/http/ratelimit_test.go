package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/promptbench/backend/internal/apperr"
)

func TestRuleTableCoversSpecification(t *testing.T) {
	rules := DefaultRules()
	// 限额取值来源：技术可行性报告 §3.2 限流表。表可以扩充，但已声明的规则不得被削弱。
	want := map[string]struct {
		limit int
		scope Scope
	}{
		"guest_read":    {60, ScopeIP},
		"user_read":     {120, ScopeUser},
		"admin_read":    {120, ScopeUser},
		"rating_write":  {30, ScopeUser},
		"comment_write": {20, ScopeUser},
		"prompt_write":  {20, ScopeUser},
		"prompt_import": {5, ScopeUser},
		"report_write":  {5, ScopeUser},
		"auth":          {10, ScopeIP},
		"admin_write":   {120, ScopeUser},
	}
	for name, expect := range want {
		rule, ok := rules[name]
		if !ok {
			t.Errorf("规则 %s 缺失", name)
			continue
		}
		if rule.Limit != expect.limit {
			t.Errorf("规则 %s 限额 = %d，期望 %d", name, rule.Limit, expect.limit)
		}
		if rule.Scope != expect.scope {
			t.Errorf("规则 %s 维度 = %s，期望 %s", name, rule.Scope, expect.scope)
		}
		if rule.Window != time.Minute {
			t.Errorf("规则 %s 窗口 = %s，期望 1 分钟", name, rule.Window)
		}
		if rule.Name != name {
			t.Errorf("规则 %s 的 Name 字段 = %s，与键不一致", name, rule.Name)
		}
	}
}

func TestWriteMethodDetection(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !isWriteMethod(m) {
			t.Errorf("%s 应判定为写方法", m)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		if isWriteMethod(m) {
			t.Errorf("%s 不应判定为写方法", m)
		}
	}
}

func TestLimiterMiddlewareRejectsBeyondLimit(t *testing.T) {
	InstallErrorModel()
	_, api := humatest.New(t)
	limiter := NewLimiter(nil, nil)

	op := huma.Operation{
		Method:      http.MethodPost,
		Path:        "/probe",
		Middlewares: huma.Middlewares{limiter.Middleware(api, "auth")},
	}
	huma.Register(api, op, func(_ context.Context, _ *struct{}) (*struct{}, error) {
		return nil, nil
	})

	rule := DefaultRules()["auth"]
	for i := 0; i < rule.Limit; i++ {
		if resp := api.Post("/probe"); resp.Code == http.StatusTooManyRequests {
			t.Fatalf("第 %d 次请求即被限流，限额为 %d", i+1, rule.Limit)
		}
	}

	resp := api.Post("/probe")
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("第 %d 次请求状态码 = %d，期望 429", rule.Limit+1, resp.Code)
	}
	if got := resp.Header().Get("Retry-After"); got == "" {
		t.Error("429 响应缺少 Retry-After 头")
	}

	var envelope ErrorEnvelope
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析限流响应失败: %v，响应体 %s", err, resp.Body.String())
	}
	if envelope.Error.Code != string(apperr.CodeRateLimited) {
		t.Fatalf("限流错误码 = %s，期望 %s", envelope.Error.Code, apperr.CodeRateLimited)
	}
}

func TestLimiterWindowRolls(t *testing.T) {
	limiter := NewLimiter(nil, nil)
	base := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return base }

	rule := Rule{Name: "probe", Limit: 2, Window: time.Minute, Scope: ScopeIP}
	key := "probe:ip:127.0.0.1"

	if allowed, _ := limiter.allow(key, rule); !allowed {
		t.Fatal("窗口内第 1 次应放行")
	}
	if allowed, _ := limiter.allow(key, rule); !allowed {
		t.Fatal("窗口内第 2 次应放行")
	}
	if allowed, _ := limiter.allow(key, rule); allowed {
		t.Fatal("窗口内第 3 次应被拒绝")
	}

	base = base.Add(time.Minute)
	if allowed, _ := limiter.allow(key, rule); !allowed {
		t.Fatal("窗口滚动后应重新放行")
	}
}

func TestLimiterCleanupDropsExpiredWindows(t *testing.T) {
	limiter := NewLimiter(nil, nil)
	base := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return base }

	rule := Rule{Name: "probe", Limit: 1, Window: time.Minute, Scope: ScopeIP}
	limiter.allow("probe:ip:10.0.0.1", rule)

	limiter.now = func() time.Time { return base.Add(2 * time.Minute) }
	limiter.Cleanup()

	limiter.mu.Lock()
	size := len(limiter.buckets)
	limiter.mu.Unlock()
	if size != 0 {
		t.Fatalf("过期窗口清理后仍有 %d 个桶", size)
	}
}

func TestLimiterMiddlewarePanicsOnUnknownRule(t *testing.T) {
	_, api := humatest.New(t)
	limiter := NewLimiter(nil, nil)
	defer func() {
		if recover() == nil {
			t.Fatal("未登记的限流规则应当 panic")
		}
	}()
	limiter.Middleware(api, "no_such_rule")
}

func TestLimiterIdentitySwitchesBetweenAddressAndUser(t *testing.T) {
	limiter := NewLimiter(nil, nil)
	rule := DefaultRules()["rating_write"]

	op := &huma.Operation{Method: http.MethodGet, Path: "/probe"}
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.RemoteAddr = "10.1.2.3:5555"
	ctx := humatest.NewContext(op, req, httptest.NewRecorder())

	if got := limiter.identity(ctx, rule); got != "rating_write:ip:10.1.2.3" {
		t.Fatalf("匿名计数键 = %s，期望按来源地址计数", got)
	}

	limiter.SetUserResolver(func(huma.Context) (int64, bool) { return 42, true })
	if got := limiter.identity(ctx, rule); got != "rating_write:u:42" {
		t.Fatalf("登录后计数键 = %s，期望按用户计数", got)
	}
}
