package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/service/admin"
)

// 模块 3：模型管理（需求 §17.3、§13）。导入走独立的限流规则 admin_import：
// 它每次都外呼 models.dev，限额必须低于普通后台写操作。
func (s *Server) registerAdminModelRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/models",
		OperationID: "adminListModels", Summary: "模型列表", Tags: []string{"后台"},
	}, s.handleAdminListModels)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/models",
		OperationID: "adminCreateModel", Summary: "添加模型", Tags: []string{"后台"},
	}, s.handleAdminCreateModel)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/models/import-logs",
		OperationID: "adminListModelImportLogs", Summary: "模型导入记录", Tags: []string{"后台"},
	}, s.handleAdminListModelImportLogs)

	Register(s, "admin_import", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/models/import/preview",
		OperationID: "adminPreviewModelImport", Summary: "预览 models.dev 导入", Tags: []string{"后台"},
	}, s.handleAdminPreviewModelImport)

	Register(s, "admin_import", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/models/import",
		OperationID: "adminImportModels", Summary: "从 models.dev 导入模型", Tags: []string{"后台"},
	}, s.handleAdminImportModels)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/models/{id}",
		OperationID: "adminGetModel", Summary: "模型详情", Tags: []string{"后台"},
	}, s.handleAdminGetModel)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/admin/models/{id}",
		OperationID: "adminUpdateModel", Summary: "编辑模型", Tags: []string{"后台"},
	}, s.handleAdminUpdateModel)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/admin/models/{id}",
		OperationID: "adminDeleteModel", Summary: "删除模型", Tags: []string{"后台"},
	}, s.handleAdminDeleteModel)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/models/{id}/enabled",
		OperationID: "adminSetModelEnabled", Summary: "启用或禁用模型", Tags: []string{"后台"},
	}, s.handleAdminSetModelEnabled)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/models/{id}/prompts",
		OperationID: "adminListModelPrompts", Summary: "模型关联提示词", Tags: []string{"后台"},
	}, s.handleAdminListModelPrompts)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/models/{id}/stats",
		OperationID: "adminListModelStats", Summary: "模型完成度统计", Tags: []string{"后台"},
	}, s.handleAdminListModelStats)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/models/{id}/snapshots",
		OperationID: "adminListModelSnapshots", Summary: "模型原始字段快照", Tags: []string{"后台"},
	}, s.handleAdminListModelSnapshots)
}

type adminListModelsInput struct {
	Page           int32  `query:"page" default:"1" minimum:"1"`
	PageSize       int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	Keyword        string `query:"keyword" maxLength:"100" doc:"按名称、标识与厂商搜索"`
	IncludeDeleted bool   `query:"include_deleted" doc:"是否包含回收站中的模型"`
}

func (s *Server) handleAdminListModels(ctx context.Context, in *adminListModelsInput) (*adminModelListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListModels(ctx, in.Keyword, in.IncludeDeleted, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminModelListResponse{}
	out.Body.Items = make([]adminModelDTO, 0, len(result.Items))
	modelIDs := make([]int64, 0, len(result.Items))
	for _, row := range result.Items {
		modelIDs = append(modelIDs, row.ID)
	}
	capabilities, err := s.deps.Admin.ModelCapabilityIDsByModel(ctx, modelIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminModelDTO(row, capabilities[row.ID]))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

// adminModelInput 是模型的写入字段（需求 §13.2 的 14 个字段）。
type adminModelInput struct {
	Name            string   `json:"name" minLength:"1" maxLength:"200" doc:"模型名称"`
	ModelKey        string   `json:"model_key" minLength:"1" maxLength:"200" doc:"模型标识"`
	ProviderKey     string   `json:"provider_key" maxLength:"100" doc:"模型厂商标识"`
	ProviderName    string   `json:"provider_name" maxLength:"200" doc:"模型厂商名称"`
	InputModalities []string `json:"input_modalities" doc:"输入模态"`
	Aliases         []string `json:"aliases" doc:"模型别名"`
	Description     string   `json:"description" maxLength:"2000" doc:"模型简介"`
	ReleaseDate     string   `json:"release_date" doc:"发布时间，YYYY-MM-DD；留空表示未知"`
	ContextLength   *int64   `json:"context_length,omitempty" minimum:"0" doc:"上下文长度"`
	IsEnabled       bool     `json:"is_enabled" doc:"是否启用"`
	CapabilityIDs   []int64  `json:"capability_ids" doc:"考察维度"`
	Note            string   `json:"note" maxLength:"500" doc:"备注"`
}

// service 把请求体转为服务层输入。日期的形态在此处校验：
// Huma 不解析自定义日期串，非法取值直接报错而不是静默丢弃。
func (in *adminModelInput) service() (admin.ModelInput, error) {
	out := admin.ModelInput{
		Name:            in.Name,
		ModelKey:        in.ModelKey,
		ProviderKey:     in.ProviderKey,
		ProviderName:    in.ProviderName,
		InputModalities: in.InputModalities,
		Aliases:         in.Aliases,
		Description:     in.Description,
		ContextLength:   in.ContextLength,
		IsEnabled:       in.IsEnabled,
		CapabilityIDs:   in.CapabilityIDs,
		Note:            in.Note,
	}
	if in.ReleaseDate != "" {
		parsed, err := time.Parse("2006-01-02", in.ReleaseDate)
		if err != nil {
			return admin.ModelInput{}, apperr.New(apperr.CodeValidationFailed, "发布时间需要 YYYY-MM-DD 形态")
		}
		out.ReleaseDate = &parsed
	}
	return out, nil
}

type adminCreateModelInput struct {
	Body adminModelInput
}

func (s *Server) adminModelResponse(ctx context.Context, row repo.Model) (*adminModelResponse, error) {
	capabilityIDs, err := s.deps.Admin.ModelCapabilityIDs(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	return &adminModelResponse{Body: modelModelDTO(row, capabilityIDs)}, nil
}

func (s *Server) handleAdminCreateModel(ctx context.Context, in *adminCreateModelInput) (*adminModelResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	input, err := in.Body.service()
	if err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.CreateModel(ctx, operator, input, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return s.adminModelResponse(ctx, row)
}

type adminModelIDInput struct {
	ID int64 `path:"id" doc:"模型 ID"`
}

func (s *Server) handleAdminGetModel(ctx context.Context, in *adminModelIDInput) (*adminModelResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.GetModel(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return s.adminModelResponse(ctx, row)
}

type adminUpdateModelInput struct {
	ID   int64 `path:"id" doc:"模型 ID"`
	Body adminModelInput
}

func (s *Server) handleAdminUpdateModel(ctx context.Context, in *adminUpdateModelInput) (*adminModelResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	input, err := in.Body.service()
	if err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.UpdateModel(ctx, operator, in.ID, input, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return s.adminModelResponse(ctx, row)
}

func (s *Server) handleAdminDeleteModel(ctx context.Context, in *adminModelIDInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Admin.DeleteModel(ctx, operator, in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type adminSetModelEnabledInput struct {
	ID   int64 `path:"id" doc:"模型 ID"`
	Body struct {
		Enabled bool `json:"enabled" doc:"是否启用"`
	}
}

func (s *Server) handleAdminSetModelEnabled(ctx context.Context, in *adminSetModelEnabledInput) (*adminModelResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.SetModelEnabled(ctx, operator, in.ID, in.Body.Enabled, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return s.adminModelResponse(ctx, row)
}

type adminModelPagedInput struct {
	ID       int64 `path:"id" doc:"模型 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100"`
}

func (s *Server) handleAdminListModelPrompts(ctx context.Context, in *adminModelPagedInput) (*adminModelPromptListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListModelPrompts(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminModelPromptListResponse{}
	out.Body.Items = make([]adminModelPromptDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminModelPromptDTO{
			ID: row.ID, Title: row.Title, Visibility: row.Visibility,
			CreatedAt: timeString(row.CreatedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

func (s *Server) handleAdminListModelStats(ctx context.Context, in *adminModelPagedInput) (*adminModelStatListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListModelStats(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminModelStatListResponse{}
	out.Body.Items = make([]adminModelStatDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminModelStatDTO{
			PromptID:           row.PromptID,
			PromptTitle:        row.Title,
			HistoryRatingCount: row.HistoryRatingCount,
			HistoryAdjusted:    row.HistoryAdjusted,
			RecentRatingCount:  row.RecentRatingCount,
			RecentAdjusted:     row.RecentAdjusted,
			ComputedAt:         timeString(row.ComputedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

func (s *Server) handleAdminListModelSnapshots(ctx context.Context, in *adminModelPagedInput) (*adminModelSnapshotListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListModelSnapshots(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminModelSnapshotListResponse{}
	out.Body.Items = make([]adminModelSnapshotDTO, 0, len(result.Items))
	for _, row := range result.Items {
		item := adminModelSnapshotDTO{
			ID: row.ID, ModelID: row.ModelID, Source: row.Source,
			CreatedAt: timeString(row.CreatedAt),
		}
		if len(row.Raw) > 0 {
			item.Raw = json.RawMessage(row.Raw)
		}
		out.Body.Items = append(out.Body.Items, item)
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminListImportLogsInput struct {
	Page     int32  `query:"page" default:"1" minimum:"1"`
	PageSize int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	Action   string `query:"action" enum:",import,overwrite,skip" doc:"按导入动作筛选"`
}

func (s *Server) handleAdminListModelImportLogs(ctx context.Context, in *adminListImportLogsInput) (*adminModelImportLogListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListModelImportLogs(ctx, in.Action, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminModelImportLogListResponse{}
	out.Body.Items = make([]adminModelImportLogDTO, 0, len(result.Items))
	for _, row := range result.Items {
		item := adminModelImportLogDTO{
			ID: row.ID, ModelID: int64PtrOf(row.ModelID), Action: row.Action,
			ProviderKey: row.ProviderKey, ModelKey: row.ModelKey,
			OperatorName: row.OperatorName, CreatedAt: timeString(row.CreatedAt),
		}
		if len(row.Payload) > 0 {
			item.Payload = json.RawMessage(row.Payload)
		}
		out.Body.Items = append(out.Body.Items, item)
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminModelImportPreviewInput struct {
	Body *struct {
		Page       int32  `json:"page,omitempty" default:"1" minimum:"1" doc:"页码"`
		PageSize   int32  `json:"page_size,omitempty" default:"50" minimum:"1" maximum:"100" doc:"每页条目数"`
		Keyword    string `json:"keyword,omitempty" maxLength:"100" doc:"按名称、标识与厂商搜索"`
		ProviderID string `json:"provider_id,omitempty" maxLength:"100" doc:"按提供商标识筛选"`
		Status     string `json:"status,omitempty" doc:"筛选状态：new 或 duplicate；留空为全部"`
		Refresh    bool   `json:"refresh,omitempty" doc:"是否重新读取 models.dev 目录"`
	}
}

func (s *Server) handleAdminPreviewModelImport(
	ctx context.Context, in *adminModelImportPreviewInput,
) (*adminModelImportPreviewResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	params := admin.ModelImportPreviewParams{Page: 1, PageSize: 50}
	if in != nil && in.Body != nil {
		params.Page = in.Body.Page
		params.PageSize = in.Body.PageSize
		params.Refresh = in.Body.Refresh
		params.Filter = admin.ModelImportFilter{
			Keyword: in.Body.Keyword, ProviderID: in.Body.ProviderID, Status: in.Body.Status,
		}
	}
	preview, err := s.deps.Admin.PreviewModelImport(ctx, params)
	if err != nil {
		return nil, err
	}

	out := &adminModelImportPreviewResponse{}
	out.Body.Items = make([]adminModelImportEntryDTO, 0, len(preview.Items))
	for _, entry := range preview.Items {
		item := adminModelImportEntryDTO{
			ProviderID: entry.ProviderID, ProviderName: entry.ProviderName,
			ModelID: entry.ModelID, Name: entry.Name, Description: entry.Description,
			ReleaseDate: entry.ReleaseDate, InputModalities: emptyIfNil(entry.InputModalities),
			ContextLength: entry.ContextLength,
		}
		if entry.Duplicate != nil {
			item.Duplicate = &adminDuplicateModelDTO{
				ID: entry.Duplicate.ID, Name: entry.Duplicate.Name,
				ProviderName: entry.Duplicate.ProviderName,
			}
		}
		out.Body.Items = append(out.Body.Items, item)
	}
	out.Body.Page = preview.Page
	out.Body.PageSize = preview.PageSize
	out.Body.Total = preview.Total
	out.Body.CatalogTotal = preview.CatalogTotal
	out.Body.DuplicateCount = preview.DuplicateCount
	out.Body.FetchedAt = formatTime(preview.FetchedAt)
	out.Body.Providers = make([]adminModelImportProviderDTO, 0, len(preview.Providers))
	for _, provider := range preview.Providers {
		out.Body.Providers = append(out.Body.Providers, adminModelImportProviderDTO{
			ID: provider.ID, Name: provider.Name,
			Total: provider.Total, Duplicates: provider.Duplicates,
		})
	}
	return out, nil
}

type adminImportKeyDTO struct {
	ProviderID string `json:"provider_id" maxLength:"100"`
	ModelID    string `json:"model_id" maxLength:"200"`
}

type adminImportFilterDTO struct {
	Keyword    string `json:"keyword,omitempty" maxLength:"100"`
	ProviderID string `json:"provider_id,omitempty" maxLength:"100"`
	Status     string `json:"status,omitempty" doc:"筛选状态：new 或 duplicate；留空为全部"`
}

type adminImportSelectionDTO struct {
	Mode    string                `json:"mode,omitempty" doc:"keys 表示按条目精确导入，filter 表示按筛选条件导入"`
	Keys    []adminImportKeyDTO   `json:"keys,omitempty" maxItems:"20000"`
	Filter  *adminImportFilterDTO `json:"filter,omitempty"`
	Exclude []adminImportKeyDTO   `json:"exclude,omitempty" maxItems:"20000"`
}

type adminImportModelsInput struct {
	Body struct {
		Overwrite bool                     `json:"overwrite" doc:"是否覆盖同名模型；为假时跳过（需求 §13.4）"`
		Selection *adminImportSelectionDTO `json:"selection,omitempty" doc:"导入范围；省略时导入目录全部条目"`
	}
}

func (s *Server) handleAdminImportModels(ctx context.Context, in *adminImportModelsInput) (*adminModelImportResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	opts := admin.ModelImportOptions{Overwrite: in.Body.Overwrite}
	if selection := in.Body.Selection; selection != nil {
		opts.Selection = admin.ModelImportSelection{
			Mode:    selection.Mode,
			Keys:    toImportKeys(selection.Keys),
			Exclude: toImportKeys(selection.Exclude),
		}
		if selection.Filter != nil {
			opts.Selection.Filter = admin.ModelImportFilter{
				Keyword:    selection.Filter.Keyword,
				ProviderID: selection.Filter.ProviderID,
				Status:     selection.Filter.Status,
			}
		}
	}
	result, err := s.deps.Admin.ImportModels(ctx, operator, opts, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	out := &adminModelImportResponse{}
	out.Body.Total = result.Total
	out.Body.Created = result.Created
	out.Body.Overwritten = result.Overwritten
	out.Body.Skipped = result.Skipped
	return out, nil
}

func toImportKeys(items []adminImportKeyDTO) []admin.ModelImportKey {
	out := make([]admin.ModelImportKey, 0, len(items))
	for _, item := range items {
		out = append(out, admin.ModelImportKey{ProviderID: item.ProviderID, ModelID: item.ModelID})
	}
	return out
}
