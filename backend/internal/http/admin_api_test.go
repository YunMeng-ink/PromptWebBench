package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/promptbench/backend/internal/domain"
	api "github.com/promptbench/backend/internal/http"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/admin"
)

// adminSession 建一个管理员账户并登录，返回带会话的客户端。
func adminSession(t *testing.T, env *testEnv) *apiClient {
	t.Helper()
	pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Username: "admin", Email: "admin@example.com",
		Password: httpTestPassword, Role: "admin", Verified: true,
	})
	client := env.newClient(t)
	env.login(t, client, "admin@example.com", httpTestPassword)
	return client
}

// memberSession 建一个普通账户并登录，用于验证后台路由的准入。
func memberSession(t *testing.T, env *testEnv) *apiClient {
	t.Helper()
	pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "plain@example.com", Password: httpTestPassword, Verified: true,
	})
	client := env.newClient(t)
	env.login(t, client, "plain@example.com", httpTestPassword)
	return client
}

func TestAdminRoutesRejectNonAdmin(t *testing.T) {
	env := newTestEnv(t)

	// 游客：未登录一律 401。
	guest := env.newClient(t)
	guest.prime()
	expectErrorCode(t, guest.get(api.APIPrefix+"/admin/dashboard"), http.StatusUnauthorized, "unauthorized")

	// 普通用户：后端拦截（前端守卫之外的第二道防线）。
	member := memberSession(t, env)
	expectErrorCode(t, member.get(api.APIPrefix+"/admin/dashboard"),
		http.StatusForbidden, "permission_denied")
	expectErrorCode(t, member.get(api.APIPrefix+"/admin/users"),
		http.StatusForbidden, "permission_denied")
	expectErrorCode(t, member.post(api.APIPrefix+"/admin/models/import", map[string]any{"overwrite": false}),
		http.StatusForbidden, "permission_denied")
	expectErrorCode(t, member.get(api.APIPrefix+"/admin/settings"),
		http.StatusForbidden, "permission_denied")
	expectErrorCode(t, member.get(api.APIPrefix+"/admin/operation-logs"),
		http.StatusForbidden, "permission_denied")
}

func TestAdminDashboardExposesEveryDeclaredMetric(t *testing.T) {
	env := newTestEnv(t)
	client := adminSession(t, env)

	resp := client.get(api.APIPrefix + "/admin/dashboard")
	expectStatus(t, resp, http.StatusOK)
	raw := readRawBody(t, resp)

	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("解析仪表盘响应失败: %v，原文 %s", err, raw)
	}

	// 计数指标与榜单都必须出现在响应里，字段名与声明一致（需求 §17.1）。
	for _, name := range domain.DashboardCounters() {
		if _, ok := body[name]; !ok {
			t.Fatalf("仪表盘缺少计数指标 %s", name)
		}
	}
	for _, name := range domain.DashboardLists() {
		if _, ok := body[name]; !ok {
			t.Fatalf("仪表盘缺少榜单 %s", name)
		}
	}
	// 空榜单必须是 []，不是 null（C-40 的空列表约定）。
	if strings.Contains(string(raw), `"hot_prompts":null`) ||
		strings.Contains(string(raw), `"recent_prompts":null`) {
		t.Fatalf("空榜单应序列化为 []，响应 %s", raw)
	}
}

func TestAdminModelImportReturnsBadGatewayThenRateLimited(t *testing.T) {
	env := newTestEnv(t)
	client := adminSession(t, env)
	env.modelsDev.setResponse(http.StatusInternalServerError, "upstream unavailable")

	// admin_import 的限额是每分钟 5 次（技术可行性报告 §3.2 之外的补充）。
	for i := 0; i < 5; i++ {
		expectErrorCode(t, client.post(api.APIPrefix+"/admin/models/import", map[string]any{"overwrite": false}),
			http.StatusBadGateway, "bad_gateway")
	}
	// 连续失败后触顶：限流计数照常累加，返回 429。
	expectErrorCode(t, client.post(api.APIPrefix+"/admin/models/import", map[string]any{"overwrite": false}),
		http.StatusTooManyRequests, "rate_limited")

	if env.modelsDev.requestCount() != 5 {
		t.Fatalf("上游访问次数 = %d，期望 5（第 6 次在限流处被拒绝）", env.modelsDev.requestCount())
	}
}

func TestAdminModelImportPreviewAndApplyOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := adminSession(t, env)

	resp := client.post(api.APIPrefix+"/admin/models/import/preview", nil)
	expectStatus(t, resp, http.StatusOK)
	preview := decodeBody[struct {
		Items          []json.RawMessage `json:"items"`
		DuplicateCount int               `json:"duplicate_count"`
	}](t, resp)
	if len(preview.Items) != 3 || preview.DuplicateCount != 0 {
		t.Fatalf("预览 = %d 条 / 重复 %d，期望 3 条 / 重复 0", len(preview.Items), preview.DuplicateCount)
	}

	resp = client.post(api.APIPrefix+"/admin/models/import", map[string]any{"overwrite": true})
	expectStatus(t, resp, http.StatusOK)
	applied := decodeBody[struct {
		Total       int `json:"total"`
		Created     int `json:"created"`
		Overwritten int `json:"overwritten"`
		Skipped     int `json:"skipped"`
	}](t, resp)
	if applied.Created != 3 || applied.Total != 3 {
		t.Fatalf("导入结果 = %+v，期望新建 3 条", applied)
	}

	// 再次预览：三条全部标出重复，管理员据此决定是否覆盖（需求 §13.4）。
	resp = client.post(api.APIPrefix+"/admin/models/import/preview", nil)
	expectStatus(t, resp, http.StatusOK)
	second := decodeBody[struct {
		Items          []json.RawMessage `json:"items"`
		DuplicateCount int               `json:"duplicate_count"`
	}](t, resp)
	if second.DuplicateCount != 3 {
		t.Fatalf("重复条目 = %d，期望 3", second.DuplicateCount)
	}

	// 列表里能看到导入来源与统计列。
	resp = client.get(api.APIPrefix + "/admin/models?keyword=alpha")
	expectStatus(t, resp, http.StatusOK)
	list := decodeBody[struct {
		Items []struct {
			Name         string `json:"name"`
			ImportSource string `json:"import_source"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, resp)
	if list.Total != 2 {
		t.Fatalf("模型总数 = %d，期望 2", list.Total)
	}
	for _, item := range list.Items {
		if item.ImportSource != "models_dev" {
			t.Fatalf("导入来源 = %s，期望 models_dev", item.ImportSource)
		}
	}
}

func TestAdminModelImportPreviewPaginatesAndImportsSelection(t *testing.T) {
	env := newTestEnv(t)
	client := adminSession(t, env)

	resp := client.post(api.APIPrefix+"/admin/models/import/preview",
		map[string]any{"page": 1, "page_size": 1, "keyword": "alpha"})
	expectStatus(t, resp, http.StatusOK)
	preview := decodeBody[struct {
		Items          []json.RawMessage `json:"items"`
		Page           int32             `json:"page"`
		PageSize       int32             `json:"page_size"`
		Total          int               `json:"total"`
		CatalogTotal   int               `json:"catalog_total"`
		DuplicateCount int               `json:"duplicate_count"`
		Providers      []struct {
			ID    string `json:"id"`
			Total int    `json:"total"`
		} `json:"providers"`
	}](t, resp)
	if len(preview.Items) != 1 || preview.Total != 2 || preview.CatalogTotal != 3 {
		t.Fatalf("分页预览 = %+v，期望 2 条匹配中返回 1 条", preview)
	}
	if preview.Page != 1 || preview.PageSize != 1 || len(preview.Providers) != 2 {
		t.Fatalf("分页与提供商摘要 = %+v", preview)
	}

	// 只导入选中的一条：未选中的条目不落库。
	resp = client.post(api.APIPrefix+"/admin/models/import", map[string]any{
		"overwrite": false,
		"selection": map[string]any{
			"mode": "keys",
			"keys": []map[string]string{{"provider_id": "alpha", "model_id": "alpha-one"}},
		},
	})
	expectStatus(t, resp, http.StatusOK)
	applied := decodeBody[struct {
		Total   int `json:"total"`
		Created int `json:"created"`
	}](t, resp)
	if applied.Total != 1 || applied.Created != 1 {
		t.Fatalf("按条目导入 = %+v，期望只新建 1 条", applied)
	}

	// 状态筛选走服务端：一条已重复，两条仍是新模型。
	resp = client.post(api.APIPrefix+"/admin/models/import/preview",
		map[string]any{"status": "new", "page_size": 50})
	expectStatus(t, resp, http.StatusOK)
	remaining := decodeBody[struct {
		Total int `json:"total"`
	}](t, resp)
	if remaining.Total != 2 {
		t.Fatalf("新模型 = %d 条，期望 2", remaining.Total)
	}

	// 目录缓存命中后，分页与筛选不会反复回源。
	if env.modelsDev.requestCount() != 1 {
		t.Fatalf("上游访问次数 = %d，期望 1", env.modelsDev.requestCount())
	}
}

func TestAdminSettingsRequireMasterKeyForSecrets(t *testing.T) {
	env := newTestEnv(t)
	pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "admin@example.com", Password: httpTestPassword, Role: "admin", Verified: true,
	})

	// 构造一个未配置主密钥的服务端：密钥类配置项必须被拒绝保存。
	deps := env.Deps(t)
	deps.Admin = admin.New(admin.Deps{
		Pool:       env.pool,
		Config:     env.config,
		Logger:     deps.Logger,
		Prompts:    env.prompt,
		Moderation: env.social,
		Stats:      env.stats,
	})
	server, err := api.New(deps)
	if err != nil {
		t.Fatalf("构造接口层失败: %v", err)
	}
	ts := serve(t, server)

	client := newClientAt(t, env, ts.URL)
	env.login(t, client, "admin@example.com", httpTestPassword)

	// 未配置主密钥时，配置项列表明确告知密钥类配置项不可保存。
	resp := client.get(api.APIPrefix + "/admin/settings")
	expectStatus(t, resp, http.StatusOK)
	settings := decodeBody[struct {
		SecretWritable bool `json:"secret_writable"`
		Items          []struct {
			Key    string `json:"key"`
			Secret bool   `json:"secret"`
		} `json:"items"`
	}](t, resp)
	if settings.SecretWritable {
		t.Fatal("未配置主密钥时 secret_writable 应为 false")
	}
	if len(settings.Items) == 0 {
		t.Fatal("配置项列表不应为空")
	}

	// 非密钥项照常可保存。返回空体的端点以 204 表示成功。
	resp = client.put(api.APIPrefix+"/admin/settings/bayes_prior", map[string]any{"value": 4})
	expectStatus(t, resp, http.StatusNoContent)

	// 密钥项被拒，错误码为 secret_unavailable。
	expectErrorCode(t, client.put(api.APIPrefix+"/admin/settings/smtp_password",
		map[string]any{"value": "s3cret"}), http.StatusUnprocessableEntity, "secret_unavailable")

	// 同一环境下换成配了主密钥的服务端，保存成功且密钥以密文落库。
	keyedServer, err := api.New(env.Deps(t))
	if err != nil {
		t.Fatalf("构造接口层失败: %v", err)
	}
	keyedTS := serve(t, keyedServer)
	keyedClient := newClientAt(t, env, keyedTS.URL)
	env.login(t, keyedClient, "admin@example.com", httpTestPassword)

	resp = keyedClient.get(api.APIPrefix + "/admin/settings")
	expectStatus(t, resp, http.StatusOK)
	keyed := decodeBody[struct {
		SecretWritable bool `json:"secret_writable"`
	}](t, resp)
	if !keyed.SecretWritable {
		t.Fatal("配置了主密钥时 secret_writable 应为 true")
	}

	resp = keyedClient.put(api.APIPrefix+"/admin/settings/smtp_password", map[string]any{"value": "s3cret"})
	expectStatus(t, resp, http.StatusNoContent)

	var cipher string
	if err := env.pool.QueryRow(t.Context(),
		`SELECT secret_ciphertext FROM config WHERE key = 'smtp_password'`).Scan(&cipher); err != nil {
		t.Fatalf("读取密文失败: %v", err)
	}
	if strings.Contains(cipher, "s3cret") {
		t.Fatal("配置表中不应出现明文口令")
	}
	plain, err := security.NewSecretBox("").Open(cipher)
	if err == nil && plain == "s3cret" {
		t.Fatal("空主密钥不应能解开密文")
	}
}

func TestAdminPromptTagAndRecycleFlowOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := adminSession(t, env)

	ownerID := pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "owner@example.com", Password: httpTestPassword, Verified: true,
	})
	promptID := pgtest.SeedPrompt(t, env.pool, ownerID, "后台管理的提示词")

	// 添加标签。
	resp := client.post(api.APIPrefix+"/admin/tags", map[string]any{
		"slug": "audit", "name_zh": "审计", "name_en": "Audit",
		"color": "#112233", "description": "", "priority": 1, "is_enabled": true,
	})
	expectStatus(t, resp, http.StatusOK)
	tag := decodeBody[struct {
		ID int64 `json:"id"`
	}](t, resp)

	resp = client.post(api.APIPrefix+"/admin/prompts/"+itoa(promptID)+"/tags",
		map[string]any{"tag_id": tag.ID})
	expectStatus(t, resp, http.StatusNoContent)

	resp = client.get(api.APIPrefix + "/admin/prompts/" + itoa(promptID))
	expectStatus(t, resp, http.StatusOK)
	detail := decodeBody[struct {
		Tags []struct {
			Slug string `json:"slug"`
		} `json:"tags"`
	}](t, resp)
	if len(detail.Tags) != 1 || detail.Tags[0].Slug != "audit" {
		t.Fatalf("提示词标签 = %+v，期望包含 audit", detail.Tags)
	}

	// 删除 → 回收站列表可见 → 恢复。
	expectStatus(t, client.delete(api.APIPrefix+"/admin/prompts/"+itoa(promptID)), http.StatusNoContent)
	resp = client.get(api.APIPrefix + "/admin/recycle-bin?object_type=prompt")
	expectStatus(t, resp, http.StatusOK)
	recycle := decodeBody[struct {
		Items []struct {
			ObjectType string `json:"object_type"`
			ObjectID   int64  `json:"object_id"`
			Title      string `json:"title"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, resp)
	if recycle.Total != 1 || recycle.Items[0].Title != "后台管理的提示词" {
		t.Fatalf("回收站 = %+v，期望取到刚删除的提示词", recycle)
	}

	resp = client.post(api.APIPrefix+"/admin/recycle-bin/prompt/"+itoa(promptID)+"/restore", nil)
	expectStatus(t, resp, http.StatusNoContent)

	// 题库类对象不允许管理端恢复。
	resp = client.post(api.APIPrefix+"/admin/recycle-bin/library/1/restore", nil)
	expectErrorCode(t, resp, http.StatusForbidden, "permission_denied")

	// 操作日志里应能看到这次删除、恢复与标签变更。
	resp = client.get(api.APIPrefix + "/admin/operation-logs?operator=" + "admin")
	expectStatus(t, resp, http.StatusOK)
	logs := decodeBody[struct {
		Items []struct {
			Action       string `json:"action"`
			OperatorName string `json:"operator_name"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, resp)
	actions := map[string]bool{}
	for _, item := range logs.Items {
		actions[item.Action] = true
	}
	for _, want := range []string{"prompt_tag_update", "prompt_delete", "prompt_restore"} {
		if !actions[want] {
			t.Fatalf("操作日志缺少 %s（已记录：%v）", want, actions)
		}
	}
}

func TestAdminOperationLogsHaveNoWriteRoutes(t *testing.T) {
	env := newTestEnv(t)

	for _, route := range env.server.RegisteredRoutes() {
		if !strings.Contains(route.Path, "/admin/operation-logs") {
			continue
		}
		if route.Method != http.MethodGet {
			t.Fatalf("操作日志模块出现写路由 %s %s（C-43 要求只增不改不删）", route.Method, route.Path)
		}
	}
}

func TestAdminListEndpointsReturnEmptyArrays(t *testing.T) {
	env := newTestEnv(t)
	client := adminSession(t, env)

	paths := []string{
		"/admin/prompts",
		"/admin/models",
		"/admin/users",
		"/admin/tags",
		"/admin/recommend-tags",
		"/admin/reports",
		"/admin/invites",
		"/admin/operation-logs",
		"/admin/announcements",
		"/admin/recycle-bin",
		"/admin/models/import-logs",
	}
	for _, path := range paths {
		resp := client.get(api.APIPrefix + path)
		expectStatus(t, resp, http.StatusOK)
		raw := readRawBody(t, resp)

		var body struct {
			Items    []json.RawMessage `json:"items"`
			Page     int               `json:"page"`
			PageSize int               `json:"page_size"`
			Total    int64             `json:"total"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("解析 %s 响应失败: %v，原文 %s", path, err, raw)
		}
		if strings.Contains(string(raw), `"items":null`) {
			t.Fatalf("%s 的空列表应为 []，响应 %s", path, raw)
		}
		// 后台列表全部有分页（阶段 6 验收标准）。
		if body.Page != 1 || body.PageSize != 20 {
			t.Fatalf("%s 的分页元信息 = %d/%d，期望 1/20", path, body.Page, body.PageSize)
		}
	}
}

func TestAdminActionCatalogueMatchesRequirement(t *testing.T) {
	env := newTestEnv(t)
	client := adminSession(t, env)

	resp := client.get(api.APIPrefix + "/admin/operation-logs/actions")
	expectStatus(t, resp, http.StatusOK)
	body := decodeBody[struct {
		Actions  []string `json:"actions"`
		Required []string `json:"required"`
	}](t, resp)

	if len(body.Required) != 18 {
		t.Fatalf("需求明列的操作类型 = %d 类，期望 18 类", len(body.Required))
	}
	declared := map[string]bool{}
	for _, action := range body.Actions {
		declared[action] = true
	}
	for _, action := range body.Required {
		if !declared[action] {
			t.Fatalf("必备操作 %s 未出现在全部操作类型中", action)
		}
	}
}

func TestAdminRoutesAreDocumentedWithUniqueOperationIDs(t *testing.T) {
	env := newTestEnv(t)
	ts := serve(t, env.server)

	resp, err := http.Get(ts.URL + api.APIPrefix + "/openapi.json")
	if err != nil {
		t.Fatalf("获取 OpenAPI 规范失败: %v", err)
	}
	raw := readRawBody(t, resp)

	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("解析 OpenAPI 规范失败: %v", err)
	}

	// 后台暴露的每一个端点都必须出现在规范里，且 OperationID 唯一。
	seen := map[string]string{}
	for path, methods := range spec.Paths {
		for method, op := range methods {
			if op.OperationID == "" {
				t.Fatalf("%s %s 缺少 OperationID", method, path)
			}
			if previous, ok := seen[op.OperationID]; ok {
				t.Fatalf("OperationID %s 重复：%s 与 %s %s", op.OperationID, previous, method, path)
			}
			seen[op.OperationID] = method + " " + path
		}
	}

	adminRoutes := 0
	for _, route := range env.server.RegisteredRoutes() {
		if !strings.Contains(route.Path, "/admin/") {
			continue
		}
		adminRoutes++
		if _, ok := spec.Paths[route.Path]; !ok {
			t.Fatalf("后台路由 %s %s 未出现在 OpenAPI 规范中", route.Method, route.Path)
		}
	}
	if adminRoutes < 40 {
		t.Fatalf("后台路由数 = %d，期望至少 40 条（C-42 的 11 个模块）", adminRoutes)
	}
}

// TestAdminDictionaryCRUDOverHTTP 覆盖考察维度与考察目的的后台维护，
// 以及被引用条目只能停用的约束。
func TestAdminDictionaryCRUDOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := adminSession(t, env)

	capabilities := decodeBody[struct {
		Items []struct {
			ID   int64  `json:"id"`
			Slug string `json:"slug"`
		} `json:"items"`
	}](t, client.get(api.APIPrefix+"/admin/capabilities?page=1&page_size=100"))
	var reasoningID int64
	for _, item := range capabilities.Items {
		if item.Slug == "reasoning" {
			reasoningID = item.ID
		}
	}
	if reasoningID == 0 {
		t.Fatal("内置考察维度缺少 reasoning")
	}

	createCapability := map[string]any{
		"slug": "test_dimension", "name_zh": "测试维度", "name_en": "Test Dimension",
		"group_key": "cognitive", "description_zh": "中文定义", "description_en": "English definition",
		"is_enabled": true, "sort_order": 99,
	}
	resp := client.post(api.APIPrefix+"/admin/capabilities", createCapability)
	expectStatus(t, resp, http.StatusOK)
	createdCapability := decodeBody[struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	}](t, resp)
	if createdCapability.ID == 0 {
		t.Fatalf("创建考察维度未返回 ID: %+v", createdCapability)
	}

	updateCapability := map[string]any{
		"slug": "test_dimension", "name_zh": "测试维度（改）", "name_en": "Test Dimension",
		"group_key": "execution", "description_zh": "中文定义", "description_en": "English definition",
		"is_enabled": true, "sort_order": 98,
	}
	resp = client.patch(api.APIPrefix+"/admin/capabilities/"+itoa(createdCapability.ID), updateCapability)
	expectStatus(t, resp, http.StatusOK)
	updatedCapability := decodeBody[struct {
		NameZh   string `json:"name_zh"`
		GroupKey string `json:"group_key"`
	}](t, resp)
	if updatedCapability.NameZh != "测试维度（改）" || updatedCapability.GroupKey != "execution" {
		t.Fatalf("考察维度更新结果 = %+v", updatedCapability)
	}
	expectStatus(t, client.delete(api.APIPrefix+"/admin/capabilities/"+itoa(createdCapability.ID)), http.StatusNoContent)

	createPurpose := map[string]any{
		"slug": "test_purpose", "name_zh": "测试目的", "name_en": "Test Purpose",
		"description_zh": "中文定义", "description_en": "English definition",
		"is_high_risk": false, "is_enabled": true, "sort_order": 99,
	}
	resp = client.post(api.APIPrefix+"/admin/purposes", createPurpose)
	expectStatus(t, resp, http.StatusOK)
	createdPurpose := decodeBody[struct {
		ID int64 `json:"id"`
	}](t, resp)
	expectStatus(t, client.delete(api.APIPrefix+"/admin/purposes/"+itoa(createdPurpose.ID)), http.StatusNoContent)

	// 被提示词引用的字典条目只能停用。
	ownerID := pgtest.SeedUser(t, env.pool, pgtest.UserOptions{
		Email: "dictionary-owner@example.com", Password: httpTestPassword, Verified: true,
	})
	pgtest.SeedPrompt(t, env.pool, ownerID, "占用字典条目的提示词")

	resp = client.get(api.APIPrefix + "/admin/purposes?page=1&page_size=100")
	expectStatus(t, resp, http.StatusOK)
	purposes := decodeBody[struct {
		Items []struct {
			ID   int64  `json:"id"`
			Slug string `json:"slug"`
		} `json:"items"`
	}](t, resp)
	var boundaryID int64
	for _, item := range purposes.Items {
		if item.Slug == "capability_boundary" {
			boundaryID = item.ID
		}
	}
	if boundaryID == 0 {
		t.Fatal("内置考察目的缺少 capability_boundary")
	}
	expectErrorCode(t, client.delete(api.APIPrefix+"/admin/purposes/"+itoa(boundaryID)),
		http.StatusConflict, "dictionary_in_use")
	expectErrorCode(t, client.delete(api.APIPrefix+"/admin/capabilities/"+itoa(reasoningID)),
		http.StatusConflict, "dictionary_in_use")

	// 主题标签类型可以写入并在响应中回显。
	resp = client.post(api.APIPrefix+"/admin/tags", map[string]any{
		"slug": "output_form", "name_zh": "输出形态", "name_en": "Output form",
		"color": "#112233", "description": "", "kind": "form",
		"priority": 1, "is_enabled": true,
	})
	expectStatus(t, resp, http.StatusOK)
	tag := decodeBody[struct {
		Kind string `json:"kind"`
	}](t, resp)
	if tag.Kind != "form" {
		t.Fatalf("标签类型 = %q，期望 form", tag.Kind)
	}
}

// readRawBody 读取响应体原文，供需要按字段名核对的用例使用。
func readRawBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	return raw
}

// newClientAt 针对同一数据库上的另一个服务实例建立客户端。
func newClientAt(t *testing.T, env *testEnv, base string) *apiClient {
	t.Helper()
	client := env.newClient(t)
	client.base = base
	return client
}
