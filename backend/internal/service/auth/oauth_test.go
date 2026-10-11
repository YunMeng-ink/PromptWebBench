package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oauth"
	"github.com/promptbench/backend/internal/platform/pgtest"
)

// fakeGitHub 是一个本地 HTTP 服务，按 GitHub 的响应结构作答。
// 用例走的是真实 oauth.Client 代码，仅替换了传输端点。
type fakeGitHub struct {
	server *httptest.Server

	mu               sync.Mutex
	userID           int64
	login            string
	userEmail        string
	emails           []map[string]any
	exchangeError    string
	userStatus       int
	failTokenRequest bool
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()

	f := &fakeGitHub{
		userID:    9001,
		login:     "octocat",
		userEmail: "octocat@example.com",
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGitHub) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.URL.Path {
	case "/login/oauth/access_token":
		if f.failTokenRequest {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if f.exchangeError != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": f.exchangeError})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gh-access-token"})

	case "/user":
		if f.userStatus != 0 {
			w.WriteHeader(f.userStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    f.userID,
			"login": f.login,
			"name":  "The Octocat",
			"email": f.userEmail,
		})

	case "/user/emails":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.emails)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeGitHub) client() *oauth.Client {
	client := oauth.NewGitHubClient("client-id", "client-secret")
	client.AuthorizeEndpoint = f.server.URL + "/login/oauth/authorize"
	client.TokenEndpoint = f.server.URL + "/login/oauth/access_token"
	client.UserEndpoint = f.server.URL + "/user"
	client.EmailsEndpoint = f.server.URL + "/user/emails"
	client.HTTPClient = f.server.Client()
	return client
}

// startLogin 走一次授权跳转，返回其中的 state。
func (f *fakeGitHub) startLogin(t *testing.T, h *harness) string {
	t.Helper()

	authURL, err := h.service.GitHubAuthorizeURL(context.Background())
	if err != nil {
		t.Fatalf("生成授权地址失败: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("授权地址无法解析: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatalf("授权地址缺少 state: %s", authURL)
	}
	return state
}

func TestGitHubLoginCreatesVerifiedAccount(t *testing.T) {
	fake := newFakeGitHub(t)
	h := newHarnessWith(t, options{gitHub: fake.client()})
	state := fake.startLogin(t, h)

	session, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state)
	if err != nil {
		t.Fatalf("GitHub 登录失败: %v", err)
	}
	if session.User.Status != domain.StatusNormal {
		t.Fatalf("已验证邮箱的 GitHub 账户状态 = %s，期望 normal", session.User.Status)
	}
	if session.User.Email != "octocat@example.com" {
		t.Fatalf("邮箱 = %s", session.User.Email)
	}
	if session.User.Username != "octocat" {
		t.Fatalf("昵称 = %s，期望取 GitHub 登录名", session.User.Username)
	}
	if h.countRows(t, `SELECT count(*) FROM oauth_accounts WHERE provider = 'github' AND provider_user_id = '9001'`) != 1 {
		t.Fatal("应创建第三方绑定")
	}
	if len(h.mail.Messages()) != 0 {
		t.Fatal("已验证邮箱不应发送验证码")
	}
}

func TestGitHubLoginWithUnverifiedEmailGoesThroughVerification(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.userEmail = ""
	fake.emails = []map[string]any{
		{"email": "unverified@example.com", "primary": true, "verified": false},
	}
	h := newHarnessWith(t, options{gitHub: fake.client()})
	state := fake.startLogin(t, h)

	session, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state)
	if err != nil {
		t.Fatalf("GitHub 登录失败: %v", err)
	}
	if session.User.Status != domain.StatusUnverified {
		t.Fatalf("未验证邮箱的账户状态 = %s，期望 unverified（C-36）", session.User.Status)
	}

	code := h.mail.LastCode(t)
	if _, err := h.service.VerifyEmail(context.Background(), "unverified@example.com", code); err != nil {
		t.Fatalf("走正常邮箱验证流程应可完成: %v", err)
	}
	if h.userStatus(t, "unverified@example.com") != string(domain.StatusNormal) {
		t.Fatal("验证后应转为正常态")
	}
}

func TestGitHubLoginKeepsUserEmailWhenProvided(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.userEmail = "primary@example.com"
	fake.emails = []map[string]any{
		{"email": "secondary@example.com", "primary": false, "verified": true},
	}
	h := newHarnessWith(t, options{gitHub: fake.client()})
	state := fake.startLogin(t, h)

	session, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state)
	if err != nil {
		t.Fatalf("GitHub 登录失败: %v", err)
	}
	if session.User.Email != "primary@example.com" {
		t.Fatalf("邮箱 = %s，应优先使用用户接口返回的邮箱", session.User.Email)
	}
}

func TestGitHubLoginBindsExistingAccountWithVerifiedEmail(t *testing.T) {
	fake := newFakeGitHub(t)
	h := newHarnessWith(t, options{gitHub: fake.client()})

	existingID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "octocat@example.com", Password: testPassword, Verified: true, Username: "existing",
	})
	state := fake.startLogin(t, h)

	session, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state)
	if err != nil {
		t.Fatalf("GitHub 登录失败: %v", err)
	}
	if session.User.ID != existingID {
		t.Fatalf("登录到账户 %d，期望复用同邮箱的存量账户 %d", session.User.ID, existingID)
	}
	if h.countRows(t, `SELECT count(*) FROM oauth_accounts WHERE user_id = $1`, existingID) != 1 {
		t.Fatal("应把 GitHub 身份绑定到存量账户")
	}
}

func TestGitHubLoginRefusesUnverifiedEmailCollision(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.userEmail = ""
	fake.emails = []map[string]any{
		{"email": "octocat@example.com", "primary": true, "verified": false},
	}
	h := newHarnessWith(t, options{gitHub: fake.client()})

	pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "octocat@example.com", Password: testPassword, Verified: true})
	state := fake.startLogin(t, h)

	_, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state)
	expectCode(t, err, apperr.CodeEmailTaken)
}

func TestGitHubLoginReusesBindingForReturningUser(t *testing.T) {
	fake := newFakeGitHub(t)
	h := newHarnessWith(t, options{gitHub: fake.client()})

	first := fake.startLogin(t, h)
	session, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", first)
	if err != nil {
		t.Fatalf("首次 GitHub 登录失败: %v", err)
	}

	second := fake.startLogin(t, h)
	again, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", second)
	if err != nil {
		t.Fatalf("再次 GitHub 登录失败: %v", err)
	}
	if again.User.ID != session.User.ID {
		t.Fatalf("账户 = %d，期望复用 %d", again.User.ID, session.User.ID)
	}
	if h.countRows(t, `SELECT count(*) FROM users WHERE lower(email) = 'octocat@example.com'`) != 1 {
		t.Fatal("不应重复建号")
	}
}

func TestGitHubRegistrationRefusedWhenClosedButBoundLoginWorks(t *testing.T) {
	fake := newFakeGitHub(t)
	h := newHarnessWith(t, options{gitHub: fake.client()})

	state := fake.startLogin(t, h)
	session, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state)
	if err != nil {
		t.Fatalf("首次登录失败: %v", err)
	}

	h.setConfig(t, "registration_policy", string(domain.RegistrationClosed))

	// 已绑定用户仍可登录（C-37）。
	again := fake.startLogin(t, h)
	if _, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", again); err != nil {
		t.Fatalf("关闭注册后已绑定用户应可登录: %v", err)
	}

	// 新用户注册被拒。
	fake.userID = 9002
	fake.login = "newcomer"
	fake.userEmail = "newcomer@example.com"
	fresh := fake.startLogin(t, h)
	_, err = h.service.CompleteGitHubLogin(context.Background(), "auth-code", fresh)
	expectCode(t, err, apperr.CodeRegistrationClosed)

	if h.countRows(t, `SELECT count(*) FROM users WHERE lower(email) = 'newcomer@example.com'`) != 0 {
		t.Fatal("关闭注册时不应创建新账户")
	}
	_ = session
}

func TestGitHubLoginRejectsTamperedState(t *testing.T) {
	fake := newFakeGitHub(t)
	h := newHarnessWith(t, options{gitHub: fake.client()})
	state := fake.startLogin(t, h)

	_, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state+"tampered")
	expectCode(t, err, apperr.CodeInvalidCode)
}

func TestGitHubLoginMapsExchangeFailure(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.exchangeError = "bad_verification_code"
	h := newHarnessWith(t, options{gitHub: fake.client()})
	state := fake.startLogin(t, h)

	_, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state)
	expectCode(t, err, apperr.CodeInvalidCode)
}

func TestGitHubLoginMapsUpstreamFailure(t *testing.T) {
	fake := newFakeGitHub(t)
	fake.userStatus = http.StatusInternalServerError
	h := newHarnessWith(t, options{gitHub: fake.client()})
	state := fake.startLogin(t, h)

	_, err := h.service.CompleteGitHubLogin(context.Background(), "auth-code", state)
	expectCode(t, err, apperr.CodeBadGateway)
}

func TestGitHubEndpointsRequireConfiguration(t *testing.T) {
	h := newHarness(t)

	_, err := h.service.GitHubAuthorizeURL(context.Background())
	expectCode(t, err, apperr.CodeOAuthNotConfigured)

	_, err = h.service.CompleteGitHubLogin(context.Background(), "code", "state")
	expectCode(t, err, apperr.CodeOAuthNotConfigured)
}

func TestGitHubAuthorizeURLContainsRequiredParameters(t *testing.T) {
	fake := newFakeGitHub(t)
	h := newHarnessWith(t, options{gitHub: fake.client()})

	authURL, err := h.service.GitHubAuthorizeURL(context.Background())
	if err != nil {
		t.Fatalf("生成授权地址失败: %v", err)
	}
	for _, fragment := range []string{"client_id=client-id", "state=", "scope=", "redirect_uri="} {
		if !strings.Contains(authURL, fragment) {
			t.Fatalf("授权地址缺少 %s: %s", fragment, authURL)
		}
	}
	if !strings.Contains(authURL, "127.0.0.1") && !strings.Contains(authURL, "localhost") {
		t.Fatalf("授权地址应指向本地测试服务: %s", authURL)
	}
}
