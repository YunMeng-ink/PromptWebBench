package domain

import (
	"testing"
	"time"
)

func TestVersionCarryFollowsC27(t *testing.T) {
	cases := []struct {
		from  Version
		major bool
		want  string
	}{
		{from: Version{Major: 1, Minor: 0}, major: false, want: "v1.1"},
		{from: Version{Major: 1, Minor: 3}, major: true, want: "v2.0"},
		// C-27：次版本进位无上限，v1.9 之后为 v1.10。
		{from: Version{Major: 1, Minor: 9}, major: false, want: "v1.10"},
		// C-27：主版本 +1 时次版本归零。
		{from: Version{Major: 2, Minor: 7}, major: true, want: "v3.0"},
	}
	for _, tc := range cases {
		if got := tc.from.Next(tc.major).String(); got != tc.want {
			t.Errorf("%s 重大=%v 进位后 = %s，期望 %s", tc.from, tc.major, got, tc.want)
		}
	}
}

func TestParseVersionRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		raw   string
		valid bool
	}{
		{raw: "v1.0", valid: true},
		{raw: "v10.25", valid: true},
		{raw: "1.0"},
		{raw: "v1"},
		{raw: "v0.1"},
		{raw: "v1.-1"},
		{raw: "va.b"},
		{raw: ""},
	}
	for _, tc := range cases {
		got, err := ParseVersion(tc.raw)
		if tc.valid && err != nil {
			t.Errorf("ParseVersion(%q) 应成功，实际 %v", tc.raw, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("ParseVersion(%q) 应失败，实际返回 %v", tc.raw, got)
		}
	}
}

func TestDayStartUsesUTCNaturalDay(t *testing.T) {
	// C-03：每日窗口以 UTC 自然日切分。同一时刻在不同时区表达时
	// 必须落到同一个 UTC 零点，这是限额在跨国访问下仍一致的前提。
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "东八区当日 09:00 等于 UTC 当日 01:00",
			in:   time.Date(2026, 10, 11, 9, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			want: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "UTC 当日 00:00:00 本身即为边界",
			in:   time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "西四区当日 20:00 已越过 UTC 次日零点",
			in:   time.Date(2026, 10, 11, 20, 0, 0, 0, time.FixedZone("EDT", -4*3600)),
			want: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		if got := DayStart(tc.in); !got.Equal(tc.want) {
			t.Errorf("%s：DayStart = %s，期望 %s", tc.name, got, tc.want)
		}
	}
}

func TestDayRangeIsHalfOpenAndSpansOneDay(t *testing.T) {
	// 左闭右开：零点整计入当日，次日零点不计入。
	in := time.Date(2026, 10, 11, 15, 30, 0, 0, time.UTC)
	start, end := DayRange(in)

	if want := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Errorf("窗口起点 = %s，期望 %s", start, want)
	}
	if want := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Errorf("窗口终点 = %s，期望 %s", end, want)
	}
	if !start.Before(in) || !in.Before(end) {
		t.Error("给定时刻应落在窗口内")
	}
	if !start.Before(end) {
		t.Error("窗口终点应严格晚于起点")
	}
}

func TestValidScoreCoversOnlyAnchorScale(t *testing.T) {
	// C-01：量表为 1-5 整数，空值不构成评分。
	for score := ScoreMin; score <= ScoreMax; score++ {
		if !ValidScore(score) {
			t.Errorf("分数 %d 应当合法", score)
		}
	}
	for _, score := range []int{0, -1, 6, 100} {
		if ValidScore(score) {
			t.Errorf("分数 %d 不应合法", score)
		}
	}
}
