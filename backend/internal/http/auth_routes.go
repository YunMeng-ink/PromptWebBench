package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/service/auth"
)

// userDTO 是账户对外表示。domain.User 携带口令哈希，绝不能直接序列化。
type userDTO struct {
	ID            int64  `json:"id" doc:"账户 ID"`
	Username      string `json:"username" doc:"昵称"`
	Email         string `json:"email" doc:"邮箱"`
	Role          string `json:"role" doc:"身份组"`
	Status        string `json:"status" doc:"账户状态"`
	Bio           string `json:"bio" doc:"简介"`
	AvatarURL     string `json:"avatar_url" doc:"头像地址"`
	EmailVerified bool   `json:"email_verified" doc:"邮箱是否已验证"`
}

func toUserDTO(user domain.User) userDTO {
	return userDTO{
		ID:            user.ID,
		Username:      user.Username,
		Email:         user.Email,
		Role:          string(user.Role),
		Status:        string(user.Status),
		Bio:           user.Bio,
		AvatarURL:     user.AvatarURL,
		EmailVerified: user.EmailVerifiedAt != nil,
	}
}

type userResponse struct {
	Body userDTO
}

// sessionResponse 通过响应头下发会话 Cookie。
type sessionResponse struct {
	SetCookie []string `header:"Set-Cookie" doc:"会话 Cookie"`
	Body      userDTO
}

func (s *Server) registerAuthRoutes() {
	Register(s, "auth", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/auth/register",
		OperationID: "register", Summary: "邮箱密码注册", Tags: []string{"认证"},
	}, s.handleRegister)

	Register(s, "auth", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/auth/verify-email",
		OperationID: "verifyEmail", Summary: "提交邮箱验证码", Tags: []string{"认证"},
	}, s.handleVerifyEmail)

	Register(s, "auth", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/auth/resend-verification",
		OperationID: "resendVerification", Summary: "重发邮箱验证码", Tags: []string{"认证"},
	}, s.handleResendVerification)

	Register(s, "auth", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/auth/login",
		OperationID: "login", Summary: "登录", Tags: []string{"认证"},
	}, s.handleLogin)

	Register(s, "profile_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/auth/logout",
		OperationID: "logout", Summary: "登出", Tags: []string{"认证"},
	}, s.handleLogout)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/auth/session",
		OperationID: "getSession", Summary: "查询当前登录态", Tags: []string{"认证"},
	}, s.handleSession)

	Register(s, "auth", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/auth/password/reset-request",
		OperationID: "requestPasswordReset", Summary: "请求密码重置验证码", Tags: []string{"认证"},
	}, s.handleRequestPasswordReset)

	Register(s, "auth", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/auth/password/reset",
		OperationID: "resetPassword", Summary: "用验证码重置口令", Tags: []string{"认证"},
	}, s.handleResetPassword)

	s.registerGitHubRoutes()
}

type registerInput struct {
	Body struct {
		Email      string `json:"email" required:"true" format:"email" maxLength:"254" doc:"注册邮箱"`
		Password   string `json:"password" required:"true" minLength:"8" maxLength:"72" doc:"登录口令"`
		InviteCode string `json:"invite_code,omitempty" maxLength:"64" doc:"邀请码，公开注册关闭时必填"`
	}
}

func (s *Server) handleRegister(ctx context.Context, in *registerInput) (*userResponse, error) {
	user, err := s.deps.Auth.Register(ctx, auth.RegisterInput{
		Email:      in.Body.Email,
		Password:   in.Body.Password,
		InviteCode: in.Body.InviteCode,
	})
	if err != nil {
		return nil, err
	}
	return &userResponse{Body: toUserDTO(user)}, nil
}

type verifyEmailInput struct {
	Body struct {
		Email string `json:"email" required:"true" format:"email" doc:"注册邮箱"`
		Code  string `json:"code" required:"true" minLength:"6" maxLength:"6" doc:"6 位验证码"`
	}
}

type verifyEmailResponse struct {
	SetCookie []string `header:"Set-Cookie" doc:"会话 Cookie"`
	Body      userDTO
}

func (s *Server) handleVerifyEmail(ctx context.Context, in *verifyEmailInput) (*verifyEmailResponse, error) {
	user, err := s.deps.Auth.VerifyEmail(ctx, in.Body.Email, in.Body.Code)
	if err != nil {
		return nil, err
	}

	// 验证通过即证明对该邮箱的控制权，直接下发会话，用户无需再登录一次。
	session, err := s.deps.Auth.SessionFor(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return &verifyEmailResponse{
		SetCookie: []string{s.sessionCookie(session).String()},
		Body:      toUserDTO(session.User),
	}, nil
}

type resendVerificationInput struct {
	Body struct {
		Email string `json:"email" required:"true" format:"email" doc:"注册邮箱"`
	}
}

func (s *Server) handleResendVerification(ctx context.Context, in *resendVerificationInput) (*struct{}, error) {
	if err := s.deps.Auth.ResendVerification(ctx, in.Body.Email); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type loginInput struct {
	Body struct {
		Email    string `json:"email" required:"true" format:"email" doc:"注册邮箱"`
		Password string `json:"password" required:"true" doc:"登录口令"`
	}
}

func (s *Server) handleLogin(ctx context.Context, in *loginInput) (*sessionResponse, error) {
	session, err := s.deps.Auth.Login(ctx, in.Body.Email, in.Body.Password, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return &sessionResponse{
		SetCookie: []string{s.sessionCookie(session).String()},
		Body:      toUserDTO(session.User),
	}, nil
}

func (s *Server) handleLogout(ctx context.Context, _ *struct{}) (*sessionResponse, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Auth.Logout(ctx, user.ID); err != nil {
		return nil, err
	}
	return &sessionResponse{
		SetCookie: []string{s.expiredSessionCookie().String()},
		Body:      toUserDTO(user),
	}, nil
}

func (s *Server) handleSession(ctx context.Context, _ *struct{}) (*userResponse, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	return &userResponse{Body: toUserDTO(user)}, nil
}

type requestPasswordResetInput struct {
	Body struct {
		Email string `json:"email" required:"true" format:"email" doc:"注册邮箱"`
	}
}

func (s *Server) handleRequestPasswordReset(ctx context.Context, in *requestPasswordResetInput) (*struct{}, error) {
	if err := s.deps.Auth.RequestPasswordReset(ctx, in.Body.Email); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type resetPasswordInput struct {
	Body struct {
		Email       string `json:"email" required:"true" format:"email" doc:"注册邮箱"`
		Code        string `json:"code" required:"true" minLength:"6" maxLength:"6" doc:"6 位验证码"`
		NewPassword string `json:"new_password" required:"true" minLength:"8" maxLength:"72" doc:"新口令"`
	}
}

func (s *Server) handleResetPassword(ctx context.Context, in *resetPasswordInput) (*struct{}, error) {
	if err := s.deps.Auth.ResetPassword(ctx, in.Body.Email, in.Body.Code, in.Body.NewPassword); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
