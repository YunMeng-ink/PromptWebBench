package formula

import "strconv"

// 推荐标签类型，与 recommend_tags.kind 的 CHECK 约束取值一致。
const (
	TagObsolete            = "obsolete"
	TagSuspectedObsolete   = "suspected_obsolete"
	TagSaturation          = "saturation"
	TagSuspectedSaturation = "suspected_saturation"
)

// 推荐标签级别，与 recommend_tags.scope 的取值一致。
const (
	ScopePrompt      = "prompt"
	ScopePromptModel = "prompt_model"
)

// ObsoleteVerdict 是「过时」类推荐的判定结果（需求 §6.1）。
type ObsoleteVerdict struct {
	// Kind 为空表示不推荐任何过时类标签。
	Kind string

	// Recovered 为真表示历史区分度偏低而近 N 日区分度回升，系统不推荐过时，
	// 已存在的待确认标签应转已忽略并记审计日志（C-15）。
	Recovered bool

	Reason string
}

// JudgeObsolete 判定过时类推荐（需求 §6.1、C-06、C-15）。
//
// 输入是两个窗口的区分度与区分度置信度。数据不足时一律不推荐：需求 §4.3 明确
// 数据不足不触发过时标签，前端显示「数据不足，暂不计算区分度」（C-06）。
//
// 判定顺序与需求一致：
//   - 两个窗口都达标且置信度中以上 → 推荐「过时」；
//   - 历史偏低但近 N 日数据不足 → 「疑似过时」；
//   - 两个窗口都偏低但置信度为低 → 「疑似过时」；
//   - 历史偏低而近 N 日回升 → 不推荐，标记 Recovered。
func JudgeObsolete(history, recent WindowSummary, disc DiscriminationParams) ObsoleteVerdict {
	historyLow := history.Sufficient && history.Discrimination <= disc.ObsoleteThreshold

	if !historyLow {
		return ObsoleteVerdict{}
	}
	if !recent.Sufficient {
		return ObsoleteVerdict{Kind: TagSuspectedObsolete, Reason: "历史区分度偏低，近 N 日数据不足，暂不判定"}
	}
	if recent.Discrimination > disc.ObsoleteThreshold {
		return ObsoleteVerdict{Recovered: true, Reason: "近期区分度回升，不再推荐过时"}
	}

	switch recent.ConfidenceLevel {
	case ConfidenceHigh, ConfidenceMedium:
		return ObsoleteVerdict{Kind: TagObsolete, Reason: "历史与近 N 日区分度均偏低，区分度置信度中以上"}
	default:
		return ObsoleteVerdict{
			Kind:   TagSuspectedObsolete,
			Reason: "历史与近 N 日区分度均偏低，但区分度置信度为低",
		}
	}
}

// SaturationVerdict 是「训练饱和」类的判定结果（需求 §6.2）。
type SaturationVerdict struct {
	// Kind 为空表示不推荐任何训练饱和类标签。
	Kind string

	// High 为真表示历史校准完成度也在高位，推荐置信度可提升为高（需求 §6.2）。
	High bool

	Reason string
}

// JudgeSaturation 判定某个模型在近 N 日窗口的训练饱和推荐（需求 §6.2）。
//
// 判断只用近 N 日校准完成度：模型达到饱和是当前状态的结论，历史只用于决定置信度
// 能否提升为高。
func JudgeSaturation(recent ModelResult, history ModelResult, threshold float64) SaturationVerdict {
	if !recent.HasData || recent.Adjusted < threshold {
		return SaturationVerdict{}
	}

	label := "该模型近 N 日校准完成度已达 " + trimFloat(threshold)
	switch recent.ConfidenceLevel {
	case ConfidenceHigh, ConfidenceMedium:
		verdict := SaturationVerdict{Kind: TagSaturation, Reason: label + "，评分置信度中以上"}
		if history.HasData && history.Adjusted >= threshold {
			verdict.High = true
			verdict.Reason += "，历史校准完成度同样处于高位"
		}
		return verdict
	default:
		return SaturationVerdict{Kind: TagSuspectedSaturation, Reason: label + "，但评分置信度为低"}
	}
}

// WindowSummary 是标签判定所需的窗口汇总量。
type WindowSummary struct {
	Sufficient      bool
	Discrimination  float64
	ConfidenceLevel ConfidenceLevel
}

// trimFloat 让阈值 4.5 显示为「4.5」、5 显示为「5」，避免推荐理由里出现 4.500000。
func trimFloat(v float64) string {
	formatted := strconv.FormatFloat(v, 'f', -1, 64)
	return formatted
}
