package api_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"testing"

	api "github.com/promptbench/backend/internal/http"
)

// pngBytes 是一段最小可用的 PNG 文件头，用途是让 multipart 上传携带真实的图片声明。
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
}

// uploadRatingImage 构造带 CSRF 头的 multipart 请求并上传一张图片。
func uploadRatingImage(t *testing.T, client *apiClient, promptID, modelID int64, contentType, filename string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="image"; filename="%s"`, filename)},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatalf("构造上传表单失败: %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("写入上传内容失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭表单失败: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s%s/prompts/%d/rating-images/%d", client.base, api.APIPrefix, promptID, modelID), &buf)
	if err != nil {
		t.Fatalf("构造上传请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: clientSessionCookie(t, client)})
	req.Header.Set(api.CSRFHeaderName, client.csrfToken())

	resp, err := client.client.Do(req)
	if err != nil {
		t.Fatalf("发送上传请求失败: %v", err)
	}
	return resp
}

type showcaseGroup struct {
	ModelID     int64  `json:"model_id"`
	ModelName   string `json:"model_name"`
	ImageCount  int64  `json:"image_count"`
	OutputCount int64  `json:"output_count"`
}

// TestShowcaseUploadAndVisibility 覆盖：评分后上传图片与模型输出 → 公开展示区可见 →
// 管理员删除后从展示区消失且审计日志留痕；管理员之外的账户不能删除。
func TestShowcaseUploadAndVisibility(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "showcase-author@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, author, "展示区提示词")
	modelID := firstModelID(t, env)

	// 未评分时上传图片被拒绝：附图必须绑定一条真实评分。
	expectErrorCode(t, uploadRatingImage(t, author, promptID, modelID, "image/png", "result.png"),
		http.StatusUnprocessableEntity, "invalid_state")

	// 评分后上传图片，图床返回的外链写入数据库并出现在展示区。
	author.post(fmt.Sprintf("%s/prompts/%d/ratings", api.APIPrefix, promptID), map[string]any{
		"model_id": modelID, "score": 4,
	})
	uploaded := decodeBody[struct {
		Image struct {
			ID  int64  `json:"id"`
			URL string `json:"url"`
		} `json:"image"`
	}](t, uploadRatingImage(t, author, promptID, modelID, "image/png", "result.png"))
	if uploaded.Image.ID == 0 || uploaded.Image.URL == "" {
		t.Fatalf("上传返回缺少图片记录: %+v", uploaded.Image)
	}

	// 提交模型输出文本。
	author.post(fmt.Sprintf("%s/prompts/%d/rating-outputs", api.APIPrefix, promptID), map[string]any{
		"model_id": modelID, "content": "模型输出示例",
	})

	guest := env.newClient(t)
	guest.prime()
	groups := decodeBody[struct {
		Items []showcaseGroup `json:"items"`
	}](t, guest.get(api.APIPrefix+"/showcase/groups"))
	found := false
	for _, group := range groups.Items {
		if group.ModelID == modelID {
			found = true
			if group.ImageCount != 1 || group.OutputCount != 1 {
				t.Fatalf("展示区计数 = %d 张图 / %d 条输出，期望 1 / 1",
					group.ImageCount, group.OutputCount)
			}
		}
	}
	if !found {
		t.Fatal("公开提示词的素材应出现在展示区模型聚合中")
	}

	entries := decodeBody[struct {
		Items []struct {
			Kind        string `json:"kind"`
			URL         string `json:"url"`
			Content     string `json:"content"`
			PromptTitle string `json:"prompt_title"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, guest.get(fmt.Sprintf("%s/showcase/models/%d/entries", api.APIPrefix, modelID)))
	if entries.Total != 2 || len(entries.Items) != 2 {
		t.Fatalf("素材数量 = %d，期望 2", entries.Total)
	}

	// 普通账户不能调用后台删除。
	member := memberSession(t, env)
	expectErrorCode(t, member.delete(fmt.Sprintf("%s/admin/showcase/images/%d", api.APIPrefix, uploaded.Image.ID)),
		http.StatusForbidden, "permission_denied")

	// 管理员删除图片：展示区不再返回该图片，审计日志写入删除动作。
	admin := adminSession(t, env)
	expectStatus(t, admin.delete(fmt.Sprintf("%s/admin/showcase/images/%d", api.APIPrefix, uploaded.Image.ID)),
		http.StatusNoContent)

	afterDelete := decodeBody[struct {
		Items []struct {
			Kind string `json:"kind"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, guest.get(fmt.Sprintf("%s/showcase/models/%d/entries", api.APIPrefix, modelID)))
	if afterDelete.Total != 1 {
		t.Fatalf("删除后素材数量 = %d，期望仅剩模型输出", afterDelete.Total)
	}
	for _, item := range afterDelete.Items {
		if item.Kind == "image" {
			t.Fatal("已删除的图片不应出现在展示区")
		}
	}

	var auditCount int
	if err := env.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM operation_logs WHERE action = 'showcase_image_delete' AND object_id = $1`,
		uploaded.Image.ID).Scan(&auditCount); err != nil {
		t.Fatalf("查询审计日志失败: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("删除图片的审计日志条数 = %d，期望 1", auditCount)
	}

	adminImages := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, admin.get(api.APIPrefix+"/admin/showcase/images"))
	if adminImages.Total != 0 {
		t.Fatalf("管理端图片列表在删除后应为空，实际 %d", adminImages.Total)
	}
}

// TestShowcaseRejectsInvalidImage 覆盖图片类型与评分归属校验。
func TestShowcaseRejectsInvalidImage(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "showcase-invalid@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, author, "图片校验")
	modelID := firstModelID(t, env)
	author.post(fmt.Sprintf("%s/prompts/%d/ratings", api.APIPrefix, promptID), map[string]any{
		"model_id": modelID, "score": 3,
	})

	// 非图片类型在进入图床之前就被拒绝。
	expectErrorCode(t, uploadRatingImage(t, author, promptID, modelID, "text/plain", "notes.txt"),
		http.StatusUnprocessableEntity, "validation_failed")
	if env.imageBed.requestCount() != 0 {
		t.Fatalf("非法类型不应外呼图床，实际调用 %d 次", env.imageBed.requestCount())
	}

	// 图床失败时返回 502，且数据库不写入外链。
	env.imageBed.setFailure(true)
	expectErrorCode(t, uploadRatingImage(t, author, promptID, modelID, "image/png", "result.png"),
		http.StatusBadGateway, "bad_gateway")
	var count int
	if err := env.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM rating_images WHERE prompt_id = $1`, promptID).Scan(&count); err != nil {
		t.Fatalf("统计图片记录失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("图床失败后不应写入图片记录，实际 %d 条", count)
	}
}

// TestShowcaseLimitsImagesPerRating 覆盖单条评分的图片张数上限。
func TestShowcaseLimitsImagesPerRating(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "showcase-limit@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, author, "图片数量上限")
	modelID := firstModelID(t, env)
	author.post(fmt.Sprintf("%s/prompts/%d/ratings", api.APIPrefix, promptID), map[string]any{
		"model_id": modelID, "score": 3,
	})

	for i := 0; i < 4; i++ {
		resp := uploadRatingImage(t, author, promptID, modelID, "image/png", fmt.Sprintf("r%d.png", i))
		expectStatus(t, resp, http.StatusOK)
	}
	// 第五张被配额拒绝，且不会调用图床。
	before := env.imageBed.requestCount()
	expectErrorCode(t, uploadRatingImage(t, author, promptID, modelID, "image/png", "r4.png"),
		http.StatusConflict, "quota_exceeded")
	if env.imageBed.requestCount() != before {
		t.Fatal("超出张数上限时不应外呼图床")
	}
}

// TestShowcaseOutputUpsertsPerRating 覆盖：同一评分重复提交输出覆盖原内容，
// 展示区只保留一条。
func TestShowcaseOutputUpsertsPerRating(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "showcase-upsert@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, author, "输出覆盖")
	modelID := firstModelID(t, env)
	author.post(fmt.Sprintf("%s/prompts/%d/ratings", api.APIPrefix, promptID), map[string]any{
		"model_id": modelID, "score": 4,
	})
	for _, content := range []string{"第一版输出", "第二版输出"} {
		expectStatus(t, author.post(
			fmt.Sprintf("%s/prompts/%d/rating-outputs", api.APIPrefix, promptID),
			map[string]any{"model_id": modelID, "content": content},
		), http.StatusOK)
	}

	guest := env.newClient(t)
	guest.prime()
	entries := decodeBody[struct {
		Items []struct {
			Kind    string `json:"kind"`
			Content string `json:"content"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, guest.get(fmt.Sprintf("%s/showcase/models/%d/entries", api.APIPrefix, modelID)))
	if entries.Total != 1 || len(entries.Items) != 1 {
		t.Fatalf("重复提交后应只有一条输出，实际 %d", entries.Total)
	}
	if entries.Items[0].Kind != "output" || entries.Items[0].Content != "第二版输出" {
		t.Fatalf("输出未被覆盖: %+v", entries.Items[0])
	}
}

// TestShowcaseHidesPrivatePrompts 覆盖：私有提示词的素材不进公开展示区。
func TestShowcaseHidesPrivatePrompts(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "showcase-private@example.com", httpTestPassword)

	purposeID, capabilityID := defaultClassification(t, env)
	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, author.post(api.APIPrefix+"/prompts", map[string]any{
		"title": "私有提示词", "body": "正文", "purpose_id": purposeID, "capability_id": capabilityID,
		"visibility": "private",
	}))
	promptID := created.Prompt.ID
	modelID := firstModelID(t, env)
	author.post(fmt.Sprintf("%s/prompts/%d/ratings", api.APIPrefix, promptID), map[string]any{
		"model_id": modelID, "score": 5,
	})
	expectStatus(t, uploadRatingImage(t, author, promptID, modelID, "image/png", "result.png"), http.StatusOK)

	guest := env.newClient(t)
	guest.prime()
	groups := decodeBody[struct {
		Items []showcaseGroup `json:"items"`
	}](t, guest.get(api.APIPrefix+"/showcase/groups"))
	for _, group := range groups.Items {
		if group.ModelID == modelID {
			t.Fatal("私有提示词的素材不应出现在公开展示区")
		}
	}
}
