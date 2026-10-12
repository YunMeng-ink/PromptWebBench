package formula

import (
	"math"
	"testing"
	"time"
)

// buildSamples 把评分列表转为重抽样单元。
func buildSamples(scores ...[]int64) []SampleSet {
	out := make([]SampleSet, 0, len(scores))
	for _, list := range scores {
		set := SampleSet{Scores: make([]float64, 0, len(list))}
		for _, s := range list {
			set.Scores = append(set.Scores, float64(s))
		}
		out = append(out, set)
	}
	return out
}

func TestBootstrap同一随机源可复现(t *testing.T) {
	samples := buildSamples(
		[]int64{2, 3, 4, 5, 4, 3, 4, 5},
		[]int64{4, 4, 4, 5, 5, 4, 3, 4},
		[]int64{5, 5, 5, 5, 4, 5, 5, 5},
		[]int64{5, 5, 5, 5, 5, 5, 5, 5},
	)
	run := func() BootstrapResult {
		return BootstrapDiscrimination(samples, 4, 32, DefaultBayes(),
			DefaultDiscrimination(), DefaultDiscriminationConfidence(),
			DefaultBootstrap(), NewSamplingSource(20261012))
	}

	first, second := run(), run()
	if first.CIConfidenceLow != second.CIConfidenceLow || first.CIConfidenceHigh != second.CIConfidenceHigh {
		t.Fatalf("同一种子两次结果不同：%v/%v 与 %v/%v",
			first.CIConfidenceLow, first.CIConfidenceHigh, second.CIConfidenceLow, second.CIConfidenceHigh)
	}
	if first.ConfidenceLevel != second.ConfidenceLevel {
		t.Fatalf("同一种子两次置信度等级不同：%v 与 %v", first.ConfidenceLevel, second.ConfidenceLevel)
	}
}

func TestBootstrap不同种子产生不同重抽样(t *testing.T) {
	samples := buildSamples(
		[]int64{1, 2, 3, 4, 5, 1, 2, 3},
		[]int64{2, 3, 4, 5, 4, 3, 2, 3},
		[]int64{4, 5, 5, 4, 5, 4, 5, 5},
	)
	first := BootstrapDiscrimination(samples, 3, 24, DefaultBayes(),
		DefaultDiscrimination(), DefaultDiscriminationConfidence(),
		DefaultBootstrap(), NewSamplingSource(1))
	second := BootstrapDiscrimination(samples, 3, 24, DefaultBayes(),
		DefaultDiscrimination(), DefaultDiscriminationConfidence(),
		DefaultBootstrap(), NewSamplingSource(2))

	if first.CIConfidenceLow == second.CIConfidenceLow && first.CIConfidenceHigh == second.CIConfidenceHigh {
		t.Fatal("不同种子应产生不同的重抽样结果")
	}
}

func TestBootstrap置信区间包住点估计且半宽为正(t *testing.T) {
	samples := buildSamples(
		[]int64{1, 2, 3, 4, 5, 2, 3, 4, 1, 2},
		[]int64{2, 3, 4, 3, 4, 2, 3, 4, 3, 2},
		[]int64{5, 4, 5, 5, 4, 5, 5, 4, 5, 5},
		[]int64{5, 5, 4, 5, 5, 5, 4, 5, 5, 5},
	)
	got := BootstrapDiscrimination(samples, 4, 40, DefaultBayes(),
		DefaultDiscrimination(), DefaultDiscriminationConfidence(),
		DefaultBootstrap(), NewSamplingSource(7))

	if got.CIConfidenceLow > got.PointEstimate || got.PointEstimate > got.CIConfidenceHigh {
		t.Fatalf("点估计 %v 应落在置信区间 [%v, %v] 内",
			got.PointEstimate, got.CIConfidenceLow, got.CIConfidenceHigh)
	}
	if got.HalfWidth <= 0 {
		t.Fatalf("半宽 = %v, 期望大于 0", got.HalfWidth)
	}
	if got.MinRatingsPerModel != 10 {
		t.Fatalf("每模型最少评分数 = %d, 期望 10", got.MinRatingsPerModel)
	}
}

func TestBootstrap模型评分完全相同时区间退化(t *testing.T) {
	// 每个模型的评分全部相同或有放回重抽样后仍取到同一组值时，区分度在迭代之间不波动，
	// 置信区间退化为一个点，半宽为 0。
	samples := buildSamples(
		[]int64{4, 4, 4, 4, 4},
		[]int64{4, 4, 4, 4, 4},
		[]int64{4, 4, 4, 4, 4},
		[]int64{4, 4, 4, 4, 4},
	)
	got := BootstrapDiscrimination(samples, 4, 20, DefaultBayes(),
		DefaultDiscrimination(), DefaultDiscriminationConfidence(),
		DefaultBootstrap(), NewSamplingSource(3))

	if got.HalfWidth != 0 {
		t.Fatalf("样本无差异时半宽 = %v, 期望 0", got.HalfWidth)
	}
}

func TestBootstrap零次迭代只返回点估计(t *testing.T) {
	samples := buildSamples(
		[]int64{1, 2, 3, 4, 5},
		[]int64{2, 3, 4, 5, 4},
		[]int64{3, 4, 5, 4, 5},
	)
	got := BootstrapDiscrimination(samples, 3, 15, DefaultBayes(),
		DefaultDiscrimination(), DefaultDiscriminationConfidence(),
		BootstrapParams{Iterations: 0}, NewSamplingSource(11))

	if got.HalfWidth != 0 {
		t.Fatalf("未执行重抽样时半宽 = %v, 期望 0", got.HalfWidth)
	}
	if got.ConfidenceLevel != ConfidenceLow {
		t.Fatalf("未执行重抽样时置信度 = %v, 期望为低", got.ConfidenceLevel)
	}
	if got.PointEstimate <= 0 {
		t.Fatalf("点估计 = %v, 期望仍给出取值", got.PointEstimate)
	}
}

func TestBootstrap无样本时安全返回(t *testing.T) {
	got := BootstrapDiscrimination(nil, 0, 0, DefaultBayes(),
		DefaultDiscrimination(), DefaultDiscriminationConfidence(),
		DefaultBootstrap(), SamplingSource{})

	if got.PointEstimate != 1 {
		t.Fatalf("无样本时点估计 = %v, 期望为 1", got.PointEstimate)
	}
	if got.ConfidenceLevel != ConfidenceLow {
		t.Fatalf("无样本时置信度 = %v, 期望为低", got.ConfidenceLevel)
	}
}

func Test区分度置信度等级门槛边界(t *testing.T) {
	conf := DefaultDiscriminationConfidence()
	cases := []struct {
		name      string
		models    int
		total     int
		perModel  int
		halfWidth float64
		want      ConfidenceLevel
	}{
		{"高档四项全部满足", 4, 30, 5, 0.5, ConfidenceHigh},
		{"半宽超过 0.5 降为中档", 4, 30, 5, 0.6, ConfidenceMedium},
		{"模型数不足降为中档", 3, 30, 5, 0.5, ConfidenceMedium},
		{"总评分数不足降为中档", 4, 15, 5, 0.5, ConfidenceMedium},
		{"每模型评分数不足降为中档", 4, 30, 3, 0.5, ConfidenceMedium},
		{"中档四项全部满足", 3, 15, 3, 1, ConfidenceMedium},
		{"半宽超过 1 判为低", 3, 15, 3, 1.1, ConfidenceLow},
		{"模型数不足判为低", 2, 40, 8, 0.2, ConfidenceLow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DiscriminationConfidenceLevel(conf, c.models, c.total, c.perModel, c.halfWidth)
			if got != c.want {
				t.Fatalf("置信度 = %v, 期望 %v", got, c.want)
			}
		})
	}
}

func TestBootstrap五百次耗时在预算内(t *testing.T) {
	// 开发方案 §6.4 验收标准：500 次单次计算 ≤ 200 ms。
	// 这里用明显大于门槛的数据量：4 个模型各 50 条评分。
	samples := make([]SampleSet, 0, 4)
	for m := 0; m < 4; m++ {
		set := SampleSet{Scores: make([]float64, 0, 50)}
		for i := 0; i < 50; i++ {
			set.Scores = append(set.Scores, float64(1+((i+m)%5)))
		}
		samples = append(samples, set)
	}

	start := time.Now()
	got := BootstrapDiscrimination(samples, 4, 200, DefaultBayes(),
		DefaultDiscrimination(), DefaultDiscriminationConfidence(),
		DefaultBootstrap(), NewSamplingSource(2026))
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Fatalf("500 次 Bootstrap 耗时 %v, 超过 200 ms 预算", elapsed)
	}
	if got.PointEstimate <= 0 {
		t.Fatalf("点估计 = %v, 期望有取值", got.PointEstimate)
	}
}

func Test百分位线性插值(t *testing.T) {
	samples := []float64{1, 2, 3, 4}
	cases := []struct {
		q    float64
		want float64
	}{
		{0, 1},
		{0.5, 2.5},
		{1, 4},
	}
	for _, c := range cases {
		if got := percentile(samples, c.q); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("percentile(%v) = %v, 期望 %v", c.q, got, c.want)
		}
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Fatalf("空样本分位数 = %v, 期望 0", got)
	}
	if got := percentile([]float64{7}, 0.5); got != 7 {
		t.Fatalf("单样本分位数 = %v, 期望 7", got)
	}
}
