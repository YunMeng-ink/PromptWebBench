package admin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/repo"
)

// TagInput 是标签的写入字段（需求 §17.5）。
type TagInput struct {
	Slug        string
	NameZh      string
	NameEn      string
	Color       string
	Description string
	Kind        string
	Priority    int32
	IsEnabled   bool
}

// colorPattern 是标签颜色的形态：#RRGGBB 或 #RGB。
var colorPattern = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// normalize 校验并规范化标签字段。
func (in TagInput) normalize() (TagInput, error) {
	out := in
	out.Slug = strings.TrimSpace(in.Slug)
	out.NameZh = strings.TrimSpace(in.NameZh)
	out.NameEn = strings.TrimSpace(in.NameEn)
	out.Color = strings.TrimSpace(in.Color)
	out.Description = strings.TrimSpace(in.Description)
	out.Kind = strings.TrimSpace(in.Kind)
	if out.Kind == "" {
		out.Kind = "theme"
	}
	if out.Kind != "theme" && out.Kind != "form" {
		return TagInput{}, apperr.New(apperr.CodeValidationFailed, "标签类型只能是 theme 或 form")
	}

	if out.Slug == "" {
		return TagInput{}, apperr.New(apperr.CodeValidationFailed, "标签标识不可为空")
	}
	if out.NameZh == "" || out.NameEn == "" {
		return TagInput{}, apperr.New(apperr.CodeValidationFailed, "标签的中英文名称都不可为空")
	}
	if !colorPattern.MatchString(out.Color) {
		return TagInput{}, apperr.New(apperr.CodeValidationFailed, "标签颜色需要是 #RRGGBB 或 #RGB 形态")
	}
	return out, nil
}

// ListTags 返回标签列表，附带每个标签被多少条提示词使用（需求 §17.5）。
//
// 使用数同时是删除前的引用提示：管理员据此判断删除会影响多少提示词。
func (s *Service) ListTags(ctx context.Context, keyword string, page, pageSize int32) (Page[repo.AdminListTagsRow], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.AdminListTagsRow]{}, err
	}
	q := repo.New(s.pool)
	params := repo.AdminListTagsParams{
		Keyword: trimSpace(keyword), LimitCount: pageSize, OffsetCount: (page - 1) * pageSize,
	}
	rows, err := q.AdminListTags(ctx, params)
	if err != nil {
		return Page[repo.AdminListTagsRow]{}, fmt.Errorf("查询标签列表: %w", err)
	}
	total, err := q.AdminCountTags(ctx, params.Keyword)
	if err != nil {
		return Page[repo.AdminListTagsRow]{}, fmt.Errorf("统计标签数量: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// CreateTag 创建标签（需求 §17.5）。
func (s *Service) CreateTag(ctx context.Context, in TagInput) (repo.Tag, error) {
	if err := s.ensure(); err != nil {
		return repo.Tag{}, err
	}
	normalized, err := in.normalize()
	if err != nil {
		return repo.Tag{}, err
	}
	row, err := repo.New(s.pool).InsertTag(ctx, repo.InsertTagParams{
		Slug:        normalized.Slug,
		NameZh:      normalized.NameZh,
		NameEn:      normalized.NameEn,
		Color:       normalized.Color,
		Description: normalized.Description,
		Kind:        normalized.Kind,
		Priority:    normalized.Priority,
		IsEnabled:   normalized.IsEnabled,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return repo.Tag{}, apperr.New(apperr.CodeConflict, "标签标识已存在")
		}
		return repo.Tag{}, fmt.Errorf("写入标签: %w", err)
	}
	return row, nil
}

// UpdateTag 编辑标签（需求 §17.5：改名称、颜色、说明、优先级与启用状态）。
func (s *Service) UpdateTag(ctx context.Context, id int64, in TagInput) (repo.Tag, error) {
	if err := s.ensure(); err != nil {
		return repo.Tag{}, err
	}
	normalized, err := in.normalize()
	if err != nil {
		return repo.Tag{}, err
	}
	row, err := repo.New(s.pool).UpdateTag(ctx, repo.UpdateTagParams{
		ID:          id,
		Slug:        normalized.Slug,
		NameZh:      normalized.NameZh,
		NameEn:      normalized.NameEn,
		Color:       normalized.Color,
		Description: normalized.Description,
		Kind:        normalized.Kind,
		Priority:    normalized.Priority,
		IsEnabled:   normalized.IsEnabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Tag{}, notFound("标签")
		}
		if isUniqueViolation(err) {
			return repo.Tag{}, apperr.New(apperr.CodeConflict, "标签标识已存在")
		}
		return repo.Tag{}, fmt.Errorf("更新标签: %w", err)
	}
	return row, nil
}

// DeleteTag 删除标签（需求 §17.5）。prompt_tags 的外键为级联删除，
// 标签被移除后相关提示词不再携带该标签。
func (s *Service) DeleteTag(ctx context.Context, id int64) error {
	if err := s.ensure(); err != nil {
		return err
	}
	q := repo.New(s.pool)
	if _, err := q.AdminGetTag(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound("标签")
		}
		return fmt.Errorf("查询标签: %w", err)
	}
	if err := q.DeleteTag(ctx, id); err != nil {
		return fmt.Errorf("删除标签: %w", err)
	}
	return nil
}
