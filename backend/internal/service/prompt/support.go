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
	if errors.Is(err, domain.ErrHighRiskCategoryForbidden) {
		return apperr.New(apperr.CodeForbidden, "普通用户不能选择高风险类别")
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
	CategoryID         int64
	AbilityID          pgtype.Int8
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
		CategoryID: r.CategoryID, AbilityID: r.AbilityID, DifficultyID: r.DifficultyID,
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
		CategoryID: r.CategoryID, AbilityID: r.AbilityID, DifficultyID: r.DifficultyID,
		Visibility: r.Visibility, SensitiveLevel: r.SensitiveLevel, UploaderID: r.UploaderID,
		OriginalUploaderID: r.OriginalUploaderID, CurrentVersionID: r.CurrentVersionID,
		LatestVersionNo: r.LatestVersionNo, MajorModified: r.MajorModified,
		CommentCount: r.CommentCount, FavoriteCount: r.FavoriteCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, IsHighRisk: r.IsHighRisk,
	}
}

// fieldsOfIncludingDeleted 映射包含回收站条目的读取行。类别名称不入 promptFields：
// 它们只服务于后台列表的展示，映射到实体上会被后续写入路径误当成可写字段。
func fieldsOfIncludingDeleted(r repo.GetPromptIncludingDeletedRow) promptFields {
	return promptFields{
		ID: r.ID, Title: r.Title, Body: r.Body, Description: r.Description,
		CategoryID: r.CategoryID, AbilityID: r.AbilityID, DifficultyID: r.DifficultyID,
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
		CategoryID: r.CategoryID, AbilityID: r.AbilityID, DifficultyID: r.DifficultyID,
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
		CategoryID: r.CategoryID, AbilityID: r.AbilityID, DifficultyID: r.DifficultyID,
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
		CategoryID:    f.CategoryID,
		Visibility:    visibility,
		Sensitive:     sensitive,
		HighRisk:      f.IsHighRisk,
		UploaderID:    f.UploaderID,
		LatestVersion: version,
		MajorModified: f.MajorModified,
		CommentCount:  f.CommentCount,
		FavoriteCount: f.FavoriteCount,
	}
	if f.AbilityID.Valid {
		out.AbilityID = &f.AbilityID.Int64
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

func loadCategory(ctx context.Context, q *repo.Queries, id int64) (domain.Category, error) {
	row, err := q.GetCategory(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Category{}, apperr.New(apperr.CodeInvalidCategory, "类别不存在")
		}
		return domain.Category{}, fmt.Errorf("查询类别: %w", err)
	}
	if !row.IsEnabled {
		return domain.Category{}, apperr.New(apperr.CodeInvalidCategory, "类别已停用")
	}
	return domain.Category{
		ID:         row.ID,
		Slug:       row.Slug,
		NameZh:     row.NameZh,
		NameEn:     row.NameEn,
		IsHighRisk: row.IsHighRisk,
		IsEnabled:  row.IsEnabled,
		SortOrder:  row.SortOrder,
	}, nil
}

// validateReferences 校验可选字典项存在且启用。类别必填由 loadCategory 负责。
func (s *Service) validateReferences(ctx context.Context, q *repo.Queries, in Input) error {
	if in.AbilityID != nil {
		row, err := q.GetAbility(ctx, *in.AbilityID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidCategory, "能力维度不存在")
			}
			return fmt.Errorf("查询能力维度: %w", err)
		}
		if !row.IsEnabled {
			return apperr.New(apperr.CodeInvalidCategory, "能力维度已停用")
		}
	}
	if in.DifficultyID != nil {
		row, err := q.GetDifficulty(ctx, *in.DifficultyID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.CodeInvalidCategory, "难度不存在")
			}
			return fmt.Errorf("查询难度: %w", err)
		}
		if !row.IsEnabled {
			return apperr.New(apperr.CodeInvalidCategory, "难度已停用")
		}
	}
	if len(in.TagIDs) > 0 {
		rows, err := q.GetTagsByIDs(ctx, in.TagIDs)
		if err != nil {
			return fmt.Errorf("查询标签: %w", err)
		}
		if len(rows) != len(in.TagIDs) {
			return apperr.New(apperr.CodeInvalidCategory, "标签不存在")
		}
		for _, row := range rows {
			if !row.IsEnabled {
				return apperr.New(apperr.CodeInvalidCategory, "标签已停用")
			}
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
// 调用方已持有同一事务，且在写入前完成判定，不存在事务外计数后插入的窗口。
func (s *Service) checkPrivateQuota(ctx context.Context, q *repo.Queries, viewer domain.Viewer, role domain.Role) error {
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
