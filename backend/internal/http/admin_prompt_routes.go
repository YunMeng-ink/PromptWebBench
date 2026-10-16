package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/service/admin"
	"github.com/promptbench/backend/internal/service/prompt"
)

// 模块 2：提示词管理（需求 §17.2）。
func (s *Server) registerAdminPromptRoutes() {
	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/prompts",
		OperationID: "adminListPrompts", Summary: "提示词列表", Tags: []string{"后台"},
	}, s.handleAdminListPrompts)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/prompts/{id}",
		OperationID: "adminGetPrompt", Summary: "提示词详情", Tags: []string{"后台"},
	}, s.handleAdminGetPrompt)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/admin/prompts/{id}",
		OperationID: "adminDeletePrompt", Summary: "删除提示词", Tags: []string{"后台"},
	}, s.handleAdminDeletePrompt)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/prompts/{id}/restore",
		OperationID: "adminRestorePrompt", Summary: "恢复提示词", Tags: []string{"后台"},
	}, s.handleAdminRestorePrompt)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/prompts/{id}/purge",
		OperationID: "adminPurgePrompt", Summary: "彻底删除提示词", Tags: []string{"后台"},
	}, s.handleAdminPurgePrompt)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/prompts/{id}/major-mark",
		OperationID: "adminMarkPromptMajor", Summary: "强制标记重大修改", Tags: []string{"后台"},
	}, s.handleAdminMarkPromptMajor)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/prompts/{id}/ratings",
		OperationID: "adminListPromptRatings", Summary: "提示词评分记录", Tags: []string{"后台"},
	}, s.handleAdminListPromptRatings)

	Register(s, "admin_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/admin/prompts/{id}/comments",
		OperationID: "adminListPromptComments", Summary: "提示词评论", Tags: []string{"后台"},
	}, s.handleAdminListPromptComments)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/admin/prompts/{id}/tags",
		OperationID: "adminAttachPromptTag", Summary: "给提示词添加标签", Tags: []string{"后台"},
	}, s.handleAdminAttachPromptTag)

	Register(s, "admin_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/admin/prompts/{id}/tags/{tagId}",
		OperationID: "adminDetachPromptTag", Summary: "移除提示词标签", Tags: []string{"后台"},
	}, s.handleAdminDetachPromptTag)
}

type adminListPromptsInput struct {
	Page       int32  `query:"page" default:"1" minimum:"1"`
	PageSize   int32  `query:"page_size" default:"20" minimum:"1" maximum:"100"`
	Keyword    string `query:"keyword" maxLength:"100" doc:"标题、描述与正文中的关键词"`
	Visibility string `query:"visibility" enum:",public,private,developer" doc:"按可见性筛选"`
	Deleted    string `query:"deleted" enum:",true,false" doc:"按删除状态筛选：true 仅回收站，false 仅在库"`
	CategoryID string `query:"category_id" doc:"按类别筛选"`
	UploaderID string `query:"uploader_id" doc:"按上传者筛选"`
}

func (in *adminListPromptsInput) filter() (admin.PromptFilter, error) {
	out := admin.PromptFilter{
		Keyword: in.Keyword, Visibility: in.Visibility, Deleted: in.Deleted,
	}
	parse := func(raw, label string) (*int64, error) {
		if raw == "" {
			return nil, nil
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, apperr.New(apperr.CodeValidationFailed, label+"应为整数")
		}
		return &value, nil
	}
	var err error
	if out.CategoryID, err = parse(in.CategoryID, "类别参数"); err != nil {
		return admin.PromptFilter{}, err
	}
	if out.UploaderID, err = parse(in.UploaderID, "上传者参数"); err != nil {
		return admin.PromptFilter{}, err
	}
	return out, nil
}

func (s *Server) handleAdminListPrompts(ctx context.Context, in *adminListPromptsInput) (*adminPromptListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	filter, err := in.filter()
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListPrompts(ctx, filter, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminPromptListResponse{}
	out.Body.Items = make([]adminPromptDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, toAdminPromptDTO(row))
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminPromptIDInput struct {
	ID int64 `path:"id" doc:"提示词 ID"`
}

func (s *Server) handleAdminGetPrompt(ctx context.Context, in *adminPromptIDInput) (*adminPromptDetailResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	detail, err := s.deps.Admin.GetPrompt(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	out := &adminPromptDetailResponse{}
	out.Body.Prompt = adminPromptDTO{
		ID:             detail.Prompt.ID,
		Title:          detail.Prompt.Title,
		Description:    detail.Prompt.Description,
		CategoryID:     detail.Prompt.CategoryID,
		CategoryNameZh: detail.Prompt.CategoryNameZh,
		CategoryNameEn: detail.Prompt.CategoryNameEn,
		AbilityID:      int64PtrOf(detail.Prompt.AbilityID),
		DifficultyID:   int64PtrOf(detail.Prompt.DifficultyID),
		Visibility:     detail.Prompt.Visibility,
		SensitiveLevel: detail.Prompt.SensitiveLevel,
		UploaderID:     detail.Prompt.UploaderID,
		VersionNo:      detail.Prompt.LatestVersionNo,
		MajorModified:  detail.Prompt.MajorModified,
		HighRisk:       detail.Prompt.IsHighRisk,
		CommentCount:   detail.Prompt.CommentCount,
		FavoriteCount:  detail.Prompt.FavoriteCount,
		Deleted:        detail.Prompt.DeletedAt.Valid,
		CreatedAt:      timeString(detail.Prompt.CreatedAt),
		UpdatedAt:      timeString(detail.Prompt.UpdatedAt),
	}
	out.Body.Body = detail.Prompt.Body
	out.Body.Tags = make([]tagDTO, 0, len(detail.Tags))
	for _, tag := range detail.Tags {
		out.Body.Tags = append(out.Body.Tags, tagDTO{
			ID: tag.ID, Slug: tag.Slug, NameZh: tag.NameZh, NameEn: tag.NameEn, Color: tag.Color,
		})
	}
	out.Body.Versions = make([]promptVersionDTO, 0, len(detail.Versions))
	for _, row := range detail.Versions {
		out.Body.Versions = append(out.Body.Versions, toPromptVersionDTO(prompt.VersionFromRow(row)))
	}
	return out, nil
}

func (s *Server) handleAdminDeletePrompt(ctx context.Context, in *adminPromptIDInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Prompt.Delete(ctx, operator, in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleAdminRestorePrompt(ctx context.Context, in *adminPromptIDInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.deps.Prompt.Restore(ctx, operator, in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleAdminPurgePrompt(ctx context.Context, in *adminPromptIDInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Prompt.Purge(ctx, operator, in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type adminMarkMajorInput struct {
	ID   int64 `path:"id" doc:"提示词 ID"`
	Body struct {
		Note string `json:"note" maxLength:"500" doc:"变更说明，写入新版本与操作日志"`
	}
}

type adminMarkMajorResponse struct {
	Body struct {
		VersionNo       string `json:"version_no"`
		ArchivedRatings int64  `json:"archived_ratings" doc:"被归档的历史评分条数"`
	}
}

func (s *Server) handleAdminMarkPromptMajor(ctx context.Context, in *adminMarkMajorInput) (*adminMarkMajorResponse, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.deps.Admin.ForceMajorMark(ctx, operator, in.ID, in.Body.Note, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	out := &adminMarkMajorResponse{}
	out.Body.VersionNo = result.VersionNo
	out.Body.ArchivedRatings = result.ArchivedRatings
	return out, nil
}

type adminListPromptRatingsInput struct {
	ID       int64 `path:"id"`
	Page     int32 `query:"page" default:"1" minimum:"1"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100"`
}

func (s *Server) handleAdminListPromptRatings(ctx context.Context, in *adminListPromptRatingsInput) (*adminRatingListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListPromptRatings(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminRatingListResponse{}
	out.Body.Items = make([]adminRatingDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminRatingDTO{
			ID: row.ID, UserID: row.UserID, UserName: row.UserName,
			ModelID: row.ModelID, ModelName: row.ModelName,
			Score: row.Score, EditCount: row.EditCount, Deleted: row.Deleted,
			CreatedAt: timeString(row.CreatedAt), UpdatedAt: timeString(row.UpdatedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

func (s *Server) handleAdminListPromptComments(ctx context.Context, in *adminListPromptRatingsInput) (*adminCommentListResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	result, err := s.deps.Admin.ListPromptComments(ctx, in.ID, page, size)
	if err != nil {
		return nil, err
	}

	out := &adminCommentListResponse{}
	out.Body.Items = make([]adminCommentDTO, 0, len(result.Items))
	for _, row := range result.Items {
		out.Body.Items = append(out.Body.Items, adminCommentDTO{
			ID: row.ID, UserID: row.UserID, AuthorName: row.AuthorName,
			ParentID: int64PtrOf(row.ParentID), Body: row.Body, LikeCount: row.LikeCount,
			Deleted: row.Deleted, CreatedAt: timeString(row.CreatedAt),
		})
	}
	out.Body.Page = result.Page
	out.Body.PageSize = result.PageSize
	out.Body.Total = result.Total
	return out, nil
}

type adminAttachTagInput struct {
	ID   int64 `path:"id" doc:"提示词 ID"`
	Body struct {
		TagID int64 `json:"tag_id" minimum:"1" doc:"标签 ID"`
	}
}

func (s *Server) handleAdminAttachPromptTag(ctx context.Context, in *adminAttachTagInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Admin.AttachPromptTag(ctx, operator, in.ID, in.Body.TagID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type adminDetachTagInput struct {
	ID    int64 `path:"id" doc:"提示词 ID"`
	TagID int64 `path:"tagId" doc:"标签 ID"`
}

func (s *Server) handleAdminDetachPromptTag(ctx context.Context, in *adminDetachTagInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Admin.DetachPromptTag(ctx, operator, in.ID, in.TagID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
