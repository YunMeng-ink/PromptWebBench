package api_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/promptbench/backend/internal/domain"
	api "github.com/promptbench/backend/internal/http"
	"github.com/promptbench/backend/internal/platform/pgtest"
)

const httpTestPassword = "HttpTest1!"

// apiUserDTO 只取用例关心的字段。
type apiUserDTO struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	EmailVerified bool   `json:"email_verified"`
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func registerViaHTTP(t *testing.T, client *apiClient, email, password string) apiUserDTO {
	t.Helper()
	client.prime()
	resp := client.post(api.APIPrefix+"/auth/register", map[string]any{
		"email":    email,
		"password": password,
	})
	expectStatus(t, resp, http.StatusOK)
	return decodeBody[apiUserDTO](t, resp)
}

// registerAndVerifyViaHTTP 走完整的注册与邮箱验证，返回已验证账户。
func registerAndVerifyViaHTTP(t *testing.T, env *testEnv, client *apiClient, email, password string) apiUserDTO {
	t.Helper()

	registerViaHTTP(t, client, email, password)
	code := env.mail.LastCode(t)

	client.prime()
	resp := client.post(api.APIPrefix+"/auth/verify-email", map[string]any{"email": email, "code": code})
	expectStatus(t, resp, http.StatusOK)
	return decodeBody[apiUserDTO](t, resp)
}

// registerVerifyLogin 完成注册、验证与登录，得到一个已登录的已验证账户。
func registerVerifyLogin(t *testing.T, env *testEnv, client *apiClient, email, password string) apiUserDTO {
	t.Helper()

	user := registerAndVerifyViaHTTP(t, env, client, email, password)
	env.login(t, client, email, password)
	return user
}

func (e *testEnv) login(t *testing.T, client *apiClient, email, password string) apiUserDTO {
	t.Helper()
	client.prime()
	resp := client.post(api.APIPrefix+"/auth/login", map[string]any{"email": email, "password": password})
	expectStatus(t, resp, http.StatusOK)
	if len(resp.Cookies()) == 0 {
		t.Fatal("登录响应未下发会话 Cookie")
	}
	user := decodeBody[apiUserDTO](t, resp)
	if user.Status == "" {
		t.Fatal("登录响应缺少账户信息")
	}
	return user
}

func TestRegisterVerifyLoginLogoutOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)

	registered := registerViaHTTP(t, client, "flow@example.com", httpTestPassword)
	if registered.Status != string(domain.StatusUnverified) {
		t.Fatalf("注册后状态 = %s，期望 unverified", registered.Status)
	}
	if registered.EmailVerified {
		t.Fatal("注册后邮箱不应为已验证")
	}

	// 未验证账户可登录并浏览，但不能生成凭据（C-23）。
	env.login(t, client, "flow@example.com", httpTestPassword)
	resp := client.get(api.APIPrefix + "/me")
	expectStatus(t, resp, http.StatusOK)
	decodeBody[apiUserDTO](t, resp)

	resp = client.post(api.APIPrefix+"/me/api-token", nil)
	expectErrorCode(t, resp, http.StatusForbidden, "email_not_verified")

	code := env.mail.LastCode(t)
	resp = client.post(api.APIPrefix+"/auth/verify-email", map[string]any{"email": "flow@example.com", "code": code})
	expectStatus(t, resp, http.StatusOK)

	verified := decodeBody[apiUserDTO](t, resp)
	if verified.Status != string(domain.StatusNormal) || !verified.EmailVerified {
		t.Fatalf("验证后账户 = %+v，期望 normal 且已验证", verified)
	}

	// 验证后可以生成令牌。
	resp = client.post(api.APIPrefix+"/me/api-token", nil)
	expectStatus(t, resp, http.StatusOK)
	token := decodeBody[struct {
		Token  string `json:"token"`
		Prefix string `json:"prefix"`
	}](t, resp)
	if token.Token == "" {
		t.Fatal("未返回令牌明文")
	}

	// 登出后会话立即失效。
	resp = client.post(api.APIPrefix+"/auth/logout", nil)
	expectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	resp = client.get(api.APIPrefix + "/me")
	expectErrorCode(t, resp, http.StatusUnauthorized, "unauthorized")
}

func TestRegisterValidationRejectsShortPassword(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()

	resp := client.post(api.APIPrefix+"/auth/register", map[string]any{
		"email":    "short@example.com",
		"password": "abc",
	})
	expectErrorCode(t, resp, http.StatusUnprocessableEntity, "validation_failed")
}

func TestRegisterValidationRejectsMalformedEmail(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()

	resp := client.post(api.APIPrefix+"/auth/register", map[string]any{
		"email":    "not-an-email",
		"password": httpTestPassword,
	})
	expectErrorCode(t, resp, http.StatusUnprocessableEntity, "validation_failed")
}

func TestSessionEndpointReportsGuestAndUser(t *testing.T) {
	env := newTestEnv(t)

	guest := env.newClient(t)
	resp := guest.get(api.APIPrefix + "/auth/session")
	expectErrorCode(t, resp, http.StatusUnauthorized, "unauthorized")

	registerVerifyLogin(t, env, guest, "session@example.com", httpTestPassword)
	resp = guest.get(api.APIPrefix + "/auth/session")
	expectStatus(t, resp, http.StatusOK)
	if user := decodeBody[apiUserDTO](t, resp); user.Email != "session@example.com" {
		t.Fatalf("会话账户 = %s", user.Email)
	}
}

func TestProtectedRoutesRequireAuthentication(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)

	expectErrorCode(t, client.get(api.APIPrefix+"/me"), http.StatusUnauthorized, "unauthorized")
	expectErrorCode(t, client.get(api.APIPrefix+"/me/api-token"), http.StatusUnauthorized, "unauthorized")
	expectErrorCode(t, client.patch(api.APIPrefix+"/me", map[string]any{"bio": "x"}), http.StatusUnauthorized, "unauthorized")
	expectErrorCode(t, client.post(api.APIPrefix+"/auth/logout", nil), http.StatusUnauthorized, "unauthorized")
}

func TestAPITokenChannelOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	registerVerifyLogin(t, env, client, "tokenflow@example.com", httpTestPassword)

	resp := client.post(api.APIPrefix+"/me/api-token", nil)
	expectStatus(t, resp, http.StatusOK)
	first := decodeBody[struct {
		Token string `json:"token"`
	}](t, resp)

	// Bearer 通道不需要 CSRF 头。
	bearer := &apiClient{t: t, base: env.ts.URL, client: &http.Client{}}
	req, err := http.NewRequest(http.MethodGet, env.ts.URL+api.APIPrefix+"/me", nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+first.Token)
	bearerResp, err := bearer.client.Do(req)
	if err != nil {
		t.Fatalf("Bearer 请求失败: %v", err)
	}
	expectStatus(t, bearerResp, http.StatusOK)
	if user := decodeBody[apiUserDTO](t, bearerResp); user.Email != "tokenflow@example.com" {
		t.Fatalf("Bearer 会话账户 = %s", user.Email)
	}

	// 重新生成后旧令牌立即失效。
	resp = client.post(api.APIPrefix+"/me/api-token", nil)
	expectStatus(t, resp, http.StatusOK)
	second := decodeBody[struct {
		Token string `json:"token"`
	}](t, resp)
	if second.Token == first.Token {
		t.Fatal("重新生成应产生不同令牌")
	}

	oldReq, _ := http.NewRequest(http.MethodGet, env.ts.URL+api.APIPrefix+"/me", nil)
	oldReq.Header.Set("Authorization", "Bearer "+first.Token)
	oldResp, err := http.DefaultClient.Do(oldReq)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	expectErrorCode(t, oldResp, http.StatusUnauthorized, "unauthorized")

	// 撤销后同样失效。
	resp = client.delete(api.APIPrefix + "/me/api-token")
	expectStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	newReq, _ := http.NewRequest(http.MethodGet, env.ts.URL+api.APIPrefix+"/me", nil)
	newReq.Header.Set("Authorization", "Bearer "+second.Token)
	newResp, err := http.DefaultClient.Do(newReq)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	expectErrorCode(t, newResp, http.StatusUnauthorized, "unauthorized")
}

func TestChangePasswordKeepsCurrentSessionAlive(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	registerVerifyLogin(t, env, client, "changepw@example.com", httpTestPassword)

	resp := client.post(api.APIPrefix+"/me/password", map[string]any{
		"old_password": httpTestPassword,
		"new_password": "Changed1Pass!",
	})
	expectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 当前会话仍可用（响应里换发了新令牌）。
	resp = client.get(api.APIPrefix + "/me")
	expectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	other := env.newClient(t)
	other.prime()
	loginResp := other.post(api.APIPrefix+"/auth/login", map[string]any{
		"email": "changepw@example.com", "password": httpTestPassword,
	})
	expectErrorCode(t, loginResp, http.StatusUnauthorized, "invalid_credentials")

	env.login(t, other, "changepw@example.com", "Changed1Pass!")
}

func TestClosedRegistrationStillAllowsInviteCode(t *testing.T) {
	env := newTestEnv(t)
	env.setConfig(t, "registration_policy", string(domain.RegistrationClosed))

	client := env.newClient(t)
	client.prime()
	resp := client.post(api.APIPrefix+"/auth/register", map[string]any{
		"email": "closed@example.com", "password": httpTestPassword,
	})
	expectErrorCode(t, resp, http.StatusForbidden, "registration_closed")

	if _, err := env.pool.Exec(context.Background(), `
		INSERT INTO invite_codes (code, kind, max_uses) VALUES ('HTTP-INVITE', 'normal', 1)`); err != nil {
		t.Fatalf("写入邀请码失败: %v", err)
	}

	resp = client.post(api.APIPrefix+"/auth/register", map[string]any{
		"email": "closed@example.com", "password": httpTestPassword, "invite_code": "HTTP-INVITE",
	})
	expectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestAdminBanFlowOverHTTP(t *testing.T) {
	env := newTestEnv(t)

	pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "admin@example.com", Password: "AdminPass1!", Role: "admin", Verified: true,
	})
	targetID := pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "target@example.com", Password: httpTestPassword, Verified: true,
	})
	pgtest.SeedPrompt(t, env.pool, targetID, "待转移提示词")

	admin := env.newClient(t)
	env.login(t, admin, "admin@example.com", "AdminPass1!")

	var systemID int64
	if err := env.pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE is_system`).Scan(&systemID); err != nil {
		t.Fatalf("读取系统账户失败: %v", err)
	}

	resp := admin.post(api.APIPrefix+"/admin/users/"+itoa(targetID)+"/ban", map[string]any{"reason": "违规"})
	expectStatus(t, resp, http.StatusOK)
	banned := decodeBody[struct {
		User                   apiUserDTO `json:"user"`
		TransferredPromptCount int        `json:"transferred_prompt_count"`
	}](t, resp)
	if banned.User.Status != string(domain.StatusBanned) {
		t.Fatalf("状态 = %s，期望 banned", banned.User.Status)
	}
	if banned.TransferredPromptCount != 1 {
		t.Fatalf("转移数量 = %d，期望 1", banned.TransferredPromptCount)
	}

	var uploader int64
	if err := env.pool.QueryRow(context.Background(),
		`SELECT uploader_id FROM prompts WHERE original_uploader_id = $1`, targetID).Scan(&uploader); err != nil {
		t.Fatalf("读取上传者失败: %v", err)
	}
	if uploader != systemID {
		t.Fatalf("提示词上传者 = %d，期望系统账户 %d", uploader, systemID)
	}

	// 被封禁账户不能重新登录。
	victim := env.newClient(t)
	victim.prime()
	resp = victim.post(api.APIPrefix+"/auth/login", map[string]any{
		"email": "target@example.com", "password": httpTestPassword,
	})
	expectErrorCode(t, resp, http.StatusForbidden, "banned")
}

func TestBannedUserSessionIsRejectedOverHTTP(t *testing.T) {
	env := newTestEnv(t)

	pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "admin2@example.com", Password: "AdminPass1!", Role: "admin", Verified: true,
	})
	targetID := pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "v@example.com", Password: httpTestPassword, Verified: true,
	})

	victim := env.newClient(t)
	env.login(t, victim, "v@example.com", httpTestPassword)
	expectStatus(t, victim.get(api.APIPrefix+"/me"), http.StatusOK)

	admin := env.newClient(t)
	env.login(t, admin, "admin2@example.com", "AdminPass1!")
	resp := admin.post(api.APIPrefix+"/admin/users/"+itoa(targetID)+"/ban", map[string]any{"reason": ""})
	expectStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// 旧会话立即失效，并明确报告封禁状态（C-24）。
	expectErrorCode(t, victim.get(api.APIPrefix+"/me"), http.StatusForbidden, "banned")
}

func TestNonAdminCannotReachAdminRoutes(t *testing.T) {
	env := newTestEnv(t)
	member := env.newClient(t)
	targetID := registerVerifyLogin(t, env, member, "member@example.com", httpTestPassword).ID

	// 普通用户与游客都不能访问后台端点。
	expectErrorCode(t, member.get(api.APIPrefix+"/admin/users/"+itoa(targetID)),
		http.StatusForbidden, "permission_denied")

	guest := env.newClient(t)
	expectErrorCode(t, guest.get(api.APIPrefix+"/admin/users/"+itoa(targetID)),
		http.StatusUnauthorized, "unauthorized")
}

func TestAdminGrantsAndRevokesDeveloper(t *testing.T) {
	env := newTestEnv(t)
	pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "admin3@example.com", Password: "AdminPass1!", Role: "admin", Verified: true,
	})
	member := env.newClient(t)
	memberID := registerAndVerifyViaHTTP(t, env, member, "dev@example.com", httpTestPassword).ID

	admin := env.newClient(t)
	env.login(t, admin, "admin3@example.com", "AdminPass1!")

	resp := admin.put(api.APIPrefix+"/admin/users/"+itoa(memberID)+"/developer", map[string]any{"enabled": true})
	expectStatus(t, resp, http.StatusOK)
	if user := decodeBody[apiUserDTO](t, resp); user.Role != string(domain.RoleDeveloper) {
		t.Fatalf("身份组 = %s，期望 developer", user.Role)
	}

	resp = admin.put(api.APIPrefix+"/admin/users/"+itoa(memberID)+"/developer", map[string]any{"enabled": false})
	expectStatus(t, resp, http.StatusOK)
	if user := decodeBody[apiUserDTO](t, resp); user.Role != string(domain.RoleUser) {
		t.Fatalf("身份组 = %s，期望 user", user.Role)
	}
}

func TestPasswordResetOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	registerAndVerifyViaHTTP(t, env, client, "reset.http@example.com", httpTestPassword)

	client.prime()
	resp := client.post(api.APIPrefix+"/auth/password/reset-request", map[string]any{"email": "reset.http@example.com"})
	expectStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	code := env.mail.LastCode(t)
	resp = client.post(api.APIPrefix+"/auth/password/reset", map[string]any{
		"email": "reset.http@example.com", "code": code, "new_password": "ResetHttp1!",
	})
	expectStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	fresh := env.newClient(t)
	env.login(t, fresh, "reset.http@example.com", "ResetHttp1!")
}

func TestGitHubStartWithoutConfigurationReturns503(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)

	expectErrorCode(t, client.get(api.APIPrefix+"/auth/oauth/github/start"),
		http.StatusServiceUnavailable, "oauth_not_configured")
}

func TestGitHubCallbackWithBadStateRedirectsWithError(t *testing.T) {
	env := newTestEnv(t)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(env.ts.URL + api.APIPrefix + "/auth/oauth/github/callback?code=x&state=bad")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码 = %d，期望 302", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if location == "" {
		t.Fatal("缺少 Location 头")
	}
	if !strings.Contains(location, "oauth/github/callback?error=") {
		t.Fatalf("Location = %s，期望带错误码的落地页地址", location)
	}
}
