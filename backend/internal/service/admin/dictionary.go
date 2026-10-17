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

var dictionarySlugPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// PurposeInput 是考察目的的写入字段。slug 只在创建时提供，创建后不可修改。
type PurposeInput struct {
	Slug          string
	NameZh        string
	NameEn        string
	DescriptionZh string
	DescriptionEn string
	IsHighRisk    bool
	IsEnabled     bool
	SortOrder     int32
}

// PurposeUpdate 是考察目的的编辑字段。
type PurposeUpdate struct {
	NameZh        string
	NameEn        string
	DescriptionZh string
	DescriptionEn string
	IsHighRisk    bool
	IsEnabled     bool
	SortOrder     int32
}

func normalizeDictionaryNames(slug, nameZh, nameEn string) (string, string, string, error) {
	slug = strings.TrimSpace(slug)
	nameZh = strings.TrimSpace(nameZh)
	nameEn = strings.TrimSpace(nameEn)
	if !dictionarySlugPattern.MatchString(slug) {
		return "", "", "", apperr.New(apperr.CodeValidationFailed,
			"标识需要以小写字母开头，只包含小写字母、数字与下划线，最长 50 个字符")
	}
	if nameZh == "" || nameEn == "" {
		return "", "", "", apperr.New(apperr.CodeValidationFailed, "中英文名称不可为空")
	}
	if len([]rune(nameZh)) > 50 || len([]rune(nameEn)) > 50 {
		return "", "", "", apperr.New(apperr.CodeValidationFailed, "名称不能超过 50 个字符")
	}
	return slug, nameZh, nameEn, nil
}

func normalizeDescriptions(descriptionZh, descriptionEn string) (string, string, error) {
	descriptionZh = strings.TrimSpace(descriptionZh)
	descriptionEn = strings.TrimSpace(descriptionEn)
	if len([]rune(descriptionZh)) > 500 || len([]rune(descriptionEn)) > 500 {
		return "", "", apperr.New(apperr.CodeValidationFailed, "定义不能超过 500 个字符")
	}
	return descriptionZh, descriptionEn, nil
}

// ListPurposes 返回考察目的列表，包含停用条目（需求 §17 后台字典维护）。
func (s *Service) ListPurposes(ctx context.Context, keyword string, page, pageSize int32) (Page[repo.Purpose], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.Purpose]{}, err
	}
	q := repo.New(s.pool)
	params := repo.AdminListPurposesParams{
		Keyword:     strings.TrimSpace(keyword),
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	}
	rows, err := q.AdminListPurposes(ctx, params)
	if err != nil {
		return Page[repo.Purpose]{}, fmt.Errorf("查询考察目的: %w", err)
	}
	total, err := q.AdminCountPurposes(ctx, params.Keyword)
	if err != nil {
		return Page[repo.Purpose]{}, fmt.Errorf("统计考察目的: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// CreatePurpose 新建考察目的。
func (s *Service) CreatePurpose(ctx context.Context, in PurposeInput) (repo.Purpose, error) {
	if err := s.ensure(); err != nil {
		return repo.Purpose{}, err
	}
	slug, nameZh, nameEn, err := normalizeDictionaryNames(in.Slug, in.NameZh, in.NameEn)
	if err != nil {
		return repo.Purpose{}, err
	}
	descriptionZh, descriptionEn, err := normalizeDescriptions(in.DescriptionZh, in.DescriptionEn)
	if err != nil {
		return repo.Purpose{}, err
	}
	row, err := repo.New(s.pool).InsertPurpose(ctx, repo.InsertPurposeParams{
		Slug: slug, NameZh: nameZh, NameEn: nameEn,
		DescriptionZh: descriptionZh, DescriptionEn: descriptionEn,
		IsHighRisk: in.IsHighRisk, IsEnabled: in.IsEnabled, SortOrder: in.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return repo.Purpose{}, apperr.New(apperr.CodeConflict, "考察目的标识已存在")
		}
		return repo.Purpose{}, fmt.Errorf("写入考察目的: %w", err)
	}
	return row, nil
}

// UpdatePurpose 编辑考察目的。标识创建后不可修改，避免破坏外部引用。
func (s *Service) UpdatePurpose(ctx context.Context, id int64, in PurposeUpdate) (repo.Purpose, error) {
	if err := s.ensure(); err != nil {
		return repo.Purpose{}, err
	}
	nameZh := strings.TrimSpace(in.NameZh)
	nameEn := strings.TrimSpace(in.NameEn)
	if nameZh == "" || nameEn == "" {
		return repo.Purpose{}, apperr.New(apperr.CodeValidationFailed, "中英文名称不可为空")
	}
	if len([]rune(nameZh)) > 50 || len([]rune(nameEn)) > 50 {
		return repo.Purpose{}, apperr.New(apperr.CodeValidationFailed, "名称不能超过 50 个字符")
	}
	descriptionZh, descriptionEn, err := normalizeDescriptions(in.DescriptionZh, in.DescriptionEn)
	if err != nil {
		return repo.Purpose{}, err
	}
	q := repo.New(s.pool)
	if _, err := q.AdminGetPurpose(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Purpose{}, apperr.New(apperr.CodeNotFound, "")
		}
		return repo.Purpose{}, fmt.Errorf("查询考察目的: %w", err)
	}
	row, err := q.UpdatePurpose(ctx, repo.UpdatePurposeParams{
		ID: id, NameZh: nameZh, NameEn: nameEn,
		DescriptionZh: descriptionZh, DescriptionEn: descriptionEn,
		IsHighRisk: in.IsHighRisk, IsEnabled: in.IsEnabled, SortOrder: in.SortOrder,
	})
	if err != nil {
		return repo.Purpose{}, fmt.Errorf("更新考察目的: %w", err)
	}
	return row, nil
}

// DeletePurpose 删除未被引用的考察目的。被引用时只能停用。
func (s *Service) DeletePurpose(ctx context.Context, id int64) error {
	if err := s.ensure(); err != nil {
		return err
	}
	q := repo.New(s.pool)
	if _, err := q.AdminGetPurpose(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.CodeNotFound, "")
		}
		return fmt.Errorf("查询考察目的: %w", err)
	}
	references, err := q.CountPurposeReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("统计考察目的引用: %w", err)
	}
	if references > 0 {
		return apperr.New(apperr.CodeDictionaryInUse, "考察目的已被引用，只能停用")
	}
	if err := q.DeletePurpose(ctx, id); err != nil {
		return fmt.Errorf("删除考察目的: %w", err)
	}
	return nil
}

// CapabilityInput 是考察维度的写入字段。slug 只在创建时提供，创建后不可修改。
type CapabilityInput struct {
	Slug          string
	NameZh        string
	NameEn        string
	GroupKey      string
	DescriptionZh string
	DescriptionEn string
	IsEnabled     bool
	SortOrder     int32
}

// CapabilityUpdate 是考察维度的编辑字段。
type CapabilityUpdate struct {
	NameZh        string
	NameEn        string
	GroupKey      string
	DescriptionZh string
	DescriptionEn string
	IsEnabled     bool
	SortOrder     int32
}

func normalizeGroupKey(groupKey string) (string, error) {
	switch groupKey {
	case "cognitive", "execution", "reliability":
		return groupKey, nil
	default:
		return "", apperr.New(apperr.CodeValidationFailed, "分组取值非法")
	}
}

// ListCapabilities 返回考察维度列表，包含停用条目。
func (s *Service) ListCapabilities(ctx context.Context, keyword string, page, pageSize int32) (Page[repo.Capability], error) {
	if err := s.ensure(); err != nil {
		return Page[repo.Capability]{}, err
	}
	q := repo.New(s.pool)
	params := repo.AdminListCapabilitiesParams{
		Keyword:     strings.TrimSpace(keyword),
		LimitCount:  pageSize,
		OffsetCount: (page - 1) * pageSize,
	}
	rows, err := q.AdminListCapabilities(ctx, params)
	if err != nil {
		return Page[repo.Capability]{}, fmt.Errorf("查询考察维度: %w", err)
	}
	total, err := q.AdminCountCapabilities(ctx, params.Keyword)
	if err != nil {
		return Page[repo.Capability]{}, fmt.Errorf("统计考察维度: %w", err)
	}
	return NewPage(rows, page, pageSize, total), nil
}

// CreateCapability 新建考察维度。
func (s *Service) CreateCapability(ctx context.Context, in CapabilityInput) (repo.Capability, error) {
	if err := s.ensure(); err != nil {
		return repo.Capability{}, err
	}
	slug, nameZh, nameEn, err := normalizeDictionaryNames(in.Slug, in.NameZh, in.NameEn)
	if err != nil {
		return repo.Capability{}, err
	}
	groupKey, err := normalizeGroupKey(in.GroupKey)
	if err != nil {
		return repo.Capability{}, err
	}
	descriptionZh, descriptionEn, err := normalizeDescriptions(in.DescriptionZh, in.DescriptionEn)
	if err != nil {
		return repo.Capability{}, err
	}
	row, err := repo.New(s.pool).InsertCapability(ctx, repo.InsertCapabilityParams{
		Slug: slug, NameZh: nameZh, NameEn: nameEn, GroupKey: groupKey,
		DescriptionZh: descriptionZh, DescriptionEn: descriptionEn,
		IsEnabled: in.IsEnabled, SortOrder: in.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return repo.Capability{}, apperr.New(apperr.CodeConflict, "考察维度标识已存在")
		}
		return repo.Capability{}, fmt.Errorf("写入考察维度: %w", err)
	}
	return row, nil
}

// UpdateCapability 编辑考察维度。标识创建后不可修改。
func (s *Service) UpdateCapability(ctx context.Context, id int64, in CapabilityUpdate) (repo.Capability, error) {
	if err := s.ensure(); err != nil {
		return repo.Capability{}, err
	}
	nameZh := strings.TrimSpace(in.NameZh)
	nameEn := strings.TrimSpace(in.NameEn)
	if nameZh == "" || nameEn == "" {
		return repo.Capability{}, apperr.New(apperr.CodeValidationFailed, "中英文名称不可为空")
	}
	if len([]rune(nameZh)) > 50 || len([]rune(nameEn)) > 50 {
		return repo.Capability{}, apperr.New(apperr.CodeValidationFailed, "名称不能超过 50 个字符")
	}
	groupKey, err := normalizeGroupKey(in.GroupKey)
	if err != nil {
		return repo.Capability{}, err
	}
	descriptionZh, descriptionEn, err := normalizeDescriptions(in.DescriptionZh, in.DescriptionEn)
	if err != nil {
		return repo.Capability{}, err
	}
	q := repo.New(s.pool)
	if _, err := q.AdminGetCapability(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Capability{}, apperr.New(apperr.CodeNotFound, "")
		}
		return repo.Capability{}, fmt.Errorf("查询考察维度: %w", err)
	}
	row, err := q.UpdateCapability(ctx, repo.UpdateCapabilityParams{
		ID: id, NameZh: nameZh, NameEn: nameEn, GroupKey: groupKey,
		DescriptionZh: descriptionZh, DescriptionEn: descriptionEn,
		IsEnabled: in.IsEnabled, SortOrder: in.SortOrder,
	})
	if err != nil {
		return repo.Capability{}, fmt.Errorf("更新考察维度: %w", err)
	}
	return row, nil
}

// DeleteCapability 删除未被引用的考察维度。被引用时只能停用。
func (s *Service) DeleteCapability(ctx context.Context, id int64) error {
	if err := s.ensure(); err != nil {
		return err
	}
	q := repo.New(s.pool)
	if _, err := q.AdminGetCapability(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.CodeNotFound, "")
		}
		return fmt.Errorf("查询考察维度: %w", err)
	}
	references, err := q.CountCapabilityReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("统计考察维度引用: %w", err)
	}
	if references > 0 {
		return apperr.New(apperr.CodeDictionaryInUse, "考察维度已被引用，只能停用")
	}
	if err := q.DeleteCapability(ctx, id); err != nil {
		return fmt.Errorf("删除考察维度: %w", err)
	}
	return nil
}
