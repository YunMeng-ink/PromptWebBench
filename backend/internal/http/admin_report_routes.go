package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/service/admin"
)

// 模块 6：举报管理（需求 §17.6）。
func (s *Server) registerAdminReportRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/reports",
		OperationID: "adminListReports", Summary: "举报列表", Tags: []string{"后台"},
	}, s.handleAdminListReports)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/reports/{id}/handle",
		OperationID: "adminHandleReport", Summary: "处置举报", Tags: []string{"后台"},
	}, s.handleAdminHandleReport)
}

type adminListReportsInput struct {
	Page       int32  `query:"page" default:"1" minimum:"1"`
	PageSize   int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	ObjectType string `query:"object_type" enum:",prompt,comment" doc:"按被举报对象类型筛选"`
	Status     string `query:"status" enum:",pending,ignored,resolved" doc:"按状态筛选"`
}

func (s *Server) handleAdminListReports(ctx context.Context, in *adminListReportsInput) (*adminReportListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListReports(ctx, admin.ReportFilter{
		ObjectType: in.ObjectType, Status: in.Status,
	}, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminReportListResponse{}
	out.Body.Items = make([]adminReportDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminReportDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminHandleReportInput struct {
	ID   int64 `path:"id" doc:"举报 ID"`
	Body struct {
		Status        string `json:"status" enum:"pending,ignored,resolved" doc:"处置后的状态"`
		Note          string `json:"note,omitempty" maxLength:"500" doc:"处理备注"`
		DeleteContent bool   `json:"delete_content" doc:"是否删除被举报内容"`
		BanAuthor     bool   `json:"ban_author" doc:"是否封禁内容作者"`
	}
}

func (s *Server) handleAdminHandleReport(ctx context.Context, in *adminHandleReportInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	status, ok := domain.ParseReportStatus(in.Body.Status)
	if !ok {
		return nil, NewError(apperr.CodeValidationFailed, "举报状态取值非法")
	}
	err = s.deps.Admin.HandleReport(ctx, operator, in.ID, admin.ReportHandleInput{
		Status: status, Note: in.Body.Note,
		DeleteContent: in.Body.DeleteContent, BanAuthor: in.Body.BanAuthor,
	}, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
