package stats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/formula"
	"github.com/promptbench/backend/internal/repo"
)

// Snapshot 是一条提示词的完整统计结果，是前端详情页与后台的唯一数据来源。
type Snapshot struct {
	PromptID int64

	History Window
	Recent  Window

	// Models 按模型给出两个窗口的完成度统计量（需求 §3.7）。
	Models []ModelSnapshot

	// Recommendation 是已确认的推荐标签（需求 §3.7「推荐标签，如训练饱和：Model A」）。
	Recommendation []RecommendTag

	// ComputedAt 是快照的计算时刻。
	ComputedAt time.Time
}

// Window 是一个时间窗口在一条提示词上的统计结果。
type Window struct {
	// Sufficient 为 false 时 Discrimination 为 nil，前端显示 C-06 规定文案。
	Sufficient       bool
	Discrimination   *float64
	Confidence       formula.ConfidenceLevel
	ValidModelCount  int
	TotalRatingCount int
	TotalUserCount   int
}

// ModelSnapshot 是一个模型在两个窗口下的完成度统计量（需求 §3.7）。
type ModelSnapshot struct {
	ModelID   int64
	ModelName string

	History ModelWindow
	Recent  ModelWindow
}

// ModelWindow 是单个模型在一个窗口下的统计量。
type ModelWindow struct {
	// HasData 为 false 表示该窗口内没有观测值，校准分取先验值，仅为占位。
	HasData bool

	Mean            *float64
	Adjusted        float64
	RatingCount     int64
	UserCount       int64
	SD              float64
	CIHalf          float64
	ConfidenceScore float64
	Confidence      formula.ConfidenceLevel
}

// RecommendTag 是已确认的推荐标签。
type RecommendTag struct {
	ID          int64
	ModelID     int64
	Scope       string
	Kind        string
	LabelZh     string
	LabelEn     string
	Confidence  formula.ConfidenceLevel
	Reason      string
	TriggeredAt time.Time
}

// Load 返回一条提示词的统计结果。
//
// 两条路径：
//   - 命中进程内缓存且未被作废：直接返回，不查库；
//   - 没有快照，或快照早于提示词与评分的最后更新时刻：同步重算后返回。
//
// 过期时同步重算而不返回旧值：刚提交评分的人正是最关心测量结果的人，
// 给他一份不含这次评分的统计是错的。详情接口的 P95 指标（G-04）已经把统计计算
// 计入预算，进程内缓存与同提示词重算合并吸收掉重复开销。
//
// 评分提交不触发重算，使评分接口保持单次写入；统计计算退化为读放大，
// 而读放大在真实访问分布下远小于写入放大。
func (s *Service) Load(ctx context.Context, v domain.Viewer, promptID int64) (Snapshot, error) {
	if err := s.ensure(); err != nil {
		return Snapshot{}, err
	}
	prompt, err := loadVisiblePrompt(ctx, s.pool, v, promptID)
	if err != nil {
		return Snapshot{}, err
	}

	if snapshot, ok := s.Fresh(promptID); ok {
		return *snapshot, nil
	}

	snapshot, exists, err := s.loadFromDB(ctx, prompt)
	if err != nil {
		return Snapshot{}, err
	}

	if !exists || s.stale(ctx, promptID, snapshot.ComputedAt) {
		recomputeErr := s.flight.do(promptID, func() error {
			// 等待期间别的调用可能已经算完，重新判断一次，避免重复计算。
			if _, ok := s.Fresh(promptID); ok {
				return nil
			}
			return s.Recompute(ctx, promptID)
		})
		if recomputeErr != nil {
			return Snapshot{}, recomputeErr
		}
		snapshot, _, err = s.loadFromDB(ctx, prompt)
		if err != nil {
			return Snapshot{}, err
		}
	}

	s.put(snapshot)
	return snapshot, nil
}

// loadFromDB 读取落库的快照。第二个返回值表明快照行是否存在：
// 不存在说明这条提示词从未计算过统计，调用方需要同步算一次。
func (s *Service) loadFromDB(ctx context.Context, prompt domain.Prompt) (Snapshot, bool, error) {
	q := repo.New(s.pool)

	out := Snapshot{PromptID: prompt.ID}
	exists := false
	row, err := q.GetPromptStats(ctx, prompt.ID)
	switch {
	case err == nil:
		exists = true
		out.History, err = windowFromRow(
			!row.HistoryInsufficient, row.HistoryDiscrimination, row.HistoryConfidence.String,
			int(row.HistoryValidModelCount), int(row.HistoryTotalRatingCount))
		if err != nil {
			return Snapshot{}, false, fmt.Errorf("读取提示词 %d 的历史窗口统计: %w", prompt.ID, err)
		}
		out.Recent, err = windowFromRow(
			!row.RecentInsufficient, row.RecentDiscrimination, row.RecentConfidence.String,
			int(row.RecentValidModelCount), int(row.RecentTotalRatingCount))
		if err != nil {
			return Snapshot{}, false, fmt.Errorf("读取提示词 %d 的近 N 日窗口统计: %w", prompt.ID, err)
		}
		if row.ComputedAt.Valid {
			out.ComputedAt = row.ComputedAt.Time
		}
	case errors.Is(err, pgx.ErrNoRows):
		out.History = Window{Confidence: formula.ConfidenceLow}
		out.Recent = Window{Confidence: formula.ConfidenceLow}
	default:
		return Snapshot{}, false, fmt.Errorf("读取提示词 %d 的统计快照: %w", prompt.ID, err)
	}

	models, err := q.ListPromptModelStats(ctx, prompt.ID)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("读取提示词 %d 的模型统计: %w", prompt.ID, err)
	}
	names := modelNames(ctx, q)
	out.Models = make([]ModelSnapshot, 0, len(models))
	for _, m := range models {
		item, err := modelSnapshotOf(m, names)
		if err != nil {
			return Snapshot{}, false, err
		}
		out.Models = append(out.Models, item)
	}

	tags, err := q.ListConfirmedRecommendTags(ctx, prompt.ID)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("读取提示词 %d 的推荐标签: %w", prompt.ID, err)
	}
	out.Recommendation = make([]RecommendTag, 0, len(tags))
	for _, tag := range tags {
		item := RecommendTag{
			ID:         tag.ID,
			ModelID:    tag.ModelID,
			Scope:      tag.Scope,
			Kind:       tag.Kind,
			LabelZh:    tag.LabelZh,
			LabelEn:    tag.LabelEn,
			Confidence: formula.ConfidenceLevel(tag.Confidence),
			Reason:     tag.Reason,
		}
		if tag.TriggeredAt.Valid {
			item.TriggeredAt = tag.TriggeredAt.Time
		}
		out.Recommendation = append(out.Recommendation, item)
	}
	return out, exists, nil
}

// modelNames 批量取启用模型名称。
func modelNames(ctx context.Context, q *repo.Queries) map[int64]string {
	rows, err := q.ListModels(ctx)
	if err != nil {
		return map[int64]string{}
	}
	out := make(map[int64]string, len(rows))
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out
}

// modelSnapshotOf 把逐模型统计行映射为服务层结构。
//
// *_mean 与 *_adjusted 两列可空：零评分模型没有原始平均分。转换失败一律报错，
// 不静默降级为 0，让快照异常在读取时暴露。
func modelSnapshotOf(m repo.PromptModelStat, names map[int64]string) (ModelSnapshot, error) {
	historyMean, err := numericValue(m.HistoryMean)
	if err != nil {
		return ModelSnapshot{}, fmt.Errorf("提示词 %d 模型 %d 的历史原始平均分: %w", m.PromptID, m.ModelID, err)
	}
	recentMean, err := numericValue(m.RecentMean)
	if err != nil {
		return ModelSnapshot{}, fmt.Errorf("提示词 %d 模型 %d 的近 N 日原始平均分: %w", m.PromptID, m.ModelID, err)
	}
	historyAdjusted, err := numericValue(m.HistoryAdjusted)
	if err != nil {
		return ModelSnapshot{}, fmt.Errorf("提示词 %d 模型 %d 的历史校准完成度: %w", m.PromptID, m.ModelID, err)
	}
	recentAdjusted, err := numericValue(m.RecentAdjusted)
	if err != nil {
		return ModelSnapshot{}, fmt.Errorf("提示词 %d 模型 %d 的近 N 日校准完成度: %w", m.PromptID, m.ModelID, err)
	}

	return ModelSnapshot{
		ModelID:   m.ModelID,
		ModelName: names[m.ModelID],
		History: ModelWindow{
			HasData:         m.HistoryRatingCount > 0,
			Mean:            historyMean,
			Adjusted:        derefOrZero(historyAdjusted),
			RatingCount:     int64(m.HistoryRatingCount),
			UserCount:       int64(m.HistoryUserCount),
			SD:              m.HistorySd,
			CIHalf:          m.HistoryCiHalf,
			ConfidenceScore: m.HistoryConfidenceScore,
			Confidence:      formula.ConfidenceLevel(m.HistoryConfidenceLevel),
		},
		Recent: ModelWindow{
			HasData:         m.RecentRatingCount > 0,
			Mean:            recentMean,
			Adjusted:        derefOrZero(recentAdjusted),
			RatingCount:     int64(m.RecentRatingCount),
			UserCount:       int64(m.RecentUserCount),
			SD:              m.RecentSd,
			CIHalf:          m.RecentCiHalf,
			ConfidenceScore: m.RecentConfidenceScore,
			Confidence:      formula.ConfidenceLevel(m.RecentConfidenceLevel),
		},
	}, nil
}

// derefOrZero 把可空数值的空值当作 0。
//
// 校准完成度列允许为空，但空值只出现在快照异常的情况：正常写入时即便零评分模型
// 也会写入先验值。原始平均分才是真正需要保留空值的那一列。
func derefOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// stale 判定快照是否已经落后于它应反映的数据。
func (s *Service) stale(ctx context.Context, promptID int64, computedAt time.Time) bool {
	if computedAt.IsZero() {
		return true
	}
	watermark, err := repo.New(s.pool).GetPromptStatsWatermark(ctx, promptID)
	if err != nil || !watermark.Valid {
		return true
	}
	return watermark.Time.After(computedAt)
}

// loadVisiblePrompt 读取当前身份可见的提示词，可见性判定只有 domain.Viewer 一处
// （开发方案 §3.1）。不可见一律返回 404，不透露对象是否存在（开发方案 §5.3）。
func loadVisiblePrompt(ctx context.Context, pool *pgxpool.Pool, v domain.Viewer, id int64) (domain.Prompt, error) {
	row, err := repo.New(pool).GetPrompt(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Prompt{}, apperr.New(apperr.CodeNotFound, "")
		}
		return domain.Prompt{}, fmt.Errorf("查询提示词: %w", err)
	}
	prompt, err := promptFromRow(row)
	if err != nil {
		return domain.Prompt{}, err
	}
	if !v.CanView(prompt.Access()) {
		return domain.Prompt{}, apperr.New(apperr.CodeNotFound, "")
	}
	return prompt, nil
}

// numericValue 把可空数值转为指针，空值返回 nil。
func numericValue(n pgtype.Numeric) (*float64, error) {
	if !n.Valid {
		return nil, nil
	}
	value, err := n.Float64Value()
	if err != nil {
		return nil, fmt.Errorf("数值列无法转为浮点数: %w", err)
	}
	result := value.Float64
	return &result, nil
}

// windowFromRow 组装一个窗口的统计结果。数据不足时区分度返回 nil，
// 由展示层输出 C-06 规定的文案而不是一个 0 值。
func windowFromRow(sufficient bool, raw pgtype.Numeric, confidence string, validModels, totalRatings int) (Window, error) {
	out := Window{
		Sufficient:       sufficient,
		Confidence:       formula.ConfidenceLevel(confidence),
		ValidModelCount:  validModels,
		TotalRatingCount: totalRatings,
	}
	if !sufficient {
		return out, nil
	}
	value, err := numericValue(raw)
	if err != nil {
		return Window{}, err
	}
	out.Discrimination = value
	return out, nil
}
