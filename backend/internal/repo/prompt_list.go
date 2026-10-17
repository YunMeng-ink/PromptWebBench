// 本文件存放筛选维度组合可变、排序方式可切换的动态查询（开发方案 §2.1「手写动态查询」）。
// 静态查询由 sqlc 生成，见同目录其余文件。

package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/domain"
)

// PromptListFilter 是公开列表的筛选维度（需求 §8.2：考察目的、考察维度、难度、关键词、
// 适用模型、标签共 6 维）。零值表示该维度不参与筛选。
type PromptListFilter struct {
	PurposeID    *int64
	CapabilityID *int64
	DifficultyID *int64
	Keyword      string
	ModelID      *int64
	TagIDs       []int64
}

// PromptListItem 是列表项。区分度与完成度取自阶段 3 写入的统计快照，
// 快照缺失时为零值并标记为数据不足。
type PromptListItem struct {
	Prompt           domain.Prompt
	PurposeNameZh    string
	PurposeNameEn    string
	CapabilityNameZh string
	CapabilityNameEn string
	DifficultyNameZh string
	DifficultyNameEn string
	Discrimination   float64
	DataSufficient   bool
	AvgAdjusted      float64
}

// PromptList 是分页结果，形状遵循 C-40：{items, page, page_size, total}。
type PromptList struct {
	Items    []PromptListItem
	Page     int32
	PageSize int32
	Total    int64
}

// PromptListPage 返回公开列表的一页。
//
// 可见性谓词由 domain.Viewer.ListScope 生成并直接拼入过滤条件。筛选与排序不重复
// 实现可见性规则，全系统可见性判定只有该函数一处（开发方案 §3.1）。
func PromptListPage(ctx context.Context, pool *pgxpool.Pool, v domain.Viewer, f PromptListFilter, sort domain.SortOrder, page, pageSize int32) (PromptList, error) {
	conditions := []string{"p.deleted_at IS NULL"}
	args := []any{}

	scope, _ := v.ListScope()
	conditions = append(conditions, scope)

	add := func(clause string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(clause, len(args)))
	}

	if f.PurposeID != nil {
		add("p.purpose_id = $%d", *f.PurposeID)
	}
	if f.CapabilityID != nil {
		add("p.capability_id = $%d", *f.CapabilityID)
	}
	if f.DifficultyID != nil {
		add("p.difficulty_id = $%d", *f.DifficultyID)
	}
	if keyword := strings.TrimSpace(f.Keyword); keyword != "" {
		args = append(args, "%"+keyword+"%")
		n := len(args)
		conditions = append(conditions, fmt.Sprintf(
			"(p.title ILIKE $%[1]d OR p.description ILIKE $%[1]d OR p.body ILIKE $%[1]d)", n))
	}
	if f.ModelID != nil {
		// 适用模型：该提示词在该模型上存在有效评分。
		add("EXISTS (SELECT 1 FROM ratings r WHERE r.prompt_id = p.id AND r.model_id = $%d AND r.deleted_at IS NULL)", *f.ModelID)
	}
	if len(f.TagIDs) > 0 {
		args = append(args, f.TagIDs)
		n := len(args)
		conditions = append(conditions, fmt.Sprintf(
			"p.id IN (SELECT prompt_id FROM prompt_tags WHERE tag_id = ANY($%d))", n))
	}

	where := strings.Join(conditions, "\n  AND ")

	total, err := countPrompts(ctx, pool, where, args)
	if err != nil {
		return PromptList{}, err
	}
	items, err := queryPrompts(ctx, pool, where, args, sort, page, pageSize)
	if err != nil {
		return PromptList{}, err
	}
	return PromptList{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// selectColumns 是列表项的取值列，统计列一律取自阶段 3 的快照表。
const selectColumns = `
    p.id, p.title, p.description, p.purpose_id, p.capability_id, p.difficulty_id,
    p.visibility, p.sensitive_level, p.uploader_id, p.latest_version_no,
    p.major_modified, p.comment_count, p.favorite_count, p.created_at, p.updated_at,
    pu.name_zh, pu.name_en, pu.is_high_risk,
    cap.name_zh, cap.name_en,
    COALESCE(d.name_zh, ''), COALESCE(d.name_en, ''),
    COALESCE(s.sort_discrimination, 0), COALESCE(s.data_sufficient, false),
    COALESCE(top.avg_adjusted, 0)`

// fromClause 连接提示词与字典、统计快照。「评分最高」排序需要各模型的校准完成度均值，
// 用 LATERAL 子查询一次取出，避免把聚合下推到每行。
// 子查询内的 avg 在无任何符合条件��模型时返回 NULL，外层已用 COALESCE 兜底；
// 但子查询本身也包一层，保证扫描与排序都不会遇到空值（开发方案 §6.4 专项回归）。
const fromClause = `
FROM prompts p
JOIN purposes pu ON pu.id = p.purpose_id
JOIN capabilities cap ON cap.id = p.capability_id
LEFT JOIN difficulties d ON d.id = p.difficulty_id
LEFT JOIN prompt_stats s ON s.prompt_id = p.id
LEFT JOIN LATERAL (
    SELECT COALESCE(avg(pms.history_adjusted), 0) AS avg_adjusted
    FROM prompt_model_stats pms
    WHERE pms.prompt_id = p.id
      AND pms.history_adjusted IS NOT NULL
      AND pms.history_rating_count > 0
) top ON true`

// orderClause 返回排序子句。数据不足的条目在按数据排序时一律排后，
// 与统计口径一致：区分度排序时数据充足的排在前面（需求 §8.2）。
func orderClause(sort domain.SortOrder) string {
	switch sort {
	case domain.SortTopRated:
		return `ORDER BY top.avg_adjusted DESC NULLS LAST, p.updated_at DESC, p.id DESC`
	case domain.SortDiscrimination:
		return `ORDER BY s.data_sufficient DESC NULLS LAST, s.sort_discrimination DESC NULLS LAST, p.updated_at DESC, p.id DESC`
	default:
		return `ORDER BY p.updated_at DESC, p.id DESC`
	}
}

func countPrompts(ctx context.Context, pool *pgxpool.Pool, where string, args []any) (int64, error) {
	query := `SELECT count(*)::bigint ` + fromClause + "\nWHERE " + where
	var total int64
	if err := pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("统计公开提示词数量: %w", err)
	}
	return total, nil
}

func queryPrompts(ctx context.Context, pool *pgxpool.Pool, where string, args []any, sort domain.SortOrder, page, pageSize int32) ([]PromptListItem, error) {
	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	query := `SELECT ` + selectColumns + fromClause + "\nWHERE " + where + "\n" +
		orderClause(sort) + fmt.Sprintf("\nLIMIT $%d OFFSET $%d", limitArg, offsetArg)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询公开提示词列表: %w", err)
	}
	defer rows.Close()

	items := []PromptListItem{}
	for rows.Next() {
		var (
			item         PromptListItem
			visibility   string
			sensitive    string
			versionNo    string
			difficultyID *int64
		)
		if err := rows.Scan(
			&item.Prompt.ID, &item.Prompt.Title, &item.Prompt.Description, &item.Prompt.PurposeID,
			&item.Prompt.CapabilityID, &difficultyID, &visibility, &sensitive, &item.Prompt.UploaderID,
			&versionNo, &item.Prompt.MajorModified, &item.Prompt.CommentCount, &item.Prompt.FavoriteCount,
			&item.Prompt.CreatedAt, &item.Prompt.UpdatedAt,
			&item.PurposeNameZh, &item.PurposeNameEn, &item.Prompt.HighRisk,
			&item.CapabilityNameZh, &item.CapabilityNameEn,
			&item.DifficultyNameZh, &item.DifficultyNameEn,
			&item.Discrimination, &item.DataSufficient, &item.AvgAdjusted,
		); err != nil {
			return nil, fmt.Errorf("扫描公开提示词列表行: %w", err)
		}

		item.Prompt.DifficultyID = difficultyID
		item.Prompt.Visibility = domain.Visibility(visibility)
		item.Prompt.Sensitive = domain.SensitiveLevel(sensitive)
		item.Prompt.LatestVersion, err = domain.ParseVersion(versionNo)
		if err != nil {
			return nil, fmt.Errorf("提示词 %d: %w", item.Prompt.ID, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历公开提示词列表: %w", err)
	}
	return items, nil
}
