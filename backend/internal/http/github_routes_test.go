package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/promptbench/backend/internal/http"
	"github.com/promptbench/backend/internal/security"
)

// noRedirectClient 不跟随重定向，便于检查 302 的落地地址。
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func getCallback(t *testing.T, base, code, state, cookieValue string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet,
		base+api.APIPrefix+"/auth/oauth/github/callback?code="+code+"&state="+state, nil)
	if err != nil {
		t.Fatalf("构造回调请求失败: %v", err)
	}
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: "pb_oauth_state", Value: cookieValue})
	}
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("请求回调失败: %v", err)
	}
	return resp
}

func expectOAuthError(t *testing.T, resp *http.Response, code string) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("回调状态码 = %d，期望 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "error="+code) {
		t.Fatalf("落地地址 %s 未携带错误码 %s", loc, code)
	}
}

// TestGitHubCallbackBindsStateToBrowser 覆盖 OAuth 登录 CSRF 防护：
// 回调的 state 随机数必须与发起登录时写入浏览器的 Cookie 一致。
func TestGitHubCallbackBindsStateToBrowser(t *testing.T) {
	e := newTestEnv(t)
	ts := httptest.NewServer(e.server.Handler())
	t.Cleanup(ts.Close)

	// 与测试环境同一把密钥签发 state，使签名校验通过、绑定校验成为唯一闸门。
	signer := security.NewOAuthStateSigner(testJWTSecret, security.OAuthStateTTL)
	validState := signer.Sign("deadbeef")

	// 无绑定 Cookie：拒绝。
	expectOAuthError(t, getCallback(t, ts.URL, "code-1", validState, ""), "oauth_state_invalid")

	// Cookie 与 state 中的随机数不一致：拒绝（登录 CSRF 场景）。
	expectOAuthError(t, getCallback(t, ts.URL, "code-2", validState, "attacker-nonce"), "oauth_state_invalid")

	// 绑定一致：通过绑定校验，进入后续流程（测试环境未配置 GitHub，报 oauth_not_configured）。
	expectOAuthError(t, getCallback(t, ts.URL, "code-3", validState, "deadbeef"), "oauth_not_configured")
}
