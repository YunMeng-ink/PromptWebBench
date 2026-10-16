package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/service/admin"
)

// 模块 9：公告管理（需求 §2.4、§17.8）。
func (s *Server) registerAdminAnnouncementRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/announcements",
		OperationID: "adminListAnnouncements", Summary: "公告列表", Tags: []string{"后台"},
	}, s.handleAdminListAnnouncements)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/announcements",
		OperationID: "adminCreateAnnouncement", Summary: "发布公告", Tags: []string{"后台"},
	}, s.handleAdminCreateAnnouncement)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/admin/announcements/{id}",
		OperationID: "adminUpdateAnnouncement", Summary: "修改公告", Tags: []string{"后台"},
	}, s.handleAdminUpdateAnnouncement)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/announcements/{id}/withdraw",
		OperationID: "adminWithdrawAnnouncement", Summary: "撤回公告", Tags: []string{"后台"},
	}, s.handleAdminWithdrawAnnouncement)
}

type adminListAnnouncementsInput struct {
	Page     int32  `query:"page" default:"1" minimum:"1"`
	PageSize int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	State    string `query:"state" enum:",live,withdrawn" doc:"按状态筛选"`
}

func (s *Server) handleAdminListAnnouncements(ctx context.Context, in *adminListAnnouncementsInput) (*adminAnnouncementListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListAnnouncements(ctx, in.State, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminAnnouncementListResponse{}
	out.Body.Items = make([]adminAnnouncementDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminAnnouncementRow(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminAnnouncementInput struct {
	Body struct {
		Title string `json:"title" minLength:"1" maxLength:"200" doc:"公告标题"`
		Body  string `json:"body" maxLength:"5000" doc:"公告正文"`
	}
}

func (s *Server) handleAdminCreateAnnouncement(ctx context.Context, in *adminAnnouncementInput) (*adminAnnouncementResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.CreateAnnouncement(ctx, operator, admin.AnnouncementInput{
		Title: in.Body.Title, Body: in.Body.Body,
	}, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return &adminAnnouncementResponse{Body: adminAnnouncementFromRow(row)}, nil
}

type adminUpdateAnnouncementInput struct {
	ID   int64 `path:"id" doc:"公告 ID"`
	Body struct {
		Title string `json:"title" minLength:"1" maxLength:"200"`
		Body  string `json:"body" maxLength:"5000"`
	}
}

func (s *Server) handleAdminUpdateAnnouncement(ctx context.Context, in *adminUpdateAnnouncementInput) (*adminAnnouncementResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.UpdateAnnouncement(ctx, in.ID, admin.AnnouncementInput{
		Title: in.Body.Title, Body: in.Body.Body,
	})
	if err != nil {
		return nil, err
	}
	return &adminAnnouncementResponse{Body: adminAnnouncementFromRow(row)}, nil
}

type adminAnnouncementIDInput struct {
	ID int64 `path:"id" doc:"公告 ID"`
}

func (s *Server) handleAdminWithdrawAnnouncement(ctx context.Context, in *adminAnnouncementIDInput) (*adminAnnouncementResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.WithdrawAnnouncement(ctx, operator, in.ID, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return &adminAnnouncementResponse{Body: adminAnnouncementFromRow(row)}, nil
}
