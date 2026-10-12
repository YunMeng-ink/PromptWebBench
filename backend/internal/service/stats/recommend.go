package stats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/promptbench/backend/internal/formula"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/repo"
)

// wrap 为写入失败补上操作与对象，使错误信息能定位到具体提示词与具体动作。
func wrap(action string, promptID int64, err error) error {
	return fmt.Errorf("%s（提示词 %d）: %w", action, promptID, err)
}

// 推荐标签的初始态：新建一律待确认，全部推荐都需管理员确认后才在前台展示
// （需求 §6、C-13）。
const recommendTagInitialState = "pending"

// 操作日志动作：区分度回升时把待确认的过时标签转已忽略（C-15）。
const actionDiscriminationRecovery = "discrimination_recovery"

// syncRecommendTags 按本次计算结果同步推荐标签。
//
// 三条规则各自对应需求的一节：
//   - 区分度回升时，待确认的过时标签转已忽略并记审计日志（C-15、需求 §6.1）；
//   - 已存在的未终结标签不重复生成，靠部分唯一索引与 ON CONFLICT DO NOTHING 双重保证
//     （C-13）；
//   - 数据不足时不生成任何过时类标签（需求 §4.3）。
func (s *Service) syncRecommendTags(ctx context.Context, q *repo.Queries, promptID int64, result computed) error {
	history, recent := result.obsoleteSummary()
	verdict := formula.JudgeObsolete(history, recent, result.params.Disc)

	// 先处理回落：区分度回升时旧的待确认标签必须转已忽略，否则它会永久占用管理端列表。
	// 只处理待确认项，已确认的由管理员显式决定。
	if verdict.Recovered {
		ignored, err := q.IgnorePendingTags(ctx, repo.IgnorePendingTagsParams{
			PromptID: promptID,
			Kinds:    []string{formula.TagObsolete, formula.TagSuspectedObsolete},
		})
		if err != nil {
			return wrap("忽略已失效的过时推荐标签", promptID, err)
		}
		if len(ignored) > 0 {
			if err := s.recordRecovery(ctx, q, promptID, ignored); err != nil {
				return err
			}
		}
	}

	if verdict.Kind != "" {
		if err := s.insertTag(ctx, q, promptID, recommendTag{
			ModelID: 0,
			Scope:   formula.ScopePrompt,
			Kind:    verdict.Kind,
			LabelZh: obsoleteLabelZh(verdict.Kind),
			LabelEn: obsoleteLabelEn(verdict.Kind),
			// C-16：「过时」用区分度置信度，不与完成度置信度混用。
			Confidence: result.bootstrap.ConfidenceLevel,
			Reason:     verdict.Reason,
			Snapshot:   obsoleteSnapshot(result),
		}); err != nil {
			return err
		}
	}

	historyByID := modelByID(result.history.Models)
	for _, recentItem := range result.recent.Models {
		historyItem, ok := historyByID[recentItem.ModelID]
		if !ok {
			continue
		}
		saturation := formula.JudgeSaturation(recentItem, historyItem, result.params.Disc.SaturationThreshold)
		if saturation.Kind == "" {
			continue
		}

		// C-16：「训练饱和」用该模型近 N 日完成度置信度。
		confidence := formula.ConfidenceMedium
		switch {
		case saturation.High:
			confidence = formula.ConfidenceHigh
		case recentItem.ConfidenceLevel == formula.ConfidenceLow:
			confidence = formula.ConfidenceLow
		}

		label := saturationLabelZh(saturation.Kind, recentItem.ModelName)
		if err := s.insertTag(ctx, q, promptID, recommendTag{
			ModelID:    recentItem.ModelID,
			Scope:      formula.ScopePromptModel,
			Kind:       saturation.Kind,
			LabelZh:    label.zh,
			LabelEn:    label.en,
			Confidence: confidence,
			Reason:     saturation.Reason,
			Snapshot:   saturationSnapshot(result, recentItem, historyItem),
		}); err != nil {
			return err
		}
	}
	return nil
}

// recommendTag 是待写入的推荐标签。
type recommendTag struct {
	ModelID    int64
	Scope      string
	Kind       string
	LabelZh    string
	LabelEn    string
	Confidence formula.ConfidenceLevel
	Reason     string
	Snapshot   map[string]any
}

// insertTag 写入一条推荐标签。
//
// 部分唯一索引只覆盖未终结的标签，已忽略或已过期的记录不参与约束，因此同一个
// 「提示词 × 模型 × 类型」可以先后产生多条历史记录。ON CONFLICT DO NOTHING 在冲突时
// 不返回行，sqlc 的 :one 会把它表现为 pgx.ErrNoRows：这是「该标签已有未终结记录」的判定
// 依据，不是错误，直接返回 nil 让重算继续。
func (s *Service) insertTag(ctx context.Context, q *repo.Queries, promptID int64, tag recommendTag) error {
	snapshot, err := json.Marshal(tag.Snapshot)
	if err != nil {
		return fmt.Errorf("序列化推荐标签快照: %w", err)
	}
	_, err = q.InsertRecommendTag(ctx, repo.InsertRecommendTagParams{
		PromptID:    promptID,
		ModelID:     tag.ModelID,
		Scope:       tag.Scope,
		Kind:        tag.Kind,
		State:       recommendTagInitialState,
		LabelZh:     tag.LabelZh,
		LabelEn:     tag.LabelEn,
		Confidence:  confidenceText(tag.Confidence),
		Reason:      tag.Reason,
		Snapshot:    snapshot,
		TriggeredAt: repo.Timestamptz(s.now()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return wrap("写入推荐标签", promptID, err)
	}
	return nil
}

// recordRecovery 为被忽略的过时标签各写一条审计日志。
//
// 日志记录在同一次统计重算事务里，与状态变更同生共死：C-15 要求保留追溯能力，
// 出现「标签已忽略但无记录」的状态会让管理员无法判断当初的判定依据。
func (s *Service) recordRecovery(ctx context.Context, q *repo.Queries, promptID int64, tags []repo.RecommendTag) error {
	recorder := oplog.NewRecorder()
	for _, tag := range tags {
		entry := oplog.Entry{
			Action:     actionDiscriminationRecovery,
			ObjectType: "recommend_tag",
			ObjectID:   tag.ID,
			Before:     map[string]any{"state": tag.State},
			After:      map[string]any{"state": "ignored"},
			Note:       "近 N 日区分度回升，系统自动忽略待确认的过时推荐标签",
		}
		if err := recorder.Record(ctx, q, entry); err != nil {
			return err
		}
	}
	return nil
}

// obsoleteSnapshot 记录触发过时推荐时的评分快照（需求 §6.3「查看触发时的评分快照」）。
func obsoleteSnapshot(result computed) map[string]any {
	return map[string]any{
		"history_discrimination":     result.history.Discrimination,
		"history_valid_model_count":  result.history.ValidModelCount,
		"history_total_rating_count": result.history.TotalRatingCount,
		"recent_discrimination":      result.recent.Discrimination,
		"recent_valid_model_count":   result.recent.ValidModelCount,
		"recent_total_rating_count":  result.recent.TotalRatingCount,
		"ci_confidence_low":          result.bootstrap.CIConfidenceLow,
		"ci_confidence_high":         result.bootstrap.CIConfidenceHigh,
	}
}

func saturationSnapshot(result computed, recent, history formula.ModelResult) map[string]any {
	return map[string]any{
		"model_id":             recent.ModelID,
		"model_name":           recent.ModelName,
		"recent_adjusted":      recent.Adjusted,
		"recent_rating_count":  recent.RatingCount,
		"recent_user_count":    recent.UserCount,
		"recent_confidence":    string(recent.ConfidenceLevel),
		"history_adjusted":     history.Adjusted,
		"history_rating_count": history.RatingCount,
		"history_confidence":   string(history.ConfidenceLevel),
	}
}

func obsoleteLabelZh(kind string) string {
	if kind == formula.TagSuspectedObsolete {
		return "疑似过时"
	}
	return "过时"
}

func obsoleteLabelEn(kind string) string {
	if kind == formula.TagSuspectedObsolete {
		return "Suspected Obsolete"
	}
	return "Obsolete"
}

type saturationLabel struct{ zh, en string }

func saturationLabelZh(kind, modelName string) saturationLabel {
	switch kind {
	case formula.TagSuspectedSaturation:
		return saturationLabel{
			zh: "疑似训练饱和：" + modelName,
			en: "Suspected Saturated: " + modelName,
		}
	default:
		return saturationLabel{
			zh: "训练饱和：" + modelName,
			en: "Saturated: " + modelName,
		}
	}
}
