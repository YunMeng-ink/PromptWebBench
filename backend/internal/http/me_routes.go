package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerMeRoutes() {
	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me",
		OperationID: "getMe", Summary: "查询个人资料", Tags: []string{"个人中心"},
	}, s.handleGetMe)

	Register(s, "profile_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/me",
		OperationID: "updateMe", Summary: "修改个人资料", Tags: []string{"个人中心"},
	}, s.handleUpdateMe)

	Register(s, "profile_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/me/password",
		OperationID: "changePassword", Summary: "修改口令", Tags: []string{"个人中心"},
	}, s.handleChangePassword)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/api-token",
		OperationID: "getAPIToken", Summary: "查询 API Token 状态", Tags: []string{"个人中心"},
	}, s.handleGetAPIToken)

	Register(s, "token_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/me/api-token",
		OperationID: "createAPIToken", Summary: "生成 API Token", Tags: []string{"个人中心"},
	}, s.handleCreateAPIToken)

	Register(s, "token_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/me/api-token",
		OperationID: "revokeAPIToken", Summary: "撤销 API Token", Tags: []string{"个人中心"},
	}, s.handleRevokeAPIToken)
}

func (s *Server) handleGetMe(ctx context.Context, _ *struct{}) (*userResponse, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	return &userResponse{Body: toUserDTO(user)}, nil
}

type updateMeInput struct {
	Body struct {
		Username  string `json:"username,omitempty" maxLength:"32" doc:"昵称，留空表示不修改"`
		Bio       string `json:"bio,omitempty" maxLength:"500" doc:"简介"`
		AvatarURL string `json:"avatar_url,omitempty" maxLength:"512" doc:"头像地址，留空表示清空；非空时须为 http(s) 绝对地址"`
	}
}

func (s *Server) handleUpdateMe(ctx context.Context, in *updateMeInput) (*userResponse, error) {
	user, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := s.deps.Auth.UpdateProfile(ctx, user.ID, in.Body.Username, in.Body.Bio, in.Body.AvatarURL)
	if err != nil {
		return nil, err
	}
	return &userResponse{Body: toUserDTO(updated)}, nil
}

type changePasswordInput struct {
	Body struct {
		OldPassword string `json:"old_password" required:"true" doc:"当前口令"`
		NewPassword string `json:"new_password" required:"true" minLength:"8" maxLength:"72" doc:"新口令"`
	}
}

func (s *Server) handleChangePassword(ctx context.Context, in *changePasswordInput) (*sessionResponse, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	session, err := s.deps.Auth.ChangePassword(ctx, user.ID, in.Body.OldPassword, in.Body.NewPassword)
	if err != nil {
		return nil, err
	}
	return &sessionResponse{
		SetCookie: []string{s.sessionCookie(session).String()},
		Body:      toUserDTO(session.User),
	}, nil
}

type apiTokenStatusResponse struct {
	Body struct {
		Present   bool   `json:"present" doc:"是否存在有效令牌"`
		Prefix    string `json:"prefix,omitempty" doc:"令牌前缀，用于辨识"`
		CreatedAt string `json:"created_at,omitempty" doc:"创建时间"`
	}
}

func (s *Server) handleGetAPIToken(ctx context.Context, _ *struct{}) (*apiTokenStatusResponse, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	status, err := s.deps.Auth.APITokenStatus(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	out := &apiTokenStatusResponse{}
	out.Body.Present = status.Present
	if status.Present {
		out.Body.Prefix = status.Prefix
		out.Body.CreatedAt = status.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return out, nil
}

type createAPITokenResponse struct {
	Body struct {
		Token  string `json:"token" doc:"令牌明文，仅本次返回"`
		Prefix string `json:"prefix" doc:"令牌前缀"`
	}
}

func (s *Server) handleCreateAPIToken(ctx context.Context, _ *struct{}) (*createAPITokenResponse, error) {
	user, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	token, err := s.deps.Auth.GenerateAPIToken(ctx, user.ID, user.Username, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	out := &createAPITokenResponse{}
	out.Body.Token = token.Plain
	out.Body.Prefix = token.Prefix
	return out, nil
}

func (s *Server) handleRevokeAPIToken(ctx context.Context, _ *struct{}) (*struct{}, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Auth.RevokeAPIToken(ctx, user.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
