package api

import (
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// Scope 声明限流的计数维度（技术可行性报告 §3.2）。
type Scope string

const (
	ScopeIP   Scope = "ip"
	ScopeUser Scope = "user"
)

// Rule 是一条限流规则。窗口统一为 1 分钟，限额与维度按报告 §3.2。
type Rule struct {
	Name   string
	Limit  int
	Window time.Duration
	Scope  Scope
}

// rules 是限流规则的集中声明表，也是「未登记限流的写路由在启动时 panic」
// 这一防护的判据来源。
var rules = map[string]Rule{
	"guest_read":    {Name: "guest_read", Limit: 60, Window: time.Minute, Scope: ScopeIP},
	"user_read":     {Name: "user_read", Limit: 120, Window: time.Minute, Scope: ScopeUser},
	"admin_read":    {Name: "admin_read", Limit: 120, Window: time.Minute, Scope: ScopeUser},
	"rating_write":  {Name: "rating_write", Limit: 30, Window: time.Minute, Scope: ScopeUser},
	"comment_write": {Name: "comment_write", Limit: 20, Window: time.Minute, Scope: ScopeUser},
	"prompt_write":  {Name: "prompt_write", Limit: 20, Window: time.Minute, Scope: ScopeUser},
	"prompt_import": {Name: "prompt_import", Limit: 5, Window: time.Minute, Scope: ScopeUser},
	"report_write":  {Name: "report_write", Limit: 5, Window: time.Minute, Scope: ScopeUser},
	"auth":          {Name: "auth", Limit: 10, Window: time.Minute, Scope: ScopeIP},
	"admin_write":   {Name: "admin_write", Limit: 120, Window: time.Minute, Scope: ScopeUser},
}

// RuleNames 返回全部已登记规则名，供启动自检与文档核对。
func RuleNames() []string {
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	return names
}

func lookupRule(name string) (Rule, bool) {
	r, ok := rules[name]
	return r, ok
}

// isWriteMethod 判定请求是否为写方法。写路由必须显式声明限流规则。
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

type bucket struct {
	windowStart time.Time
	count       int
}

// Limiter 是进程内固定窗口限流器。单实例部署下不引入外部存储（技术可行性报告 §4.10）。
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	trusted []netip.Prefix

	// userResolver 由认证层在阶段 1 注入；未注入或未登录时按来源地址计数。
	userResolver func(huma.Context) (int64, bool)

	now func() time.Time
}

func NewLimiter(trusted []netip.Prefix) *Limiter {
	return &Limiter{
		buckets: map[string]*bucket{},
		trusted: trusted,
		now:     time.Now,
	}
}

// SetUserResolver 注入登录用户解析函数，用于 ScopeUser 规则。
func (l *Limiter) SetUserResolver(f func(huma.Context) (int64, bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.userResolver = f
}

// allow 返回是否放行以及需等待的时长。
func (l *Limiter) allow(key string, rule Rule) (bool, time.Duration) {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok || now.Sub(b.windowStart) >= rule.Window {
		l.buckets[key] = &bucket{windowStart: now, count: 1}
		return true, 0
	}
	if b.count < rule.Limit {
		b.count++
		return true, 0
	}
	return false, rule.Window - now.Sub(b.windowStart)
}

// identity 计算计数键。ScopeUser 未登录时退化为按地址计数，避免匿名请求共用同一配额。
func (l *Limiter) identity(ctx huma.Context, rule Rule) string {
	l.mu.Lock()
	resolver := l.userResolver
	l.mu.Unlock()

	if rule.Scope == ScopeUser && resolver != nil {
		if uid, ok := resolver(ctx); ok {
			return fmt.Sprintf("%s:u:%d", rule.Name, uid)
		}
	}
	ip := ClientIP(ctx.RemoteAddr(), ctx.Header("X-Forwarded-For"), l.trusted)
	return fmt.Sprintf("%s:ip:%s", rule.Name, ip)
}

// Middleware 返回绑定到某条规则的 Huma 中间件。超限时直接写出 429 并终止链路。
func (l *Limiter) Middleware(api huma.API, ruleName string) func(huma.Context, func(huma.Context)) {
	rule, ok := lookupRule(ruleName)
	if !ok {
		panic(fmt.Errorf("未登记的限流规则 %q", ruleName))
	}
	return func(ctx huma.Context, next func(huma.Context)) {
		allowed, retryAfter := l.allow(l.identity(ctx, rule), rule)
		if allowed {
			next(ctx)
			return
		}
		ctx.SetHeader("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		huma.WriteErr(api, ctx, http.StatusTooManyRequests, "")
	}
}

// Cleanup 清理已过期窗口，由后台定时任务调用。
func (l *Limiter) Cleanup() {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, b := range l.buckets {
		if now.Sub(b.windowStart) >= time.Minute {
			delete(l.buckets, key)
		}
	}
}
