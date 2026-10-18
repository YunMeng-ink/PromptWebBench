package prompt

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// translatePermission 把领域层的权限判定失败翻译成错误码。
// 领域层不依赖 apperr，错误码在此处统一产生。
func translatePermission(err error) error {
	if errors.Is(err, domain.ErrHighRiskPurposeForbidden) {
		return apperr.New(apperr.CodeForbidden, "普通用户不能选择高风险考察目的")
	}
	if errors.Is(err, domain.ErrDeveloperVisibilityForbidden) {
		return apperr.New(apperr.CodeForbidden, "普通用户不能选择开发者可见")
	}
	return err
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// promptFields 是 prompts 表一行的公共列。sqlc 为联表查询生成了若干字段相同的结构体，
// 各自一个类型，无法用同一个类型参数接收；此处把取值收敛到一个结构体再统一映射。
type promptFields struct {
	ID                 int64
	Title              string
	Body               string
	Description        string
	PurposeID          int64
	CapabilityID       int64
	DifficultyID       pgtype.Int8
	Visibility         string
	SensitiveLevel     string
	UploaderID         int64
	OriginalUploaderID pgtype.Int8
	CurrentVersionID   pgtype.Int8
	LatestVersionNo    string
	MajorModified      bool
	CommentCount       int32
	FavoriteCount      int32
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
	IsHighRisk         bool
}

// prompts 表的联表查询行由 sqlc 各生成一个结构体（GetPromptRow、LockPromptRow、
// ListOwnPromptsRow 字段一致但类型不同）。此处把每种行的取值收集到同一个结构体，
// 再交给唯一的映射函数，避免为同一映射规则写三份。
func fieldsOfPrompt(r repo.Prompt) promptFields {
	return promptFields{
		ID: r.ID, Title: r.Title, Body: r.Body, Description: r.Description,
		PurposeID: r.PurposeID, CapabilityID: r.CapabilityID, DifficultyID: r.DifficultyID,
		Visibility: r.Visibility, SensitiveLevel: r.SensitiveLevel, UploaderID: r.UploaderID,
		OriginalUploaderID: r.OriginalUploaderID, CurrentVersionID: r.CurrentVersionID,
		LatestVersionNo: r.LatestVersionNo, MajorModified: r.MajorModified,
		CommentCount: r.CommentCount, FavoriteCount: r.FavoriteCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func fieldsOfGetPrompt(r repo.GetPromptRow) promptFields {
	return promptFields{
		ID: r.ID, Title: r.Title, Body: r.Body, Description: r.Description,
		PurposeID: r.PurposeID, CapabilityID: r.CapabilityID, DifficultyID: r.DifficultyID,
		Visibility: r.Visibility, SensitiveLevel: r.SensitiveLevel, UploaderID: r.UploaderID,
		OriginalUploaderID: r.OriginalUploaderID, CurrentVersionID: r.CurrentVersionID,
		LatestVersionNo: r.LatestVersionNo, MajorModified: r.MajorModified,
		CommentCount: r.CommentCount, FavoriteCount: r.FavoriteCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, IsHighRisk: r.IsHighRisk,
	}
}

// fieldsOfIncludingDeleted 映射包含回收站条目的读取行。考察目的名称不入 promptFields：
// 它们只服务于后台列表的展示，映射到实体上会被后续写入路径误当成可写字段。
func fieldsOfIncludingDeleted(r repo.GetPromptIncludingDeletedRow) promptFields {
	return promptFields{
		ID: r.ID, Title: r.Title, Body: r.Body, Description: r.Description,
		PurposeID: r.PurposeID, CapabilityID: r.CapabilityID, DifficultyID: r.DifficultyID,
		Visibility: r.Visibility, SensitiveLevel: r.SensitiveLevel, UploaderID: r.UploaderID,
		OriginalUploaderID: r.OriginalUploaderID, CurrentVersionID: r.CurrentVersionID,
		LatestVersionNo: r.LatestVersionNo, MajorModified: r.MajorModified,
		CommentCount: r.CommentCount, FavoriteCount: r.FavoriteCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, IsHighRisk: r.IsHighRisk,
	}
}

func fieldsOfLockPrompt(r repo.LockPromptRow) promptFields {
	return promptFields{
		ID: r.ID, Title: r.Title, Body: r.Body, Description: r.Description,
		PurposeID: r.PurposeID, CapabilityID: r.CapabilityID, DifficultyID: r.DifficultyID,
		Visibility: r.Visibility, SensitiveLevel: r.SensitiveLevel, UploaderID: r.UploaderID,
		OriginalUploaderID: r.OriginalUploaderID, CurrentVersionID: r.CurrentVersionID,
		LatestVersionNo: r.LatestVersionNo, MajorModified: r.MajorModified,
		CommentCount: r.CommentCount, FavoriteCount: r.FavoriteCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, IsHighRisk: r.IsHighRisk,
	}
}

// fieldsOfLockIncludingDeleted 映射彻底删除前锁定的行（含回收站条目）。
func fieldsOfLockIncludingDeleted(r repo.LockPromptIncludingDeletedRow) promptFields {
	return promptFields{
		ID: r.ID, Title: r.Title, Body: r.Body, Description: r.Description,
		PurposeID: r.PurposeID, CapabilityID: r.CapabilityID, DifficultyID: r.DifficultyID,
		Visibility: r.Visibility, SensitiveLevel: r.SensitiveLevel, UploaderID: r.UploaderID,
		OriginalUploaderID: r.OriginalUploaderID, CurrentVersionID: r.CurrentVersionID,
		LatestVersionNo: r.LatestVersionNo, MajorModified: r.MajorModified,
		CommentCount: r.CommentCount, FavoriteCount: r.FavoriteCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, IsHighRisk: r.IsHighRisk,
	}
}

func fieldsOfOwnPrompt(r repo.ListOwnPromptsRow) promptFields {
	return promptFields{
		ID: r.ID, Title: r.Title, Body: r.Body, Description: r.Description,
		PurposeID: r.PurposeID, CapabilityID: r.CapabilityID, DifficultyID: r.DifficultyID,
		Visibility: r.Visibility, SensitiveLevel: r.SensitiveLevel, UploaderID: r.UploaderID,
		OriginalUploaderID: r.OriginalUploaderID, CurrentVersionID: r.CurrentVersionID,
		LatestVersionNo: r.LatestVersionNo, MajorModified: r.MajorModified,
		CommentCount: r.CommentCount, FavoriteCount: r.FavoriteCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, IsHighRisk: r.IsHighRisk,
	}
}

// promptFromRow 把数据库行映射为提示词实体。可见性、敏感级别与版本号的取值受
// CHECK 约束与写入路径保护，出现未登记取值说明库被外部改动，直接返回错误而不静默降级。
func promptFromRow(f promptFields) (domain.Prompt, error) {
	visibility, ok := domain.ParseVisibility(f.Visibility)
	if !ok {
		return domain.Prompt{}, fmt.Errorf("提示词 %d 的可见性取值 %q 未登记", f.ID, f.Visibility)
	}
	sensitive, ok := domain.ParseSensitiveLevel(f.SensitiveLevel)
	if !ok {
		return domain.Prompt{}, fmt.Errorf("提示词 %d 的敏感级别取值 %q 未登记", f.ID, f.SensitiveLevel)
	}
	version, err := domain.ParseVersion(f.LatestVersionNo)
	if err != nil {
		return domain.Prompt{}, fmt.Errorf("提示词 %d: %w", f.ID, err)
	}

	out := domain.Prompt{
		ID:            f.ID,
		Title:         f.Title,
		Body:          f.Body,
		Description:   f.Description,
		PurposeID:     f.PurposeID,
		CapabilityID:  f.CapabilityID,
		Visibility:    visibility,
		Sensitive:     sensitive,
		HighRisk:      f.IsHighRisk,
		UploaderID:    f.UploaderID,
		LatestVersion: version,
		MajorModified: f.MajorModified,
		CommentCount:  f.CommentCount,
		FavoriteCount: f.FavoriteCount,
	}
	if f.DifficultyID.Valid {
		out.DifficultyID = &f.DifficultyID.Int64
	}
	if f.OriginalUploaderID.Valid {
		out.OriginalUploaderID = &f.OriginalUploaderID.Int64
	}
	if f.CurrentVersionID.Valid {
		out.CurrentVersionID = &f.CurrentVersionID.Int64
	}
	if f.CreatedAt.Valid {
		out.CreatedAt = f.CreatedAt.Time
	}
	if f.UpdatedAt.Valid {
		out.UpdatedAt = f.UpdatedAt.Time
	}
	return out, nil
}

func loadPurpose(ctx context.Context, q *repo.Queries, id int64) (domain.Purpose, error) {
	row, err := q.GetPurpose(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Purpose{}, apperr.New(apperr.CodeInvalidClassification, "考察目的不存在")
		}
		return domain.Purpose{}, fmt.Errorf("查询考察目的: %w", err)
	}
	if !row.IsEnabled {
		return domain.Purpose{}, apperr.New(apperr.CodeInvalidClassification, "考察目的已停用")
	}
	return domain.Purpose{
		ID:            row.ID,
		Slug:          row.Slug,
		NameZh:        row.NameZh,
		NameEn:        row.NameEn,
		DescriptionZh: row.DescriptionZh,
		DescriptionEn: row.DescriptionEn,
		IsHighRisk:    row.IsHighRisk,
		IsEnabled:     row.IsEnabled,
		SortOrder:     row.SortOrder,
	}, nil
}

func loadCapability(ctx context.Context, q *repo.Queries, id int64) (domain.Capability, error) {
	row, err := q.GetCapability(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Capability{}, apperr.New(apperr.CodeInvalidClassification, "考察维度不存在")
		}
		return domain.Capability{}, fmt.Errorf("查询考察维度: %w", err)
	}
	if !row.IsEnabled {
		return domain.Capability{}, apperr.New(apperr.CodeInvalidClassification, "考察维度已停用")
	}
	return domain.Capability{
		ID:            row.ID,
		Slug:          row.Slug,
		NameZh:        row.NameZh,
		NameEn:        row.NameEn,
		GroupKey:      row.GroupKey,
		DescriptionZh: row.DescriptionZh,
		DescriptionEn: row.DescriptionEn,
		IsEnabled:     row.IsEnabled,
		SortOrder:     row.SortOrder,
	}, nil
}

// normalizeCapabilityIDs 合并单值兼容字段，去重并限制在 1 至 5 个。
func normalizeCapabilityIDs(in Input) ([]int64, error) {
	raw := in.CapabilityIDs
	if len(raw) == 0 && in.CapabilityID > 0 {
		raw = []int64{in.CapabilityID}
	}
	seen := make(map[int64]struct{}, len(raw))
	out := make([]int64, 0, len(raw))
	for _, id := range raw {
		if id <= 0 {
			return nil, apperr.New(apperr.CodeInvalidClassification, "考察维度取值非法")
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, apperr.New(apperr.CodeInvalidClassification, "考察维度不可为空")
	}
	if len(out) > domain.PromptCapabilityLimit {
		return nil, apperr.New(apperr.CodeInvalidClassification,
			fmt.Sprintf("考察维度最多选择 %d 个", domain.PromptCapabilityLimit))
	}
	return out, nil
}

// validateReferences 校验考察维度与可选字典项存在且启用，返回规范化后的维度集合。
// 考察目的必填由 loadPurpose 负责。
func (s *Service) validateReferences(ctx context.Context, q *repo.Queries, in Input) ([]int64, error) {
	capabilityIDs, err := normalizeCapabilityIDs(in)
	if err != nil {
		return nil, err
	}
	for _, capabilityID := range capabilityIDs {
		if _, err := loadCapability(ctx, q, capabilityID); err != nil {
			return nil, err
		}
	}
	if in.DifficultyID != nil {
		row, err := q.GetDifficulty(ctx, *in.DifficultyID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, apperr.New(apperr.CodeInvalidClassification, "难度不存在")
			}
			return nil, fmt.Errorf("查询难度: %w", err)
		}
		if !row.IsEnabled {
			return nil, apperr.New(apperr.CodeInvalidClassification, "难度已停用")
		}
	}
	if len(in.TagIDs) > 0 {
		rows, err := q.GetTagsByIDs(ctx, in.TagIDs)
		if err != nil {
			return nil, fmt.Errorf("查询标签: %w", err)
		}
		if len(rows) != len(in.TagIDs) {
			return nil, apperr.New(apperr.CodeInvalidClassification, "标签不存在")
		}
		for _, row := range rows {
			if !row.IsEnabled {
				return nil, apperr.New(apperr.CodeInvalidClassification, "标签已停用")
			}
		}
	}
	return capabilityIDs, nil
}

// replaceCapabilities 重设提示词的考察维度集合。整体替换使取消勾选可以生效。
func replaceCapabilities(ctx context.Context, q *repo.Queries, promptID int64, capabilityIDs []int64) error {
	if err := q.ReplacePromptCapabilities(ctx, promptID); err != nil {
		return fmt.Errorf("清空提示词考察维度: %w", err)
	}
	for index, capabilityID := range capabilityIDs {
		if err := q.AttachPromptCapability(ctx, repo.AttachPromptCapabilityParams{
			PromptID:     promptID,
			CapabilityID: capabilityID,
			SortOrder:    int16(index),
		}); err != nil {
			return fmt.Errorf("写入提示词考察维度: %w", err)
		}
	}
	return nil
}

// attachVersionCapabilities 保存版本发布时的考察维度集合。
func attachVersionCapabilities(ctx context.Context, q *repo.Queries, versionID int64, capabilityIDs []int64) error {
	for index, capabilityID := range capabilityIDs {
		if err := q.AttachPromptVersionCapability(ctx, repo.AttachPromptVersionCapabilityParams{
			PromptVersionID: versionID,
			CapabilityID:    capabilityID,
			SortOrder:       int16(index),
		}); err != nil {
			return fmt.Errorf("写入版本考察维度: %w", err)
		}
	}
	return nil
}

// replaceTags 重设提示词的标签集合。整体替换使「取消勾选」能真正生效，
// 追加语义会让移除标签无法表达。
func replaceTags(ctx context.Context, q *repo.Queries, promptID int64, tagIDs []int64) error {
	if err := q.ReplacePromptTags(ctx, promptID); err != nil {
		return fmt.Errorf("清空提示词标签: %w", err)
	}
	for _, tagID := range tagIDs {
		if err := q.AttachPromptTag(ctx, repo.AttachPromptTagParams{
			PromptID: promptID,
			TagID:    tagID,
		}); err != nil {
			return fmt.Errorf("写入提示词标签: %w", err)
		}
	}
	return nil
}

// checkPrivateQuota 在事务内判定私有提示词额度（C-21）。
// 先按用户取咨询锁再计数：并发创建各自读到未满后都插入会突破上限，
// 锁使「计数 → 判定 → 插入」串行化（与收藏夹、题库配额同一纪律）。
func (s *Service) checkPrivateQuota(ctx context.Context, q *repo.Queries, viewer domain.Viewer, role domain.Role) error {
	if err := q.LockPromptOwner(ctx, viewer.UserID); err != nil {
		return fmt.Errorf("锁定私有提示词额度: %w", err)
	}
	limit, err := s.privateQuota(ctx, role)
	if err != nil {
		return err
	}
	count, err := q.CountPrivatePrompts(ctx, viewer.UserID)
	if err != nil {
		return fmt.Errorf("统计私有提示词数量: %w", err)
	}
	if count >= limit {
		return apperr.New(apperr.CodeQuotaExceeded, "私有提示词额度已用尽")
	}
	return nil
}

// privateQuota 按角色取私有提示词上限（需求 §8.3）。
func (s *Service) privateQuota(ctx context.Context, role domain.Role) (int64, error) {
	key := "private_prompt_quota_user"
	if role.IsDeveloper() {
		key = "private_prompt_quota_developer"
	}
	limit, err := s.config.GetInt(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("读取私有提示词额度配置: %w", err)
	}
	return limit, nil
}
