package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/service/showcase"
)

// 模型结果展示区（用户侧）：评分附图上传、模型输出文本提交、公开聚合与素材列表。
func (s *Server) registerShowcaseRoutes() {
	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/showcase/groups",
		OperationID: "listShowcaseGroups", Summary: "展示区模型分类", Tags: []string{"展示区"},
	}, s.handleShowcaseGroups)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/showcase/models/{model_id}/entries",
		OperationID: "listShowcaseEntries", Summary: "展示区模型素材", Tags: []string{"展示区"},
	}, s.handleShowcaseEntries)

	Register(s, "image_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/prompts/{id}/rating-images/{model_id}",
		OperationID: "uploadRatingImage", Summary: "上传模型结果图片", Tags: []string{"展示区"},
	}, s.handleUploadRatingImage)

	Register(s, "result_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/prompts/{id}/rating-outputs",
		OperationID: "submitRatingOutput", Summary: "提交模型输出文本", Tags: []string{"展示区"},
	}, s.handleSubmitRatingOutput)
}

type showcaseGroupDTO struct {
	ModelID           int64  `json:"model_id"`
	ModelName         string `json:"model_name"`
	ProviderKey       string `json:"provider_key"`
	ProviderName      string `json:"provider_name"`
	ImageCount        int64  `json:"image_count"`
	OutputCount       int64  `json:"output_count"`
	LastContributedAt string `json:"last_contributed_at"`
}

type showcaseGroupsResponse struct {
	Body struct {
		Items []showcaseGroupDTO `json:"items"`
	}
}

func (s *Server) handleShowcaseGroups(ctx context.Context, _ *struct{}) (*showcaseGroupsResponse, error) {
	groups, err := s.deps.Showcase.Groups(ctx)
	if err != nil {
		return nil, err
	}
	out := &showcaseGroupsResponse{}
	out.Body.Items = make([]showcaseGroupDTO, 0, len(groups))
	for _, group := range groups {
		out.Body.Items = append(out.Body.Items, showcaseGroupDTO{
			ModelID:           group.ModelID,
			ModelName:         group.ModelName,
			ProviderKey:       group.ProviderKey,
			ProviderName:      group.ProviderName,
			ImageCount:        group.ImageCount,
			OutputCount:       group.OutputCount,
			LastContributedAt: group.LastContributedAt,
		})
	}
	return out, nil
}

type showcaseEntriesInput struct {
	ModelID  int64 `path:"model_id" doc:"模型 ID"`
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"60" doc:"每页条数"`
}

type showcaseEntryDTO struct {
	Kind        string `json:"kind" enum:"image,output" doc:"素材类型"`
	ID          int64  `json:"id"`
	PromptID    int64  `json:"prompt_id"`
	PromptTitle string `json:"prompt_title"`
	Username    string `json:"username"`
	URL         string `json:"url,omitempty" doc:"图片外链；kind=image 时有值"`
	Content     string `json:"content,omitempty" doc:"模型输出文本；kind=output 时有值"`
	CreatedAt   string `json:"created_at"`
}

type showcaseEntriesResponse struct {
	Body struct {
		Items    []showcaseEntryDTO `json:"items"`
		Page     int32              `json:"page"`
		PageSize int32              `json:"page_size"`
		Total    int64              `json:"total"`
	}
}

func (s *Server) handleShowcaseEntries(ctx context.Context, in *showcaseEntriesInput) (*showcaseEntriesResponse, error) {
	page, size := pageValues(in.Page, in.PageSize)
	entries, total, err := s.deps.Showcase.ModelEntries(ctx, in.ModelID, page, size)
	if err != nil {
		return nil, err
	}
	out := &showcaseEntriesResponse{}
	out.Body.Items = make([]showcaseEntryDTO, 0, len(entries))
	for _, entry := range entries {
		out.Body.Items = append(out.Body.Items, showcaseEntryDTO{
			Kind:        entry.Kind,
			ID:          entry.ID,
			PromptID:    entry.PromptID,
			PromptTitle: entry.PromptTitle,
			Username:    entry.Username,
			URL:         entry.URL,
			Content:     entry.Content,
			CreatedAt:   entry.CreatedAt,
		})
	}
	out.Body.Page = page
	out.Body.PageSize = size
	out.Body.Total = total
	return out, nil
}

type uploadRatingImageInput struct {
	ID      int64 `path:"id" doc:"提示词 ID"`
	ModelID int64 `path:"model_id" doc:"模型 ID"`
	RawBody huma.MultipartFormFiles[struct {
		Image huma.FormFile `form:"image" required:"true" doc:"模型结果图片（jpg/png/webp/gif/bmp/tiff）"`
	}]
}

type ratingImageResponse struct {
	Body struct {
		Image struct {
			ID        int64  `json:"id"`
			URL       string `json:"url"`
			Filename  string `json:"filename"`
			CreatedAt string `json:"created_at"`
		} `json:"image"`
	}
}

func (s *Server) handleUploadRatingImage(ctx context.Context, in *uploadRatingImageInput) (*ratingImageResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	file := in.RawBody.Data().Image
	if !file.IsSet {
		return nil, apperr.New(apperr.CodeValidationFailed, "缺少图片文件")
	}
	if file.Size > showcase.MaxImageBytes {
		return nil, apperr.New(apperr.CodeValidationFailed, "图片不能超过 8 MB")
	}
	image, err := s.deps.Showcase.UploadImage(ctx, actor, showcase.UploadInput{
		PromptID:    in.ID,
		ModelID:     in.ModelID,
		File:        file.File,
		ContentType: file.ContentType,
		Filename:    file.Filename,
		Size:        file.Size,
	})
	if err != nil {
		return nil, err
	}
	out := &ratingImageResponse{}
	out.Body.Image.ID = image.ID
	out.Body.Image.URL = image.URL
	out.Body.Image.Filename = image.Filename
	out.Body.Image.CreatedAt = image.CreatedAt
	return out, nil
}

type submitRatingOutputInput struct {
	ID   int64 `path:"id" doc:"提示词 ID"`
	Body struct {
		ModelID int64  `json:"model_id" required:"true" minimum:"1" doc:"模型 ID"`
		Content string `json:"content" required:"true" maxLength:"8000" doc:"模型输出文本"`
	}
}

type ratingOutputResponse struct {
	Body struct {
		Output struct {
			ID        int64  `json:"id"`
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
		} `json:"output"`
	}
}

func (s *Server) handleSubmitRatingOutput(ctx context.Context, in *submitRatingOutputInput) (*ratingOutputResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	output, err := s.deps.Showcase.SubmitOutput(ctx, actor, showcase.OutputInput{
		PromptID: in.ID,
		ModelID:  in.Body.ModelID,
		Content:  in.Body.Content,
	})
	if err != nil {
		return nil, err
	}
	out := &ratingOutputResponse{}
	out.Body.Output.ID = output.ID
	out.Body.Output.Content = output.Content
	out.Body.Output.CreatedAt = output.CreatedAt
	return out, nil
}

// 管理员图片治理：列表与删除。违规判断由管理员完成，删除为软删除并留痕。
type adminShowcaseListInput struct {
	Page     int32 `query:"page" default:"1" minimum:"1"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"60"`
}

type adminShowcaseImageDTO struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	PromptID    int64  `json:"prompt_id"`
	PromptTitle string `json:"prompt_title"`
	ModelID     int64  `json:"model_id"`
	ModelName   string `json:"model_name"`
	Uploader    string `json:"uploader"`
	CreatedAt   string `json:"created_at"`
}

type adminShowcaseImagesResponse struct {
	Body struct {
		Items    []adminShowcaseImageDTO `json:"items"`
		Page     int32                   `json:"page"`
		PageSize int32                   `json:"page_size"`
		Total    int64                   `json:"total"`
	}
}

func (s *Server) handleAdminShowcaseImages(ctx context.Context, in *adminShowcaseListInput) (*adminShowcaseImagesResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	items, total, err := s.deps.Showcase.AdminImages(ctx, page, size)
	if err != nil {
		return nil, err
	}
	out := &adminShowcaseImagesResponse{}
	out.Body.Items = make([]adminShowcaseImageDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, adminShowcaseImageDTO{
			ID:          item.ID,
			URL:         item.URL,
			Filename:    item.Filename,
			PromptID:    item.PromptID,
			PromptTitle: item.PromptTitle,
			ModelID:     item.ModelID,
			ModelName:   item.ModelName,
			Uploader:    item.Uploader,
			CreatedAt:   item.CreatedAt,
		})
	}
	out.Body.Page = page
	out.Body.PageSize = size
	out.Body.Total = total
	return out, nil
}

type adminShowcaseOutputDTO struct {
	ID          int64  `json:"id"`
	Content     string `json:"content"`
	PromptID    int64  `json:"prompt_id"`
	PromptTitle string `json:"prompt_title"`
	ModelID     int64  `json:"model_id"`
	ModelName   string `json:"model_name"`
	Uploader    string `json:"uploader"`
	CreatedAt   string `json:"created_at"`
}

type adminShowcaseOutputsResponse struct {
	Body struct {
		Items    []adminShowcaseOutputDTO `json:"items"`
		Page     int32                    `json:"page"`
		PageSize int32                    `json:"page_size"`
		Total    int64                    `json:"total"`
	}
}

func (s *Server) handleAdminShowcaseOutputs(ctx context.Context, in *adminShowcaseListInput) (*adminShowcaseOutputsResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)
	items, total, err := s.deps.Showcase.AdminOutputs(ctx, page, size)
	if err != nil {
		return nil, err
	}
	out := &adminShowcaseOutputsResponse{}
	out.Body.Items = make([]adminShowcaseOutputDTO, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, adminShowcaseOutputDTO{
			ID:          item.ID,
			Content:     item.Content,
			PromptID:    item.PromptID,
			PromptTitle: item.PromptTitle,
			ModelID:     item.ModelID,
			ModelName:   item.ModelName,
			Uploader:    item.Uploader,
			CreatedAt:   item.CreatedAt,
		})
	}
	out.Body.Page = page
	out.Body.PageSize = size
	out.Body.Total = total
	return out, nil
}

type adminShowcaseItemInput struct {
	ID int64 `path:"id" doc:"素材 ID"`
}

func (s *Server) handleAdminDeleteShowcaseImage(ctx context.Context, in *adminShowcaseItemInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Showcase.AdminDeleteImage(ctx, operator, in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) handleAdminDeleteShowcaseOutput(ctx context.Context, in *adminShowcaseItemInput) (*struct{}, error) {
	operator, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Showcase.AdminDeleteOutput(ctx, operator, in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}
