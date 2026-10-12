package formula

import (
	"math"
	"testing"
)

// buildInput 由一组评分构造 ModelInput，聚合量与 SQL 侧等价。
func buildInput(modelID int64, name string, scores ...int64) ModelInput {
	in := ModelInput{ModelID: modelID, ModelName: name}
	for _, s := range scores {
		v := float64(s)
		in.ScoreSum += v
		in.ScoreSquare += v * v
		in.RatingCount++
	}
	in.UserCount = in.RatingCount
	return in
}

func TestAdjustedScore复现需求公式(t *testing.T) {
	bayes := DefaultBayes()

	cases := []struct {
		name string
		mean float64
		n    int64
		want float64
	}{
		{"零评分退化为先验", 0, 0, 3},
		{"单条评分被拉向先验", 5, 1, (1*5 + 5*3.0) / 6.0},
		{"五条评分与先验等权", 5, 5, (5*5 + 5*3.0) / 10.0},
		{"大量评分贴近原始均值", 4.6, 100, (100*4.6 + 5*3.0) / 105.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AdjustedScore(c.mean, c.n, bayes)
			if math.Abs(got-c.want) > 1e-9 {
				t.Fatalf("校准平均分 = %v, 期望 %v", got, c.want)
			}
		})
	}
}

func TestAdjustedScore服从k为零时退化为原始平均分(t *testing.T) {
	got := AdjustedScore(4.2, 7, BayesParams{Prior: 3, K: 0})
	if math.Abs(got-4.2) > 1e-9 {
		t.Fatalf("k = 0 时校准平均分 = %v, 期望等于原始平均分 4.2", got)
	}
}

func Test区分度复现需求算例一(t *testing.T) {
	// 需求 §4.5 示例一：range = 3.6，有效模型 4 个，总评分 25 条，系数为 1，
	// 区分度 = 4.6。
	adjusted := []float64{1.4, 2.6, 4.3, 5.0}
	got := ComputeDiscrimination(adjusted, 4, 25, DefaultDiscrimination())
	if math.Abs(got-4.6) > 1e-9 {
		t.Fatalf("区分度 = %v, 期望 4.6", got)
	}
}

func Test区分度复现需求算例二(t *testing.T) {
	// 需求 §4.5 示例二：range = 0.2，系数为 1，区分度 = 1.2。
	adjusted := []float64{3.1, 3.2, 3.0, 3.2}
	got := ComputeDiscrimination(adjusted, 4, 25, DefaultDiscrimination())
	if math.Abs(got-1.2) > 1e-9 {
		t.Fatalf("区分度 = %v, 期望 1.2", got)
	}
}

func Test区分度极差为零时为下界(t *testing.T) {
	got := ComputeDiscrimination([]float64{3, 3, 3, 3}, 4, 40, DefaultDiscrimination())
	if got != 1 {
		t.Fatalf("全部模型相同时区分度 = %v, 期望 1", got)
	}
}

func Test区分度被夹紧在量表区间内(t *testing.T) {
	// 校准分理论取值在 [1,5]，极差最大 4，公式结果最大 5；此处用越界输入验证夹紧。
	got := ComputeDiscrimination([]float64{0, 10}, 4, 40, DefaultDiscrimination())
	if got != 5 {
		t.Fatalf("极差 10 时区分度 = %v, 期望夹紧到 5", got)
	}
}

func Test区分度无有效模型时为下界(t *testing.T) {
	if got := ComputeDiscrimination(nil, 0, 0, DefaultDiscrimination()); got != 1 {
		t.Fatalf("无有效模型时区分度 = %v, 期望 1", got)
	}
}

func Test数据充分系数按两个分母独立取值(t *testing.T) {
	disc := DefaultDiscrimination()
	cases := []struct {
		models  int
		ratings int
		want    float64
	}{
		{0, 0, 0},
		{2, 40, 0.5},
		{4, 10, 0.5},
		{4, 20, 1},
		{10, 100, 1},
	}
	for _, c := range cases {
		got := DataCoefficient(c.models, c.ratings, disc)
		if math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("系数(%d 模型, %d 评分) = %v, 期望 %v", c.models, c.ratings, got, c.want)
		}
	}
}

func Test数据充分门槛三项同时满足(t *testing.T) {
	disc := DefaultDiscrimination()
	bayes := DefaultBayes()
	conf := DefaultConfidence()

	// 三个模型各 3 条评分共 9 条：总评分数不足 10，门槛不满足。
	inputs := []ModelInput{
		buildInput(1, "A", 4, 4, 4),
		buildInput(2, "B", 3, 3, 3),
		buildInput(3, "C", 2, 2, 2),
	}
	got := ComputeWindow(WindowHistory, inputs, 9, bayes, disc, conf)
	if got.Sufficient {
		t.Fatal("总评分数 9 条低于门槛 10 条时不应判定为数据充足")
	}
	if got.Discrimination != 0 {
		t.Fatalf("数据不足时区分度应为 0，实际 %v", got.Discrimination)
	}

	// 补一条到 10 条：三项门槛同时满足。
	inputs[2] = buildInput(3, "C", 2, 2, 2, 2)
	got = ComputeWindow(WindowHistory, inputs, 10, bayes, disc, conf)
	if !got.Sufficient {
		t.Fatal("三项门槛同时满足时应判定为数据充足")
	}
	if got.ValidModelCount != 3 {
		t.Fatalf("有效模型数 = %d, 期望 3", got.ValidModelCount)
	}
	if got.TotalRatingCount != 10 {
		t.Fatalf("总评分数 = %d, 期望 10", got.TotalRatingCount)
	}
}

func Test近N日无评分的模型不计入有效模型与极差(t *testing.T) {
	// 开发方案 §6.4 专项回归的数据形态：模型有历史评分但近 N 日无评分。
	// 该模型必须被保留在输出中，但不参与极差计算，否则区分度被人为拉高。
	disc := DefaultDiscrimination()
	bayes := DefaultBayes()
	conf := DefaultConfidence()

	inputs := []ModelInput{
		buildInput(1, "A", 5, 5, 5, 5, 5, 5, 5, 5),
		buildInput(2, "B", 4, 4, 4, 4, 4, 4, 4, 4),
		buildInput(3, "C", 5, 5, 5, 5, 5, 5, 5, 5),
		{ModelID: 4, ModelName: "D", RatingCount: 0},
	}
	got := ComputeWindow(WindowRecent, inputs, 24, bayes, disc, conf)

	if len(got.Models) != 4 {
		t.Fatalf("输出模型数 = %d, 期望保留全部 4 个", len(got.Models))
	}
	if got.Models[3].HasData {
		t.Fatal("无评分的模型不应标记为有数据")
	}
	if got.Models[3].Adjusted != bayes.Prior {
		t.Fatalf("无评分的模型校准分 = %v, 期望退化为先验 %v", got.Models[3].Adjusted, bayes.Prior)
	}
	if got.Models[3].ConfidenceLevel != ConfidenceLow {
		t.Fatalf("无评分的模型置信度 = %v, 期望为低", got.Models[3].ConfidenceLevel)
	}
	if got.ValidModelCount != 3 {
		t.Fatalf("有效模型数 = %d, 期望 3", got.ValidModelCount)
	}

	// 只有 A、B、C 参与极差；无评分的 D 若被纳入会把最小值拉为先验值 3。
	onlyABC := ComputeDiscrimination([]float64{
		got.Models[0].Adjusted, got.Models[1].Adjusted, got.Models[2].Adjusted,
	}, got.ValidModelCount, got.TotalRatingCount, disc)
	if math.Abs(got.Discrimination-onlyABC) > 1e-9 {
		t.Fatalf("区分度 = %v, 期望与只含有效模型的计算一致 %v", got.Discrimination, onlyABC)
	}
}

func Test模型统计量边界(t *testing.T) {
	bayes := DefaultBayes()
	conf := DefaultConfidence()

	t.Run("单条评分标准差与半宽均为零", func(t *testing.T) {
		got := ComputeModel(buildInput(1, "A", 4), bayes, conf)
		if got.SD != 0 {
			t.Fatalf("n = 1 时标准差 = %v, 期望 0", got.SD)
		}
		if got.CIHalf != 0 {
			t.Fatalf("n = 1 时半宽 = %v, 期望 0", got.CIHalf)
		}
		if got.ConfidenceLevel != ConfidenceLow {
			t.Fatalf("n = 1 时置信度 = %v, 期望为低", got.ConfidenceLevel)
		}
	})

	t.Run("全部评分相同时标准差为零", func(t *testing.T) {
		got := ComputeModel(buildInput(1, "A", 5, 5, 5, 5, 5, 5), bayes, conf)
		if got.SD != 0 {
			t.Fatalf("常数评分的标准差 = %v, 期望 0", got.SD)
		}
		if math.IsNaN(got.SD) {
			t.Fatal("标准差不得为 NaN")
		}
	})

	t.Run("零评分模型校准分为先验", func(t *testing.T) {
		got := ComputeModel(ModelInput{ModelID: 1, ModelName: "A"}, bayes, conf)
		if got.Adjusted != bayes.Prior {
			t.Fatalf("零评分校准分 = %v, 期望先验 %v", got.Adjusted, bayes.Prior)
		}
		if got.ConfidenceScore != 0 {
			t.Fatalf("零评分置信度分数 = %v, 期望 0", got.ConfidenceScore)
		}
	})
}

func Test样本标准差与半宽符合公式(t *testing.T) {
	// 评分 2,4,4,4,5,5,7,9：均值 5，样本方差 32/7，标准差约 2.13809。
	in := buildInput(1, "A", 2, 4, 4, 4, 5, 5, 7, 9)
	got := ComputeModel(in, DefaultBayes(), DefaultConfidence())

	if math.Abs(got.Mean-5) > 1e-9 {
		t.Fatalf("原始平均分 = %v, 期望 5", got.Mean)
	}
	wantSD := math.Sqrt(32.0 / 7.0)
	if math.Abs(got.SD-wantSD) > 1e-9 {
		t.Fatalf("标准差 = %v, 期望 %v", got.SD, wantSD)
	}
	wantHalf := 1.96 * wantSD / math.Sqrt(8)
	if math.Abs(got.CIHalf-wantHalf) > 1e-9 {
		t.Fatalf("半宽 = %v, 期望 %v", got.CIHalf, wantHalf)
	}
}

func Test完成度置信度分数按需求公式(t *testing.T) {
	conf := DefaultConfidence()

	// n = 20、u = 5、半宽 0：0.4 + 0.3 + 0.3 = 1.0
	if got := ConfidenceScore(20, 5, 0, conf); math.Abs(got-1) > 1e-9 {
		t.Fatalf("满分情形置信度 = %v, 期望 1", got)
	}
	// n = 10、u = 3、半宽 0.5：0.2 + 0.18 + 0.15 = 0.53
	if got := ConfidenceScore(10, 3, 0.5, conf); math.Abs(got-0.53) > 1e-9 {
		t.Fatalf("部分情形置信度 = %v, 期望 0.53", got)
	}
	// 半宽超过 1 时第三项为 0，不产生负分。
	if got := ConfidenceScore(1, 1, 2, conf); got < 0 {
		t.Fatalf("半宽超出归一化尺度时置信度 = %v, 期望不为负", got)
	}
}

func Test完成度置信度等级的硬门槛(t *testing.T) {
	cases := []struct {
		name  string
		score float64
		n     int64
		u     int64
		want  ConfidenceLevel
	}{
		{"分数与门槛均达标判高", 0.8, 20, 5, ConfidenceHigh},
		{"分数达标但评分数不足判低", 0.8, 19, 5, ConfidenceLow},
		{"分数达标但用户数不足判低", 0.8, 20, 4, ConfidenceLow},
		{"中档全部达标判中", 0.55, 8, 3, ConfidenceMedium},
		{"中档分数达标但评分数不足判低", 0.55, 7, 3, ConfidenceLow},
		{"其余情况判低", 0.2, 3, 1, ConfidenceLow},
		{"0.75 边界值达标判高", 0.75, 20, 5, ConfidenceHigh},
		{"0.50 边界值达标判中", 0.5, 8, 3, ConfidenceMedium},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CompletionConfidenceLevel(c.score, c.n, c.u)
			if got != c.want {
				t.Fatalf("等级 = %v, 期望 %v", got, c.want)
			}
		})
	}
}

func Test过时类推荐判定(t *testing.T) {
	disc := DefaultDiscrimination()

	cases := []struct {
		name      string
		history   WindowSummary
		recent    WindowSummary
		wantKind  string
		recovered bool
	}{
		{
			name:     "两个窗口均偏低且置信度中以上推荐过时",
			history:  WindowSummary{Sufficient: true, Discrimination: 1.8},
			recent:   WindowSummary{Sufficient: true, Discrimination: 1.5, ConfidenceLevel: ConfidenceMedium},
			wantKind: TagObsolete,
		},
		{
			name:     "历史偏低而近 N 日数据不足推荐疑似过时",
			history:  WindowSummary{Sufficient: true, Discrimination: 1.8},
			recent:   WindowSummary{Sufficient: false},
			wantKind: TagSuspectedObsolete,
		},
		{
			name:     "两个窗口均偏低但置信度低推荐疑似过时",
			history:  WindowSummary{Sufficient: true, Discrimination: 1.8},
			recent:   WindowSummary{Sufficient: true, Discrimination: 1.5, ConfidenceLevel: ConfidenceLow},
			wantKind: TagSuspectedObsolete,
		},
		{
			name:      "历史偏低而近 N 日回升不推荐",
			history:   WindowSummary{Sufficient: true, Discrimination: 1.8},
			recent:    WindowSummary{Sufficient: true, Discrimination: 2.4, ConfidenceLevel: ConfidenceHigh},
			recovered: true,
		},
		{
			name:     "历史区分度不低不推荐",
			history:  WindowSummary{Sufficient: true, Discrimination: 3.1},
			recent:   WindowSummary{Sufficient: true, Discrimination: 1.5, ConfidenceLevel: ConfidenceHigh},
			wantKind: "",
		},
		{
			name:     "历史数据不足不触发过时标签",
			history:  WindowSummary{Sufficient: false},
			recent:   WindowSummary{Sufficient: true, Discrimination: 1.5, ConfidenceLevel: ConfidenceHigh},
			wantKind: "",
		},
		{
			name:     "区分度恰好等于阈值判为偏低",
			history:  WindowSummary{Sufficient: true, Discrimination: 2},
			recent:   WindowSummary{Sufficient: true, Discrimination: 2, ConfidenceLevel: ConfidenceMedium},
			wantKind: TagObsolete,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := JudgeObsolete(c.history, c.recent, disc)
			if got.Kind != c.wantKind {
				t.Fatalf("标签类型 = %q, 期望 %q", got.Kind, c.wantKind)
			}
			if got.Recovered != c.recovered {
				t.Fatalf("回升标记 = %v, 期望 %v", got.Recovered, c.recovered)
			}
			if got.Kind != "" && got.Reason == "" {
				t.Fatal("给出推荐时必须同时给出推荐理由")
			}
		})
	}
}

func Test训练饱和类推荐判定(t *testing.T) {
	threshold := DefaultDiscrimination().SaturationThreshold

	cases := []struct {
		name    string
		recent  ModelResult
		history ModelResult
		want    string
		high    bool
	}{
		{
			name:    "近 N 日达标且置信度中以上推荐训练饱和",
			recent:  ModelResult{HasData: true, Adjusted: 4.7, ConfidenceLevel: ConfidenceMedium},
			history: ModelResult{HasData: true, Adjusted: 4.2},
			want:    TagSaturation,
		},
		{
			name:    "历史同样处于高位时置信度提升为高",
			recent:  ModelResult{HasData: true, Adjusted: 4.7, ConfidenceLevel: ConfidenceMedium},
			history: ModelResult{HasData: true, Adjusted: 4.6},
			want:    TagSaturation,
			high:    true,
		},
		{
			name:    "近 N 日达标但置信度低推荐疑似训练饱和",
			recent:  ModelResult{HasData: true, Adjusted: 4.7, ConfidenceLevel: ConfidenceLow},
			history: ModelResult{HasData: true, Adjusted: 4.6},
			want:    TagSuspectedSaturation,
		},
		{
			name:    "未达阈值不推荐",
			recent:  ModelResult{HasData: true, Adjusted: 4.4, ConfidenceLevel: ConfidenceHigh},
			history: ModelResult{HasData: true, Adjusted: 4.6},
			want:    "",
		},
		{
			name:    "无评分不推荐",
			recent:  ModelResult{Adjusted: 3, ConfidenceLevel: ConfidenceLow},
			history: ModelResult{HasData: true, Adjusted: 4.6},
			want:    "",
		},
		{
			name:    "恰好等于阈值视为达标",
			recent:  ModelResult{HasData: true, Adjusted: 4.5, ConfidenceLevel: ConfidenceMedium},
			history: ModelResult{HasData: true, Adjusted: 1},
			want:    TagSaturation,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := JudgeSaturation(c.recent, c.history, threshold)
			if got.Kind != c.want {
				t.Fatalf("标签类型 = %q, 期望 %q", got.Kind, c.want)
			}
			if got.High != c.high {
				t.Fatalf("置信度提升标记 = %v, 期望 %v", got.High, c.high)
			}
			if got.Kind != "" && got.Reason == "" {
				t.Fatal("给出推荐时必须同时给出推荐理由")
			}
		})
	}
}
