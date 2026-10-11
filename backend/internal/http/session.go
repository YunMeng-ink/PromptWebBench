package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/service/auth"
)

// SessionCookieName 是会话 Cookie 名称。
const SessionCookieName = "pb_session"

// viewerContextKey 是身份信息在请求上下文中的键。
type viewerContextKey struct{}

// viewerState 记录本次请求的身份判定结果。err 区分「未登录」与「账户被封禁」，
// 使受保护端点在封禁时返回 banned 而不是 unauthorized。
type viewerState struct {
	user *domain.User
	err  error
	ip   string
}

// Authenticate 是全局 Huma 中间件：解析会话 Cookie 或 Bearer 令牌，把身份写入上下文。
// 解析失败不中断请求，由受保护端点自行决定是否拒绝。
func (s *Server) Authenticate(ctx huma.Context, next func(huma.Context)) {
	state := s.resolveViewer(ctx)
	next(huma.WithValue(ctx, viewerContextKey{}, state))
}

func (s *Server) resolveViewer(ctx huma.Context) viewerState {
	ip := ClientIP(ctx.RemoteAddr(), ctx.Header("X-Forwarded-For"), s.trusted)

	if header := ctx.Header("Authorization"); strings.HasPrefix(header, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == "" {
			return viewerState{err: apperr.New(apperr.CodeUnauthorized, ""), ip: ip}
		}
		user, err := s.deps.Auth.ResolveAPIToken(ctx.Context(), token)
		if err != nil {
			return viewerState{err: err, ip: ip}
		}
		return viewerState{user: &user, ip: ip}
	}

	raw := cookieValue(ctx.Header("Cookie"), SessionCookieName)
	if raw == "" {
		return viewerState{err: apperr.New(apperr.CodeUnauthorized, ""), ip: ip}
	}

	claims, err := s.signer.Verify(raw)
	if err != nil {
		return viewerState{err: apperr.New(apperr.CodeUnauthorized, ""), ip: ip}
	}
	user, err := s.deps.Auth.ResolveSession(ctx.Context(), claims.UserID, claims.TokenVersion)
	if err != nil {
		return viewerState{err: err, ip: ip}
	}
	return viewerState{user: &user, ip: ip}
}

// clientIP 返回 Authenticate 中间件解析出的客户端地址。
func clientIP(ctx context.Context) string {
	state, ok := ctx.Value(viewerContextKey{}).(viewerState)
	if !ok {
		return ""
	}
	return state.ip
}

// viewer 返回当前身份；未登录时返回错误。
func viewer(ctx context.Context) (domain.User, error) {
	state, ok := ctx.Value(viewerContextKey{}).(viewerState)
	if !ok || state.user == nil {
		if state.err != nil {
			return domain.User{}, state.err
		}
		return domain.User{}, apperr.New(apperr.CodeUnauthorized, "")
	}
	return *state.user, nil
}

// requireUser 要求登录并返回当前身份。
func requireUser(ctx context.Context) (domain.User, error) {
	return viewer(ctx)
}

// requireVerified 要求账户处于正常态：未验证账户可浏览但不可产生内容与凭据（C-23）。
func requireVerified(ctx context.Context) (domain.User, error) {
	user, err := viewer(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if !user.CanWriteContent() {
		return domain.User{}, apperr.New(apperr.CodeEmailNotVerified, "")
	}
	return user, nil
}

// requireAdmin 要求管理员身份，附带的开发者与用户权限由 domain.User.IsDeveloper 判定。
func requireAdmin(ctx context.Context) (domain.User, error) {
	user, err := viewer(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if !user.IsAdmin() {
		return domain.User{}, apperr.New(apperr.CodePermissionDenied, "")
	}
	return user, nil
}

// sessionCookie 构造会话 Cookie。HttpOnly 使脚本无法读取，SameSite=Lax 阻断跨站写请求。
func (s *Server) sessionCookie(session auth.Session) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.deps.Env.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	}
}

// expiredSessionCookie 构造立即过期的会话 Cookie，用于登出。
func (s *Server) expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.deps.Env.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	}
}

// cookieValue 用标准库解析 Cookie 头。
func cookieValue(header, name string) string {
	if header == "" {
		return ""
	}
	req := &http.Request{Header: http.Header{"Cookie": []string{header}}}
	cookie, err := req.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}
