package api

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/service/admin"
)

// 后台接口的响应形状。分页沿用 C-40 的 {items, page, page_size, total}。

// ---------- 通用转换 ----------

// timeString 把可空时间戳转为 RFC 3339 文本，无值时为空串。
func timeString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return formatTime(ts.Time)
}

// timePtrOf 把可空时间戳转为可选文本。
func timePtrOf(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	value := formatTime(ts.Time)
	return &value
}

// int64PtrOf 把可空整数转为可选整数。
func int64PtrOf(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

// timePtrOfTime 把可选时刻转为可选 RFC 3339 文本。
func timePtrOfTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	value := formatTime(*t)
	return &value
}

// stringPtrOf 把可空文本转为可选文本。
func stringPtrOf(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	value := v.String
	return &value
}

// pageBody 是分页响应的公共字段，内联到各列表响应中。
type pageBody struct {
	Page     int32 `json:"page"`
	PageSize int32 `json:"page_size"`
	Total    int64 `json:"total"`
}

// ---------- 模块 1：统计仪表盘 ----------

type dashboardPromptDTO struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Visibility    string `json:"visibility"`
	CommentCount  int32  `json:"comment_count"`
	FavoriteCount int32  `json:"favorite_count"`
	CreatedAt     string `json:"created_at"`
}

type dashboardResponse struct {
	Body struct {
		// 计数指标共 17 项，字段名与技术可行性报告 §2.14 之外的 dashboardCounters 声明一一对应。
		UserTotal         int64 `json:"user_total"`
		UserToday         int64 `json:"user_today"`
		UserRoleUser      int64 `json:"user_role_user"`
		UserRoleDeveloper int64 `json:"user_role_developer"`
		UserBanned        int64 `json:"user_banned"`
		PromptTotal       int64 `json:"prompt_total"`
		PromptPublic      int64 `json:"prompt_public"`
		PromptPrivate     int64 `json:"prompt_private"`
		PromptDeveloper   int64 `json:"prompt_developer"`
		PromptToday       int64 `json:"prompt_today"`
		RatingTotal       int64 `json:"rating_total"`
		CommentTotal      int64 `json:"comment_total"`
		ReportPending     int64 `json:"report_pending"`
		RecycleTotal      int64 `json:"recycle_total"`
		ModelTotal        int64 `json:"model_total"`
		InviteTotal       int64 `json:"invite_total"`
		RecommendPending  int64 `json:"recommend_pending"`

		HotPrompts    []dashboardPromptDTO `json:"hot_prompts"`
		RecentPrompts []dashboardPromptDTO `json:"recent_prompts"`
	}
}

func toDashboardPrompt(item admin.HotPrompt) dashboardPromptDTO {
	return dashboardPromptDTO{
		ID:            item.ID,
		Title:         item.Title,
		Visibility:    string(item.Visibility),
		CommentCount:  item.CommentCount,
		FavoriteCount: item.FavoriteCount,
		CreatedAt:     formatTime(item.CreatedAt),
	}
}

func toDashboardPrompts(items []admin.HotPrompt) []dashboardPromptDTO {
	out := make([]dashboardPromptDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toDashboardPrompt(item))
	}
	return out
}

// ---------- 模块 2：提示词管理 ----------

type adminPromptDTO struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	PurposeID        int64  `json:"purpose_id"`
	PurposeNameZh    string `json:"purpose_name_zh"`
	PurposeNameEn    string `json:"purpose_name_en"`
	CapabilityID     int64  `json:"capability_id"`
	CapabilityNameZh string `json:"capability_name_zh"`
	CapabilityNameEn string `json:"capability_name_en"`
	DifficultyID     *int64 `json:"difficulty_id,omitempty"`
	Visibility       string `json:"visibility"`
	SensitiveLevel   string `json:"sensitive_level"`
	UploaderID       int64  `json:"uploader_id"`
	UploaderName     string `json:"uploader_name"`
	VersionNo        string `json:"version_no"`
	MajorModified    bool   `json:"major_modified"`
	HighRisk         bool   `json:"is_high_risk"`
	CommentCount     int32  `json:"comment_count"`
	FavoriteCount    int32  `json:"favorite_count"`
	Deleted          bool   `json:"deleted"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func toAdminPromptDTO(row repo.AdminListPromptsRow) adminPromptDTO {
	return adminPromptDTO{
		ID:               row.ID,
		Title:            row.Title,
		Description:      row.Description,
		PurposeID:        row.PurposeID,
		PurposeNameZh:    row.PurposeNameZh,
		PurposeNameEn:    row.PurposeNameEn,
		CapabilityID:     row.CapabilityID,
		CapabilityNameZh: row.CapabilityNameZh,
		CapabilityNameEn: row.CapabilityNameEn,
		DifficultyID:     int64PtrOf(row.DifficultyID),
		Visibility:       row.Visibility,
		SensitiveLevel:   row.SensitiveLevel,
		UploaderID:       row.UploaderID,
		UploaderName:     row.UploaderName,
		VersionNo:        row.LatestVersionNo,
		MajorModified:    row.MajorModified,
		HighRisk:         row.IsHighRisk,
		CommentCount:     row.CommentCount,
		FavoriteCount:    row.FavoriteCount,
		Deleted:          row.Deleted,
		CreatedAt:        timeString(row.CreatedAt),
		UpdatedAt:        timeString(row.UpdatedAt),
	}
}

type adminPromptListResponse struct {
	Body struct {
		Items    []adminPromptDTO `json:"items"`
		Page     int32            `json:"page"`
		PageSize int32            `json:"page_size"`
		Total    int64            `json:"total"`
	}
}

type adminPromptDetailResponse struct {
	Body struct {
		Prompt adminPromptDTO `json:"prompt"`
		Body   string         `json:"body"`
		Tags   []tagDTO       `json:"tags"`
		// Versions 是全部历史版本，按主次版本号倒序（需求 §17.2「查看提示词版本」）。
		Versions []promptVersionDTO `json:"versions"`
	}
}

type adminRatingDTO struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	UserName  string `json:"user_name"`
	ModelID   int64  `json:"model_id"`
	ModelName string `json:"model_name"`
	Score     int16  `json:"score"`
	EditCount int32  `json:"edit_count"`
	Deleted   bool   `json:"deleted"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type adminRatingListResponse struct {
	Body struct {
		Items    []adminRatingDTO `json:"items"`
		Page     int32            `json:"page"`
		PageSize int32            `json:"page_size"`
		Total    int64            `json:"total"`
	}
}

type adminCommentDTO struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	AuthorName string `json:"author_name"`
	ParentID   *int64 `json:"parent_id,omitempty"`
	Body       string `json:"body"`
	LikeCount  int32  `json:"like_count"`
	Deleted    bool   `json:"deleted"`
	CreatedAt  string `json:"created_at"`
}

type adminCommentListResponse struct {
	Body struct {
		Items    []adminCommentDTO `json:"items"`
		Page     int32             `json:"page"`
		PageSize int32             `json:"page_size"`
		Total    int64             `json:"total"`
	}
}

// adminRecommendTagDTO 是推荐标签的后台形态（需求 §17.5、§6.3）。
type adminRecommendTagDTO struct {
	ID          int64           `json:"id"`
	PromptID    int64           `json:"prompt_id"`
	PromptTitle string          `json:"prompt_title"`
	ModelID     int64           `json:"model_id"`
	ModelName   string          `json:"model_name"`
	Scope       string          `json:"scope"`
	Kind        string          `json:"kind"`
	State       string          `json:"state"`
	LabelZh     string          `json:"label_zh"`
	LabelEn     string          `json:"label_en"`
	Confidence  string          `json:"confidence"`
	Reason      string          `json:"reason"`
	Snapshot    json.RawMessage `json:"snapshot,omitempty"`
	TriggeredAt string          `json:"triggered_at"`
	ReviewedAt  *string         `json:"reviewed_at,omitempty"`
}

type adminRecommendTagListResponse struct {
	Body struct {
		Items    []adminRecommendTagDTO `json:"items"`
		Page     int32                  `json:"page"`
		PageSize int32                  `json:"page_size"`
		Total    int64                  `json:"total"`
	}
}

// ---------- 模块 3：模型管理 ----------

type adminModelDTO struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	ModelKey        string   `json:"model_key"`
	ProviderKey     string   `json:"provider_key"`
	ProviderName    string   `json:"provider_name"`
	InputModalities []string `json:"input_modalities"`
	Aliases         []string `json:"aliases"`
	Description     string   `json:"description"`
	ReleaseDate     string   `json:"release_date"`
	ContextLength   *int64   `json:"context_length,omitempty"`
	IsEnabled       bool     `json:"is_enabled"`
	CapabilityIDs   []int64  `json:"capability_ids"`
	Note            string   `json:"note"`
	ImportSource    string   `json:"import_source"`
	RatingCount     int64    `json:"rating_count"`
	PromptCount     int64    `json:"prompt_count"`
	Deleted         bool     `json:"deleted"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

func toAdminModelDTO(row repo.AdminListModelsRow, capabilityIDs []int64) adminModelDTO {
	out := adminModelDTO{
		ID:              row.ID,
		Name:            row.Name,
		ModelKey:        row.ModelKey,
		ProviderKey:     row.ProviderKey,
		ProviderName:    row.ProviderName,
		InputModalities: emptyIfNil(row.InputModalities),
		Aliases:         emptyIfNil(row.Aliases),
		Description:     row.Description,
		ContextLength:   int64PtrOf(row.ContextLength),
		IsEnabled:       row.IsEnabled,
		CapabilityIDs:   emptyInt64IfNil(capabilityIDs),
		Note:            row.Note,
		ImportSource:    row.ImportSource,
		RatingCount:     row.RatingCount,
		PromptCount:     row.PromptCount,
		Deleted:         row.DeletedAt.Valid,
		CreatedAt:       timeString(row.CreatedAt),
		UpdatedAt:       timeString(row.UpdatedAt),
	}
	if row.ReleaseDate.Valid {
		out.ReleaseDate = row.ReleaseDate.Time.Format("2006-01-02")
	}
	return out
}

// modelModelDTO 把 models 表行转为后台模型形态（写入路径的返回值）。
func modelModelDTO(row repo.Model, capabilityIDs []int64) adminModelDTO {
	out := adminModelDTO{
		ID:              row.ID,
		Name:            row.Name,
		ModelKey:        row.ModelKey,
		ProviderKey:     row.ProviderKey,
		ProviderName:    row.ProviderName,
		InputModalities: emptyIfNil(row.InputModalities),
		Aliases:         emptyIfNil(row.Aliases),
		Description:     row.Description,
		ContextLength:   int64PtrOf(row.ContextLength),
		IsEnabled:       row.IsEnabled,
		CapabilityIDs:   emptyInt64IfNil(capabilityIDs),
		Note:            row.Note,
		ImportSource:    row.ImportSource,
		Deleted:         row.DeletedAt.Valid,
		CreatedAt:       timeString(row.CreatedAt),
		UpdatedAt:       timeString(row.UpdatedAt),
	}
	if row.ReleaseDate.Valid {
		out.ReleaseDate = row.ReleaseDate.Time.Format("2006-01-02")
	}
	return out
}

// emptyInt64IfNil 保证整数数组字段序列化为 [] 而不是 null。
func emptyInt64IfNil(items []int64) []int64 {
	if items == nil {
		return []int64{}
	}
	return items
}

// emptyIfNil 保证数组字段序列化为 [] 而不是 null。
func emptyIfNil(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

type adminModelListResponse struct {
	Body struct {
		Items    []adminModelDTO `json:"items"`
		Page     int32           `json:"page"`
		PageSize int32           `json:"page_size"`
		Total    int64           `json:"total"`
	}
}

type adminModelResponse struct {
	Body adminModelDTO
}

type adminModelImportEntryDTO struct {
	ProviderID      string   `json:"provider_id"`
	ProviderName    string   `json:"provider_name"`
	ModelID         string   `json:"model_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	ReleaseDate     string   `json:"release_date"`
	InputModalities []string `json:"input_modalities"`
	ContextLength   *int64   `json:"context_length,omitempty"`
	// Duplicate 非空表示该条目与平台已有模型重复（需求 §13.4）。
	Duplicate *adminDuplicateModelDTO `json:"duplicate,omitempty"`
}

type adminDuplicateModelDTO struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ProviderName string `json:"provider_name"`
}

type adminModelImportPreviewResponse struct {
	Body struct {
		Items          []adminModelImportEntryDTO `json:"items"`
		DuplicateCount int                        `json:"duplicate_count"`
	}
}

type adminModelImportResultDTO struct {
	Total       int `json:"total"`
	Created     int `json:"created"`
	Overwritten int `json:"overwritten"`
	Skipped     int `json:"skipped"`
}

type adminModelImportResponse struct {
	Body adminModelImportResultDTO
}

type adminModelImportLogDTO struct {
	ID           int64           `json:"id"`
	ModelID      *int64          `json:"model_id,omitempty"`
	Action       string          `json:"action"`
	ProviderKey  string          `json:"provider_key"`
	ModelKey     string          `json:"model_key"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	OperatorName string          `json:"operator_name"`
	CreatedAt    string          `json:"created_at"`
}

type adminModelImportLogListResponse struct {
	Body struct {
		Items    []adminModelImportLogDTO `json:"items"`
		Page     int32                    `json:"page"`
		PageSize int32                    `json:"page_size"`
		Total    int64                    `json:"total"`
	}
}

type adminModelPromptDTO struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Visibility string `json:"visibility"`
	CreatedAt  string `json:"created_at"`
}

type adminModelPromptListResponse struct {
	Body struct {
		Items    []adminModelPromptDTO `json:"items"`
		Page     int32                 `json:"page"`
		PageSize int32                 `json:"page_size"`
		Total    int64                 `json:"total"`
	}
}

type adminModelStatDTO struct {
	PromptID           int64   `json:"prompt_id"`
	PromptTitle        string  `json:"prompt_title"`
	HistoryRatingCount int32   `json:"history_rating_count"`
	HistoryAdjusted    float64 `json:"history_adjusted"`
	RecentRatingCount  int32   `json:"recent_rating_count"`
	RecentAdjusted     float64 `json:"recent_adjusted"`
	ComputedAt         string  `json:"computed_at"`
}

type adminModelStatListResponse struct {
	Body struct {
		Items    []adminModelStatDTO `json:"items"`
		Page     int32               `json:"page"`
		PageSize int32               `json:"page_size"`
		Total    int64               `json:"total"`
	}
}

type adminModelSnapshotDTO struct {
	ID        int64           `json:"id"`
	ModelID   int64           `json:"model_id"`
	Source    string          `json:"source"`
	Raw       json.RawMessage `json:"raw,omitempty"`
	CreatedAt string          `json:"created_at"`
}

type adminModelSnapshotListResponse struct {
	Body struct {
		Items    []adminModelSnapshotDTO `json:"items"`
		Page     int32                   `json:"page"`
		PageSize int32                   `json:"page_size"`
		Total    int64                   `json:"total"`
	}
}

// ---------- 模块 4：用户管理 ----------

type adminUserDTO struct {
	userDTO
	IsSystem     bool   `json:"is_system"`
	BannedReason string `json:"banned_reason"`
	PromptCount  int64  `json:"prompt_count"`
	HasPassword  bool   `json:"has_password"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func toAdminUserDTO(row repo.AdminListUsersRow) adminUserDTO {
	return adminUserDTO{
		userDTO: userDTO{
			ID:            row.ID,
			Username:      row.Username,
			Email:         row.Email,
			Role:          row.Role,
			Status:        row.Status,
			Bio:           row.Bio,
			AvatarURL:     row.AvatarUrl,
			EmailVerified: row.EmailVerifiedAt.Valid,
		},
		IsSystem:     row.IsSystem,
		BannedReason: row.BannedReason,
		PromptCount:  row.PromptCount,
		HasPassword:  row.PasswordHash.Valid && row.PasswordHash.String != "",
		CreatedAt:    timeString(row.CreatedAt),
		UpdatedAt:    timeString(row.UpdatedAt),
	}
}

type adminUserListResponse struct {
	Body struct {
		Items    []adminUserDTO `json:"items"`
		Page     int32          `json:"page"`
		PageSize int32          `json:"page_size"`
		Total    int64          `json:"total"`
	}
}

type adminUserRatingDTO struct {
	ID          int64  `json:"id"`
	PromptID    int64  `json:"prompt_id"`
	PromptTitle string `json:"prompt_title"`
	ModelID     int64  `json:"model_id"`
	ModelName   string `json:"model_name"`
	Score       int16  `json:"score"`
	EditCount   int32  `json:"edit_count"`
	Deleted     bool   `json:"deleted"`
	CreatedAt   string `json:"created_at"`
}

type adminUserRatingListResponse struct {
	Body struct {
		Items    []adminUserRatingDTO `json:"items"`
		Page     int32                `json:"page"`
		PageSize int32                `json:"page_size"`
		Total    int64                `json:"total"`
	}
}

type adminUserCommentDTO struct {
	ID          int64  `json:"id"`
	PromptID    int64  `json:"prompt_id"`
	PromptTitle string `json:"prompt_title"`
	ParentID    *int64 `json:"parent_id,omitempty"`
	Body        string `json:"body"`
	Deleted     bool   `json:"deleted"`
	CreatedAt   string `json:"created_at"`
}

type adminUserCommentListResponse struct {
	Body struct {
		Items    []adminUserCommentDTO `json:"items"`
		Page     int32                 `json:"page"`
		PageSize int32                 `json:"page_size"`
		Total    int64                 `json:"total"`
	}
}

type adminUserLibraryDTO struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	QuestionCount int32  `json:"question_count"`
	Deleted       bool   `json:"deleted"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type adminUserLibraryListResponse struct {
	Body struct {
		Items    []adminUserLibraryDTO `json:"items"`
		Page     int32                 `json:"page"`
		PageSize int32                 `json:"page_size"`
		Total    int64                 `json:"total"`
	}
}

type adminUserReportDTO struct {
	ID         int64  `json:"id"`
	ObjectType string `json:"object_type"`
	ObjectID   int64  `json:"object_id"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	CreatedAt  string `json:"created_at"`
}

type adminUserReportListResponse struct {
	Body struct {
		Items    []adminUserReportDTO `json:"items"`
		Page     int32                `json:"page"`
		PageSize int32                `json:"page_size"`
		Total    int64                `json:"total"`
	}
}

// ---------- 模块 5：标签管理 ----------

type adminTagDTO struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	NameZh      string `json:"name_zh"`
	NameEn      string `json:"name_en"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Priority    int32  `json:"priority"`
	IsEnabled   bool   `json:"is_enabled"`
	UsageCount  int64  `json:"usage_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toAdminTagDTO(row repo.AdminListTagsRow) adminTagDTO {
	return adminTagDTO{
		ID:          row.ID,
		Slug:        row.Slug,
		NameZh:      row.NameZh,
		NameEn:      row.NameEn,
		Color:       row.Color,
		Description: row.Description,
		Kind:        row.Kind,
		Priority:    row.Priority,
		IsEnabled:   row.IsEnabled,
		UsageCount:  row.UsageCount,
		CreatedAt:   timeString(row.CreatedAt),
		UpdatedAt:   timeString(row.UpdatedAt),
	}
}

// tagRowDTO 把 tags 表行转为后台标签形态（写入路径的返回值）。
func tagRowDTO(row repo.Tag) adminTagDTO {
	return adminTagDTO{
		ID:          row.ID,
		Slug:        row.Slug,
		NameZh:      row.NameZh,
		NameEn:      row.NameEn,
		Color:       row.Color,
		Description: row.Description,
		Kind:        row.Kind,
		Priority:    row.Priority,
		IsEnabled:   row.IsEnabled,
		CreatedAt:   timeString(row.CreatedAt),
		UpdatedAt:   timeString(row.UpdatedAt),
	}
}

type adminTagListResponse struct {
	Body struct {
		Items    []adminTagDTO `json:"items"`
		Page     int32         `json:"page"`
		PageSize int32         `json:"page_size"`
		Total    int64         `json:"total"`
	}
}

type adminTagResponse struct {
	Body adminTagDTO
}

// ---------- 模块 6：举报管理 ----------

type adminReportDTO struct {
	ID           int64   `json:"id"`
	ReporterID   int64   `json:"reporter_id"`
	ReporterName string  `json:"reporter_name"`
	ObjectType   string  `json:"object_type"`
	ObjectID     int64   `json:"object_id"`
	Reason       string  `json:"reason"`
	Detail       string  `json:"detail"`
	Status       string  `json:"status"`
	HandlerID    *int64  `json:"handler_id,omitempty"`
	HandlerName  string  `json:"handler_name"`
	HandleNote   string  `json:"handle_note"`
	HandledAt    *string `json:"handled_at,omitempty"`
	CreatedAt    string  `json:"created_at"`

	// 被举报对象的摘要：提示词举报给标题，评论举报给正文与所属提示词。
	PromptTitle     string `json:"prompt_title"`
	CommentBody     string `json:"comment_body"`
	CommentPromptID int64  `json:"comment_prompt_id"`
}

func toAdminReportDTO(row repo.AdminListReportsRow) adminReportDTO {
	return adminReportDTO{
		ID:              row.ID,
		ReporterID:      row.ReporterID,
		ReporterName:    row.ReporterName,
		ObjectType:      row.ObjectType,
		ObjectID:        row.ObjectID,
		Reason:          row.Reason,
		Detail:          row.Detail,
		Status:          row.Status,
		HandlerID:       int64PtrOf(row.HandlerID),
		HandlerName:     row.HandlerName,
		HandleNote:      row.HandleNote,
		HandledAt:       timePtrOf(row.HandledAt),
		CreatedAt:       timeString(row.CreatedAt),
		PromptTitle:     row.PromptTitle,
		CommentBody:     row.CommentBody,
		CommentPromptID: row.CommentPromptID,
	}
}

type adminReportListResponse struct {
	Body struct {
		Items    []adminReportDTO `json:"items"`
		Page     int32            `json:"page"`
		PageSize int32            `json:"page_size"`
		Total    int64            `json:"total"`
	}
}

// ---------- 模块 7：注册与邀请码管理 ----------

type adminInviteDTO struct {
	ID          int64   `json:"id"`
	Code        string  `json:"code"`
	Kind        string  `json:"kind"`
	CreatorName string  `json:"creator_name"`
	MaxUses     int32   `json:"max_uses"`
	UsedCount   int32   `json:"used_count"`
	Disabled    bool    `json:"disabled"`
	Note        string  `json:"note"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

func toAdminInviteDTO(row repo.AdminListInviteCodesRow) adminInviteDTO {
	return adminInviteDTO{
		ID:          row.ID,
		Code:        row.Code,
		Kind:        row.Kind,
		CreatorName: row.CreatorName,
		MaxUses:     row.MaxUses,
		UsedCount:   row.UsedCount,
		Disabled:    row.Disabled,
		Note:        row.Note,
		ExpiresAt:   timePtrOf(row.ExpiresAt),
		CreatedAt:   timeString(row.CreatedAt),
	}
}

// inviteRowDTO 把 invite_codes 表行转为后台形态（生成路径的返回值）。
func inviteRowDTO(row repo.InviteCode) adminInviteDTO {
	return adminInviteDTO{
		ID:        row.ID,
		Code:      row.Code,
		Kind:      row.Kind,
		MaxUses:   row.MaxUses,
		UsedCount: row.UsedCount,
		Disabled:  row.Disabled,
		Note:      row.Note,
		ExpiresAt: timePtrOf(row.ExpiresAt),
		CreatedAt: timeString(row.CreatedAt),
	}
}

type adminInviteListResponse struct {
	Body struct {
		Items    []adminInviteDTO `json:"items"`
		Page     int32            `json:"page"`
		PageSize int32            `json:"page_size"`
		Total    int64            `json:"total"`
	}
}

type adminInviteCreateResponse struct {
	Body struct {
		Items []adminInviteDTO `json:"items"`
	}
}

type adminInviteResponse struct {
	Body adminInviteDTO
}

type adminInviteUsageDTO struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	UsedAt   string `json:"used_at"`
}

type adminInviteUsageListResponse struct {
	Body struct {
		Items    []adminInviteUsageDTO `json:"items"`
		Page     int32                 `json:"page"`
		PageSize int32                 `json:"page_size"`
		Total    int64                 `json:"total"`
	}
}

// ---------- 模块 8：操作日志 ----------

type adminOperationLogDTO struct {
	ID           int64           `json:"id"`
	OperatorID   *int64          `json:"operator_id,omitempty"`
	OperatorName string          `json:"operator_name"`
	Action       string          `json:"action"`
	ObjectType   string          `json:"object_type"`
	ObjectID     *int64          `json:"object_id,omitempty"`
	BeforeData   json.RawMessage `json:"before_data,omitempty"`
	AfterData    json.RawMessage `json:"after_data,omitempty"`
	Note         string          `json:"note"`
	IP           string          `json:"ip"`
	CreatedAt    string          `json:"created_at"`
}

func toAdminOperationLogDTO(row repo.OperationLog) adminOperationLogDTO {
	out := adminOperationLogDTO{
		ID:           row.ID,
		OperatorID:   int64PtrOf(row.OperatorID),
		OperatorName: row.OperatorName,
		Action:       row.Action,
		ObjectType:   row.ObjectType,
		ObjectID:     int64PtrOf(row.ObjectID),
		Note:         row.Note,
		IP:           row.Ip,
		CreatedAt:    timeString(row.CreatedAt),
	}
	if len(row.BeforeData) > 0 {
		out.BeforeData = json.RawMessage(row.BeforeData)
	}
	if len(row.AfterData) > 0 {
		out.AfterData = json.RawMessage(row.AfterData)
	}
	return out
}

type adminOperationLogListResponse struct {
	Body struct {
		Items    []adminOperationLogDTO `json:"items"`
		Page     int32                  `json:"page"`
		PageSize int32                  `json:"page_size"`
		Total    int64                  `json:"total"`
	}
}

type adminOperationLogActionsResponse struct {
	Body struct {
		// Actions 是全部已登记的操作类型；Required 标记需求 §17.8 明列的 18 类。
		Actions  []string `json:"actions"`
		Required []string `json:"required"`
	}
}

// ---------- 模块 9：公告管理 ----------

type adminAnnouncementDTO struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	Body          string  `json:"body"`
	PublisherName string  `json:"publisher_name"`
	State         string  `json:"state"`
	PublishedAt   *string `json:"published_at,omitempty"`
	WithdrawnAt   *string `json:"withdrawn_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

// adminAnnouncementRow 把公告列表行转为后台形态（含发布者昵称与状态）。
func adminAnnouncementRow(row repo.AdminListAnnouncementsRow) adminAnnouncementDTO {
	return adminAnnouncementDTO{
		ID:            row.ID,
		Title:         row.Title,
		Body:          row.Body,
		PublisherName: row.PublisherName,
		State:         announcementRowState(row),
		PublishedAt:   timePtrOf(row.PublishedAt),
		WithdrawnAt:   timePtrOf(row.WithdrawnAt),
		CreatedAt:     timeString(row.CreatedAt),
	}
}

// announcementRowState 与 admin.AnnouncementState 同义，输入为列表行。
func announcementRowState(row repo.AdminListAnnouncementsRow) string {
	switch {
	case row.WithdrawnAt.Valid:
		return "withdrawn"
	case row.PublishedAt.Valid:
		return "live"
	default:
		return "draft"
	}
}

// adminAnnouncementFromRow 把 announcements 表行转为后台形态（写入路径的返回值）。
func adminAnnouncementFromRow(row repo.Announcement) adminAnnouncementDTO {
	return adminAnnouncementDTO{
		ID:          row.ID,
		Title:       row.Title,
		Body:        row.Body,
		State:       admin.AnnouncementState(row),
		PublishedAt: timePtrOf(row.PublishedAt),
		WithdrawnAt: timePtrOf(row.WithdrawnAt),
		CreatedAt:   timeString(row.CreatedAt),
	}
}

type adminAnnouncementListResponse struct {
	Body struct {
		Items    []adminAnnouncementDTO `json:"items"`
		Page     int32                  `json:"page"`
		PageSize int32                  `json:"page_size"`
		Total    int64                  `json:"total"`
	}
}

type adminAnnouncementResponse struct {
	Body adminAnnouncementDTO
}

// ---------- 模块 10：回收站管理 ----------

type adminRecycleEntryDTO struct {
	ID            int64           `json:"id"`
	ObjectType    string          `json:"object_type"`
	ObjectID      int64           `json:"object_id"`
	Title         string          `json:"title"`
	OwnerID       *int64          `json:"owner_id,omitempty"`
	OwnerName     string          `json:"owner_name"`
	LibraryID     *int64          `json:"library_id,omitempty"`
	DeletedBy     *int64          `json:"deleted_by,omitempty"`
	DeletedByName string          `json:"deleted_by_name"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
	DeletedAt     string          `json:"deleted_at"`
	ExpiresAt     string          `json:"expires_at"`
}

func toAdminRecycleEntryDTO(row repo.AdminListRecycleBinRow) adminRecycleEntryDTO {
	out := adminRecycleEntryDTO{
		ID:            row.ID,
		ObjectType:    row.ObjectType,
		ObjectID:      row.ObjectID,
		Title:         admin.RecycleEntryTitle(row.Snapshot),
		OwnerID:       int64PtrOf(row.OwnerID),
		OwnerName:     row.OwnerName,
		LibraryID:     int64PtrOf(row.LibraryID),
		DeletedBy:     int64PtrOf(row.DeletedBy),
		DeletedByName: row.DeletedByName,
		DeletedAt:     timeString(row.DeletedAt),
		ExpiresAt:     timeString(row.ExpiresAt),
	}
	if len(row.Snapshot) > 0 {
		out.Snapshot = json.RawMessage(row.Snapshot)
	}
	return out
}

type adminRecycleListResponse struct {
	Body struct {
		Items    []adminRecycleEntryDTO `json:"items"`
		Page     int32                  `json:"page"`
		PageSize int32                  `json:"page_size"`
		Total    int64                  `json:"total"`
	}
}

// ---------- 模块 11：系统设置 ----------

// adminConfigDTO 与 service/admin.ConfigValue 同形，键名保持 snake_case。
type adminConfigDTO struct {
	Key         string          `json:"key"`
	Kind        string          `json:"kind"`
	Description string          `json:"description"`
	Secret      bool            `json:"secret"`
	Value       json.RawMessage `json:"value,omitempty"`
	Effective   json.RawMessage `json:"effective,omitempty"`
	Source      string          `json:"source"`
	UpdatedAt   *string         `json:"updated_at,omitempty"`
}

type adminConfigListResponse struct {
	Body struct {
		Items []adminConfigDTO `json:"items"`
		// SecretWritable 表明主密钥是否已配置；为假时密钥类配置项无法保存。
		SecretWritable bool `json:"secret_writable"`
	}
}

// ---------- 分页响应 ----------
