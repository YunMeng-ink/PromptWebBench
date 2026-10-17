package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/service/admin"
)

// registerAdminDictionaryRoutes 注册考察目的与考察维度的字典维护端点。
func (s *Server) registerAdminDictionaryRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/purposes",
		OperationID: "adminListPurposes", Summary: "考察目的列表", Tags: []string{"后台"},
	}, s.handleAdminListPurposes)
	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/purposes",
		OperationID: "adminCreatePurpose", Summary: "创建考察目的", Tags: []string{"后台"},
	}, s.handleAdminCreatePurpose)
	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/admin/purposes/{id}",
		OperationID: "adminUpdatePurpose", Summary: "编辑考察目的", Tags: []string{"后台"},
	}, s.handleAdminUpdatePurpose)
	Register(s, "admin_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/admin/purposes/{id}",
		OperationID: "adminDeletePurpose", Summary: "删除考察目的", Tags: []string{"后台"},
	}, s.handleAdminDeletePurpose)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/capabilities",
		OperationID: "adminListCapabilities", Summary: "考察维度列表", Tags: []string{"后台"},
	}, s.handleAdminListCapabilities)
	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/capabilities",
		OperationID: "adminCreateCapability", Summary: "创建考察维度", Tags: []string{"后台"},
	}, s.handleAdminCreateCapability)
	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/admin/capabilities/{id}",
		OperationID: "adminUpdateCapability", Summary: "编辑考察维度", Tags: []string{"后台"},
	}, s.handleAdminUpdateCapability)
	Register(s, "admin_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/admin/capabilities/{id}",
		OperationID: "adminDeleteCapability", Summary: "删除考察维度", Tags: []string{"后台"},
	}, s.handleAdminDeleteCapability)
}

type adminPurposeDTO struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	NameZh        string `json:"name_zh"`
	NameEn        string `json:"name_en"`
	DescriptionZh string `json:"description_zh"`
	DescriptionEn string `json:"description_en"`
	IsHighRisk    bool   `json:"is_high_risk"`
	IsEnabled     bool   `json:"is_enabled"`
	SortOrder     int32  `json:"sort_order"`
}

func toAdminPurposeDTO(row repo.Purpose) adminPurposeDTO {
	return adminPurposeDTO{
		ID: row.ID, Slug: row.Slug, NameZh: row.NameZh, NameEn: row.NameEn,
		DescriptionZh: row.DescriptionZh, DescriptionEn: row.DescriptionEn,
		IsHighRisk: row.IsHighRisk, IsEnabled: row.IsEnabled, SortOrder: row.SortOrder,
	}
}

type adminCapabilityDTO struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	NameZh        string `json:"name_zh"`
	NameEn        string `json:"name_en"`
	GroupKey      string `json:"group_key"`
	DescriptionZh string `json:"description_zh"`
	DescriptionEn string `json:"description_en"`
	IsEnabled     bool   `json:"is_enabled"`
	SortOrder     int32  `json:"sort_order"`
}

func toAdminCapabilityDTO(row repo.Capability) adminCapabilityDTO {
	return adminCapabilityDTO{
		ID: row.ID, Slug: row.Slug, NameZh: row.NameZh, NameEn: row.NameEn,
		GroupKey: row.GroupKey, DescriptionZh: row.DescriptionZh, DescriptionEn: row.DescriptionEn,
		IsEnabled: row.IsEnabled, SortOrder: row.SortOrder,
	}
}

type adminDictionaryListInput struct {
	Page     int32  `query:"page" default:"1" minimum:"1"`
	PageSize int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	Keyword  string `query:"keyword" maxLength:"100" doc:"按标识与中英文名称搜索"`
}

type adminPurposeListResponse struct {
	Body struct {
		Items    []adminPurposeDTO `json:"items"`
		Page     int32             `json:"page"`
		PageSize int32             `json:"page_size"`
		Total    int64             `json:"total"`
	}
}

func (s *Server) handleAdminListPurposes(ctx context.Context, in *adminDictionaryListInput) (*adminPurposeListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListPurposes(ctx, in.Keyword, page, size)
	if err != nil {
		return nil, err
	}
	out := &adminPurposeListResponse{}
	out.Body.Items = make([]adminPurposeDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminPurposeDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminPurposeInput struct {
	Slug          string `json:"slug" minLength:"1" maxLength:"50" doc:"标识，创建后不可修改"`
	NameZh        string `json:"name_zh" minLength:"1" maxLength:"50" doc:"中文名称"`
	NameEn        string `json:"name_en" minLength:"1" maxLength:"50" doc:"英文名称"`
	DescriptionZh string `json:"description_zh" maxLength:"500" doc:"中文定义"`
	DescriptionEn string `json:"description_en" maxLength:"500" doc:"英文定义"`
	IsHighRisk    bool   `json:"is_high_risk" doc:"是否高风险"`
	IsEnabled     bool   `json:"is_enabled" doc:"是否启用"`
	SortOrder     int32  `json:"sort_order" doc:"排序"`
}

type adminPurposeUpdateInput struct {
	ID   int64 `path:"id" doc:"考察目的 ID"`
	Body adminPurposeInput
}

type adminPurposeCreateInput struct {
	Body adminPurposeInput
}

type adminPurposeResponse struct {
	Body adminPurposeDTO
}

func (s *Server) handleAdminCreatePurpose(ctx context.Context, in *adminPurposeCreateInput) (*adminPurposeResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.CreatePurpose(ctx, admin.PurposeInput{
		Slug: in.Body.Slug, NameZh: in.Body.NameZh, NameEn: in.Body.NameEn,
		DescriptionZh: in.Body.DescriptionZh, DescriptionEn: in.Body.DescriptionEn,
		IsHighRisk: in.Body.IsHighRisk, IsEnabled: in.Body.IsEnabled, SortOrder: in.Body.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return &adminPurposeResponse{Body: toAdminPurposeDTO(row)}, nil
}

func (s *Server) handleAdminUpdatePurpose(ctx context.Context, in *adminPurposeUpdateInput) (*adminPurposeResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.UpdatePurpose(ctx, in.ID, admin.PurposeUpdate{
		NameZh: in.Body.NameZh, NameEn: in.Body.NameEn,
		DescriptionZh: in.Body.DescriptionZh, DescriptionEn: in.Body.DescriptionEn,
		IsHighRisk: in.Body.IsHighRisk, IsEnabled: in.Body.IsEnabled, SortOrder: in.Body.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return &adminPurposeResponse{Body: toAdminPurposeDTO(row)}, nil
}

type adminDictionaryIDInput struct {
	ID int64 `path:"id" doc:"字典条目 ID"`
}

func (s *Server) handleAdminDeletePurpose(ctx context.Context, in *adminDictionaryIDInput) (*struct{}, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.deps.Admin.DeletePurpose(ctx, in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type adminCapabilityListResponse struct {
	Body struct {
		Items    []adminCapabilityDTO `json:"items"`
		Page     int32                `json:"page"`
		PageSize int32                `json:"page_size"`
		Total    int64                `json:"total"`
	}
}

func (s *Server) handleAdminListCapabilities(ctx context.Context, in *adminDictionaryListInput) (*adminCapabilityListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListCapabilities(ctx, in.Keyword, page, size)
	if err != nil {
		return nil, err
	}
	out := &adminCapabilityListResponse{}
	out.Body.Items = make([]adminCapabilityDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminCapabilityDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminCapabilityInput struct {
	Slug          string `json:"slug" minLength:"1" maxLength:"50" doc:"标识，创建后不可修改"`
	NameZh        string `json:"name_zh" minLength:"1" maxLength:"50" doc:"中文名称"`
	NameEn        string `json:"name_en" minLength:"1" maxLength:"50" doc:"英文名称"`
	GroupKey      string `json:"group_key" enum:"cognitive,execution,reliability" doc:"分组"`
	DescriptionZh string `json:"description_zh" maxLength:"500" doc:"中文定义"`
	DescriptionEn string `json:"description_en" maxLength:"500" doc:"英文定义"`
	IsEnabled     bool   `json:"is_enabled" doc:"是否启用"`
	SortOrder     int32  `json:"sort_order" doc:"排序"`
}

type adminCapabilityCreateInput struct {
	Body adminCapabilityInput
}

type adminCapabilityUpdateInput struct {
	ID   int64 `path:"id" doc:"考察维度 ID"`
	Body adminCapabilityInput
}

type adminCapabilityResponse struct {
	Body adminCapabilityDTO
}

func (s *Server) handleAdminCreateCapability(ctx context.Context, in *adminCapabilityCreateInput) (*adminCapabilityResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.CreateCapability(ctx, admin.CapabilityInput{
		Slug: in.Body.Slug, NameZh: in.Body.NameZh, NameEn: in.Body.NameEn,
		GroupKey:      in.Body.GroupKey,
		DescriptionZh: in.Body.DescriptionZh, DescriptionEn: in.Body.DescriptionEn,
		IsEnabled: in.Body.IsEnabled, SortOrder: in.Body.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return &adminCapabilityResponse{Body: toAdminCapabilityDTO(row)}, nil
}

func (s *Server) handleAdminUpdateCapability(ctx context.Context, in *adminCapabilityUpdateInput) (*adminCapabilityResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.UpdateCapability(ctx, in.ID, admin.CapabilityUpdate{
		NameZh: in.Body.NameZh, NameEn: in.Body.NameEn, GroupKey: in.Body.GroupKey,
		DescriptionZh: in.Body.DescriptionZh, DescriptionEn: in.Body.DescriptionEn,
		IsEnabled: in.Body.IsEnabled, SortOrder: in.Body.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return &adminCapabilityResponse{Body: toAdminCapabilityDTO(row)}, nil
}

func (s *Server) handleAdminDeleteCapability(ctx context.Context, in *adminDictionaryIDInput) (*struct{}, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.deps.Admin.DeleteCapability(ctx, in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
