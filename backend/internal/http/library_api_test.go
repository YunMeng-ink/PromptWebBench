package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	api "github.com/promptbench/backend/internal/http"
)

type libraryDTO struct {
	ID                int64  `json:"id"`
	OwnerID           int64  `json:"owner_id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Role              string `json:"role"`
	QuestionCount     int32  `json:"question_count"`
	LiveQuestionCount int64  `json:"live_question_count"`
	QuestionLimit     int32  `json:"question_limit"`
}

type questionDTO struct {
	ID              int64    `json:"id"`
	LibraryID       int64    `json:"library_id"`
	Kind            string   `json:"kind"`
	Title           string   `json:"title"`
	Body            string   `json:"body"`
	Tags            []string `json:"tags"`
	SortOrder       int32    `json:"sort_order"`
	SourcePromptID  *int64   `json:"source_prompt_id"`
	SourceAvailable bool     `json:"source_available"`
}

// createLibraryOverHTTP 建库并返回它。
func createLibraryOverHTTP(t *testing.T, client *apiClient, name string) libraryDTO {
	t.Helper()
	resp := client.post(api.APIPrefix+"/libraries", map[string]any{"name": name})
	expectStatus(t, resp, http.StatusOK)
	return decodeBody[struct {
		Library libraryDTO `json:"library"`
	}](t, resp).Library
}

// addReferenceQuestionOverHTTP 把一条提示词作为引用题加入题库。
func addReferenceQuestionOverHTTP(t *testing.T, client *apiClient, libraryID, promptID int64) questionDTO {
	t.Helper()
	resp := client.post(api.APIPrefix+"/libraries/"+itoa(libraryID)+"/questions", map[string]any{
		"prompt_id": promptID,
	})
	expectStatus(t, resp, http.StatusOK)
	return decodeBody[struct {
		Question questionDTO `json:"question"`
	}](t, resp).Question
}

// developerPromptOverHTTP 以开发者身份上传一条开发者可见的提示词。
func developerPromptOverHTTP(t *testing.T, developer *apiClient, title string) int64 {
	t.Helper()
	purposeID := purposeIDs(t, developer)["capability_boundary"]
	capabilityID := capabilityIDs(t, developer)["reasoning"]
	resp := developer.post(api.APIPrefix+"/prompts", map[string]any{
		"title": title, "body": "开发者专属正文", "purpose_id": purposeID,
		"capability_id": capabilityID, "visibility": "developer",
	})
	expectStatus(t, resp, http.StatusOK)
	return decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, resp).Prompt.ID
}

// TestLibraryLifecycleOverHTTP 覆盖题库的创建、题目、标签、排序、导出与回收站。
func TestLibraryLifecycleOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	owner := env.newClient(t)
	owner.prime()
	registerVerifyLogin(t, env, owner, "library-owner@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, owner, "引用题来源")
	lib := createLibraryOverHTTP(t, owner, "我的题库")

	resp := owner.patch(api.APIPrefix+"/libraries/"+itoa(lib.ID), map[string]any{
		"description": "题库描述",
	})
	expectStatus(t, resp, http.StatusOK)

	reference := addReferenceQuestionOverHTTP(t, owner, lib.ID, promptID)
	if reference.Kind != "reference" {
		t.Fatalf("引用题类型为 %q", reference.Kind)
	}
	if reference.Body != "正文" {
		t.Fatalf("引用题正文为 %q，期望取自来源提示词", reference.Body)
	}
	if reference.SourcePromptID == nil || *reference.SourcePromptID != promptID {
		t.Fatalf("引用题未带出来源提示词标识")
	}

	derived := decodeBody[struct {
		Question questionDTO `json:"question"`
	}](t, func() *http.Response {
		resp := owner.post(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions", map[string]any{
			"title": "派生题", "body": "派生正文", "description": "派生说明",
		})
		expectStatus(t, resp, http.StatusOK)
		return resp
	}()).Question

	// 贴标签、去重与移除。
	for _, tag := range []string{"标签甲", "标签甲", "标签乙"} {
		resp := owner.put(api.APIPrefix+"/library-questions/"+itoa(derived.ID)+"/tags", map[string]any{"tag": tag})
		expectStatus(t, resp, http.StatusNoContent)
	}
	detail := decodeBody[struct {
		Question questionDTO `json:"question"`
	}](t, owner.get(api.APIPrefix+"/library-questions/"+itoa(derived.ID)))
	if strings.Join(detail.Question.Tags, ",") != "标签甲,标签乙" {
		t.Fatalf("题目标签为 %v", detail.Question.Tags)
	}
	resp = owner.delete(api.APIPrefix + "/library-questions/" + itoa(derived.ID) + "/tags?tag=" + url.QueryEscape("标签乙"))
	expectStatus(t, resp, http.StatusNoContent)
	detail = decodeBody[struct {
		Question questionDTO `json:"question"`
	}](t, owner.get(api.APIPrefix+"/library-questions/"+itoa(derived.ID)))
	if strings.Join(detail.Question.Tags, ",") != "标签甲" {
		t.Fatalf("移除标签后为 %v", detail.Question.Tags)
	}

	// 排序：把派生题排到前面。
	orderResp := owner.put(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions/order", map[string]any{
		"ids": []int64{derived.ID, reference.ID},
	})
	expectStatus(t, orderResp, http.StatusNoContent)

	list := decodeBody[struct {
		Items []questionDTO `json:"items"`
		Total int64         `json:"total"`
	}](t, owner.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions"))
	if list.Total != 2 {
		t.Fatalf("题目总数为 %d，期望 2", list.Total)
	}
	if list.Items[0].ID != derived.ID {
		t.Fatalf("排序后首题为 %d，期望 %d", list.Items[0].ID, derived.ID)
	}

	// 没有标签的题目，标签字段必须是空列表而不是空值（C-40）。
	questionResp := owner.get(api.APIPrefix + "/library-questions/" + itoa(reference.ID))
	raw, err := io.ReadAll(questionResp.Body)
	questionResp.Body.Close()
	if err != nil {
		t.Fatalf("读取题目响应失败: %v", err)
	}
	if !strings.Contains(string(raw), `"tags":[]`) {
		t.Fatalf("无标签题目的标签字段不是空列表：%s", raw)
	}

	// 导出：15 项要素齐全。
	export := decodeBody[struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}](t, owner.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/export"))
	if export.Filename != "我的题库.md" {
		t.Fatalf("导出文件名为 %q", export.Filename)
	}
	for _, marker := range []string{
		"# 我的题库", "- 题库描述：题库描述", "- 创建者：", "- 导出时间：", "- 题目数量：2",
		"## 1. 派生题", "- 考察目的：", "- 考察维度：", "- 题目标签：", "- 来源提示词：",
		"- 来源版本：", "- 修改说明：", "### 提示词正文", "### 题目说明",
	} {
		if !strings.Contains(export.Content, marker) {
			t.Fatalf("导出内容缺少 %q：\n%s", marker, export.Content)
		}
	}

	// 删除题目 → 回收站 → 恢复 → 再删除 → 彻底删除。
	resp = owner.delete(api.APIPrefix + "/library-questions/" + itoa(reference.ID))
	expectStatus(t, resp, http.StatusNoContent)
	recycle := decodeBody[struct {
		Items []struct {
			Question  questionDTO `json:"question"`
			DeletedBy int64       `json:"deleted_by"`
			ExpiresAt string      `json:"expires_at"`
		} `json:"items"`
	}](t, owner.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/recycle-bin"))
	if len(recycle.Items) != 1 {
		t.Fatalf("回收站条目为 %d 条，期望 1 条", len(recycle.Items))
	}
	if recycle.Items[0].ExpiresAt == "" {
		t.Fatalf("回收站条目缺少到期时刻")
	}

	resp = owner.post(api.APIPrefix+"/library-questions/"+itoa(reference.ID)+"/restore", nil)
	expectStatus(t, resp, http.StatusOK)
	resp = owner.delete(api.APIPrefix + "/library-questions/" + itoa(reference.ID))
	expectStatus(t, resp, http.StatusNoContent)
	resp = owner.delete(api.APIPrefix + "/library-questions/" + itoa(reference.ID) + "/purge")
	expectStatus(t, resp, http.StatusNoContent)
	// 引用题占用的容量在彻底删除后归还。
	detailLib := decodeBody[struct {
		Library libraryDTO `json:"library"`
	}](t, owner.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)))
	if detailLib.Library.QuestionCount != 0 {
		t.Fatalf("彻底删除引用题后题目计数为 %d", detailLib.Library.QuestionCount)
	}

	// 删除题库 → 回收站 → 恢复 → 彻底删除。
	resp = owner.delete(api.APIPrefix + "/libraries/" + itoa(lib.ID))
	expectStatus(t, resp, http.StatusNoContent)
	expectErrorCode(t, owner.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)), http.StatusNotFound, "not_found")

	bin := decodeBody[struct {
		Items []struct {
			LibraryID int64  `json:"library_id"`
			Name      string `json:"name"`
		} `json:"items"`
	}](t, owner.get(api.APIPrefix+"/me/libraries/recycle-bin"))
	if len(bin.Items) != 1 || bin.Items[0].Name != "我的题库" {
		t.Fatalf("题库回收站内容为 %+v", bin.Items)
	}

	resp = owner.post(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/restore", nil)
	expectStatus(t, resp, http.StatusOK)
	resp = owner.delete(api.APIPrefix + "/libraries/" + itoa(lib.ID))
	expectStatus(t, resp, http.StatusNoContent)
	resp = owner.delete(api.APIPrefix + "/libraries/" + itoa(lib.ID) + "/purge")
	expectStatus(t, resp, http.StatusNoContent)
	expectErrorCode(t, owner.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)), http.StatusNotFound, "not_found")
}

// TestCollaboratorCannotReadDeveloperVisibleQuestionOverHTTP 是专项回归的 HTTP 侧。
//
// 题库题目查询若只校验成员身份而不复查被引用提示词的可见性，普通用户协作者
// 就能读到开发者专属内容的全文，导出接口同样泄漏。这里断言读取返回 404、
// 列表与导出都不含该题目。
func TestCollaboratorCannotReadDeveloperVisibleQuestionOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	developer := env.newClient(t)
	developer.prime()
	registerDeveloper(t, env, developer, "library-dev@example.com", httpTestPassword)

	ownerPrompt := developerPromptOverHTTP(t, developer, "开发者专属提示词")
	lib := createLibraryOverHTTP(t, developer, "开发者题库")
	question := addReferenceQuestionOverHTTP(t, developer, lib.ID, ownerPrompt)

	collaborator := env.newClient(t)
	collaborator.prime()
	registerVerifyLogin(t, env, collaborator, "library-collaborator@example.com", httpTestPassword)

	// 协作者尚无访问权，先由所有者邀请。
	collaboratorName := decodeBody[struct {
		Username string `json:"username"`
	}](t, collaborator.get(api.APIPrefix+"/me")).Username
	resp := developer.post(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/members", map[string]any{
		"username": collaboratorName,
	})
	expectStatus(t, resp, http.StatusOK)

	expectErrorCode(t,
		collaborator.get(api.APIPrefix+"/library-questions/"+itoa(question.ID)),
		http.StatusNotFound, "not_found")

	list := decodeBody[struct {
		Items []questionDTO `json:"items"`
		Total int64         `json:"total"`
	}](t, collaborator.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions"))
	if list.Total != 0 {
		t.Fatalf("协作者看到 %d 道开发者专属题目", list.Total)
	}

	export := decodeBody[struct {
		Content string `json:"content"`
	}](t, collaborator.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/export"))
	if strings.Contains(export.Content, "开发者专属正文") {
		t.Fatalf("导出内容泄漏了开发者专属正文：\n%s", export.Content)
	}

	// 协作者也不能把开发者可见的提示词加进题库。
	expectErrorCode(t, collaborator.post(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions", map[string]any{
		"prompt_id": ownerPrompt,
	}), http.StatusNotFound, "not_found")

	// 所有者作为开发者可以看到。
	seen := decodeBody[struct {
		Question questionDTO `json:"question"`
	}](t, developer.get(api.APIPrefix+"/library-questions/"+itoa(question.ID)))
	if seen.Question.Body != "开发者专属正文" {
		t.Fatalf("所有者读到的正文为 %q", seen.Question.Body)
	}
}

// TestLibraryRoutesRejectGuestsAndOutsidersOverHTTP 验证游客与非成员一律被挡。
func TestLibraryRoutesRejectGuestsAndOutsidersOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	owner := env.newClient(t)
	owner.prime()
	registerVerifyLogin(t, env, owner, "library-outsider-owner@example.com", httpTestPassword)
	lib := createLibraryOverHTTP(t, owner, "私有题库")

	guest := env.newClient(t)
	guest.prime()
	expectErrorCode(t, guest.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)), http.StatusNotFound, "not_found")
	expectErrorCode(t, guest.get(api.APIPrefix+"/me/libraries"), http.StatusUnauthorized, "unauthorized")
	expectErrorCode(t, guest.post(api.APIPrefix+"/libraries", map[string]any{"name": "游客题库"}),
		http.StatusUnauthorized, "unauthorized")

	outsider := env.newClient(t)
	outsider.prime()
	registerVerifyLogin(t, env, outsider, "library-outsider@example.com", httpTestPassword)
	expectErrorCode(t, outsider.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)), http.StatusNotFound, "not_found")
	expectErrorCode(t, outsider.get(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/export"),
		http.StatusNotFound, "not_found")
	expectErrorCode(t, outsider.post(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions", map[string]any{
		"title": "外部题目", "body": "正文",
	}), http.StatusNotFound, "not_found")
}

// TestLibraryQuotaAndValidationOverHTTP 覆盖配额与字段校验的对外表现。
func TestLibraryQuotaAndValidationOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "library-quota@example.com", httpTestPassword)

	lib := createLibraryOverHTTP(t, client, "唯一题库")
	expectErrorCode(t, client.post(api.APIPrefix+"/libraries", map[string]any{"name": "第二个题库"}),
		http.StatusConflict, "quota_exceeded")
	expectErrorCode(t, client.post(api.APIPrefix+"/libraries", map[string]any{"name": ""}),
		http.StatusUnprocessableEntity, "validation_failed")

	// 引用题不接受正文：正文一律取自来源提示词。
	expectErrorCode(t, client.post(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions", map[string]any{
		"prompt_id": 1, "title": "不该带标题",
	}), http.StatusUnprocessableEntity, "validation_failed")
	// 派生题必须同时给出标题与正文。
	expectErrorCode(t, client.post(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions", map[string]any{
		"title": "只有标题",
	}), http.StatusUnprocessableEntity, "validation_failed")
	// 引用不存在的提示词按不可见处理。
	expectErrorCode(t, client.post(api.APIPrefix+"/libraries/"+itoa(lib.ID)+"/questions", map[string]any{
		"prompt_id": 999999,
	}), http.StatusNotFound, "not_found")
}

// TestLibraryRoutesAreDocumentedInOpenAPI 断言题库端点出现在生成的规范里。
func TestLibraryRoutesAreDocumentedInOpenAPI(t *testing.T) {
	srv := newTestServer(t)
	ts := serve(t, srv)

	resp, err := http.Get(ts.URL + api.APIPrefix + "/openapi.json")
	if err != nil {
		t.Fatalf("获取 OpenAPI 规范失败: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取规范失败: %v", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("规范不是合法 JSON: %v", err)
	}
	paths := spec["paths"].(map[string]any)

	for _, path := range []string{
		"/me/libraries",
		"/me/libraries/recycle-bin",
		"/libraries",
		"/libraries/{id}",
		"/libraries/{id}/export",
		"/libraries/{id}/members",
		"/libraries/{id}/members/{user_id}",
		"/libraries/{id}/questions",
		"/libraries/{id}/questions/order",
		"/libraries/{id}/recycle-bin",
		"/library-questions/{id}",
		"/library-questions/{id}/tags",
		"/library-questions/{id}/revisions",
	} {
		key := api.APIPrefix + path
		if _, ok := paths[key]; !ok {
			t.Fatalf("规范缺少 %s，已登记路径: %v", key, keysOf(paths))
		}
	}

	// 题目类型与题库角色以枚举形式出现在规范中，前端据此生成选项。
	specText := string(body)
	for _, value := range []string{"reference", "derived", "collaborator"} {
		if !strings.Contains(specText, fmt.Sprintf("%q", value)) {
			t.Fatalf("规范中缺少枚举取值 %s", value)
		}
	}
}
