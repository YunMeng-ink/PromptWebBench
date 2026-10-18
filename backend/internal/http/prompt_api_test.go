package api_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"

	api "github.com/promptbench/backend/internal/http"
)

// stage2 环境：真实数据库 + 真实 HTTP，服务于阶段 2 的接口层用例。

func TestPromptListReturnsPaginationShape(t *testing.T) {
	// C-40：响应为 {items, page, page_size, total}，空列表为 [] 而非 null。
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()

	resp := client.get(api.APIPrefix + "/prompts?page=1&page_size=10")
	expectStatus(t, resp, http.StatusOK)

	body := decodeBody[struct {
		Items    []map[string]any `json:"items"`
		Page     int32            `json:"page"`
		PageSize int32            `json:"page_size"`
		Total    int64            `json:"total"`
	}](t, resp)

	if body.Page != 1 || body.PageSize != 10 {
		t.Errorf("分页回显 = page %d page_size %d，期望 1 与 10", body.Page, body.PageSize)
	}
	if body.Total != 0 {
		t.Errorf("空库的 total = %d，期望 0", body.Total)
	}
	if body.Items == nil {
		t.Error("空列表必须为 [] 而非 null")
	}
	if len(body.Items) != 0 {
		t.Errorf("空库的 items 长度 = %d，期望 0", len(body.Items))
	}
}

func TestPromptListClampsPageSizeToMaximum(t *testing.T) {
	// C-40：page_size 上限 100。超出上限在请求层即被拒绝，
	// 因此不会产生一次超大页的查询。
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()

	expectErrorCode(t, client.get(api.APIPrefix+"/prompts?page=1&page_size=500"),
		http.StatusUnprocessableEntity, "validation_failed")

	resp := client.get(api.APIPrefix + "/prompts?page=1&page_size=100")
	expectStatus(t, resp, http.StatusOK)
	body := decodeBody[struct {
		PageSize int32 `json:"page_size"`
	}](t, resp)
	if body.PageSize != 100 {
		t.Errorf("page_size = %d，期望 100", body.PageSize)
	}
}

func TestHighRiskPurposeHiddenFromOrdinaryUserOverHTTP(t *testing.T) {
	// C-20 在接口层的验收：普通用户既不能选高风险考察目的，也读不到已有的高风险内容。
	env := newTestEnv(t)

	devClient := env.newClient(t)
	registerDeveloper(t, env, devClient, "dev-highrisk@example.com", httpTestPassword)

	ordinary := env.newClient(t)
	ordinary.prime()
	registerVerifyLogin(t, env, ordinary, "user-highrisk@example.com", httpTestPassword)

	// 普通用户的考察目的列表不应出现高风险条目。
	for slug := range purposeIDs(t, ordinary) {
		if slug == "adversarial" {
			t.Fatal("普通用户的考察目的列表不应包含高风险条目")
		}
	}

	devPurposes := purposeIDs(t, devClient)
	adversarialID, ok := devPurposes["adversarial"]
	if !ok {
		t.Fatal("开发者的考察目的列表应包含高风险条目")
	}
	capabilityList := capabilityIDs(t, devClient)
	defaultCapabilityID, ok := capabilityList["reasoning"]
	if !ok {
		t.Fatal("考察维度字典缺少 reasoning")
	}

	// 创建路径：普通用户选高风险考察目的 → 403。
	expectErrorCode(t, ordinary.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "普通用户的高风险提示词", "body": "正文", "purpose_id": adversarialID,
		"capability_id": defaultCapabilityID, "visibility": "public",
	}), http.StatusForbidden, "forbidden")

	// 开发者创建一条高风险且公开的提示词。
	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
		VersionNo string `json:"version_no"`
	}](t, devClient.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "高风险公开提示词", "body": "正文", "purpose_id": adversarialID,
		"capability_id": capabilityList["safety"], "visibility": "public",
	}))
	promptID := created.Prompt.ID

	// 直链：游客与普通用户都拿 404，而不是 403。
	expectErrorCode(t, ordinary.get(fmt.Sprintf("%s/prompts/%d", api.APIPrefix, promptID)),
		http.StatusNotFound, "not_found")

	guest := env.newClient(t)
	guest.prime()
	expectErrorCode(t, guest.get(fmt.Sprintf("%s/prompts/%d", api.APIPrefix, promptID)),
		http.StatusNotFound, "not_found")

	// 开发者本人可以读取。
	expectStatus(t, devClient.get(fmt.Sprintf("%s/prompts/%d", api.APIPrefix, promptID)), http.StatusOK)
}

func TestPromptSupportsMultipleCapabilities(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "multi-capability@example.com", httpTestPassword)

	purposes := purposeIDs(t, client)
	capabilities := capabilityIDs(t, client)
	selectedCapabilityIDs := []int64{
		capabilities["reasoning"], capabilities["coding"], capabilities["tool_use"],
	}
	for _, id := range selectedCapabilityIDs {
		if id == 0 {
			t.Fatal("考察维度字典缺少 reasoning、coding 或 tool_use")
		}
	}

	created := decodeBody[struct {
		Prompt struct {
			ID            int64   `json:"id"`
			CapabilityID  int64   `json:"capability_id"`
			CapabilityIDs []int64 `json:"capability_ids"`
		} `json:"prompt"`
	}](t, client.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "多维度提示词", "body": "正文", "purpose_id": purposes["capability_boundary"],
		"capability_ids": selectedCapabilityIDs, "visibility": "public",
	}))
	if len(created.Prompt.CapabilityIDs) != 3 {
		t.Fatalf("创建响应 capability_ids = %v，期望 3 个", created.Prompt.CapabilityIDs)
	}
	if created.Prompt.CapabilityID != selectedCapabilityIDs[0] {
		t.Fatalf("主考察维度 = %d，期望首个 %d", created.Prompt.CapabilityID, selectedCapabilityIDs[0])
	}

	detail := decodeBody[struct {
		Prompt struct {
			CapabilityIDs []int64 `json:"capability_ids"`
		} `json:"prompt"`
		Capabilities []struct {
			ID int64 `json:"id"`
		} `json:"capabilities"`
	}](t, client.get(fmt.Sprintf("%s/prompts/%d", api.APIPrefix, created.Prompt.ID)))
	if len(detail.Prompt.CapabilityIDs) != 3 || len(detail.Capabilities) != 3 {
		t.Fatalf("详情 capability_ids = %v，capabilities = %d 个", detail.Prompt.CapabilityIDs, len(detail.Capabilities))
	}

	// 按第二个维度筛选同样命中。
	list := decodeBody[struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, client.get(fmt.Sprintf("%s/prompts?capability_id=%d", api.APIPrefix, selectedCapabilityIDs[1])))
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != created.Prompt.ID {
		t.Fatalf("按第二维度筛选 = %+v，期望命中该提示词", list)
	}

	// 超过 5 个维度在请求层即被拒绝。
	expectErrorCode(t, client.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "六维度提示词", "body": "正文", "purpose_id": purposes["capability_boundary"],
		"capability_ids": []int64{
			capabilities["reasoning"], capabilities["coding"], capabilities["tool_use"],
			capabilities["language"], capabilities["long_context"], capabilities["math"],
		},
	}), http.StatusUnprocessableEntity, "validation_failed")
}

func TestOrdinaryUserCannotPatchToHighRiskPurpose(t *testing.T) {
	// C-20 的更新路径回归：这是普通用户绕过限制的主要方式。
	env := newTestEnv(t)
	developer := env.newClient(t)
	registerDeveloper(t, env, developer, "dev-patch@example.com", httpTestPassword)

	purposes := purposeIDs(t, developer)
	purposeID, highRiskID := purposes["capability_boundary"], purposes["adversarial"]
	if purposeID == 0 || highRiskID == 0 {
		t.Fatal("考察目的字典缺少 capability_boundary 或 adversarial")
	}
	capabilityID := capabilityIDs(t, developer)["reasoning"]
	if capabilityID == 0 {
		t.Fatal("考察维度字典缺少 reasoning")
	}

	// 普通目的、公开可见性，先创建成功。
	ordinary := env.newClient(t)
	ordinary.prime()
	registerVerifyLogin(t, env, ordinary, "user-patch@example.com", httpTestPassword)

	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, ordinary.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "可修改的提示词", "body": "正文", "purpose_id": purposeID,
		"capability_id": capabilityID, "visibility": "public",
	}))
	promptID := created.Prompt.ID

	// 直接改考察目的为高风险 → 403。
	expectErrorCode(t, ordinary.patch(fmt.Sprintf("%s/prompts/%d", api.APIPrefix, promptID), map[string]any{
		"title": "改为高风险", "body": "正文", "purpose_id": highRiskID,
		"capability_id": capabilityID, "visibility": "public",
	}), http.StatusForbidden, "forbidden")

	// 考察目的未变，提示词仍可公开读取。
	detail := decodeBody[struct {
		Prompt struct {
			PurposeID int64 `json:"purpose_id"`
		} `json:"prompt"`
	}](t, ordinary.get(fmt.Sprintf("%s/prompts/%d", api.APIPrefix, promptID)))
	if detail.Prompt.PurposeID != purposeID {
		t.Errorf("被拒绝的修改不应生效，考察目的 = %d，期望 %d", detail.Prompt.PurposeID, purposeID)
	}

	// 改可见性为 developer → 同样 403。
	expectErrorCode(t, ordinary.patch(fmt.Sprintf("%s/prompts/%d", api.APIPrefix, promptID), map[string]any{
		"title": "改为开发者可见", "body": "正文", "purpose_id": purposeID,
		"capability_id": capabilityID, "visibility": "developer",
	}), http.StatusForbidden, "forbidden")
}

func TestRatingOverHTTPEnforcesScaleAndUniqueness(t *testing.T) {
	// C-01 与评分唯一性在接口层的表现。
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "author-rating@example.com", httpTestPassword)

	modelID := firstModelID(t, env)
	purposeID, capabilityID := defaultClassification(t, env)

	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, author.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "被评分的提示词", "body": "正文", "purpose_id": purposeID,
		"capability_id": capabilityID, "visibility": "public",
	}))
	promptID := created.Prompt.ID

	path := fmt.Sprintf("%s/prompts/%d/ratings", api.APIPrefix, promptID)

	// 量表外的分值被拒。
	expectErrorCode(t, author.post(path, map[string]any{"model_id": modelID, "score": 0}),
		http.StatusUnprocessableEntity, "validation_failed")
	expectErrorCode(t, author.post(path, map[string]any{"model_id": modelID, "score": 6}),
		http.StatusUnprocessableEntity, "validation_failed")

	// 合法分值写入成功，并回传修改次数上限。
	rating := decodeBody[struct {
		Score     int16 `json:"score"`
		EditCount int32 `json:"edit_count"`
		MaxEdits  int32 `json:"max_edits"`
	}](t, author.post(path, map[string]any{"model_id": modelID, "score": 4}))
	if rating.Score != 4 || rating.EditCount != 0 {
		t.Errorf("首次评分 = %d，edit_count = %d", rating.Score, rating.EditCount)
	}
	if rating.MaxEdits != 3 {
		t.Errorf("单条修改次数上限 = %d，需求 §3.4 规定为 3", rating.MaxEdits)
	}

	// 重复提交同一「提示词 × 模型」→ 409。
	expectErrorCode(t, author.post(path, map[string]any{"model_id": modelID, "score": 5}),
		http.StatusConflict, "rating_exists")

	// 修改 3 次后第 4 次被拒。
	for i := 0; i < 3; i++ {
		expectStatus(t, author.put(path, map[string]any{"model_id": modelID, "score": 2}), http.StatusOK)
	}
	expectErrorCode(t, author.put(path, map[string]any{"model_id": modelID, "score": 5}),
		http.StatusConflict, "rating_edit_limit_exceeded")
}

func TestGuestCannotRate(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "author-guest-rate@example.com", httpTestPassword)

	purposeID, capabilityID := defaultClassification(t, env)
	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, author.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "公开提示词", "body": "正文", "purpose_id": purposeID,
		"capability_id": capabilityID, "visibility": "public",
	}))

	modelID := firstModelID(t, env)
	guest := env.newClient(t)
	guest.prime()

	path := fmt.Sprintf("%s/prompts/%d/ratings", api.APIPrefix, created.Prompt.ID)
	expectErrorCode(t, guest.post(path, map[string]any{"model_id": modelID, "score": 4}),
		http.StatusUnauthorized, "unauthorized")
}

func TestPromptListFiltersByPurposeAndKeyword(t *testing.T) {
	// 6 维筛选中的考察目的与关键词两维在接口层可用。
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "filter-author@example.com", httpTestPassword)

	purposes := purposeIDs(t, author)
	boundaryID, correctnessID := purposes["capability_boundary"], purposes["correctness"]
	if boundaryID == 0 || correctnessID == 0 {
		t.Fatal("考察目的字典缺少 capability_boundary 或 correctness")
	}
	capabilityID := capabilityIDs(t, author)["reasoning"]
	if capabilityID == 0 {
		t.Fatal("考察维度字典缺少 reasoning")
	}

	author.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "推理类提示词", "body": "正文一", "purpose_id": boundaryID,
		"capability_id": capabilityID, "visibility": "public",
	})
	author.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "数学类提示词", "body": "正文二", "purpose_id": correctnessID,
		"capability_id": capabilityID, "visibility": "public",
	})

	guest := env.newClient(t)
	guest.prime()

	byPurpose := decodeBody[struct {
		Items []struct {
			Title     string `json:"title"`
			PurposeID int64  `json:"purpose_id"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, guest.get(fmt.Sprintf("%s/prompts?purpose_id=%d", api.APIPrefix, boundaryID)))

	if byPurpose.Total != 1 || len(byPurpose.Items) != 1 {
		t.Fatalf("按考察目的筛选 total = %d，items = %d，期望各 1", byPurpose.Total, len(byPurpose.Items))
	}
	if byPurpose.Items[0].Title != "推理类提示词" {
		t.Errorf("筛选结果标题 = %s，期望 推理类提示词", byPurpose.Items[0].Title)
	}

	byKeyword := decodeBody[struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, guest.get(api.APIPrefix+"/prompts?keyword="+"%E6%95%B0%E5%AD%A6%E7%B1%BB"))
	if byKeyword.Total != 1 || byKeyword.Items[0].Title != "数学类提示词" {
		t.Errorf("按关键词筛选结果 = %+v，期望仅有 数学类提示词", byKeyword.Items)
	}
}

func TestPromptListSortOrdersAreAccepted(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "sort-author@example.com", httpTestPassword)

	purposeID, capabilityID := defaultClassification(t, env)
	author.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "排序提示词", "body": "正文", "purpose_id": purposeID,
		"capability_id": capabilityID, "visibility": "public",
	})

	guest := env.newClient(t)
	guest.prime()
	for _, sort := range []string{"latest", "top_rated", "discrimination"} {
		expectStatus(t, guest.get(api.APIPrefix+"/prompts?sort="+sort), http.StatusOK)
	}
	expectErrorCode(t, guest.get(api.APIPrefix+"/prompts?sort=unknown"),
		http.StatusUnprocessableEntity, "validation_failed")
}

func TestMajorChangeResponseCarriesNotice(t *testing.T) {
	// 需求 §7.3：重大修改后前台提示该提示词已发生重大修改。
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "major-author@example.com", httpTestPassword)

	purposeID, capabilityID := defaultClassification(t, env)
	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, author.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "将被重大修改的提示词", "body": "正文", "purpose_id": purposeID,
		"capability_id": capabilityID, "visibility": "public",
	}))
	promptID := created.Prompt.ID
	path := fmt.Sprintf("%s/prompts/%d", api.APIPrefix, promptID)

	updated := decodeBody[struct {
		VersionNo       string `json:"version_no"`
		MajorChange     bool   `json:"major_change"`
		ArchivedRatings int64  `json:"archived_ratings"`
		Notice          string `json:"notice"`
	}](t, author.patch(path, map[string]any{
		"title": "重大修改后的标题", "body": "正文", "purpose_id": purposeID,
		"capability_id": capabilityID, "visibility": "public", "is_major_change": true,
	}))

	if updated.VersionNo != "v2.0" {
		t.Errorf("重大修改后版本号 = %s，期望 v2.0", updated.VersionNo)
	}
	if !updated.MajorChange {
		t.Error("响应应标记 major_change")
	}
	if updated.Notice == "" {
		t.Error("重大修改后响应应带回前台提示文案")
	}

	// 详情页同样带回提示标记。
	detail := decodeBody[struct {
		Prompt struct {
			MajorModified bool `json:"major_modified"`
		} `json:"prompt"`
		MajorModificationNotice string `json:"major_modification_notice"`
	}](t, author.get(path))
	if !detail.Prompt.MajorModified {
		t.Error("重大修改后提示词应标记为已发生重大修改")
	}
	if detail.MajorModificationNotice == "" {
		t.Error("详情页应带回重大修改提示文案")
	}
}

func TestImportMarkdownOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "import-author@example.com", httpTestPassword)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "sample.md")
	if err != nil {
		t.Fatalf("构造上传表单失败: %v", err)
	}
	content := "---\ndescription: 导入的描述\n---\n\n# 导入的标题\n\n这是正文。\n"
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("写入上传内容失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭表单失败: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		author.base+api.APIPrefix+"/prompts/import-markdown", &buf)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: clientSessionCookie(t, author)})
	req.Header.Set(api.CSRFHeaderName, author.csrfToken())

	resp, err := author.client.Do(req)
	if err != nil {
		t.Fatalf("发送导入请求失败: %v", err)
	}
	expectStatus(t, resp, http.StatusOK)

	parsed := decodeBody[struct {
		Title       string `json:"title"`
		Body        string `json:"body"`
		Description string `json:"description"`
	}](t, resp)
	if parsed.Title != "导入的标题" {
		t.Errorf("导入标题 = %q，期望 导入的标题", parsed.Title)
	}
	if parsed.Description != "导入的描述" {
		t.Errorf("导入描述 = %q，期望 导入的描述", parsed.Description)
	}
	if parsed.Body == "" {
		t.Error("导入正文不应为空")
	}
}

func TestDictionaryEndpointsArePublic(t *testing.T) {
	// 字典组对游客开放（开发方案 §5.5）。
	env := newTestEnv(t)
	guest := env.newClient(t)
	guest.prime()

	for _, path := range []string{"/purposes", "/capabilities", "/difficulties", "/tags", "/models"} {
		resp := guest.get(api.APIPrefix + path)
		expectStatus(t, resp, http.StatusOK)
	}
}

// firstModelID 返回第一个启用模型的 ID。模型由阶段 6 的后台与 models.dev 导入维护，
// 接口层用例自建一个，保证不依赖外部数据。
func firstModelID(t *testing.T, env *testEnv) int64 {
	t.Helper()
	if _, err := env.pool.Exec(t.Context(),
		`INSERT INTO models (name, model_key, provider_key)
		 SELECT '接口测试模型', 'api-test-model', 'test'
		 WHERE NOT EXISTS (SELECT 1 FROM models WHERE model_key = 'api-test-model')`); err != nil {
		t.Fatalf("创建测试模型失败: %v", err)
	}
	var id int64
	if err := env.pool.QueryRow(t.Context(),
		`SELECT id FROM models WHERE model_key = 'api-test-model'`).Scan(&id); err != nil {
		t.Fatalf("读取测试模型失败: %v", err)
	}
	return id
}

// registerDeveloper 在给定客户端上注册并登录一名开发者。身份组由管理服务直接授予，
// 后台端点在阶段 6 才开放，此处不走 HTTP 管理接口。授予后重新登录，
// 使会话携带新的角色。
func registerDeveloper(t *testing.T, env *testEnv, client *apiClient, email, password string) apiUserDTO {
	t.Helper()
	user := registerVerifyLogin(t, env, client, email, password)
	if _, err := env.pool.Exec(t.Context(),
		`UPDATE users SET role = 'developer' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("授予开发者身份失败: %v", err)
	}
	env.login(t, client, email, password)
	return user
}

// purposeIDs 返回 slug 到考察目的 ID 的映射，供用例按名称取 ID。
func purposeIDs(t *testing.T, client *apiClient) map[string]int64 {
	t.Helper()
	body := decodeBody[struct {
		Items []struct {
			ID   int64  `json:"id"`
			Slug string `json:"slug"`
		} `json:"items"`
	}](t, client.get(api.APIPrefix+"/purposes"))

	out := make(map[string]int64, len(body.Items))
	for _, item := range body.Items {
		out[item.Slug] = item.ID
	}
	return out
}

// capabilityIDs 返回 slug 到考察维度 ID 的映射。
func capabilityIDs(t *testing.T, client *apiClient) map[string]int64 {
	t.Helper()
	body := decodeBody[struct {
		Items []struct {
			ID   int64  `json:"id"`
			Slug string `json:"slug"`
		} `json:"items"`
	}](t, client.get(api.APIPrefix+"/capabilities"))

	out := make(map[string]int64, len(body.Items))
	for _, item := range body.Items {
		out[item.Slug] = item.ID
	}
	return out
}

// defaultClassification 返回创建提示词使用的默认考察目的与考察维度。
func defaultClassification(t *testing.T, env *testEnv) (int64, int64) {
	t.Helper()
	var purposeID, capabilityID int64
	if err := env.pool.QueryRow(t.Context(),
		`SELECT id FROM purposes WHERE slug = 'capability_boundary'`).Scan(&purposeID); err != nil {
		t.Fatalf("读取默认考察目的失败: %v", err)
	}
	if err := env.pool.QueryRow(t.Context(),
		`SELECT id FROM capabilities WHERE slug = 'reasoning'`).Scan(&capabilityID); err != nil {
		t.Fatalf("读取默认考察维度失败: %v", err)
	}
	return purposeID, capabilityID
}

// clientSessionCookie 取出测试客户端持有的会话 Cookie 值，供需要自行构造
// multipart 请求的用例复用同一登录态。
func clientSessionCookie(t *testing.T, c *apiClient) string {
	t.Helper()
	parsed, err := url.Parse(c.base)
	if err != nil {
		t.Fatalf("解析基地址失败: %v", err)
	}
	for _, cookie := range c.client.Jar.Cookies(parsed) {
		if cookie.Name == api.SessionCookieName {
			return cookie.Value
		}
	}
	t.Fatal("测试客户端未持有会话 Cookie")
	return ""
}

// TestPaginationClampsHugePage 验证超大页码被夹紧，不会溢出成负 OFFSET 报 500。
func TestPaginationClampsHugePage(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()

	resp := client.get(api.APIPrefix + "/prompts?page=2147483647&page_size=100")
	expectStatus(t, resp, http.StatusOK)

	body := decodeBody[struct {
		Page int32 `json:"page"`
	}](t, resp)
	if body.Page != 100000 {
		t.Fatalf("回显页码 = %d，期望夹紧到上限 100000", body.Page)
	}
}
