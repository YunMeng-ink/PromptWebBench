package domain

import "time"

// SortOrder 是公开列表的排序方式（需求 §8.2）。
type SortOrder string

const (
	// SortLatest 最新更新：按更新时间降序。
	SortLatest SortOrder = "latest"
	// SortTopRated 评分最高：按统计快照的加权完成度分降序，数据不足的排在后面。
	SortTopRated SortOrder = "top_rated"
	// SortDiscrimination 区分度最高：按统计快照排序，数据充足的排在前面，
	// 再按区分度降序（需求 §8.2、开发方案 §6.4）。
	SortDiscrimination SortOrder = "discrimination"
)

// AllSortOrders 返回全部可选排序，供参数校验与字典声明使用。
func AllSortOrders() []SortOrder {
	return []SortOrder{SortLatest, SortTopRated, SortDiscrimination}
}

// ParseSortOrder 解析排序参数，非法取值返回 false。
func ParseSortOrder(raw string) (SortOrder, bool) {
	for _, s := range AllSortOrders() {
		if SortOrder(raw) == s {
			return s, true
		}
	}
	return "", false
}

// Category 是提示词类别（需求 §8.6、C-19）。
type Category struct {
	ID         int64
	Slug       string
	NameZh     string
	NameEn     string
	IsHighRisk bool
	IsEnabled  bool
	SortOrder  int32
}

// Ability 是能力维度字典项。
type Ability struct {
	ID        int64
	Slug      string
	NameZh    string
	NameEn    string
	IsEnabled bool
	SortOrder int32
}

// Difficulty 是难度字典项。
type Difficulty struct {
	ID        int64
	Slug      string
	NameZh    string
	NameEn    string
	Level     int16
	IsEnabled bool
	SortOrder int32
}

// Tag 是提示词标签。
type Tag struct {
	ID       int64
	Slug     string
	NameZh   string
	NameEn   string
	Color    string
	Priority int32
}

// Prompt 是提示词实体，对应 prompts 表的当前状态。
// HighRisk 是类别的高风险标记快照，由查询时连同类别一并带出，
// 使可见性判定的输入齐备，无需判定处再查一次数据库。
type Prompt struct {
	ID           int64
	Title        string
	Body         string
	Description  string
	CategoryID   int64
	AbilityID    *int64
	DifficultyID *int64
	Visibility   Visibility
	Sensitive    SensitiveLevel
	HighRisk     bool
	UploaderID   int64

	OriginalUploaderID *int64
	CurrentVersionID   *int64
	LatestVersion      Version
	MajorModified      bool
	CommentCount       int32
	FavoriteCount      int32
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Access 是可见性判定所需的三个要素。提示词的类别高风险标记在查询时一并带出，
// 使判定所需的输入齐备，无需判定处再查一次数据库。
type Access struct {
	Level      Visibility
	HighRisk   bool
	UploaderID int64
}

// Access 返回该提示词的可见性判定要素。高风险标记必须一并带入：
// 漏掉它会让「类别高风险 + 可见性 public」的提示词对普通用户可见，
// 正是 C-20 要防止的绕过形态。
func (p Prompt) Access() Access {
	return Access{Level: p.Visibility, HighRisk: p.HighRisk, UploaderID: p.UploaderID}
}

// PromptVersion 是提示词的一个历史版本。
type PromptVersion struct {
	ID           int64
	PromptID     int64
	Version      Version
	Title        string
	Body         string
	Description  string
	CategoryID   int64
	AbilityID    *int64
	DifficultyID *int64
	Visibility   Visibility
	Sensitive    SensitiveLevel
	IsMajor      bool
	ChangeNote   string
	AuthorID     int64
	CreatedAt    time.Time
}

// Rating 是用户对「提示词 × 模型」的完成度评分（需求 §3.1）。
type Rating struct {
	ID              int64
	UserID          int64
	PromptID        int64
	PromptVersionID int64
	ModelID         int64
	Score           int16
	EditCount       int32
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// 评分量表的取值区间（C-01）：1-5 整数，空值不写入。
const (
	ScoreMin = 1
	ScoreMax = 5
)

// ValidScore 判定分值是否落在量表区间内。
func ValidScore(score int) bool {
	return score >= ScoreMin && score <= ScoreMax
}

// DayStart 返回给定时刻所在的 UTC 自然日零点（C-03）。
// 每日计数窗口一律由应用层切分，SQL 侧不出现时区相关表达式（开发方案 §3.4）。
func DayStart(t time.Time) time.Time {
	return t.UTC().Truncate(24 * time.Hour)
}

// DayRange 返回 [起点, 终点)：起点为给定时刻所在 UTC 自然日零点，
// 终点为次日零点。窗口边界左闭右开。
func DayRange(t time.Time) (time.Time, time.Time) {
	start := DayStart(t)
	return start, start.Add(24 * time.Hour)
}
