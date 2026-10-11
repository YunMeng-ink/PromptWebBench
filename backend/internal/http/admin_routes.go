package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// 阶段 1 只提供账户治理所需的后台端点：封禁、解封、身份组授予与撤销，
// 以及按 ID 查看账户。用户列表与其余后台模块在阶段 6 提供。
func (s *Server) registerAdminAccountRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/users/{id}",
		OperationID: "adminGetUser", Summary: "查看账户", Tags: []string{"后台"},
	}, s.handleAdminGetUser)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/users/{id}/ban",
		OperationID: "adminBanUser", Summary: "封禁账户", Tags: []string{"后台"},
	}, s.handleAdminBanUser)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/users/{id}/unban",
		OperationID: "adminUnbanUser", Summary: "解封账户", Tags: []string{"后台"},
	}, s.handleAdminUnbanUser)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPut, Path: APIPrefix + "/admin/users/{id}/developer",
		OperationID: "adminSetDeveloper", Summary: "授予或撤销开发者身份", Tags: []string{"后台"},
	}, s.handleAdminSetDeveloper)
}

type adminUserInput struct {
	ID int64 `path:"id" doc:"目标账户 ID"`
}

func (s *Server) handleAdminGetUser(ctx context.Context, in *adminUserInput) (*userResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	user, err := s.deps.Admin.UserByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &userResponse{Body: toUserDTO(user)}, nil
}

type adminBanUserInput struct {
	ID   int64 `path:"id" doc:"目标账户 ID"`
	Body struct {
		Reason string `json:"reason,omitempty" maxLength:"200" doc:"封禁原因"`
	}
}

type adminBanUserResponse struct {
	Body struct {
		User                   userDTO `json:"user"`
		TransferredPromptCount int     `json:"transferred_prompt_count" doc:"转移到系统账户的提示词数量"`
	}
}

func (s *Server) handleAdminBanUser(ctx context.Context, in *adminBanUserInput) (*adminBanUserResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.deps.Admin.BanUser(ctx, operator, in.ID, in.Body.Reason, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	out := &adminBanUserResponse{}
	out.Body.User = toUserDTO(result.User)
	out.Body.TransferredPromptCount = result.TransferredPrompts
	return out, nil
}

func (s *Server) handleAdminUnbanUser(ctx context.Context, in *adminUserInput) (*userResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.deps.Admin.UnbanUser(ctx, operator, in.ID, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return &userResponse{Body: toUserDTO(user)}, nil
}

type adminSetDeveloperInput struct {
	ID   int64 `path:"id" doc:"目标账户 ID"`
	Body struct {
		Enabled bool `json:"enabled" doc:"是否具备开发者身份"`
	}
}

func (s *Server) handleAdminSetDeveloper(ctx context.Context, in *adminSetDeveloperInput) (*userResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.deps.Admin.SetDeveloper(ctx, operator, in.ID, in.Body.Enabled, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return &userResponse{Body: toUserDTO(user)}, nil
}
