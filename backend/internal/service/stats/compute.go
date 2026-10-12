package stats

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/promptbench/backend/internal/formula"
	"github.com/promptbench/backend/internal/repo"
)

// inputsFrom 把聚合行转为公式层的输入。
func inputsFrom(rows []repo.ListPromptModelAggregatesRow) []formula.ModelInput {
	out := make([]formula.ModelInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, formula.ModelInput{
			ModelID:     row.ModelID,
			ModelName:   row.ModelName,
			RatingCount: row.RatingCount,
			UserCount:   row.UserCount,
			ScoreSum:    row.ScoreSum,
			ScoreSquare: row.ScoreSquare,
		})
	}
	return out
}

// windowAggregates 读取一个时间窗口内按模型聚合的评分数据。
//
// recent 为真时取近 N 日窗口，否则取历史窗口（全部有效评分）。
func (s *Service) windowAggregates(
	ctx context.Context, q *repo.Queries, promptID int64, w windows, recent bool,
) ([]repo.ListPromptModelAggregatesRow, error) {
	params := repo.ListPromptModelAggregatesParams{PromptID: promptID}
	if recent {
		params.WindowStart = repo.Timestamptz(w.recentStart)
		params.WindowEnd = repo.Timestamptz(w.recentEnd)
	}
	rows, err := q.ListPromptModelAggregates(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("查询提示词 %d 的评分聚合: %w", promptID, err)
	}
	return rows, nil
}

// windowUserCount 读取一个窗口内跨模型去重后的独立用户数。
func (s *Service) windowUserCount(
	ctx context.Context, q *repo.Queries, promptID int64, w windows, recent bool,
) (int, error) {
	params := repo.CountPromptWindowUsersParams{PromptID: promptID}
	if recent {
		params.WindowStart = repo.Timestamptz(w.recentStart)
		params.WindowEnd = repo.Timestamptz(w.recentEnd)
	}
	count, err := q.CountPromptWindowUsers(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("统计提示词 %d 的窗口用户数: %w", promptID, err)
	}
	return int(count), nil
}

// sampleSets 读取一个窗口内逐条评分，按模型归集为 Bootstrap 的重抽样单元。
//
// 只返回达到每模型最低评分数的有效模型：数据不足的模型不参与区分度计算，
// 把它们纳入会让重抽样后的有效模型数在迭代之间抖动。
func (s *Service) sampleSets(
	ctx context.Context, q *repo.Queries, promptID int64, w windows, recent bool,
	minPerModel int,
) ([]formula.SampleSet, error) {
	params := repo.ListPromptWindowScoresParams{PromptID: promptID}
	if recent {
		params.WindowStart = repo.Timestamptz(w.recentStart)
		params.WindowEnd = repo.Timestamptz(w.recentEnd)
	}
	rows, err := q.ListPromptWindowScores(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("查询提示词 %d 的评分样本: %w", promptID, err)
	}

	order := make([]int64, 0, len(rows))
	grouped := map[int64][]float64{}
	for _, row := range rows {
		if _, seen := grouped[row.ModelID]; !seen {
			order = append(order, row.ModelID)
		}
		grouped[row.ModelID] = append(grouped[row.ModelID], float64(row.Score))
	}

	out := make([]formula.SampleSet, 0, len(order))
	for _, modelID := range order {
		if len(grouped[modelID]) < minPerModel {
			continue
		}
		out = append(out, formula.SampleSet{Scores: grouped[modelID]})
	}
	return out, nil
}

// compute 对一条提示词的两个窗口完成全部计算。
type computed struct {
	params  params
	windows windows
	history formula.WindowResult
	recent  formula.WindowResult

	// bootstrap 是近 N 日区分度的置信区间，数据不足时为零值。
	bootstrap formula.BootstrapResult

	// currentVersionID 是快照记录的提示词当前版本。
	currentVersionID int64
	computedAt       time.Time
}

// compute 在给定查询器上完成计算。传入事务内的查询器时，计算与写入处于同一事务。
func (s *Service) compute(ctx context.Context, q *repo.Queries, promptID, versionID int64) (computed, error) {
	p := s.load(ctx)
	now := s.now()
	w := s.windowBounds(now, p.RecentWindow)

	historyRows, err := s.windowAggregates(ctx, q, promptID, w, false)
	if err != nil {
		return computed{}, err
	}
	recentRows, err := s.windowAggregates(ctx, q, promptID, w, true)
	if err != nil {
		return computed{}, err
	}

	historyUsers, err := s.windowUserCount(ctx, q, promptID, w, false)
	if err != nil {
		return computed{}, err
	}
	recentUsers, err := s.windowUserCount(ctx, q, promptID, w, true)
	if err != nil {
		return computed{}, err
	}

	history := formula.ComputeWindow(formula.WindowHistory, inputsFrom(historyRows),
		historyUsers, p.Bayes, p.Disc, p.Confidence)
	recent := formula.ComputeWindow(formula.WindowRecent, inputsFrom(recentRows),
		recentUsers, p.Bayes, p.Disc, p.Confidence)

	out := computed{
		params:           p,
		windows:          w,
		history:          history,
		recent:           recent,
		currentVersionID: versionID,
		computedAt:       now.UTC(),
	}

	// 数据不足时区分度无意义，也不执行 Bootstrap：需求 §4.3 明确该情形下不计算区分度。
	if !recent.Sufficient {
		return out, nil
	}

	samples, err := s.sampleSets(ctx, q, promptID, w, true, p.Disc.MinRatingsPerModel)
	if err != nil {
		return computed{}, err
	}
	out.bootstrap = formula.BootstrapDiscrimination(samples,
		recent.ValidModelCount, recent.TotalRatingCount,
		p.Bayes, p.Disc, p.DiscConf, p.Bootstrap, s.source)
	return out, nil
}

// summary 把两个窗口折成标签判定所需的汇总量。
func (c computed) obsoleteSummary() (formula.WindowSummary, formula.WindowSummary) {
	history := formula.WindowSummary{
		Sufficient:     c.history.Sufficient,
		Discrimination: c.history.Discrimination,
	}
	recent := formula.WindowSummary{
		Sufficient:      c.recent.Sufficient,
		Discrimination:  c.recent.Discrimination,
		ConfidenceLevel: c.bootstrap.ConfidenceLevel,
	}
	return history, recent
}

// confidenceText 把置信度等级落库为 TEXT，与 CHECK 约束的取值一致。
//
// 零值在这里归为低：prompt_model_stats.confidence_level 不允许为空（阶段 3 专项回归的
// 写入路径），零值若原样写入会让带 CHECK 约束的列写入失败。空值只可能来自零评分模型，
// 而零评分模型的置信度按需求恒为低。
func confidenceText(level formula.ConfidenceLevel) string {
	if level == "" {
		return string(formula.ConfidenceLow)
	}
	return string(level)
}

// discriminationValue 在数据充足时返回区分度，否则返回 0。
func (c computed) discriminationValue(sufficient bool, v float64) float64 {
	if !sufficient {
		return 0
	}
	return formula.Round(v)
}

// snapshotRow 组装 prompt_stats 的写入参数。
//
// 充分性由 history_insufficient 与 recent_insufficient 两列表达，读路径据此决定是否
// 暴露区分度，因此数据不足时区分度列写 0 即可：0 在该状态下没有含义，读路径不会
// 把它当成「区分度为零」。这样取值类型全部为非空浮点，写入路径不需要构造可空参数
// （阶段 3 专项回归的写入路径要求零值串写入带 CHECK 约束的列时不失败）。
func (c computed) snapshotRow(promptID int64) repo.UpsertPromptStatsParams {
	historyValue := c.discriminationValue(c.history.Sufficient, c.history.Discrimination)
	recentValue := c.discriminationValue(c.recent.Sufficient, c.recent.Discrimination)

	// 排序取值取两个窗口里较大的区分度：需求 §8.2 只要求「按区分度排序」，未指定窗口；
	// 取较大者让列表反映当前最显著的能力差异。两个窗口都不充足时一律排后。
	sortSufficient := c.history.Sufficient || c.recent.Sufficient
	sortDiscrimination := 0.0
	switch {
	case c.recent.Sufficient && (!c.history.Sufficient || c.recent.Discrimination > c.history.Discrimination):
		sortDiscrimination = c.recent.Discrimination
	case c.history.Sufficient:
		sortDiscrimination = c.history.Discrimination
	default:
		sortSufficient = false
	}

	return repo.UpsertPromptStatsParams{
		PromptID:                promptID,
		HistoryDiscrimination:   historyValue,
		HistoryConfidence:       confidenceText(c.bootstrap.ConfidenceLevel),
		HistoryValidModelCount:  int32(c.history.ValidModelCount),
		HistoryTotalRatingCount: int32(c.history.TotalRatingCount),
		HistoryInsufficient:     !c.history.Sufficient,
		RecentDiscrimination:    recentValue,
		RecentConfidence:        confidenceText(c.bootstrap.ConfidenceLevel),
		RecentValidModelCount:   int32(c.recent.ValidModelCount),
		RecentTotalRatingCount:  int32(c.recent.TotalRatingCount),
		RecentInsufficient:      !c.recent.Sufficient,
		DataSufficient:          sortSufficient,
		SortDiscrimination:      formula.Round(sortDiscrimination),
		WindowStart:             repo.Timestamptz(c.windows.recentStart),
		ComputedAt:              repo.Timestamptz(c.computedAt),
	}
}

// modelRow 组装 prompt_model_stats 的写入参数。
//
// 无评分时原始平均分写 0：同 snapshotRow，充分性由 rating_count 为 0 表达。
func (c computed) modelRow(promptID, modelID int64, history, recent formula.ModelResult) repo.UpsertPromptModelStatParams {
	return repo.UpsertPromptModelStatParams{
		PromptID:               promptID,
		ModelID:                modelID,
		PromptVersionID:        c.currentVersionID,
		HistoryMean:            formula.Round(history.Mean),
		HistoryAdjusted:        formula.Round(history.Adjusted),
		HistoryRatingCount:     int32(history.RatingCount),
		HistoryUserCount:       int32(history.UserCount),
		HistorySd:              round3(history.SD),
		HistoryCiHalf:          round3(history.CIHalf),
		HistoryConfidenceScore: round4(history.ConfidenceScore),
		HistoryConfidenceLevel: confidenceText(history.ConfidenceLevel),
		RecentMean:             formula.Round(recent.Mean),
		RecentAdjusted:         formula.Round(recent.Adjusted),
		RecentRatingCount:      int32(recent.RatingCount),
		RecentUserCount:        int32(recent.UserCount),
		RecentSd:               round3(recent.SD),
		RecentCiHalf:           round3(recent.CIHalf),
		RecentConfidenceScore:  round4(recent.ConfidenceScore),
		RecentConfidenceLevel:  confidenceText(recent.ConfidenceLevel),
		ComputedAt:             repo.Timestamptz(c.computedAt),
	}
}

// round3 与 round4 分别对齐 NUMERIC(6,3) 与 NUMERIC(5,4) 的存储精度。
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// modelByID 把窗口结果按模型 ID 建索引。
func modelByID(results []formula.ModelResult) map[int64]formula.ModelResult {
	out := make(map[int64]formula.ModelResult, len(results))
	for _, item := range results {
		out[item.ModelID] = item
	}
	return out
}
