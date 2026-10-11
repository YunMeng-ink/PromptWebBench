package rating

import (
	"context"
	"fmt"
	"time"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// Mine 是「我的评分」列表项。
type Mine struct {
	ID          int64
	PromptID    int64
	PromptTitle string
	ModelID     int64
	ModelName   string
	Score       int16
	EditCount   int32
	CreatedAt   time.Time
}

// PromptModelRating 是详情页的模型评分汇总。
type PromptModelRating struct {
	ModelID     int64
	ModelName   string
	RatingCount int64
	UserCount   int64
	MeanScore   float64

	// MyScore 与 MyEditCount 仅对登录身份非空，游客一律为 nil。
	MyScore     *int16
	MyEditCount *int32
}

// MaxEdits 返回单条评分的修改次数上限，供前端展示剩余可改次数。
func (s *Service) MaxEdits(ctx context.Context) (int32, error) {
	limit, err := s.config.GetInt(ctx, "rating_max_edits_per_record")
	if err != nil {
		return 0, fmt.Errorf("读取单条评分修改次数上限: %w", err)
	}
	return int32(limit), nil
}

// ListMine 返回当前用户的评分列表。
func (s *Service) ListMine(ctx context.Context, userID int64, page, pageSize int32) ([]Mine, int64, error) {
	q := repo.New(s.pool)
	rows, err := q.ListMyRatings(ctx, repo.ListMyRatingsParams{
		UserID: userID,
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("查询我的评分: %w", err)
	}
	total, err := q.CountMyRatings(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("统计我的评分数量: %w", err)
	}

	items := make([]Mine, 0, len(rows))
	for _, row := range rows {
		item := Mine{
			ID:          row.ID,
			PromptID:    row.PromptID,
			PromptTitle: row.PromptTitle,
			ModelID:     row.ModelID,
			Score:       row.Score,
			EditCount:   row.EditCount,
		}
		if row.CreatedAt.Valid {
			item.CreatedAt = row.CreatedAt.Time
		}
		items = append(items, item)
	}

	// 模型名称按 ID 批量取，避免逐行查询。
	models, err := s.modelNames(ctx, items)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		items[i].ModelName = models[items[i].ModelID]
	}
	return items, total, nil
}

// ListByPrompt 返回某提示词在各模型上的评分汇总。
// v 为空表示游客，此时不返回任何人的评分。
func (s *Service) ListByPrompt(ctx context.Context, v domain.Viewer, promptID int64) ([]PromptModelRating, error) {
	rows, err := repo.New(s.pool).ListPromptModelRatings(ctx, promptID)
	if err != nil {
		return nil, fmt.Errorf("查询提示词评分汇总: %w", err)
	}

	mine := map[int64]domain.Rating{}
	if v.Logged {
		own, err := repo.New(s.pool).ListMyRatingsForPrompt(ctx, repo.ListMyRatingsForPromptParams{
			UserID:   v.UserID,
			PromptID: promptID,
		})
		if err != nil {
			return nil, fmt.Errorf("查询我的评分: %w", err)
		}
		for _, row := range own {
			r, err := ratingFromRow(row)
			if err != nil {
				return nil, err
			}
			mine[row.ModelID] = r
		}
	}

	items := make([]PromptModelRating, 0, len(rows))
	for _, row := range rows {
		item := PromptModelRating{
			ModelID:     row.ModelID,
			ModelName:   row.ModelName,
			RatingCount: row.RatingCount,
			UserCount:   row.UserCount,
		}
		// 无评分的模型在该提示词上没有观测值，均值置 0 并由计数表明「尚无评分」。
		item.MeanScore = row.MeanScore
		if r, ok := mine[row.ModelID]; ok {
			score := r.Score
			editCount := r.EditCount
			item.MyScore = &score
			item.MyEditCount = &editCount
		}
		items = append(items, item)
	}
	return items, nil
}

// modelNames 批量取模型名称，返回 ID 到名称的映射。
func (s *Service) modelNames(ctx context.Context, items []Mine) (map[int64]string, error) {
	names := map[int64]string{}
	if len(items) == 0 {
		return names, nil
	}
	rows, err := repo.New(s.pool).ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询模型列表: %w", err)
	}
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}
