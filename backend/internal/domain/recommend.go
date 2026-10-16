package domain

// RecommendTagKind 是推荐标签的类型（需求 §6.1、§6.2）。取值与
// recommend_tags.kind 的 CHECK 约束一致，也与 formula 包生成标签时的取值一致。
type RecommendTagKind string

const (
	RecommendObsolete            RecommendTagKind = "obsolete"
	RecommendSuspectedObsolete   RecommendTagKind = "suspected_obsolete"
	RecommendSaturation          RecommendTagKind = "saturation"
	RecommendSuspectedSaturation RecommendTagKind = "suspected_saturation"
)

// AllRecommendTagKinds 返回全部推荐标签类型。
func AllRecommendTagKinds() []RecommendTagKind {
	return []RecommendTagKind{
		RecommendObsolete, RecommendSuspectedObsolete,
		RecommendSaturation, RecommendSuspectedSaturation,
	}
}

// RecommendTagKindStrings 返回全部推荐标签类型的字符串形式，供 SQL 参数与枚举声明使用。
func RecommendTagKindStrings() []string {
	kinds := AllRecommendTagKinds()
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, string(kind))
	}
	return out
}

// RecommendTagState 是推荐标签的状态（需求 §6.3、C-12 ~ C-15）。
type RecommendTagState string

const (
	RecommendPending   RecommendTagState = "pending"
	RecommendConfirmed RecommendTagState = "confirmed"
	RecommendIgnored   RecommendTagState = "ignored"
	RecommendExpired   RecommendTagState = "expired"
	RecommendReview    RecommendTagState = "review"
)

// AllRecommendTagStates 返回全部推荐标签状态。
func AllRecommendTagStates() []RecommendTagState {
	return []RecommendTagState{
		RecommendPending, RecommendConfirmed, RecommendIgnored,
		RecommendExpired, RecommendReview,
	}
}

// ParseRecommendTagState 解析推荐标签状态，非法取值返回 false。
func ParseRecommendTagState(raw string) (RecommendTagState, bool) {
	for _, state := range AllRecommendTagStates() {
		if RecommendTagState(raw) == state {
			return state, true
		}
	}
	return "", false
}

// RecommendTagLabelMaxRunes 是推荐标签文案的字数上限。管理员在处置时可以改写文案。
const RecommendTagLabelMaxRunes = 100
