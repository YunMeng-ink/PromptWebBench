package prompt

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// Detail 是详情页所需的全部数据。
type Detail struct {
	Prompt     domain.Prompt
	Version    domain.PromptVersion
	Purpose    domain.Purpose
	Capability domain.Capability
	Difficulty *domain.Difficulty
	Tags       []domain.Tag
	Versions   []domain.PromptVersion
	Uploader   string
}

// Get 读取提示词详情。
//
// 可见性判定调用 domain.Viewer.CanView，判定不通过一律返回 not_found
// （开发方案 §5.3）：高风险考察目的即便可见性为 public，对普通用户与游客同样 404（C-20）。
//
// 本方法是「内容访问」的唯一入口：高风险考察目的内容在这里写入审计日志（G-12），
// 授予与拒绝都留痕。只做可见性闸门、不读取内容的调用方应改用 Visible。
func (s *Service) Get(ctx context.Context, v domain.Viewer, id int64, ip string) (Detail, error) {
	q := repo.New(s.pool)
	row, err := q.GetPrompt(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, apperr.New(apperr.CodeNotFound, "")
		}
		return Detail{}, fmt.Errorf("查询提示词: %w", err)
	}

	prompt, err := promptFromRow(fieldsOfGetPrompt(row))
	if err != nil {
		return Detail{}, err
	}

	visible := v.CanView(prompt.Access())
	if prompt.HighRisk {
		if err := s.auditSensitiveAccess(ctx, v, prompt.ID, ip, visible); err != nil {
			return Detail{}, err
		}
	}
	if !visible {
		return Detail{}, apperr.New(apperr.CodeNotFound, "")
	}

	versions, err := q.ListPromptVersions(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("查询提示词版本: %w", err)
	}
	if len(versions) == 0 {
		return Detail{}, apperr.New(apperr.CodeInternal, "提示词缺少历史版本")
	}

	current := versions[0]
	tags, err := q.ListPromptTags(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("查询提示词标签: %w", err)
	}

	purposeRow, err := q.GetPurpose(ctx, prompt.PurposeID)
	if err != nil {
		return Detail{}, fmt.Errorf("查询考察目的: %w", err)
	}
	capabilityRow, err := q.GetCapability(ctx, prompt.CapabilityID)
	if err != nil {
		return Detail{}, fmt.Errorf("查询考察维度: %w", err)
	}
	var difficulty *domain.Difficulty
	if prompt.DifficultyID != nil {
		row, err := q.GetDifficulty(ctx, *prompt.DifficultyID)
		if err != nil {
			return Detail{}, fmt.Errorf("查询难度: %w", err)
		}
		difficulty = &domain.Difficulty{
			ID: row.ID, Slug: row.Slug, NameZh: row.NameZh, NameEn: row.NameEn,
			Level: row.Level, IsEnabled: row.IsEnabled,
		}
	}

	// _ 标识此处刻意不处理 GetUserByID 的失败：提示词已可访问时，
	// 上传者昵称缺失不应让详情页失败，前端据此显示 ID 兜底。
	uploader, _ := q.GetUserByID(ctx, row.UploaderID)

	return Detail{
		Prompt:  prompt,
		Version: VersionFromRow(current),
		Purpose: domain.Purpose{
			ID: purposeRow.ID, Slug: purposeRow.Slug, NameZh: purposeRow.NameZh,
			NameEn: purposeRow.NameEn, DescriptionZh: purposeRow.DescriptionZh,
			DescriptionEn: purposeRow.DescriptionEn, IsHighRisk: purposeRow.IsHighRisk,
			IsEnabled: purposeRow.IsEnabled, SortOrder: purposeRow.SortOrder,
		},
		Capability: domain.Capability{
			ID: capabilityRow.ID, Slug: capabilityRow.Slug, NameZh: capabilityRow.NameZh,
			NameEn: capabilityRow.NameEn, GroupKey: capabilityRow.GroupKey,
			DescriptionZh: capabilityRow.DescriptionZh, DescriptionEn: capabilityRow.DescriptionEn,
			IsEnabled: capabilityRow.IsEnabled, SortOrder: capabilityRow.SortOrder,
		},
		Difficulty: difficulty,
		Tags:       tagsFromRows(tags),
		Versions:   VersionsFromRows(versions),
		Uploader:   uploader.Username,
	}, nil
}

// Visible 校验当前身份能否看到该提示词，不读取详情内容。
// 供「以可见性为闸门、不需要内容」的调用方使用（如评分列表），
// 使审计记录只由真正的详情读取产生。
func (s *Service) Visible(ctx context.Context, v domain.Viewer, id int64) error {
	row, err := repo.New(s.pool).GetPrompt(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.CodeNotFound, "")
		}
		return fmt.Errorf("查询提示词: %w", err)
	}
	prompt, err := promptFromRow(fieldsOfGetPrompt(row))
	if err != nil {
		return err
	}
	if !v.CanView(prompt.Access()) {
		return apperr.New(apperr.CodeNotFound, "")
	}
	return nil
}

// auditSensitiveAccess 为高风险考察目的内容的一次访问写入审计日志（G-12）。
// granted 为假表示本次访问被可见性判定拒绝，同样留痕：被拒访问正是管控对象。
func (s *Service) auditSensitiveAccess(ctx context.Context, v domain.Viewer, promptID int64, ip string, granted bool) error {
	name := "游客"
	if v.Logged {
		user, err := repo.New(s.pool).GetUserByID(ctx, v.UserID)
		if err != nil {
			return fmt.Errorf("查询访问者 %d: %w", v.UserID, err)
		}
		name = user.Username
	}
	result := "denied"
	if granted {
		result = "granted"
	}
	return s.oplog.Record(ctx, repo.New(s.pool), oplog.Entry{
		OperatorID:   v.UserID,
		OperatorName: name,
		Action:       oplog.ActionSensitiveContentAccess,
		ObjectType:   "prompt",
		ObjectID:     promptID,
		After: map[string]any{
			"result":      result,
			"viewer_role": string(v.Role),
			"logged":      v.Logged,
		},
		Note: result,
		IP:   ip,
	})
}

// lockPrompt 在事务内锁定提示词行，供写入路径使用（开发方案 §3.5）。
func lockPrompt(ctx context.Context, q *repo.Queries, id int64) (domain.Prompt, error) {
	row, err := q.LockPrompt(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Prompt{}, apperr.New(apperr.CodeNotFound, "")
		}
		return domain.Prompt{}, fmt.Errorf("锁定提示词: %w", err)
	}
	prompt, err := promptFromRow(fieldsOfLockPrompt(row))
	if err != nil {
		return domain.Prompt{}, err
	}
	return prompt, nil
}

// List 返回公开列表。可见性谓词由 domain.Viewer.ListScope 生成并注入查询，
// 6 维筛选与 3 种排序在 repo 层的动态查询中实现。
func (s *Service) List(ctx context.Context, v domain.Viewer, f repo.PromptListFilter, sort domain.SortOrder, page, pageSize int32) (repo.PromptList, error) {
	return repo.PromptListPage(ctx, s.pool, v, f, sort, page, pageSize)
}

// ListOwn 列出当前用户上传的提示词。可见性由 domain.Viewer.OwnScope 判定归属，
// 任意可见性与高风险标记都在结果内——上传者当然能看到自己全部的内容。
func (s *Service) ListOwn(ctx context.Context, v domain.Viewer, page, pageSize int32) (repo.PromptList, error) {
	q := repo.New(s.pool)
	rows, err := q.ListOwnPrompts(ctx, repo.ListOwnPromptsParams{
		UploaderID: v.UserID,
		Limit:      pageSize,
		Offset:     (page - 1) * pageSize,
	})
	if err != nil {
		return repo.PromptList{}, fmt.Errorf("查询我的提示词: %w", err)
	}
	total, err := q.CountOwnPrompts(ctx, v.UserID)
	if err != nil {
		return repo.PromptList{}, fmt.Errorf("统计我的提示词数量: %w", err)
	}

	items := make([]repo.PromptListItem, 0, len(rows))
	for _, row := range rows {
		prompt, err := promptFromRow(fieldsOfOwnPrompt(row))
		if err != nil {
			return repo.PromptList{}, err
		}
		items = append(items, repo.PromptListItem{Prompt: prompt})
	}
	return repo.PromptList{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// VersionFromRow 把版本行映射为版本实体。导出供管理后台复用同一映射规则。
func VersionFromRow(row repo.PromptVersion) domain.PromptVersion {
	out := domain.PromptVersion{
		ID:           row.ID,
		PromptID:     row.PromptID,
		Version:      domain.Version{Major: int(row.Major), Minor: int(row.Minor)},
		Title:        row.Title,
		Body:         row.Body,
		Description:  row.Description,
		PurposeID:    row.PurposeID,
		CapabilityID: row.CapabilityID,
		Visibility:   domain.Visibility(row.Visibility),
		Sensitive:    domain.SensitiveLevel(row.SensitiveLevel),
		IsMajor:      row.IsMajor,
		ChangeNote:   row.ChangeNote,
		AuthorID:     row.AuthorID,
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	if row.DifficultyID.Valid {
		out.DifficultyID = &row.DifficultyID.Int64
	}
	return out
}

// VersionsFromRows 批量映射版本行。
func VersionsFromRows(rows []repo.PromptVersion) []domain.PromptVersion {
	out := make([]domain.PromptVersion, 0, len(rows))
	for _, row := range rows {
		out = append(out, VersionFromRow(row))
	}
	return out
}

func tagsFromRows(rows []repo.Tag) []domain.Tag {
	out := make([]domain.Tag, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Tag{
			ID:       row.ID,
			Slug:     row.Slug,
			NameZh:   row.NameZh,
			NameEn:   row.NameEn,
			Color:    row.Color,
			Kind:     row.Kind,
			Priority: row.Priority,
		})
	}
	return out
}
