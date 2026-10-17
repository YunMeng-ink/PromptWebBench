package api

import (
	"crypto/hmac"
	"errors"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/security"
)

// oauthStateCookieName 是绑定 OAuth 登录流程与发起浏览器的 Cookie 名。
// /start 把 state 中的一次性随机数写入该 Cookie，/callback 比对两者，
// 防止攻击者把「自己的授权」链接发给受害者完成登录 CSRF。
const oauthStateCookieName = "pb_oauth_state"

// GitHub 授权流程由后端处理回调：浏览器在 GitHub 授权后回到后端，
// 后端签发会话 Cookie 再把浏览器送回前端的落地页。
func (s *Server) registerGitHubRoutes() {
	s.echo.GET(APIPrefix+"/auth/oauth/github/start", s.handleGitHubStart)
	s.echo.GET(APIPrefix+"/auth/oauth/github/callback", s.handleGitHubCallback)
}

func (s *Server) handleGitHubStart(c *echo.Context) error {
	target, nonce, err := s.deps.Auth.GitHubAuthorizeURL(c.Request().Context())
	if err != nil {
		return err
	}
	c.SetCookie(&http.Cookie{
		Name:     oauthStateCookieName,
		Value:    nonce,
		Path:     "/",
		MaxAge:   int(security.OAuthStateTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.deps.Env.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return c.Redirect(http.StatusFound, target)
}

func (s *Server) handleGitHubCallback(c *echo.Context) error {
	// 每次回调都可能外呼 GitHub 令牌端点，先按来源地址限流（复用 auth 规则），
	// 无效 code 的洪泛不会放大成对外流量与日志刷屏。
	allowed, _ := s.limiter.AllowIP("auth", c.Request().RemoteAddr, c.Request().Header.Get("X-Forwarded-For"))
	if !allowed {
		return NewError(apperr.CodeRateLimited, "")
	}

	landing := s.deps.Auth.GitHubLandingURL()
	code := c.QueryParam("code")
	state := c.QueryParam("state")
	if code == "" || state == "" {
		return c.Redirect(http.StatusFound, withErrorCode(landing, apperr.CodeInvalidCode))
	}

	// 回调必须来自发起登录的浏览器：state 的随机数要与 /start 写入的 Cookie 一致。
	cookie, err := c.Cookie(oauthStateCookieName)
	if err != nil || cookie.Value == "" {
		return c.Redirect(http.StatusFound, withErrorCode(landing, apperr.CodeInvalidOAuthState))
	}
	if err := s.verifyOAuthStateBinding(state, cookie.Value); err != nil {
		return c.Redirect(http.StatusFound, withErrorCode(landing, apperr.CodeInvalidOAuthState))
	}

	session, err := s.deps.Auth.CompleteGitHubLogin(c.Request().Context(), code, state)
	if err != nil {
		s.deps.Logger.Warn("GitHub 登录失败", "error", err.Error())
		return c.Redirect(http.StatusFound, withErrorCode(landing, errorCodeOf(err)))
	}

	// 登录完成即作废绑定 Cookie：state 是一次性的，防止回放。
	s.clearOAuthStateCookie(c)
	c.SetCookie(s.sessionCookie(session))
	return c.Redirect(http.StatusFound, landing)
}

// verifyOAuthStateBinding 校验 state 的签名与时效，并把其中的随机数与
// 发起登录时写入浏览器的 Cookie 做常数时间比较。
func (s *Server) verifyOAuthStateBinding(state, cookieValue string) error {
	nonce, err := s.deps.Auth.OAuthStateNonce(state)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(nonce), []byte(cookieValue)) {
		return security.ErrInvalidOAuthState
	}
	return nil
}

// clearOAuthStateCookie 作废绑定 Cookie。
func (s *Server) clearOAuthStateCookie(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.deps.Env.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// withErrorCode 在落地页地址上附加错误码，供前端展示。
func withErrorCode(landing string, code apperr.Code) string {
	return landing + "?error=" + url.QueryEscape(string(code))
}

func errorCodeOf(err error) apperr.Code {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return apperr.CodeInternal
}
