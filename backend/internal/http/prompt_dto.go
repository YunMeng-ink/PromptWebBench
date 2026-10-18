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

// maxPageNumber 是页码上限：(page-1)*page_size 全程 int32 运算，超大页码会溢出成
// 负 OFFSET 并让数据库报错转 500。此处统一夹紧，全部列表端点共用。
const maxPageNumber = 100000

// pageValues 返回夹紧后的分页取值。请求层已做形态校验，此处的夹紧是纵深防御。
func pageValues(page, size int32) (int32, int32) {
	if page < 1 {
		page = 1
	}
	if page > maxPageNumber {
		page = maxPageNumber
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
	ID             int64   `json:"id"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	PurposeID      int64   `json:"purpose_id"`
	CapabilityID   int64   `json:"capability_id"`
	CapabilityIDs  []int64 `json:"capability_ids"`
	DifficultyID   *int64  `json:"difficulty_id,omitempty"`
	Visibility     string  `json:"visibility"`
	SensitiveLevel string  `json:"sensitive_level"`
	UploaderID     int64   `json:"uploader_id"`
	VersionNo      string  `json:"version_no"`
	MajorModified  bool    `json:"major_modified"`
	CommentCount   int32   `json:"comment_count"`
	FavoriteCount  int32   `json:"favorite_count"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type promptListItemDTO struct {
	promptDTO
	PurposeNameZh    string  `json:"purpose_name_zh"`
	PurposeNameEn    string  `json:"purpose_name_en"`
	CapabilityNameZh string  `json:"capability_name_zh"`
	CapabilityNameEn string  `json:"capability_name_en"`
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
		PurposeID:      p.PurposeID,
		CapabilityID:   p.CapabilityID,
		CapabilityIDs:  capabilityIDsOrPrimary(p.CapabilityIDs, p.CapabilityID),
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

// capabilityIDsOrPrimary 在完整集合缺失时退回主维度，保证旧读取路径仍给出非空集合。
func capabilityIDsOrPrimary(ids []int64, primary int64) []int64 {
	if len(ids) > 0 {
		return ids
	}
	if primary > 0 {
		return []int64{primary}
	}
	return []int64{}
}

// formatTime 以 RFC 3339 输出 UTC 时刻，全系统时间字段统一为该形态。
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

type promptVersionDTO struct {
	ID             int64   `json:"id"`
	VersionNo      string  `json:"version_no"`
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	Description    string  `json:"description"`
	PurposeID      int64   `json:"purpose_id"`
	CapabilityID   int64   `json:"capability_id"`
	CapabilityIDs  []int64 `json:"capability_ids"`
	DifficultyID   *int64  `json:"difficulty_id,omitempty"`
	Visibility     string  `json:"visibility"`
	SensitiveLevel string  `json:"sensitive_level"`
	IsMajor        bool    `json:"is_major"`
	ChangeNote     string  `json:"change_note"`
	AuthorID       int64   `json:"author_id"`
	CreatedAt      string  `json:"created_at"`
}

func toPromptVersionDTO(v domain.PromptVersion) promptVersionDTO {
	return promptVersionDTO{
		ID:             v.ID,
		VersionNo:      v.Version.String(),
		Title:          v.Title,
		Body:           v.Body,
		Description:    v.Description,
		PurposeID:      v.PurposeID,
		CapabilityID:   v.CapabilityID,
		CapabilityIDs:  capabilityIDsOrPrimary(v.CapabilityIDs, v.CapabilityID),
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
	Kind   string `json:"kind"`
}

type promptDetailResponse struct {
	Body struct {
		Prompt       promptDTO          `json:"prompt"`
		Version      promptVersionDTO   `json:"version"`
		Versions     []promptVersionDTO `json:"versions"`
		Purpose      purposeDTO         `json:"purpose"`
		Capability   capabilityDTO      `json:"capability"`
		Capabilities []capabilityDTO    `json:"capabilities"`
		Difficulty   *difficultyDTO     `json:"difficulty,omitempty"`
		Tags         []tagDTO           `json:"tags"`
		Uploader     string             `json:"uploader"`
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
			ID: t.ID, Slug: t.Slug, NameZh: t.NameZh, NameEn: t.NameEn, Color: t.Color, Kind: t.Kind,
		})
	}
	out.Body.Purpose = toPurposeDTO(detail.Purpose)
	out.Body.Capability = toCapabilityDTO(detail.Capability)
	out.Body.Capabilities = make([]capabilityDTO, 0, len(detail.Capabilities))
	for _, capability := range detail.Capabilities {
		out.Body.Capabilities = append(out.Body.Capabilities, toCapabilityDTO(capability))
	}
	if len(out.Body.Capabilities) == 0 {
		out.Body.Capabilities = append(out.Body.Capabilities, out.Body.Capability)
	}
	if detail.Difficulty != nil {
		difficulty := toDifficultyDTO(*detail.Difficulty)
		out.Body.Difficulty = &difficulty
	}

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
