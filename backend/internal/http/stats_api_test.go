package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	api "github.com/promptbench/backend/internal/http"
)

// statsResponse 是统计端点的响应形状。
type statsResponse struct {
	PromptID       int64 `json:"prompt_id"`
	History        windowDTO
	Recent         windowDTO
	Models         []modelStatsDTO
	Recommendation []recommendTagDTO
	ComputedAt     string `json:"computed_at"`
}

type windowDTO struct {
	Sufficient       bool     `json:"sufficient"`
	Discrimination   *float64 `json:"discrimination"`
	Confidence       string   `json:"confidence"`
	ValidModelCount  int      `json:"valid_model_count"`
	TotalRatingCount int      `json:"total_rating_count"`
}

type modelStatsDTO struct {
	ModelID   int64  `json:"model_id"`
	ModelName string `json:"model_name"`
	History   struct {
		HasData     bool     `json:"has_data"`
		Mean        *float64 `json:"mean"`
		Adjusted    float64  `json:"adjusted"`
		RatingCount int64    `json:"rating_count"`
		UserCount   int64    `json:"user_count"`
		Confidence  string   `json:"confidence"`
	} `json:"history"`
	Recent struct {
		HasData     bool     `json:"has_data"`
		Mean        *float64 `json:"mean"`
		Adjusted    float64  `json:"adjusted"`
		RatingCount int64    `json:"rating_count"`
		UserCount   int64    `json:"user_count"`
		Confidence  string   `json:"confidence"`
	} `json:"recent"`
}

type recommendTagDTO struct {
	ID         int64  `json:"id"`
	ModelID    int64  `json:"model_id"`
	Scope      string `json:"scope"`
	Kind       string `json:"kind"`
	LabelZh    string `json:"label_zh"`
	LabelEn    string `json:"label_en"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

// seedRate 直接写入一条评分。统计用例需要控制评分创建时间，而评分接口不接受该参数。
// 零值时刻按「当前时刻」处理，使调用方只需在构造历史评分时显式传时间。
func seedRate(t *testing.T, pool *pgxpool.Pool, promptID, modelID, userID int64, score int, createdAt time.Time) {
	t.Helper()
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	var versionID int64
	if err := pool.QueryRow(t.Context(),
		`SELECT current_version_id FROM prompts WHERE id = $1`, promptID).Scan(&versionID); err != nil {
		t.Fatalf("读取当前版本失败: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO ratings (user_id, prompt_id, prompt_version_id, model_id, score, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`,
		userID, promptID, versionID, modelID, score, createdAt); err != nil {
		t.Fatalf("写入评分失败: %v", err)
	}
}

// seedModel 建立一个启用的测试模型，名字带随机后缀避免与其它用例冲突。
func seedModel(t *testing.T, env *testEnv, name string) int64 {
	t.Helper()
	var id int64
	if err := env.pool.QueryRow(t.Context(), `
		INSERT INTO models (name, model_key, provider_key)
		VALUES ($1, $1 || '-' || gen_random_uuid()::text, 'test') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatalf("创建模型 %s 失败: %v", name, err)
	}
	return id
}

// seedRaters 批量建立评分用户。
func seedRaters(t *testing.T, env *testEnv, n int) []int64 {
	t.Helper()
	rows, err := env.pool.Query(t.Context(), `
		INSERT INTO users (username, email, role, status, email_verified_at)
		SELECT 'h_rater_' || gen_random_uuid()::text,
		       'h_rater_' || gen_random_uuid()::text || '@example.test',
		       'user', 'normal', now()
		FROM generate_series(1, $1) RETURNING id`, n)
	if err != nil {
		t.Fatalf("批量创建评分用户失败: %v", err)
	}
	defer rows.Close()

	out := make([]int64, 0, n)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("扫描用户 ID 失败: %v", err)
		}
		out = append(out, id)
	}
	return out
}

// statsPath 返回某提示词的统计端点路径。
func statsPath(promptID int64) string {
	return fmt.Sprintf("%s/prompts/%d/stats", api.APIPrefix, promptID)
}

// TestStatsEndpointReportsInsufficientDataWithNullDiscrimination 覆盖 C-06：
// 数据不足时区分度为 null，前端据此显示「数据不足，暂不计算区分度」。
func TestStatsEndpointReportsInsufficientDataWithNullDiscrimination(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "stats-insufficient@example.com", httpTestPassword)

	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, client.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "数据不足的提示词", "body": "正文", "category_id": 1,
	}))

	stats := decodeBody[statsResponse](t, client.get(statsPath(created.Prompt.ID)))
	if stats.Recent.Sufficient {
		t.Fatal("刚上传的提示词没有任何评分，近 N 日数据不应充足")
	}
	if stats.Recent.Discrimination != nil {
		t.Fatalf("数据不足时区分度应为 null，实际 %v", *stats.Recent.Discrimination)
	}
	if stats.Recent.Confidence != "low" {
		t.Fatalf("数据不足时区分度置信度 = %s，期望 low", stats.Recent.Confidence)
	}
}

// TestStatsEndpointComputesDiscriminationOnRealRatings 用真实评分验证接口返回值与手算一致。
func TestStatsEndpointComputesDiscriminationOnRealRatings(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "stats-computed@example.com", httpTestPassword)

	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, client.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "有评分的提示词", "body": "正文", "category_id": 1,
	}))
	promptID := created.Prompt.ID

	models := []int64{
		seedModel(t, env, "统计模型甲"),
		seedModel(t, env, "统计模型乙"),
		seedModel(t, env, "统计模型丙"),
		seedModel(t, env, "统计模型丁"),
	}
	raters := seedRaters(t, env, 8)

	// 四个模型给出差异明显的评分：全部落在近 N 日窗口内。
	scores := []int{1, 3, 4, 5}
	for m, modelID := range models {
		for i := 0; i < 8; i++ {
			seedRate(t, env.pool, promptID, modelID, raters[i], scores[m], time.Time{})
		}
	}

	stats := decodeBody[statsResponse](t, client.get(statsPath(promptID)))
	if !stats.Recent.Sufficient {
		t.Fatalf("4 个模型各 8 条评分应满足数据门槛，实际有效模型 %d、总评分 %d",
			stats.Recent.ValidModelCount, stats.Recent.TotalRatingCount)
	}
	if stats.Recent.Discrimination == nil {
		t.Fatal("数据充足时区分度不应为 null")
	}
	if *stats.Recent.Discrimination <= 1 {
		t.Fatalf("四个模型评分差异明显，区分度 = %v，应高于 1", *stats.Recent.Discrimination)
	}
	if stats.Recent.ValidModelCount != 4 {
		t.Fatalf("有效模型数 = %d，期望 4", stats.Recent.ValidModelCount)
	}
	if stats.Recent.TotalRatingCount != 32 {
		t.Fatalf("近 N 日总评分数 = %d，期望 32", stats.Recent.TotalRatingCount)
	}

	// 逐模型的额展示字段（需求 §3.7）：原始平均分、校准分、评分数、独立用户数、置信度。
	for _, m := range stats.Models {
		if m.Recent.RatingCount == 0 {
			continue
		}
		if m.Recent.Mean == nil {
			t.Fatalf("模型 %s 有评分时原始平均分不应为 null", m.ModelName)
		}
		if m.Recent.UserCount != 8 {
			t.Fatalf("模型 %s 的独立评分用户数 = %d，期望 8", m.ModelName, m.Recent.UserCount)
		}
		if m.Recent.Confidence == "" {
			t.Fatalf("模型 %s 的置信度等级为空", m.ModelName)
		}
	}
}

// TestStatsEndpointToleratesModelsWithoutRecentRatings 是「模型有历史评分但近 N 日无评分」
// 形态在接口层的回归：详情页必须正常返回，不能 500。
func TestStatsEndpointToleratesModelsWithoutRecentRatings(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "stats-no-recent@example.com", httpTestPassword)

	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, client.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "近期无评分的提示词", "body": "正文", "category_id": 1,
	}))
	promptID := created.Prompt.ID

	models := []int64{
		seedModel(t, env, "旧模型甲"),
		seedModel(t, env, "旧模型乙"),
		seedModel(t, env, "旧模型丙"),
		seedModel(t, env, "旧模型丁"),
	}
	raters := seedRaters(t, env, 8)
	old := time.Now().Add(-30 * 24 * time.Hour)

	// 四个模型都只有历史评分；模型丁在近 N 日完全没有评分。
	scores := []int{5, 4, 3, 1}
	for m, modelID := range models {
		for i := 0; i < 8; i++ {
			seedRate(t, env.pool, promptID, modelID, raters[i], scores[m], old)
		}
	}
	// 近 N 日只有前三个模型有评分，用另一批用户避免唯一索引冲突。
	recentRaters := seedRaters(t, env, 6)
	for m := 0; m < 3; m++ {
		for i := 0; i < 6; i++ {
			seedRate(t, env.pool, promptID, models[m], recentRaters[i], scores[m], time.Time{})
		}
	}

	resp := client.get(statsPath(promptID))
	expectStatus(t, resp, http.StatusOK)

	// 历史窗口有四个模型的数据，近 N 日只有三个。
	stats := decodeBody[statsResponse](t, resp)
	if stats.History.ValidModelCount != 4 {
		t.Fatalf("历史有效模型数 = %d，期望 4", stats.History.ValidModelCount)
	}
	if stats.Recent.ValidModelCount != 3 {
		t.Fatalf("近 N 日有效模型数 = %d，期望 3（模型丁在该窗口无评分）", stats.Recent.ValidModelCount)
	}

	var found bool
	for _, m := range stats.Models {
		if m.ModelID != models[3] {
			continue
		}
		found = true
		if !m.History.HasData {
			t.Fatal("模型丁在历史窗口应有数据")
		}
		if m.Recent.HasData {
			t.Fatal("模型丁在近 N 日窗口不应有数据")
		}
		if m.Recent.RatingCount != 0 {
			t.Fatalf("模型丁近 N 日评分数 = %d，期望 0", m.Recent.RatingCount)
		}
	}
	if !found {
		t.Fatal("响应中缺少模型丁的统计")
	}
}

// TestConfirmedRecommendationAppearsInStats 覆盖需求 §3.7：详情页展示推荐标签，
// 且只展示已确认的标签（需求 §6.3 要求全部推荐经管理员确认后才在前台展示）。
func TestConfirmedRecommendationAppearsInStats(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "stats-tags@example.com", httpTestPassword)

	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, client.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "带推荐标签的提示词", "body": "正文", "category_id": 1,
	}))
	promptID := created.Prompt.ID
	modelID := seedModel(t, env, "饱和模型")

	// 20 条 5 分评分让校准完成度达到 4.6，超过饱和阈值 4.5（需求 §6.2）。
	raters := seedRaters(t, env, 20)
	for i := 0; i < 20; i++ {
		seedRate(t, env.pool, promptID, modelID, raters[i], 5, time.Time{})
	}

	// 触发重算并让它落库。
	client.get(statsPath(promptID))

	// 后台把推荐标签确认为已确认。后台端点属于阶段 6，这里直接改库。
	if _, err := env.pool.Exec(t.Context(),
		`UPDATE recommend_tags SET state = 'confirmed' WHERE prompt_id = $1`, promptID); err != nil {
		t.Fatalf("确认推荐标签失败: %v", err)
	}
	// 缓存里的快照是在标签仍为待确认时生成的，作废后重新读取。
	env.stats.MarkStale(promptID)

	stats := decodeBody[statsResponse](t, client.get(statsPath(promptID)))
	if len(stats.Recommendation) == 0 {
		t.Fatal("已确认的推荐标签应出现在统计响应中")
	}
	tag := stats.Recommendation[0]
	if tag.Kind != "saturation" {
		t.Fatalf("标签类型 = %s，期望 saturation（20 条 5 分评分，校准完成度 4.6）", tag.Kind)
	}
	if tag.ModelID != modelID {
		t.Fatalf("标签绑定的模型 = %d，期望 %d", tag.ModelID, modelID)
	}
	if tag.LabelZh == "" || tag.LabelEn == "" {
		t.Fatal("标签文案的中英两份都不可为空")
	}
	if tag.Reason == "" {
		t.Fatal("推荐标签必须携带推荐理由（需求 §6.3）")
	}
	if tag.Confidence != "high" {
		t.Fatalf("标签置信度 = %s，期望 high（20 条评分、20 个用户、半宽 0）", tag.Confidence)
	}
}

// TestPendingRecommendationIsHiddenFromFrontend 覆盖需求 §6.3 前半句：
// 待确认的推荐标签不得出现在前台。
func TestPendingRecommendationIsHiddenFromFrontend(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "stats-pending@example.com", httpTestPassword)

	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, client.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "待确认标签的提示词", "body": "正文", "category_id": 1,
	}))
	promptID := created.Prompt.ID
	modelID := seedModel(t, env, "待确认饱和模型")

	raters := seedRaters(t, env, 20)
	for i := 0; i < 20; i++ {
		seedRate(t, env.pool, promptID, modelID, raters[i], 5, time.Time{})
	}
	client.get(statsPath(promptID))

	var pending int
	if err := env.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM recommend_tags WHERE prompt_id = $1 AND state = 'pending'`, promptID).Scan(&pending); err != nil {
		t.Fatalf("统计待确认标签失败: %v", err)
	}
	if pending == 0 {
		t.Fatal("重算应生成待确认的推荐标签")
	}

	stats := decodeBody[statsResponse](t, client.get(statsPath(promptID)))
	if len(stats.Recommendation) != 0 {
		t.Fatalf("待确认的推荐标签不应出现在前台，实际返回 %d 条", len(stats.Recommendation))
	}
}

// TestStatsEndpointIsDocumentedInOpenAPI 断言统计端点出现在生成的规范里，
// 且区分度字段允许 null（C-06 要求数据不足时不给出数值）。
func TestStatsEndpointIsDocumentedInOpenAPI(t *testing.T) {
	srv := newTestServer(t)
	ts := serve(t, srv)

	resp, err := http.Get(ts.URL + api.APIPrefix + "/openapi.json")
	if err != nil {
		t.Fatalf("获取 OpenAPI 规范失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取规范失败: %v", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("规范不是合法 JSON: %v", err)
	}

	paths := spec["paths"].(map[string]any)
	statsPathKey := api.APIPrefix + "/prompts/{id}/stats"
	if _, ok := paths[statsPathKey]; !ok {
		t.Fatalf("规范缺少 %s，已登记路径: %v", statsPathKey, keysOf(paths))
	}

	// 沿响应 schema 的引用链走到窗口组件，再从 recent 的引用走到区分度字段。
	responseRef := dig(t, spec, "paths", statsPathKey, "get", "responses", "200",
		"content", "application/json", "schema", "$ref").(string)
	responseName := componentName(responseRef)
	recentRef := dig(t, spec, "components", "schemas", responseName, "properties",
		"recent", "$ref").(string)
	windowName := componentName(recentRef)

	discrimination := dig(t, spec, "components", "schemas", windowName, "properties", "discrimination")
	types, _ := discrimination.(map[string]any)["type"].([]any)
	if !containsString(types, "null") {
		t.Fatalf("区分度的类型 = %v，必须允许 null（C-06）", types)
	}
}

// componentName 从 "#/components/schemas/X" 形式的引用中取出组件名。
func componentName(ref string) string {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		panic("非预期的组件引用: " + ref)
	}
	return strings.TrimPrefix(ref, prefix)
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func containsString(values []any, want string) bool {
	for _, v := range values {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}
