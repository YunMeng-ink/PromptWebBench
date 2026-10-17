package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/service/admin"
)

// 模块 5：标签管理与推荐标签（需求 §17.5 与第 6 章）。
func (s *Server) registerAdminTagRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/tags",
		OperationID: "adminListTags", Summary: "标签列表", Tags: []string{"后台"},
	}, s.handleAdminListTags)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/tags",
		OperationID: "adminCreateTag", Summary: "创建标签", Tags: []string{"后台"},
	}, s.handleAdminCreateTag)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/admin/tags/{id}",
		OperationID: "adminUpdateTag", Summary: "编辑标签", Tags: []string{"后台"},
	}, s.handleAdminUpdateTag)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/admin/tags/{id}",
		OperationID: "adminDeleteTag", Summary: "删除标签", Tags: []string{"后台"},
	}, s.handleAdminDeleteTag)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/recommend-tags",
		OperationID: "adminListRecommendTags", Summary: "推荐标签列表", Tags: []string{"后台"},
	}, s.handleAdminListRecommendTags)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/recommend-tags/review",
		OperationID: "adminReviewRecommendTags", Summary: "批量处置推荐标签", Tags: []string{"后台"},
	}, s.handleAdminReviewRecommendTags)
}

type adminListTagsInput struct {
	Page     int32  `query:"page" default:"1" minimum:"1"`
	PageSize int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	Keyword  string `query:"keyword" maxLength:"100" doc:"按标识与中英文名称搜索"`
}

func (s *Server) handleAdminListTags(ctx context.Context, in *adminListTagsInput) (*adminTagListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListTags(ctx, in.Keyword, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminTagListResponse{}
	out.Body.Items = make([]adminTagDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminTagDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

// adminTagInput 是标签的写入字段（需求 §17.5）。
type adminTagInput struct {
	Slug        string `json:"slug" minLength:"1" maxLength:"50" doc:"标签标识"`
	NameZh      string `json:"name_zh" minLength:"1" maxLength:"50" doc:"中文名称"`
	NameEn      string `json:"name_en" minLength:"1" maxLength:"50" doc:"英文名称"`
	Color       string `json:"color" maxLength:"9" doc:"十六进制颜色"`
	Description string `json:"description" maxLength:"200" doc:"标签说明"`
	Kind        string `json:"kind,omitempty" default:"theme" enum:"theme,form" doc:"主题标签类型"`
	Priority    int32  `json:"priority" minimum:"-1000" maximum:"1000" doc:"优先级，越大越靠前"`
	IsEnabled   bool   `json:"is_enabled" doc:"是否启用"`
}

func (in *adminTagInput) service() admin.TagInput {
	return admin.TagInput{
		Slug: in.Slug, NameZh: in.NameZh, NameEn: in.NameEn, Color: in.Color,
		Description: in.Description, Kind: in.Kind,
		Priority: in.Priority, IsEnabled: in.IsEnabled,
	}
}

type adminCreateTagInput struct {
	Body adminTagInput
}

func (s *Server) handleAdminCreateTag(ctx context.Context, in *adminCreateTagInput) (*adminTagResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.CreateTag(ctx, in.Body.service())
	if err != nil {
		return nil, err
	}
	return &adminTagResponse{Body: tagRowDTO(row)}, nil
}

type adminUpdateTagInput struct {
	ID   int64 `path:"id" doc:"标签 ID"`
	Body adminTagInput
}

func (s *Server) handleAdminUpdateTag(ctx context.Context, in *adminUpdateTagInput) (*adminTagResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	row, err := s.deps.Admin.UpdateTag(ctx, in.ID, in.Body.service())
	if err != nil {
		return nil, err
	}
	return &adminTagResponse{Body: tagRowDTO(row)}, nil
}

type adminTagIDInput struct {
	ID int64 `path:"id" doc:"标签 ID"`
}

func (s *Server) handleAdminDeleteTag(ctx context.Context, in *adminTagIDInput) (*struct{}, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.deps.Admin.DeleteTag(ctx, in.ID); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type adminListRecommendTagsInput struct {
	Page     int32    `query:"page" default:"1" minimum:"1"`
	PageSize int32    `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	States   []string `query:"states" doc:"按状态筛选：pending、confirmed、ignored、expired、review"`
	Kind     string   `query:"kind" doc:"按标签类型筛选"`
}

func (s *Server) handleAdminListRecommendTags(ctx context.Context, in *adminListRecommendTagsInput) (*adminRecommendTagListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListRecommendTags(ctx, in.States, in.Kind, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminRecommendTagListResponse{}
	out.Body.Items = make([]adminRecommendTagDTO, 0, len(result.Items))
	for _, row := range result.Items {
		item := adminRecommendTagDTO{
			ID: row.ID, PromptID: row.PromptID, PromptTitle: row.PromptTitle,
			ModelID: row.ModelID, ModelName: row.ModelName,
			Scope: row.Scope, Kind: row.Kind, State: row.State,
			LabelZh: row.LabelZh, LabelEn: row.LabelEn,
			Confidence: row.Confidence, Reason: row.Reason,
			TriggeredAt: timeString(row.TriggeredAt),
			ReviewedAt:  timePtrOf(row.ReviewedAt),
		}
		if len(row.Snapshot) > 0 {
			item.Snapshot = json.RawMessage(row.Snapshot)
		}
		out.Body.Items = append(out.Body.Items, item)
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminReviewRecommendTagsInput struct {
	Body struct {
		IDs []int64 `json:"ids" minItems:"1" maxItems:"100" doc:"要处置的推荐标签 ID"`
		// State 只能是 confirmed 或 ignored（C-13）。
		State   string `json:"state" enum:"confirmed,ignored" doc:"处置结果"`
		LabelZh string `json:"label_zh,omitempty" maxLength:"100" doc:"改写中文文案，留空保留原文案"`
		LabelEn string `json:"label_en,omitempty" maxLength:"100" doc:"改写英文文案，留空保留原文案"`
	}
}

func (s *Server) handleAdminReviewRecommendTags(ctx context.Context, in *adminReviewRecommendTagsInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	state, ok := domain.ParseRecommendTagState(in.Body.State)
	if !ok {
		return nil, NewError(apperr.CodeValidationFailed, "推荐标签状态取值非法")
	}
	err = s.deps.Admin.ReviewRecommendTags(ctx, operator, in.Body.IDs, admin.RecommendTagReview{
		State: state, LabelZh: in.Body.LabelZh, LabelEn: in.Body.LabelEn,
	}, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
