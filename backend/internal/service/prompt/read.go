package prompt

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Detail 是详情页所需的全部数据。
type Detail struct {
	Prompt   domain.Prompt
	Version  domain.PromptVersion
	Category domain.Category
	Tags     []domain.Tag
	Versions []domain.PromptVersion
	Uploader string
}

// Get 读取提示词详情。
//
// 可见性判定调用 domain.Viewer.CanView，判定不通过一律返回 not_found
// （开发方案 §5.3）：高风险类别即便可见性为 public，对普通用户与游客同样 404（C-20）。
func (s *Service) Get(ctx context.Context, v domain.Viewer, id int64) (Detail, error) {
	row, err := repo.New(s.pool).GetPrompt(ctx, id)
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

	if !v.CanView(prompt.Access()) {
		return Detail{}, apperr.New(apperr.CodeNotFound, "")
	}

	versions, err := repo.New(s.pool).ListPromptVersions(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("查询提示词版本: %w", err)
	}
	if len(versions) == 0 {
		return Detail{}, apperr.New(apperr.CodeInternal, "提示词缺少历史版本")
	}

	current := versions[0]
	tags, err := repo.New(s.pool).ListPromptTags(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("查询提示词标签: %w", err)
	}

	var category domain.Category
	if cat, err := repo.New(s.pool).GetCategory(ctx, row.CategoryID); err == nil {
		category = domain.Category{
			ID: cat.ID, Slug: cat.Slug, NameZh: cat.NameZh, NameEn: cat.NameEn,
			IsHighRisk: cat.IsHighRisk, IsEnabled: cat.IsEnabled, SortOrder: cat.SortOrder,
		}
	}

	// _ 标识此处刻意不处理 GetUserByID 的失败：提示词已可访问时，
	// 上传者昵称缺失不应让详情页失败，前端据此显示 ID 兜底。
	uploader, _ := repo.New(s.pool).GetUserByID(ctx, row.UploaderID)

	return Detail{
		Prompt:   prompt,
		Version:  versionFromRow(current),
		Category: category,
		Tags:     tagsFromRows(tags),
		Versions: versionListFromRows(versions),
		Uploader: uploader.Username,
	}, nil
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

// Delete 软删除提示词，进入回收站（需求 §11）。仅所有者或管理员可删除（C-18）。
func (s *Service) Delete(ctx context.Context, actor domain.User, id int64) error {
	viewer := domain.ViewerFor(actor)
	if !viewer.CanWrite() {
		return apperr.New(apperr.CodeEmailNotVerified, "")
	}
	return repo.InTx(ctx, s.pool, func(q *repo.Queries) error {
		locked, err := lockPrompt(ctx, q, id)
		if err != nil {
			return err
		}
		if !viewer.CanManage(locked.Access()) {
			return apperr.New(apperr.CodeNotFound, "")
		}
		if _, err := q.SoftDeletePrompt(ctx, id); err != nil {
			return fmt.Errorf("软删除提示词: %w", err)
		}
		return nil
	})
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

func versionFromRow(row repo.PromptVersion) domain.PromptVersion {
	out := domain.PromptVersion{
		ID:          row.ID,
		PromptID:    row.PromptID,
		Version:     domain.Version{Major: int(row.Major), Minor: int(row.Minor)},
		Title:       row.Title,
		Body:        row.Body,
		Description: row.Description,
		CategoryID:  row.CategoryID,
		Visibility:  domain.Visibility(row.Visibility),
		Sensitive:   domain.SensitiveLevel(row.SensitiveLevel),
		IsMajor:     row.IsMajor,
		ChangeNote:  row.ChangeNote,
		AuthorID:    row.AuthorID,
	}
	if row.CreatedAt.Valid {
		out.CreatedAt = row.CreatedAt.Time
	}
	if row.AbilityID.Valid {
		out.AbilityID = &row.AbilityID.Int64
	}
	if row.DifficultyID.Valid {
		out.DifficultyID = &row.DifficultyID.Int64
	}
	return out
}

func versionListFromRows(rows []repo.PromptVersion) []domain.PromptVersion {
	out := make([]domain.PromptVersion, 0, len(rows))
	for _, row := range rows {
		out = append(out, versionFromRow(row))
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
			Priority: row.Priority,
		})
	}
	return out
}
