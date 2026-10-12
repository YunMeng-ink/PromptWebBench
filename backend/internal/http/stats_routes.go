package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/promptbench/backend/internal/service/stats"
)

func (s *Server) registerStatsRoutes() {
	Register(s, "guest_read", huma.Operation{
		Method: http.MethodGet, Path: APIPrefix + "/prompts/{id}/stats",
		OperationID: "getPromptStats", Summary: "提示词统计", Tags: []string{"提示词"},
	}, s.handleGetPromptStats)
}

type getPromptStatsInput struct {
	ID int64 `path:"id" doc:"提示词 ID"`
}

// windowDTO 是一个时间窗口的统计结果。
//
// Discrimination 在数据不足时为 null，前端据此显示 C-06 规定的
// 「数据不足，暂不计算区分度」，而不是显示 0（需求 §4.6）。
type windowDTO struct {
	Sufficient       bool     `json:"sufficient" doc:"数据是否满足门槛"`
	Discrimination   *float64 `json:"discrimination" doc:"区分度；数据不足时为 null"`
	Confidence       string   `json:"confidence" enum:"high,medium,low" doc:"置信度等级"`
	ValidModelCount  int      `json:"valid_model_count" doc:"有效模型数"`
	TotalRatingCount int      `json:"total_rating_count" doc:"窗口内总评分数"`
}

// modelWindowDTO 是单个模型在一个窗口内的完成度统计（需求 §3.7）。
type modelWindowDTO struct {
	HasData         bool     `json:"has_data" doc:"该窗口内是否有观测值"`
	Mean            *float64 `json:"mean" doc:"原始平均分；无评分时为 null"`
	Adjusted        float64  `json:"adjusted" doc:"校准完成度"`
	RatingCount     int64    `json:"rating_count" doc:"评分数"`
	UserCount       int64    `json:"user_count" doc:"独立评分用户数"`
	SD              float64  `json:"sd" doc:"评分标准差"`
	CIHalf          float64  `json:"ci_half" doc:"95% 置信区间半宽"`
	ConfidenceScore float64  `json:"confidence_score" doc:"置信度分数"`
	Confidence      string   `json:"confidence" enum:"high,medium,low" doc:"置信度等级"`
}

type modelStatsDTO struct {
	ModelID   int64          `json:"model_id"`
	ModelName string         `json:"model_name"`
	History   modelWindowDTO `json:"history"`
	Recent    modelWindowDTO `json:"recent"`
}

// recommendTagDTO 是已确认的推荐标签（需求 §6.3）。
type recommendTagDTO struct {
	ID          int64  `json:"id"`
	ModelID     int64  `json:"model_id" doc:"提示词级标签为 0"`
	Scope       string `json:"scope" enum:"prompt,prompt_model"`
	Kind        string `json:"kind" enum:"obsolete,suspected_obsolete,saturation,suspected_saturation"`
	LabelZh     string `json:"label_zh"`
	LabelEn     string `json:"label_en"`
	Confidence  string `json:"confidence" enum:"high,medium,low"`
	Reason      string `json:"reason" doc:"推荐理由"`
	TriggeredAt string `json:"triggered_at"`
}

type promptStatsResponse struct {
	Body struct {
		PromptID       int64             `json:"prompt_id"`
		History        windowDTO         `json:"history"`
		Recent         windowDTO         `json:"recent"`
		Models         []modelStatsDTO   `json:"models"`
		Recommendation []recommendTagDTO `json:"recommendation"`
		ComputedAt     string            `json:"computed_at"`
	}
}

func (s *Server) handleGetPromptStats(ctx context.Context, in *getPromptStatsInput) (*promptStatsResponse, error) {
	v, err := viewerFor(ctx)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.deps.Stats.Load(ctx, v, in.ID)
	if err != nil {
		return nil, err
	}
	return buildPromptStats(snapshot), nil
}

func buildPromptStats(snapshot stats.Snapshot) *promptStatsResponse {
	out := &promptStatsResponse{}
	out.Body.PromptID = snapshot.PromptID
	out.Body.History = toWindowDTO(snapshot.History)
	out.Body.Recent = toWindowDTO(snapshot.Recent)
	out.Body.ComputedAt = formatTime(snapshot.ComputedAt)

	out.Body.Models = make([]modelStatsDTO, 0, len(snapshot.Models))
	for _, m := range snapshot.Models {
		out.Body.Models = append(out.Body.Models, modelStatsDTO{
			ModelID:   m.ModelID,
			ModelName: m.ModelName,
			History:   toModelWindowDTO(m.History),
			Recent:    toModelWindowDTO(m.Recent),
		})
	}

	out.Body.Recommendation = make([]recommendTagDTO, 0, len(snapshot.Recommendation))
	for _, t := range snapshot.Recommendation {
		out.Body.Recommendation = append(out.Body.Recommendation, recommendTagDTO{
			ID:          t.ID,
			ModelID:     t.ModelID,
			Scope:       t.Scope,
			Kind:        t.Kind,
			LabelZh:     t.LabelZh,
			LabelEn:     t.LabelEn,
			Confidence:  string(t.Confidence),
			Reason:      t.Reason,
			TriggeredAt: formatTime(t.TriggeredAt),
		})
	}
	return out
}

func toWindowDTO(w stats.Window) windowDTO {
	return windowDTO{
		Sufficient:       w.Sufficient,
		Discrimination:   w.Discrimination,
		Confidence:       string(w.Confidence),
		ValidModelCount:  w.ValidModelCount,
		TotalRatingCount: w.TotalRatingCount,
	}
}

func toModelWindowDTO(m stats.ModelWindow) modelWindowDTO {
	return modelWindowDTO{
		HasData:         m.HasData,
		Mean:            m.Mean,
		Adjusted:        m.Adjusted,
		RatingCount:     m.RatingCount,
		UserCount:       m.UserCount,
		SD:              m.SD,
		CIHalf:          m.CIHalf,
		ConfidenceScore: m.ConfidenceScore,
		Confidence:      string(m.Confidence),
	}
}
