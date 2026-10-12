package stats

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/promptbench/backend/internal/formula"
	"github.com/promptbench/backend/internal/repo"
)

// tags 返回该提示词的全部推荐标签。
func (f *fixture) tags(t *testing.T, promptID int64) []repo.RecommendTag {
	t.Helper()
	rows, err := repo.New(f.pool).ListRecommendTagsByPrompt(context.Background(), promptID)
	if err != nil {
		t.Fatalf("读取推荐标签失败: %v", err)
	}
	return rows
}

// tagsOfKind 返回指定类型的推荐标签。
func (f *fixture) tagsOfKind(t *testing.T, promptID int64, kind string) []repo.RecommendTag {
	t.Helper()
	var out []repo.RecommendTag
	for _, tag := range f.tags(t, promptID) {
		if tag.Kind == kind {
			out = append(out, tag)
		}
	}
	return out
}

// repeatScore 返回 n 个相同的分值，用于批量插入。
func repeatScore(score, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = score
	}
	return out
}

// invertAdjusted 求出达到目标校准完成度所需的原始平均分。
// 由 adjusted = (n * mean + k * prior) / (n + k) 反解 mean。
func invertAdjusted(target float64, n int) float64 {
	bayes := formula.DefaultBayes()
	return (target*float64(n) + bayes.K*target - bayes.K*bayes.Prior) / float64(n)
}

// closeEnough 比较两个浮点数，容差与快照的两位小数精度一致。
func closeEnough(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// readQueryFiles 读取 db/query 下的全部 SQL 文件。
func readQueryFiles(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "db", "query")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("读取查询目录失败: %v", err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", entry.Name(), err)
		}
		out[entry.Name()] = string(content)
	}
	return out
}

// containsAggregateFilter 判定一段 SQL 是否含有未包 COALESCE 的 FILTER 聚合。
//
// 注释里出现的 FILTER（说明为何要包 COALESCE）不参与判定，先剥去注释再检查。
func containsAggregateFilter(content string) bool {
	statements := stripSQLComments(content)
	lower := strings.ToLower(statements)
	offset := 0
	for {
		index := strings.Index(lower[offset:], "filter (where")
		if index < 0 {
			return false
		}
		if !precededByCoalesce(statements, offset+index) {
			return true
		}
		offset += index + len("filter (where")
	}
}

// stripSQLComments 去掉行注释与块注释，保留语句文本。
func stripSQLComments(content string) string {
	var out strings.Builder
	for index := 0; index < len(content); index++ {
		if content[index] == '-' && index+1 < len(content) && content[index+1] == '-' {
			for index < len(content) && content[index] != '\n' {
				index++
			}
			out.WriteByte('\n')
			continue
		}
		if content[index] == '/' && index+1 < len(content) && content[index+1] == '*' {
			end := strings.Index(content[index+2:], "*/")
			if end < 0 {
				break
			}
			index += end + 3
			out.WriteByte(' ')
			continue
		}
		out.WriteByte(content[index])
	}
	return out.String()
}

// precededByCoalesce 判断 FILTER 之前紧邻的聚合表达式是否被 COALESCE 包住。
func precededByCoalesce(content string, index int) bool {
	head := strings.ToLower(content[:index])
	// 取 FILTER 之前最后一个左括号之后的内容，判断聚合表达式的起点。
	if at := strings.LastIndex(head, "("); at >= 0 {
		head = head[at+1:]
	}
	head = strings.TrimSpace(head)
	// 起点可能是 avg(、count( 等聚合函数名，也可能已被 COALESCE( 包住。
	return strings.HasPrefix(head, "coalesce(") || strings.HasPrefix(head, "sum(") ||
		strings.HasPrefix(head, "max(") || strings.HasPrefix(head, "min(")
}
