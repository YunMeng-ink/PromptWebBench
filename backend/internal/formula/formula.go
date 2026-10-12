// Package formula 承载平台全部数值计算：贝叶斯校准平均分、区分度、完成度置信度
// 与 Bootstrap 区分度置信度（需求 §3.6、§4.4、§5.1、§5.2）。
//
// 本包只做纯计算，不做 I/O、不读配置、不读时钟：全部参数由调用方以结构体传入，
// 随机源由调用方注入。这样每一个公式都能脱离数据库单测，算例可以逐条复现
// （开发方案 §2.3 层职责边界）。
package formula

import "math"

// ConfidenceLevel 是置信度等级（需求 §5.1、§5.2）。
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

// WindowKind 是时间窗口的种类。两个窗口共用全部公式，只在取值来源与展示位置上不同
// （需求 §3.3）。
type WindowKind string

const (
	// WindowHistory 历史窗口：该提示词在该模型上的全部有效评分。
	WindowHistory WindowKind = "history"
	// WindowRecent 近 N 日窗口：最近 N 个 UTC 自然日内的有效评分，含当天（C-03）。
	WindowRecent WindowKind = "recent"
)

// BayesParams 是贝叶斯校准参数（需求 §3.6，键 bayes_prior / bayes_k）。
type BayesParams struct {
	Prior float64
	K     float64
}

// DefaultBayes 是需求给出的默认值：prior = 3，k = 5。
func DefaultBayes() BayesParams {
	return BayesParams{Prior: 3, K: 5}
}

// DiscriminationParams 是区分度的门槛与系数分母（需求 §4.3、§4.4）。
//
// MinModels、MinRatingsPerModel、MinTotalRatings 是数据充分性门槛；
// ModelCountTarget、RatingCountTarget 是数据充分系数的分母。
// 门槛与分母是两个独立参数，强行统一会改变需求算例的结果值（C-07），因此分开声明。
type DiscriminationParams struct {
	MinModels           int
	MinRatingsPerModel  int
	MinTotalRatings     int
	ModelCountTarget    float64
	RatingCountTarget   float64
	ObsoleteThreshold   float64
	SaturationThreshold float64
}

// DefaultDiscrimination 是需求给出的默认值（可行性报告 §2.14 配置表）。
func DefaultDiscrimination() DiscriminationParams {
	return DiscriminationParams{
		MinModels:           3,
		MinRatingsPerModel:  3,
		MinTotalRatings:     10,
		ModelCountTarget:    4,
		RatingCountTarget:   20,
		ObsoleteThreshold:   2,
		SaturationThreshold: 4.5,
	}
}

// ConfidenceParams 是完成度置信度公式中的两个分母（需求 §5.1）。
// 它们参与分数计算，等级门槛则是需求 §5.1 直接给定的另一组常量，两者分开声明。
type ConfidenceParams struct {
	RatingTarget   float64
	UserTarget     float64
	HalfWidthScale float64
}

// DefaultConfidence 返回需求 §5.1 的默认值：评分条数分母 20、独立用户数分母 5、
// 半宽归一化尺度 1.0。
func DefaultConfidence() ConfidenceParams {
	return ConfidenceParams{RatingTarget: 20, UserTarget: 5, HalfWidthScale: 1}
}

// DiscriminationConfidenceParams 是区分度置信度的门槛（需求 §5.2）。
type DiscriminationConfidenceParams struct {
	HighMinModels        int
	HighMinTotalRatings  int
	HighMinPerModel      int
	HighMaxHalfWidth     float64
	MediumMinModels      int
	MediumMinTotalRating int
	MediumMinPerModel    int
	MediumMaxHalfWidth   float64
}

// DefaultDiscriminationConfidence 返回需求 §5.2 的默认值。
func DefaultDiscriminationConfidence() DiscriminationConfidenceParams {
	return DiscriminationConfidenceParams{
		HighMinModels:        4,
		HighMinTotalRatings:  30,
		HighMinPerModel:      5,
		HighMaxHalfWidth:     0.5,
		MediumMinModels:      3,
		MediumMinTotalRating: 15,
		MediumMinPerModel:    3,
		MediumMaxHalfWidth:   1,
	}
}

// Clamp 把取值夹到 [low, high]。区分度受 1～5 的量表约束，必须夹紧
// （需求 §4.4 限制范围）。
func Clamp(v, low, high float64) float64 {
	switch {
	case v < low:
		return low
	case v > high:
		return high
	default:
		return v
	}
}

// Round 保留两位小数，与数据库 NUMERIC(4,2) 的存储精度一致：
// 服务层算出的值直接落库，不做同样的四舍五入会让展示值与快照值出现末位差。
func Round(v float64) float64 {
	return math.Round(v*100) / 100
}
