package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/service/admin"
)

// 模块 7：注册与邀请码管理（需求 §17.7）。
func (s *Server) registerAdminInviteRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/invites",
		OperationID: "adminListInvites", Summary: "邀请码列表", Tags: []string{"后台"},
	}, s.handleAdminListInvites)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/invites",
		OperationID: "adminCreateInvites", Summary: "生成邀请码", Tags: []string{"后台"},
	}, s.handleAdminCreateInvites)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/invites/{id}/disabled",
		OperationID: "adminSetInviteDisabled", Summary: "禁用或启用邀请码", Tags: []string{"后台"},
	}, s.handleAdminSetInviteDisabled)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/invites/{id}/usages",
		OperationID: "adminListInviteUsages", Summary: "邀请码使用记录", Tags: []string{"后台"},
	}, s.handleAdminListInviteUsages)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/registration-policy",
		OperationID: "adminGetRegistrationPolicy", Summary: "当前注册策略", Tags: []string{"后台"},
	}, s.handleAdminGetRegistrationPolicy)
}

type adminListInvitesInput struct {
	Page     int32  `query:"page" default:"1" minimum:"1"`
	PageSize int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	Kind     string `query:"kind" enum:",normal,developer" doc:"按邀请码类型筛选"`
	Disabled string `query:"disabled" enum:",true,false" doc:"按禁用状态筛选"`
}

func (s *Server) handleAdminListInvites(ctx context.Context, in *adminListInvitesInput) (*adminInviteListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListInviteCodes(ctx, in.Kind, in.Disabled, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminInviteListResponse{}
	out.Body.Items = make([]adminInviteDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminInviteDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminCreateInvitesInput struct {
	Body struct {
		Kind          string `json:"kind" enum:"normal,developer" doc:"邀请码类型"`
		Count         int32  `json:"count" default:"1" minimum:"1" maximum:"50" doc:"生成数量"`
		MaxUses       int32  `json:"max_uses" default:"1" minimum:"1" maximum:"1000" doc:"单个邀请码最大使用次数"`
		ExpiresInDays int32  `json:"expires_in_days" minimum:"0" maximum:"365" doc:"有效期天数；0 表示永不过期"`
		Note          string `json:"note,omitempty" maxLength:"200" doc:"备注"`
	}
}

func (s *Server) handleAdminCreateInvites(ctx context.Context, in *adminCreateInvitesInput) (*adminInviteCreateResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Admin.CreateInviteCodes(ctx, operator, admin.InviteInput{
		Kind:          in.Body.Kind,
		Count:         in.Body.Count,
		MaxUses:       in.Body.MaxUses,
		ExpiresInDays: in.Body.ExpiresInDays,
		Note:          in.Body.Note,
	}, clientIP(ctx))
	if err != nil {
		return nil, err
	}

	out := &adminInviteCreateResponse{}
	out.Body.Items = make([]adminInviteDTO, 0, len(rows))
	for _, row := range rows {
		out.Body.Items = append(out.Body.Items, inviteRowDTO(row))
	}
	return out, nil
}

type adminSetInviteDisabledInput struct {
	ID   int64 `path:"id" doc:"邀请码 ID"`
	Body struct {
		Disabled bool `json:"disabled" doc:"是否禁用"`
	}
}

func (s *Server) handleAdminSetInviteDisabled(ctx context.Context, in *adminSetInviteDisabledInput) (*adminInviteResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.SetInviteCodeDisabled(ctx, in.ID, in.Body.Disabled)
	if err != nil {
		return nil, err
	}
	return &adminInviteResponse{Body: inviteRowDTO(row)}, nil
}

type adminInviteUsagesInput struct {
	ID       int64 `path:"id" doc:"邀请码 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100"`
}

func (s *Server) handleAdminListInviteUsages(ctx context.Context, in *adminInviteUsagesInput) (*adminInviteUsageListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListInviteCodeUsages(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminInviteUsageListResponse{}
	out.Body.Items = make([]adminInviteUsageDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminInviteUsageDTO{
			ID: row.ID, UserID: row.UserID, Username: row.Username,
			UsedAt: timeString(row.UsedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminRegistrationPolicyResponse struct {
	Body struct {
		Policy string `json:"policy" enum:"open,closed,invite_only" doc:"当前注册策略"`
	}
}

func (s *Server) handleAdminGetRegistrationPolicy(ctx context.Context, _ *struct{}) (*adminRegistrationPolicyResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	policy, err := s.deps.Admin.RegistrationPolicy(ctx)
	if err != nil {
		return nil, err
	}
	out := &adminRegistrationPolicyResponse{}
	out.Body.Policy = string(policy)
	return out, nil
}
