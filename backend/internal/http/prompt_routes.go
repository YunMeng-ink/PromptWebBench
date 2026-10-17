package api

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/service/prompt"
)

func (s *Server) registerPromptRoutes() {
	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/prompts",
		OperationID: "listPrompts", Summary: "提示词公开列表",
		Tags: []string{"提示词"},
	}, s.handleListPrompts)

	Register(s, "prompt_write", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/prompts",
		OperationID: "createPrompt", Summary: "上传提示词", Tags: []string{"提示词"},
	}, s.handleCreatePrompt)

	Register(s, "user_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/me/prompts",
		OperationID: "listMyPrompts", Summary: "我的提示词", Tags: []string{"个人中心"},
	}, s.handleListMyPrompts)

	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/prompts/{id}",
		OperationID: "getPrompt", Summary: "提示词详情", Tags: []string{"提示词"},
	}, s.handleGetPrompt)

	Register(s, "prompt_write", huma.Operation{
		Method: http.MethodPatch, Path: APIPrefix + "/prompts/{id}",
		OperationID: "updatePrompt", Summary: "修改提示词", Tags: []string{"提示词"},
	}, s.handleUpdatePrompt)

	Register(s, "prompt_write", huma.Operation{
		Method: http.MethodDelete, Path: APIPrefix + "/prompts/{id}",
		OperationID: "deletePrompt", Summary: "删除提示词", Tags: []string{"提示词"},
	}, s.handleDeletePrompt)

	Register(s, "prompt_import", huma.Operation{
		Method: http.MethodPost, Path: APIPrefix + "/prompts/import-markdown",
		OperationID: "importMarkdownPrompt", Summary: "导入 Markdown 文件",
		Tags: []string{"提示词"},
	}, s.handleImportMarkdown)
}

// listPromptsInput 是公开列表的 6 维筛选与排序参数（需求 §8.2）。
//
// Huma 不支持指针形态的 query 参数，可选项一律以字符串接收并在 filter 中解析，
// 空串表示该维度不参与筛选。
type listPromptsInput struct {
	Page         int32    `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize     int32    `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数，默认 20，上限 100"`
	PurposeID    string   `query:"purpose_id" doc:"按考察目的筛选"`
	CapabilityID string   `query:"capability_id" doc:"按考察维度筛选"`
	DifficultyID string   `query:"difficulty_id" doc:"按难度筛选"`
	Keyword      string   `query:"keyword" maxLength:"100" doc:"标题、正文与描述中的关键词"`
	ModelID      string   `query:"model_id" doc:"按适用模型筛选"`
	TagIDs       []string `query:"tag_ids" doc:"按标签筛选，命中任一标签"`
	Sort         string   `query:"sort" default:"latest" enum:"latest,top_rated,discrimination" doc:"排序方式"`
}

// filter 把请求参数翻译为数据层筛选条件。
func (in *listPromptsInput) filter() (repo.PromptListFilter, error) {
	out := repo.PromptListFilter{Keyword: in.Keyword}

	id := func(raw string) (*int64, error) {
		if raw == "" {
			return nil, nil
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, apperr.New(apperr.CodeValidationFailed, "筛选参数应为整数")
		}
		return &parsed, nil
	}

	var err error
	if out.PurposeID, err = id(in.PurposeID); err != nil {
		return repo.PromptListFilter{}, err
	}
	if out.CapabilityID, err = id(in.CapabilityID); err != nil {
		return repo.PromptListFilter{}, err
	}
	if out.DifficultyID, err = id(in.DifficultyID); err != nil {
		return repo.PromptListFilter{}, err
	}
	if out.ModelID, err = id(in.ModelID); err != nil {
		return repo.PromptListFilter{}, err
	}
	for _, raw := range in.TagIDs {
		if raw == "" {
			continue
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return repo.PromptListFilter{}, apperr.New(apperr.CodeValidationFailed, "标签参数应为整数")
		}
		out.TagIDs = append(out.TagIDs, parsed)
	}
	return out, nil
}

func (s *Server) handleListPrompts(ctx context.Context, in *listPromptsInput) (*promptListResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	sort, ok := domain.ParseSortOrder(in.Sort)
	if !ok {
		return nil, apperr.New(apperr.CodeValidationFailed, "排序方式取值非法")
	}
	filter, err := in.filter()
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)

	list, err := s.deps.Prompt.List(ctx, v, filter, sort, page, size)
	if err != nil {
		return nil, err
	}
	return buildPromptList(list), nil
}

type listMyPromptsInput struct {
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数，默认 20，上限 100"`
}

func (s *Server) handleListMyPrompts(ctx context.Context, in *listMyPromptsInput) (*promptListResponse, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageValues(in.Page, in.PageSize)

	list, err := s.deps.Prompt.ListOwn(ctx, domain.ViewerFor(user), page, size)
	if err != nil {
		return nil, err
	}
	return buildPromptList(list), nil
}

func buildPromptList(list repo.PromptList) *promptListResponse {
	out := &promptListResponse{}
	out.Body.Items = make([]promptListItemDTO, 0, len(list.Items))
	for _, item := range list.Items {
		out.Body.Items = append(out.Body.Items, promptListItemDTO{
			promptDTO:        toPromptDTO(item.Prompt),
			PurposeNameZh:    item.PurposeNameZh,
			PurposeNameEn:    item.PurposeNameEn,
			CapabilityNameZh: item.CapabilityNameZh,
			CapabilityNameEn: item.CapabilityNameEn,
			DifficultyNameZh: item.DifficultyNameZh,
			DifficultyNameEn: item.DifficultyNameEn,
			Discrimination:   item.Discrimination,
			DataSufficient:   item.DataSufficient,
			AverageAdjusted:  item.AvgAdjusted,
		})
	}
	out.Body.Page = list.Page
	out.Body.PageSize = list.PageSize
	out.Body.Total = list.Total
	return out
}

// promptBody 是创建与修改共用的请求体。
//
// 带 default 标签的可选字段一律配 omitempty：Huma 按「能否在 JSON 中省略」判断必填，
// 缺少 omitempty 时即便有 default 也被判为必填，调用方必须显式传值。
type promptBody struct {
	Title          string  `json:"title" required:"true" minLength:"1" maxLength:"200" doc:"标题"`
	Body           string  `json:"body" required:"true" minLength:"1" maxLength:"200000" doc:"提示词正文"`
	Description    string  `json:"description,omitempty" maxLength:"2000" doc:"描述说明"`
	PurposeID      int64   `json:"purpose_id" required:"true" minimum:"1" doc:"考察目的"`
	CapabilityID   int64   `json:"capability_id" required:"true" minimum:"1" doc:"考察维度"`
	DifficultyID   *int64  `json:"difficulty_id,omitempty" minimum:"1" doc:"难度"`
	Visibility     string  `json:"visibility,omitempty" default:"public" enum:"public,private,developer" doc:"可见性，默认公开"`
	SensitiveLevel string  `json:"sensitive_level,omitempty" default:"low" enum:"low,medium,high" doc:"敏感级别，默认 low"`
	TagIDs         []int64 `json:"tag_ids,omitempty" doc:"标签集合"`
	ChangeNote     string  `json:"change_note,omitempty" maxLength:"500" doc:"版本变更说明"`
	IsMajorChange  bool    `json:"is_major_change,omitempty" doc:"是否标记为重大修改"`
}

// toInput 把请求体翻译为服务层输入。可见性与敏感级别的枚举取值已由 Huma 的
// enum 标签校验；省略时按 default 标签的取值补齐，使服务层始终拿到完整输入。
func (b promptBody) toInput() (prompt.Input, error) {
	visibility := b.Visibility
	if visibility == "" {
		visibility = string(domain.VisibilityPublic)
	}
	parsedVisibility, ok := domain.ParseVisibility(visibility)
	if !ok {
		return prompt.Input{}, apperr.New(apperr.CodeValidationFailed, "可见性取值非法")
	}

	sensitive := b.SensitiveLevel
	if sensitive == "" {
		sensitive = string(domain.SensitiveLow)
	}
	parsedSensitive, ok := domain.ParseSensitiveLevel(sensitive)
	if !ok {
		return prompt.Input{}, apperr.New(apperr.CodeValidationFailed, "敏感级别取值非法")
	}

	return prompt.Input{
		Title:         b.Title,
		Body:          b.Body,
		Description:   b.Description,
		PurposeID:     b.PurposeID,
		CapabilityID:  b.CapabilityID,
		DifficultyID:  b.DifficultyID,
		Visibility:    parsedVisibility,
		Sensitive:     parsedSensitive,
		TagIDs:        b.TagIDs,
		ChangeNote:    b.ChangeNote,
		IsMajorChange: b.IsMajorChange,
	}, nil
}

type createPromptInput struct {
	Body promptBody
}

type promptWriteResponse struct {
	Body struct {
		Prompt    promptDTO `json:"prompt"`
		VersionNo string    `json:"version_no"`
		// MajorChange 为真时前端展示重大修改说明与归档条数（需求 §7.3）。
		MajorChange     bool   `json:"major_change"`
		ArchivedRatings int64  `json:"archived_ratings"`
		Notice          string `json:"notice,omitempty"`
	}
}

func (s *Server) handleCreatePrompt(ctx context.Context, in *createPromptInput) (*promptWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	body, err := in.Body.toInput()
	if err != nil {
		return nil, err
	}
	result, err := s.deps.Prompt.Create(ctx, actor, body)
	if err != nil {
		return nil, err
	}
	return buildPromptWrite(result), nil
}

type updatePromptInput struct {
	ID   int64 `path:"id" doc:"提示词 ID"`
	Body promptBody
}

func (s *Server) handleUpdatePrompt(ctx context.Context, in *updatePromptInput) (*promptWriteResponse, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	body, err := in.Body.toInput()
	if err != nil {
		return nil, err
	}
	result, err := s.deps.Prompt.Update(ctx, actor, in.ID, body)
	if err != nil {
		return nil, err
	}
	return buildPromptWrite(result), nil
}

func buildPromptWrite(result prompt.Result) *promptWriteResponse {
	out := &promptWriteResponse{}
	out.Body.Prompt = toPromptDTO(result.Prompt)
	out.Body.VersionNo = result.VersionNo
	out.Body.MajorChange = result.MajorChange
	out.Body.ArchivedRatings = result.ArchivedRatings
	if result.MajorChange {
		out.Body.Notice = majorModificationNotice
	}
	return out
}

type getPromptInput struct {
	ID int64 `path:"id" doc:"提示词 ID"`
}

func (s *Server) handleGetPrompt(ctx context.Context, in *getPromptInput) (*promptDetailResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	detail, err := s.deps.Prompt.Get(ctx, v, in.ID, clientIP(ctx))
	if err != nil {
		return nil, err
	}
	out := &promptDetailResponse{}
	s.fillDetail(out, detail)
	return out, nil
}

type deletePromptInput struct {
	ID int64 `path:"id" doc:"提示词 ID"`
}

func (s *Server) handleDeletePrompt(ctx context.Context, in *deletePromptInput) (*struct{}, error) {
	actor, err := requireVerified(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Prompt.Delete(ctx, actor, in.ID, clientIP(ctx)); err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

type importMarkdownInput struct {
	RawBody huma.MultipartFormFiles[struct {
		File huma.FormFile `form:"file" required:"true" doc:"Markdown 文件"`
	}]
}

type importMarkdownResponse struct {
	Body struct {
		Title       string `json:"title" doc:"解析出的标题，导入后仍可修改"`
		Body        string `json:"body" doc:"提示词正文"`
		Description string `json:"description" doc:"描述说明"`
	}
}

func (s *Server) handleImportMarkdown(ctx context.Context, in *importMarkdownInput) (*importMarkdownResponse, error) {
	if _, err := requireVerified(ctx); err != nil {
		return nil, err
	}
	file := in.RawBody.Data().File
	if !file.IsSet {
		return nil, apperr.New(apperr.CodeValidationFailed, "缺少文件")
	}
	if !hasMarkdownSuffix(file.Filename) {
		return nil, apperr.New(apperr.CodeValidationFailed, "仅支持 .md 文件")
	}
	if file.Size > prompt.ImportLimit {
		return nil, apperr.New(apperr.CodeValidationFailed, "文件超过大小上限")
	}

	content := make([]byte, file.Size)
	if _, err := io.ReadFull(file.File, content); err != nil {
		return nil, err
	}

	parsed, err := prompt.ParseMarkdown(string(content))
	if err != nil {
		return nil, err
	}

	out := &importMarkdownResponse{}
	out.Body.Title = parsed.Title
	out.Body.Body = parsed.Body
	out.Body.Description = parsed.Description
	return out, nil
}

func hasMarkdownSuffix(name string) bool {
	lower := lowerASCII(name)
	return len(lower) >= 3 && lower[len(lower)-3:] == ".md"
}

func lowerASCII(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + ('a' - 'A')
		}
	}
	return string(out)
}
