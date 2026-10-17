package auth_test

import (
	"context"
	"testing"

	"github.com/promptbench/backend/internal/repo"
)

// TestExportMyDataCoversThreeKinds 验证 G-08：导出含提示词、评分与评论三类数据。
func TestExportMyDataCoversThreeKinds(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "export@example.com", testPassword)

	// 造三类数据：一条提示词、一条评分、一条评论。
	modelID := seedModel(t, h, "export-model")
	promptID := seedPrompt(t, h, userID, "导出的提示词")
	seedRating(t, h, userID, promptID, modelID)
	seedComment(t, h, userID, promptID, "导出的评论")

	exported, err := h.service.ExportMyData(ctx, userID)
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if len(exported.Prompts) != 1 {
		t.Fatalf("提示词 = %d 条，期望 1 条", len(exported.Prompts))
	}
	if len(exported.Ratings) != 1 {
		t.Fatalf("评分 = %d 条，期望 1 条", len(exported.Ratings))
	}
	if len(exported.Comments) != 1 {
		t.Fatalf("评论 = %d 条，期望 1 条", len(exported.Comments))
	}
	if exported.Prompts[0].Title != "导出的提示词" {
		t.Fatalf("提示词标题 = %q", exported.Prompts[0].Title)
	}
	if exported.Prompts[0].PurposeNameZh == "" || exported.Prompts[0].CapabilityNameZh == "" {
		t.Error("导出应带出考察目的与考察维度名称")
	}
	if exported.Ratings[0].ModelName != "export-model" {
		t.Fatalf("评分模型名 = %q", exported.Ratings[0].ModelName)
	}
	if exported.Comments[0].Body != "导出的评论" {
		t.Fatalf("评论正文 = %q", exported.Comments[0].Body)
	}
	if exported.Filename == "" {
		t.Error("导出应带文件名")
	}
}

// TestDeleteAccountAnonymizesAndTransfers 验证 G-09：注销后账户匿名化、
// 会话失效、提示词转移到系统账户并保留原上传者。
func TestDeleteAccountAnonymizesAndTransfers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "delete-me@example.com", testPassword)

	var originalUsername string
	if err := h.pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&originalUsername); err != nil {
		t.Fatalf("读取昵称失败: %v", err)
	}
	promptID := seedPrompt(t, h, userID, "注销用户的提示词")

	if err := h.service.DeleteAccount(ctx, userID, "10.0.0.9"); err != nil {
		t.Fatalf("注销失败: %v", err)
	}

	// 账户软删且匿名化：不可登录、邮箱与昵称释放。
	var (
		status, email, username string
		deleted                 bool
		hasPassword             bool
	)
	if err := h.pool.QueryRow(ctx, `
		SELECT status, email, username, deleted_at IS NOT NULL, password_hash IS NOT NULL
		FROM users WHERE id = $1`, userID).Scan(&status, &email, &username, &deleted, &hasPassword); err != nil {
		t.Fatalf("读取注销后账户失败: %v", err)
	}
	if !deleted {
		t.Error("注销后账户应处于软删状态")
	}
	if username != "已注销用户" {
		t.Errorf("注销后昵称 = %q，期望「已注销用户」", username)
	}
	if email == "delete-me@example.com" {
		t.Error("注销后邮箱应被替换，释放原邮箱占用")
	}
	if hasPassword {
		t.Error("注销后口令哈希应被清空")
	}

	// 同一邮箱可以重新注册。
	if _, err := h.register(t, "delete-me@example.com", testPassword); err != nil {
		t.Fatalf("注销后原邮箱应可重新注册: %v", err)
	}

	// 提示词转移到系统账户，原上传者保留。
	var uploaderID int64
	var originalID *int64
	if err := h.pool.QueryRow(ctx,
		`SELECT uploader_id, original_uploader_id FROM prompts WHERE id = $1`, promptID).Scan(&uploaderID, &originalID); err != nil {
		t.Fatalf("读取提示词归属失败: %v", err)
	}
	var systemID int64
	if err := h.pool.QueryRow(ctx, `SELECT id FROM users WHERE is_system`).Scan(&systemID); err != nil {
		t.Fatalf("读取系统账户失败: %v", err)
	}
	if uploaderID != systemID {
		t.Errorf("提示词上传者 = %d，期望系统账户 %d", uploaderID, systemID)
	}
	if originalID == nil || *originalID != userID {
		t.Errorf("原上传者 = %v，期望 %d", originalID, userID)
	}

	// 审计日志留痕。
	logs, err := repo.New(h.pool).AdminListOperationLogs(ctx, repo.AdminListOperationLogsParams{
		Action:     "account_delete",
		LimitCount: 10,
	})
	if err != nil {
		t.Fatalf("读取审计日志失败: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("注销审计记录 = %d 条，期望 1 条", len(logs))
	}
	if logs[0].OperatorName != originalUsername {
		t.Errorf("审计记录操作人 = %q，期望 %q", logs[0].OperatorName, originalUsername)
	}
}

// TestDeleteAccountBlocksLastAdmin 验证最后一个管理员不能注销自己。
func TestDeleteAccountBlocksLastAdmin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := h.registerAndVerify(t, "admin-last@example.com", testPassword)
	if _, err := h.pool.Exec(ctx, `UPDATE users SET role = 'admin' WHERE id = $1`, userID); err != nil {
		t.Fatalf("授予管理员失败: %v", err)
	}

	err := h.service.DeleteAccount(ctx, userID, "")
	if err == nil {
		t.Fatal("最后一个管理员注销应被拒绝")
	}
	if h.countRows(t, `SELECT count(*) FROM users WHERE id = $1 AND deleted_at IS NULL`, userID) != 1 {
		t.Error("被拒的注销不应改变账户状态")
	}
}

// TestDeleteAccountInvalidatesSession 验证注销后既有会话立即失效（token version 提升）。
func TestDeleteAccountInvalidatesSession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	email := "delete-session@example.com"
	userID := h.registerAndVerify(t, email, testPassword)
	session := h.login(t, email, testPassword)

	if err := h.service.DeleteAccount(ctx, userID, ""); err != nil {
		t.Fatalf("注销失败: %v", err)
	}
	if _, err := h.service.ResolveSession(ctx, session.User.ID, session.User.TokenVersion); err == nil {
		t.Fatal("注销后旧会话应失效")
	}
}

// ---------- 数据构造 ----------

func seedModel(t *testing.T, h *harness, key string) int64 {
	t.Helper()
	var id int64
	if err := h.pool.QueryRow(context.Background(), `
		INSERT INTO models (name, model_key, provider_key)
		VALUES ($1, $1, 'export-test') RETURNING id`, key).Scan(&id); err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}
	return id
}

func seedPrompt(t *testing.T, h *harness, uploaderID int64, title string) int64 {
	t.Helper()
	ctx := context.Background()

	var purposeID, capabilityID int64
	if err := h.pool.QueryRow(ctx, `SELECT id FROM purposes WHERE slug = 'capability_boundary'`).Scan(&purposeID); err != nil {
		t.Fatalf("读取考察目的失败: %v", err)
	}
	if err := h.pool.QueryRow(ctx, `SELECT id FROM capabilities WHERE slug = 'reasoning'`).Scan(&capabilityID); err != nil {
		t.Fatalf("读取考察维度失败: %v", err)
	}

	var promptID, versionID int64
	if err := h.pool.QueryRow(ctx, `
		INSERT INTO prompts (title, body, purpose_id, capability_id, uploader_id)
		VALUES ($1, '正文', $2, $3, $4) RETURNING id`, title, purposeID, capabilityID, uploaderID).Scan(&promptID); err != nil {
		t.Fatalf("创建提示词失败: %v", err)
	}
	if err := h.pool.QueryRow(ctx, `
		INSERT INTO prompt_versions (prompt_id, version_no, major, minor, title, body, purpose_id, capability_id, visibility, sensitive_level, author_id)
		VALUES ($1, 'v1.0', 1, 0, $2, '正文', $3, $4, 'public', 'low', $5) RETURNING id`,
		promptID, title, purposeID, capabilityID, uploaderID).Scan(&versionID); err != nil {
		t.Fatalf("创建提示词版本失败: %v", err)
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE prompts SET current_version_id = $2 WHERE id = $1`, promptID, versionID); err != nil {
		t.Fatalf("回填当前版本失败: %v", err)
	}
	return promptID
}

func seedRating(t *testing.T, h *harness, userID, promptID, modelID int64) {
	t.Helper()
	var versionID int64
	if err := h.pool.QueryRow(context.Background(),
		`SELECT current_version_id FROM prompts WHERE id = $1`, promptID).Scan(&versionID); err != nil {
		t.Fatalf("读取当前版本失败: %v", err)
	}
	if _, err := h.pool.Exec(context.Background(), `
		INSERT INTO ratings (user_id, prompt_id, prompt_version_id, model_id, score)
		VALUES ($1, $2, $3, $4, 4)`, userID, promptID, versionID, modelID); err != nil {
		t.Fatalf("创建评分失败: %v", err)
	}
}

func seedComment(t *testing.T, h *harness, userID, promptID int64, body string) {
	t.Helper()
	if _, err := h.pool.Exec(context.Background(),
		`INSERT INTO comments (prompt_id, user_id, body) VALUES ($1, $2, $3)`, promptID, userID, body); err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}
}
