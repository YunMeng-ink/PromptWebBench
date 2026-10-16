package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/service/admin"
)

// 模块 4：用户管理（需求 §17.4）。用户上传的提示词由 /admin/prompts?uploader_id=
// 提供，不在本模块重复实现同一份列表。
func (s *Server) registerAdminUserRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/users",
		OperationID: "adminListUsers", Summary: "用户列表", Tags: []string{"后台"},
	}, s.handleAdminListUsers)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/users/{id}/ratings",
		OperationID: "adminListUserRatings", Summary: "用户评分记录", Tags: []string{"后台"},
	}, s.handleAdminListUserRatings)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/users/{id}/comments",
		OperationID: "adminListUserComments", Summary: "用户评论", Tags: []string{"后台"},
	}, s.handleAdminListUserComments)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/users/{id}/libraries",
		OperationID: "adminListUserLibraries", Summary: "用户题库", Tags: []string{"后台"},
	}, s.handleAdminListUserLibraries)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/users/{id}/reports",
		OperationID: "adminListUserReports", Summary: "用户举报记录", Tags: []string{"后台"},
	}, s.handleAdminListUserReports)
}

type adminListUsersInput struct {
	Page     int32  `query:"page" default:"1" minimum:"1"`
	PageSize int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	Keyword  string `query:"keyword" maxLength:"100" doc:"按昵称与邮箱搜索"`
	Role     string `query:"role" enum:",user,developer,admin" doc:"按身份组筛选"`
	Status   string `query:"status" enum:",normal,unverified,banned" doc:"按账户状态筛选"`
}

func (s *Server) handleAdminListUsers(ctx context.Context, in *adminListUsersInput) (*adminUserListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListUsers(ctx, admin.UserFilter{
		Keyword: in.Keyword, Role: in.Role, Status: in.Status,
	}, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminUserListResponse{}
	out.Body.Items = make([]adminUserDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminUserDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminUserPagedInput struct {
	ID       int64 `path:"id" doc:"账户 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100"`
}

func (s *Server) handleAdminListUserRatings(ctx context.Context, in *adminUserPagedInput) (*adminUserRatingListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListUserRatings(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminUserRatingListResponse{}
	out.Body.Items = make([]adminUserRatingDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminUserRatingDTO{
			ID: row.ID, PromptID: row.PromptID, PromptTitle: row.PromptTitle,
			ModelID: row.ModelID, ModelName: row.ModelName,
			Score: row.Score, EditCount: row.EditCount, Deleted: row.Deleted,
			CreatedAt: timeString(row.CreatedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

func (s *Server) handleAdminListUserComments(ctx context.Context, in *adminUserPagedInput) (*adminUserCommentListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListUserComments(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminUserCommentListResponse{}
	out.Body.Items = make([]adminUserCommentDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminUserCommentDTO{
			ID: row.ID, PromptID: row.PromptID, PromptTitle: row.PromptTitle,
			ParentID: int64PtrOf(row.ParentID), Body: row.Body, Deleted: row.Deleted,
			CreatedAt: timeString(row.CreatedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

func (s *Server) handleAdminListUserLibraries(ctx context.Context, in *adminUserPagedInput) (*adminUserLibraryListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListUserLibraries(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminUserLibraryListResponse{}
	out.Body.Items = make([]adminUserLibraryDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminUserLibraryDTO{
			ID: row.ID, Name: row.Name, Description: row.Description,
			QuestionCount: row.QuestionCount, Deleted: row.Deleted,
			CreatedAt: timeString(row.CreatedAt), UpdatedAt: timeString(row.UpdatedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

func (s *Server) handleAdminListUserReports(ctx context.Context, in *adminUserPagedInput) (*adminUserReportListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListUserReports(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminUserReportListResponse{}
	out.Body.Items = make([]adminUserReportDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminUserReportDTO{
			ID: row.ID, ObjectType: row.ObjectType, ObjectID: row.ObjectID,
			Reason: row.Reason, Status: row.Status, Detail: row.Detail,
			CreatedAt: timeString(row.CreatedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}
