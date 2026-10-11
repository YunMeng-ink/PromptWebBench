package api

import (
	"context"
	"errors"
	"time"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/service/prompt"
)

// majorModificationNotice 是重大修改后的前台提示文案（需求 §7.3）。
const majorModificationNotice = "该提示词已发生重大修改，当前评分基于最新版本。"

// 分页参数的契约（C-40）：page 从 1 起，page_size 默认 20、上限 100。
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// pageInput 是全部列表端点共用的分页参数。
//
// 这些字段直接内联到各自的输入结构体，不通过嵌入复用：Huma 只对结构体自身的字段
// 应用 default 标签，嵌入结构体内的 query 参数既不生效，也不参与 OpenAPI 生成。
type pageInput struct {
	Page     int32 `query:"page" default:"1" minimum:"1" doc:"页码，从 1 起"`
	PageSize int32 `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页条数，默认 20，上限 100"`
}

// pageValues 返回夹紧后的分页取值。请求层已做形态校验，此处的夹紧是纵深防御。
func pageValues(page, size int32) (int32, int32) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return page, size
}

type promptDTO struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	CategoryID     int64  `json:"category_id"`
	AbilityID      *int64 `json:"ability_id,omitempty"`
	DifficultyID   *int64 `json:"difficulty_id,omitempty"`
	Visibility     string `json:"visibility"`
	SensitiveLevel string `json:"sensitive_level"`
	UploaderID     int64  `json:"uploader_id"`
	VersionNo      string `json:"version_no"`
	MajorModified  bool   `json:"major_modified"`
	CommentCount   int32  `json:"comment_count"`
	FavoriteCount  int32  `json:"favorite_count"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type promptListItemDTO struct {
	promptDTO
	CategoryNameZh   string  `json:"category_name_zh"`
	CategoryNameEn   string  `json:"category_name_en"`
	AbilityNameZh    string  `json:"ability_name_zh,omitempty"`
	AbilityNameEn    string  `json:"ability_name_en,omitempty"`
	DifficultyNameZh string  `json:"difficulty_name_zh,omitempty"`
	DifficultyNameEn string  `json:"difficulty_name_en,omitempty"`
	Discrimination   float64 `json:"discrimination" doc:"区分度；阶段 3 之前为 0"`
	DataSufficient   bool    `json:"data_sufficient" doc:"数据是否充足；阶段 3 之前为 false"`
	AverageAdjusted  float64 `json:"average_adjusted" doc:"校准完成度均值；阶段 3 之前为 0"`
}

// promptListResponse 是分页响应的形状（C-40）。Items 为空时序列化为 [] 而非 null。
type promptListResponse struct {
	Body struct {
		Items    []promptListItemDTO `json:"items"`
		Page     int32               `json:"page"`
		PageSize int32               `json:"page_size"`
		Total    int64               `json:"total"`
	}
}

func toPromptDTO(p domain.Prompt) promptDTO {
	return promptDTO{
		ID:             p.ID,
		Title:          p.Title,
		Description:    p.Description,
		CategoryID:     p.CategoryID,
		AbilityID:      p.AbilityID,
		DifficultyID:   p.DifficultyID,
		Visibility:     string(p.Visibility),
		SensitiveLevel: string(p.Sensitive),
		UploaderID:     p.UploaderID,
		VersionNo:      p.LatestVersion.String(),
		MajorModified:  p.MajorModified,
		CommentCount:   p.CommentCount,
		FavoriteCount:  p.FavoriteCount,
		CreatedAt:      formatTime(p.CreatedAt),
		UpdatedAt:      formatTime(p.UpdatedAt),
	}
}

// formatTime 以 RFC 3339 输出 UTC 时刻，全系统时间字段统一为该形态。
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

type promptVersionDTO struct {
	ID             int64  `json:"id"`
	VersionNo      string `json:"version_no"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	Description    string `json:"description"`
	CategoryID     int64  `json:"category_id"`
	AbilityID      *int64 `json:"ability_id,omitempty"`
	DifficultyID   *int64 `json:"difficulty_id,omitempty"`
	Visibility     string `json:"visibility"`
	SensitiveLevel string `json:"sensitive_level"`
	IsMajor        bool   `json:"is_major"`
	ChangeNote     string `json:"change_note"`
	AuthorID       int64  `json:"author_id"`
	CreatedAt      string `json:"created_at"`
}

func toPromptVersionDTO(v domain.PromptVersion) promptVersionDTO {
	return promptVersionDTO{
		ID:             v.ID,
		VersionNo:      v.Version.String(),
		Title:          v.Title,
		Body:           v.Body,
		Description:    v.Description,
		CategoryID:     v.CategoryID,
		AbilityID:      v.AbilityID,
		DifficultyID:   v.DifficultyID,
		Visibility:     string(v.Visibility),
		SensitiveLevel: string(v.Sensitive),
		IsMajor:        v.IsMajor,
		ChangeNote:     v.ChangeNote,
		AuthorID:       v.AuthorID,
		CreatedAt:      formatTime(v.CreatedAt),
	}
}

type tagDTO struct {
	ID     int64  `json:"id"`
	Slug   string `json:"slug"`
	NameZh string `json:"name_zh"`
	NameEn string `json:"name_en"`
	Color  string `json:"color"`
}

type promptDetailResponse struct {
	Body struct {
		Prompt   promptDTO          `json:"prompt"`
		Version  promptVersionDTO   `json:"version"`
		Versions []promptVersionDTO `json:"versions"`
		Category struct {
			ID         int64  `json:"id"`
			Slug       string `json:"slug"`
			NameZh     string `json:"name_zh"`
			NameEn     string `json:"name_en"`
			IsHighRisk bool   `json:"is_high_risk"`
		} `json:"category"`
		Tags     []tagDTO `json:"tags"`
		Uploader string   `json:"uploader"`
		// MajorModificationNotice 在提示词发生过重大修改时非空（需求 §7.3 第 5 步）。
		MajorModificationNotice string `json:"major_modification_notice,omitempty"`
	}
}

func (s *Server) fillDetail(out *promptDetailResponse, detail prompt.Detail) {
	out.Body.Prompt = toPromptDTO(detail.Prompt)
	out.Body.Version = toPromptVersionDTO(detail.Version)
	out.Body.Uploader = detail.Uploader

	out.Body.Versions = make([]promptVersionDTO, 0, len(detail.Versions))
	for _, v := range detail.Versions {
		out.Body.Versions = append(out.Body.Versions, toPromptVersionDTO(v))
	}
	out.Body.Tags = make([]tagDTO, 0, len(detail.Tags))
	for _, t := range detail.Tags {
		out.Body.Tags = append(out.Body.Tags, tagDTO{
			ID: t.ID, Slug: t.Slug, NameZh: t.NameZh, NameEn: t.NameEn, Color: t.Color,
		})
	}
	out.Body.Category.ID = detail.Category.ID
	out.Body.Category.Slug = detail.Category.Slug
	out.Body.Category.NameZh = detail.Category.NameZh
	out.Body.Category.NameEn = detail.Category.NameEn
	out.Body.Category.IsHighRisk = detail.Category.IsHighRisk

	if detail.Prompt.MajorModified {
		out.Body.MajorModificationNotice = majorModificationNotice
	}
}

// viewerFor 返回当前身份对应的可见性判定入口。
//
// 未登录与被封禁都退化为游客身份：可见性判定只回答「能否看到」，
// 写入权限由 requireVerified 等函数单独把关，两者不混在一处。
func viewerFor(ctx context.Context) (domain.Viewer, error) {
	user, err := viewer(ctx)
	if err != nil {
		var appErr *apperr.Error
		if errors.As(err, &appErr) &&
			(appErr.Code == apperr.CodeUnauthorized || appErr.Code == apperr.CodeBanned) {
			return domain.Guest(), nil
		}
		return domain.Viewer{}, err
	}
	return domain.ViewerFor(user), nil
}
