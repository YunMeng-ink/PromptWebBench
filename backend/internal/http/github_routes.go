package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"

	"github.com/promptbench/backend/internal/apperr"
)

// GitHub 授权流程由后端处理回调：浏览器在 GitHub 授权后回到后端，
// 后端签发会话 Cookie 再把浏览器送回前端的落地页。
func (s *Server) registerGitHubRoutes() {
	s.echo.GET(APIPrefix+"/auth/oauth/github/start", s.handleGitHubStart)
	s.echo.GET(APIPrefix+"/auth/oauth/github/callback", s.handleGitHubCallback)
}

func (s *Server) handleGitHubStart(c *echo.Context) error {
	target, err := s.deps.Auth.GitHubAuthorizeURL(c.Request().Context())
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, target)
}

func (s *Server) handleGitHubCallback(c *echo.Context) error {
	landing := s.deps.Auth.GitHubLandingURL()
	code := c.QueryParam("code")
	state := c.QueryParam("state")
	if code == "" || state == "" {
		return c.Redirect(http.StatusFound, withErrorCode(landing, apperr.CodeInvalidCode))
	}

	session, err := s.deps.Auth.CompleteGitHubLogin(c.Request().Context(), code, state)
	if err != nil {
		s.deps.Logger.Warn("GitHub 登录失败", "error", err.Error())
		return c.Redirect(http.StatusFound, withErrorCode(landing, errorCodeOf(err)))
	}

	c.SetCookie(s.sessionCookie(session))
	return c.Redirect(http.StatusFound, landing)
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
