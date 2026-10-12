package formula

import "math"

// ModelInput 是一个模型在一个时间窗口内的评分原始数据。
//
// 校准平均分由 (评分之和, 评分条数) 决定，标准差由 (平方和, 评分和) 决定。
// 这两组量都由数据库聚合得出，公式层只做纯计算。
type ModelInput struct {
	ModelID     int64
	ModelName   string
	RatingCount int64
	UserCount   int64
	ScoreSum    float64
	ScoreSquare float64
}

// ModelResult 是一个模型在一个时间窗口内的全部统计量。
type ModelResult struct {
	ModelID   int64
	ModelName string

	// Mean 为原始平均分（需求 §3.5）。评分条数为 0 时 HasData 为 false，Mean 无意义。
	Mean    float64
	HasData bool

	// Adjusted 为贝叶斯校准平均分（需求 §3.6）。校准分在 n = 0 时取先验值本身，
	// 因此 Adjusted 在 HasData 为 false 时依然有意义。
	Adjusted float64

	RatingCount int64
	UserCount   int64

	// SD 为样本标准差，分母取 n-1；n < 2 时取 0（C-10）。
	SD float64

	// CIHalf 为 95% 置信区间半宽 h = 1.96 * sd / sqrt(n)；n < 2 时取 0（C-09、C-10）。
	CIHalf float64

	ConfidenceScore float64
	ConfidenceLevel ConfidenceLevel
}

// WindowResult 是一个时间窗口在一条提示词上的全部统计结果。
type WindowResult struct {
	Kind WindowKind

	Models []ModelResult

	// ValidModelCount 是有效模型数：该窗口内有效评分数达到每模型最低条数的模型个数（C-05）。
	ValidModelCount int

	// TotalRatingCount 是该窗口内的总有效评分数。
	TotalRatingCount int

	// TotalUserCount 是该窗口内的独立评分用户数，由调用方传入去重后的真实值。
	TotalUserCount int

	// Discrimination 是区分度（需求 §4.4）。Sufficient 为 false 时无意义。
	Discrimination float64
	Sufficient     bool
}

// ComputeWindow 对一个时间窗口内的全部模型完成聚合计算。
//
// inputs 可以包含「有历史评分但近 N 日无评分」的模型（RatingCount 为 0）：这类模型在
// 窗口内没有观测值，不计入有效模型数，也不参与极差计算，但仍保留在返回的 Models 中，
// 由展示层区分「尚无评分」与「评分偏低」（开发方案 §6.4 专项回归）。
//
// totalUserCount 必须是该窗口内跨模型去重后的用户数。各模型的用户集合互相重叠，
// 相加会高估，而高估会直接把置信度抬到高档。
func ComputeWindow(
	kind WindowKind,
	inputs []ModelInput,
	totalUserCount int,
	bayes BayesParams,
	disc DiscriminationParams,
	conf ConfidenceParams,
) WindowResult {
	out := WindowResult{
		Kind:           kind,
		TotalUserCount: totalUserCount,
		Models:         make([]ModelResult, 0, len(inputs)),
	}

	adjustedValid := make([]float64, 0, len(inputs))
	for _, in := range inputs {
		item := ComputeModel(in, bayes, conf)
		out.Models = append(out.Models, item)
		out.TotalRatingCount += int(item.RatingCount)
		if !item.HasData {
			continue
		}
		if int(item.RatingCount) >= disc.MinRatingsPerModel {
			out.ValidModelCount++
			adjustedValid = append(adjustedValid, item.Adjusted)
		}
	}

	// 数据充分性门槛（需求 §4.3）：有效模型数、每模型评分数、总评分数三项同时满足。
	out.Sufficient = out.ValidModelCount >= disc.MinModels &&
		out.TotalRatingCount >= disc.MinTotalRatings &&
		len(adjustedValid) == out.ValidModelCount

	if out.Sufficient {
		out.Discrimination = ComputeDiscrimination(adjustedValid, out.ValidModelCount,
			out.TotalRatingCount, disc)
	}
	return out
}

// ComputeModel 计算单个模型在一个窗口内的完成度统计量。
func ComputeModel(in ModelInput, bayes BayesParams, conf ConfidenceParams) ModelResult {
	out := ModelResult{
		ModelID:     in.ModelID,
		ModelName:   in.ModelName,
		RatingCount: in.RatingCount,
		UserCount:   in.UserCount,
		HasData:     in.RatingCount > 0,
	}

	if !out.HasData {
		// 无评分：校准分退化为先验值，标准差与半宽取 0。置信度为 0 而不是先验值本身
		// 的贡献：没有观测数据就没有任何证据支撑（C-10）。
		out.Adjusted = bayes.Prior
		out.ConfidenceLevel = ConfidenceLow
		return out
	}

	n := float64(in.RatingCount)
	out.Mean = in.ScoreSum / n
	out.Adjusted = AdjustedScore(out.Mean, in.RatingCount, bayes)
	out.SD = SampleStdDev(in.ScoreSquare, in.ScoreSum, in.RatingCount)
	out.CIHalf = CIHalfWidth(out.SD, in.RatingCount)
	out.ConfidenceScore = ConfidenceScore(in.RatingCount, in.UserCount, out.CIHalf, conf)
	out.ConfidenceLevel = CompletionConfidenceLevel(out.ConfidenceScore, in.RatingCount, in.UserCount)
	return out
}

// AdjustedScore 是贝叶斯校准平均分（需求 §3.6）：
//
//	adjusted = (n * mean + k * prior) / (n + k)
//
// n 为 0 时分母仍为 k，结果恰为先验值。
func AdjustedScore(mean float64, n int64, bayes BayesParams) float64 {
	denominator := float64(n) + bayes.K
	if denominator == 0 {
		return bayes.Prior
	}
	return (float64(n)*mean + bayes.K*bayes.Prior) / denominator
}

// SampleStdDev 返回样本标准差，分母取 n-1（需求 §5.1）。
//
// 传入的是评分平方和而非逐条评分：标准差可由 (Σx², Σx, n) 还原同一个式子，
// 由数据库聚合得出即可，无需把全部评分取回内存。
//
//	sd = sqrt(max(0, (Σx² - (Σx)²/n) / (n-1)))
//
// n < 2 时取 0（C-10）。
func SampleStdDev(scoreSquare, scoreSum float64, n int64) float64 {
	if n < 2 {
		return 0
	}
	nf := float64(n)
	variance := (scoreSquare - (scoreSum*scoreSum)/nf) / (nf - 1)
	if variance <= 0 {
		// 所有评分相同时浮点运算可能得到极小的负数，取 0 避免开方得到 NaN。
		return 0
	}
	return math.Sqrt(variance)
}

// CIHalfWidth 返回 95% 置信区间半宽（需求 §5.1、C-09）：
//
//	h = 1.96 * sd / sqrt(n)
//
// n < 2 时取 0（C-10）。
func CIHalfWidth(sd float64, n int64) float64 {
	if n < 2 {
		return 0
	}
	return 1.96 * sd / math.Sqrt(float64(n))
}

// ConfidenceScore 是完成度置信度分数（需求 §5.1）：
//
//	confidence = 0.4 * min(1, n/20)
//	            + 0.3 * min(1, u/5)
//	            + 0.3 * max(0, 1 - h/1.0)
func ConfidenceScore(n, u int64, halfWidth float64, conf ConfidenceParams) float64 {
	first := 0.4 * clamp01(float64(n)/conf.RatingTarget)
	second := 0.3 * clamp01(float64(u)/conf.UserTarget)
	third := 0.3 * clamp01(1-halfWidth/conf.HalfWidthScale)
	return first + second + third
}

// 完成度置信度的等级门槛（需求 §5.1）。这几个数值在需求里是直接给定的常量，
// 不随配置变化，因此写成常量而不是从 ConfidenceParams 推导。
const (
	highConfidenceScore       = 0.75
	mediumConfidenceScore     = 0.50
	completionHighMinRating   = 20
	completionHighMinUser     = 5
	completionMediumMinRating = 8
	completionMediumMinUser   = 3
)

// CompletionConfidenceLevel 按需求 §5.1 的门槛给出完成度置信度等级。
//
// 等级先由分数落入哪一档决定，再校验该档的两个硬门槛：分数落在高档区间而硬门槛不满足
// 时判为低，不向下顺延到中档（C-11）。硬门槛的作用是防止小样本被公式抬上去。
func CompletionConfidenceLevel(score float64, n, u int64) ConfidenceLevel {
	switch {
	case score >= highConfidenceScore:
		if n >= completionHighMinRating && u >= completionHighMinUser {
			return ConfidenceHigh
		}
		return ConfidenceLow
	case score >= mediumConfidenceScore:
		if n >= completionMediumMinRating && u >= completionMediumMinUser {
			return ConfidenceMedium
		}
		return ConfidenceLow
	default:
		return ConfidenceLow
	}
}

// ComputeDiscrimination 计算区分度（需求 §4.4）：
//
//	range = max(adjusted) - min(adjusted)
//	data_coefficient = min(1, valid_model_count / 4) * min(1, total_rating_count / 20)
//	discrimination = clamp(1 + range * data_coefficient, 1, 5)
//
// adjusted 只包含有效模型的校准分。
func ComputeDiscrimination(adjusted []float64, validModelCount, totalRatingCount int, disc DiscriminationParams) float64 {
	if len(adjusted) == 0 {
		return 1
	}
	minValue, maxValue := adjusted[0], adjusted[0]
	for _, v := range adjusted[1:] {
		if v < minValue {
			minValue = v
		}
		if v > maxValue {
			maxValue = v
		}
	}
	spread := maxValue - minValue
	coefficient := DataCoefficient(validModelCount, totalRatingCount, disc)
	return Clamp(1+spread*coefficient, 1, 5)
}

// DataCoefficient 返回数据充分系数（需求 §4.4）。
func DataCoefficient(validModelCount, totalRatingCount int, disc DiscriminationParams) float64 {
	models := 1.0
	if disc.ModelCountTarget > 0 {
		models = clamp01(float64(validModelCount) / disc.ModelCountTarget)
	}
	ratings := 1.0
	if disc.RatingCountTarget > 0 {
		ratings = clamp01(float64(totalRatingCount) / disc.RatingCountTarget)
	}
	return models * ratings
}

func clamp01(v float64) float64 { return Clamp(v, 0, 1) }
