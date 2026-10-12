package formula

import (
	"math"
	"math/rand/v2"
	"sort"
)

// BootstrapParams 是 Bootstrap 重抽样的参数（需求 §5.2、C-08）。
type BootstrapParams struct {
	Iterations int
}

// DefaultBootstrap 返回需求给出的重复次数：500 次（C-08）。
func DefaultBootstrap() BootstrapParams {
	return BootstrapParams{Iterations: 500}
}

// SamplingSource 是 Bootstrap 的抽样来源。
//
// 随机源可注入，保证同一组输入在测试中重复得到同一结果（开发方案 §6.4 测试重点）。
// 生产调用方不构造本类型，取全局源。
type SamplingSource struct {
	rand *rand.Rand
}

// NewSamplingSource 用给定种子构造可复现的抽样源。
func NewSamplingSource(seed uint64) SamplingSource {
	return SamplingSource{rand: rand.New(rand.NewPCG(seed, 0x9E3779B97F4A7C15))}
}

func (s SamplingSource) next(n int) int {
	if s.rand == nil {
		return rand.IntN(n)
	}
	return s.rand.IntN(n)
}

// SampleSet 是一个模型在一个窗口内的原始评分样本，Bootstrap 的重抽样单元。
type SampleSet struct {
	Scores []float64
}

// BootstrapResult 是区分度置信度的计算结果。
type BootstrapResult struct {
	// PointEstimate 是用全部样本直接算出的区分度，作为点估计与置信区间的中心参照。
	PointEstimate    float64
	CIConfidenceLow  float64
	CIConfidenceHigh float64

	// HalfWidth 是 95% 置信区间半宽 (high - low) / 2。
	HalfWidth float64

	// MinRatingsPerModel 是各有效模型中最少的评分数，作为置信度门槛的输入。
	MinRatingsPerModel int

	ConfidenceLevel ConfidenceLevel
}

// BootstrapDiscrimination 对区分度执行 Bootstrap 重抽样，返回 95% 置信区间
// 与由该区间得出的置信度等级（需求 §5.2、C-08、C-09）。
//
// 步骤与需求一致：每个模型有放回重抽样 → 重算校准完成度 → 重算区分度 → 重复
// Iterations 次 → 取 2.5% 与 97.5% 分位。
//
// 只对有效模型重抽样：数据不足的模型本就不参与区分度计算，把它纳入会让重算出的
// 有效模型数在迭代之间抖动，置信区间被稀释成无意义的宽区间。
func BootstrapDiscrimination(
	samples []SampleSet,
	validModelCount, totalRatingCount int,
	bayes BayesParams,
	disc DiscriminationParams,
	conf DiscriminationConfidenceParams,
	params BootstrapParams,
	source SamplingSource,
) BootstrapResult {
	out := BootstrapResult{ConfidenceLevel: ConfidenceLow}

	adjusted := make([]float64, 0, len(samples))
	minPerModel := 0
	for i, set := range samples {
		adjusted = append(adjusted, AdjustedScore(mean(set.Scores), int64(len(set.Scores)), bayes))
		if i == 0 || len(set.Scores) < minPerModel {
			minPerModel = len(set.Scores)
		}
	}
	out.MinRatingsPerModel = minPerModel
	out.PointEstimate = ComputeDiscrimination(adjusted, validModelCount, totalRatingCount, disc)

	if params.Iterations <= 0 || len(samples) == 0 {
		return out
	}

	estimates := make([]float64, 0, params.Iterations)
	resampled := make([]float64, 0, len(samples))
	buffer := make([]float64, 0, maxSampleLen(samples))
	for i := 0; i < params.Iterations; i++ {
		resampled = resampled[:0]
		for _, set := range samples {
			buffer = resample(buffer[:0], set.Scores, source)
			resampled = append(resampled, AdjustedScore(mean(buffer), int64(len(buffer)), bayes))
		}
		estimates = append(estimates,
			ComputeDiscrimination(resampled, validModelCount, totalRatingCount, disc))
	}

	sort.Float64s(estimates)
	out.CIConfidenceLow = percentile(estimates, 0.025)
	out.CIConfidenceHigh = percentile(estimates, 0.975)
	out.HalfWidth = (out.CIConfidenceHigh - out.CIConfidenceLow) / 2
	out.ConfidenceLevel = DiscriminationConfidenceLevel(conf, validModelCount,
		totalRatingCount, minPerModel, out.HalfWidth)
	return out
}

// resample 对样本做有放回重抽样，结果写入 dst 并返回。dst 复用使迭代过程零分配。
func resample(dst []float64, scores []float64, source SamplingSource) []float64 {
	n := len(scores)
	if n == 0 {
		return dst[:0]
	}
	dst = dst[:0]
	for i := 0; i < n; i++ {
		dst = append(dst, scores[source.next(n)])
	}
	return dst
}

func maxSampleLen(samples []SampleSet) int {
	longest := 0
	for _, set := range samples {
		if len(set.Scores) > longest {
			longest = len(set.Scores)
		}
	}
	return longest
}

func mean(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, v := range scores {
		sum += v
	}
	return sum / float64(len(scores))
}

// percentile 返回有序样本的分位数，位置采用线性插值。
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := q * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// DiscriminationConfidenceLevel 按需求 §5.2 的门槛给出区分度置信度等级。
//
// 四组条件（有效模型数、总评分数、每模型评分数、区间半宽）全部满足才判高档，
// 否则看是否满足中档的对应条件，都不满足为低。
func DiscriminationConfidenceLevel(
	conf DiscriminationConfidenceParams,
	validModelCount, totalRatingCount, minPerModelRatings int,
	halfWidth float64,
) ConfidenceLevel {
	meetsHigh := validModelCount >= conf.HighMinModels &&
		totalRatingCount >= conf.HighMinTotalRatings &&
		minPerModelRatings >= conf.HighMinPerModel &&
		halfWidth <= conf.HighMaxHalfWidth
	if meetsHigh {
		return ConfidenceHigh
	}
	meetsMedium := validModelCount >= conf.MediumMinModels &&
		totalRatingCount >= conf.MediumMinTotalRating &&
		minPerModelRatings >= conf.MediumMinPerModel &&
		halfWidth <= conf.MediumMaxHalfWidth
	if meetsMedium {
		return ConfidenceMedium
	}
	return ConfidenceLow
}
