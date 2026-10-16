package admin_test

import (
	"context"
	"testing"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/service/admin"
)

// ---------- 模块 1：统计仪表盘 ----------

func TestDashboardCountsReflectSeededData(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()

	memberID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "member@example.com", Verified: true})
	developerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "dev@example.com", Role: "developer", Verified: true,
	})
	pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "banned@example.com", Status: "banned",
	})
	promptID := pgtest.SeedPrompt(t, h.pool, memberID, "被统计的提示词")
	pgtest.SeedPromptWith(t, h.pool, pgtest.PromptOptions{
		UploaderID: developerID, Title: "开发者可见提示词", Visibility: "developer",
	})
	modelID := h.seedModel(t, "统计模型", "stats-model", "stats-provider")
	h.seedRating(t, memberID, promptID, modelID, 5)

	if _, err := h.pool.Exec(ctx, `
		INSERT INTO comments (prompt_id, user_id, body) VALUES ($1, $2, '一条评论')`,
		promptID, memberID); err != nil {
		t.Fatalf("写入评论失败: %v", err)
	}
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO reports (reporter_id, object_type, object_id, reason)
		VALUES ($1, 'prompt', $2, 'spam')`, memberID, promptID); err != nil {
		t.Fatalf("写入举报失败: %v", err)
	}
	h.seedRecommendTag(t, promptID, modelID, "obsolete", "pending")

	board, err := h.service.LoadDashboard(ctx)
	if err != nil {
		t.Fatalf("读取仪表盘失败: %v", err)
	}

	counts := board.Counts
	// 四个账户：系统默认账户、普通用户、开发者、被封禁用户。
	// 系统账户与封禁账户的身份组都是 user，因此普通用户计数为 3。
	if counts.UserTotal != 4 {
		t.Fatalf("用户总数 = %d，期望 4", counts.UserTotal)
	}
	if counts.UserRoleUser != 3 || counts.UserRoleDeveloper != 1 {
		t.Fatalf("身份组计数 = user:%d developer:%d，期望 3 与 1",
			counts.UserRoleUser, counts.UserRoleDeveloper)
	}
	if counts.UserBanned != 1 {
		t.Fatalf("封禁用户数 = %d，期望 1", counts.UserBanned)
	}
	if counts.UserToday != 4 || counts.PromptToday != 2 {
		t.Fatalf("今日新增 = 用户:%d 提示词:%d，期望 4 与 2", counts.UserToday, counts.PromptToday)
	}
	if counts.PromptTotal != 2 || counts.PromptPublic != 1 || counts.PromptDeveloper != 1 {
		t.Fatalf("提示词计数 = 总数:%d 公开:%d 开发者:%d，期望 2、1、1",
			counts.PromptTotal, counts.PromptPublic, counts.PromptDeveloper)
	}
	if counts.RatingTotal != 1 || counts.CommentTotal != 1 {
		t.Fatalf("评分:%d 评论:%d，期望各 1", counts.RatingTotal, counts.CommentTotal)
	}
	if counts.ReportPending != 1 || counts.RecommendPending != 1 {
		t.Fatalf("待处理举报:%d 待确认推荐标签:%d，期望各 1", counts.ReportPending, counts.RecommendPending)
	}
	if counts.ModelTotal != 1 || counts.InviteTotal != 0 || counts.RecycleTotal != 0 {
		t.Fatalf("模型:%d 邀请码:%d 回收站:%d，期望 1、0、0",
			counts.ModelTotal, counts.InviteTotal, counts.RecycleTotal)
	}
	// 两条提示词都没有收藏与评论，热门榜按 ID 升序取到它们；最近上传榜同理。
	if len(board.Hot) != 2 || board.Hot[0].ID != promptID {
		t.Fatalf("热门提示词 = %+v，期望首条为提示词 %d", board.Hot, promptID)
	}
	if len(board.Recent) != 2 {
		t.Fatalf("最近上传提示词 = %d 条，期望 2 条", len(board.Recent))
	}
}

// ---------- 模块 2：提示词管理 ----------

func TestAdminDeletePromptEntersRecycleBinAndExpiresRecommendTags(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "owner@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "待删除提示词")
	tagID := h.seedRecommendTag(t, promptID, 0, "obsolete", "confirmed")

	if err := h.prompt.Delete(ctx, operator, promptID, "10.0.0.1"); err != nil {
		t.Fatalf("删除提示词失败: %v", err)
	}

	if h.scalar(t, `SELECT count(*) FROM prompts WHERE id = $1 AND deleted_at IS NOT NULL`, promptID) != 1 {
		t.Fatal("提示词应被软删除")
	}
	if h.scalar(t, `SELECT count(*) FROM recycle_bin
		WHERE object_type = 'prompt' AND object_id = $1 AND restored_at IS NULL AND purged_at IS NULL`, promptID) != 1 {
		t.Fatal("删除应写入回收站记录（需求 §11.1）")
	}
	// 到期时刻按配置的保留天数计算（需求 §11.3 默认 14 天）。
	// 删除时刻由数据库取当前时间，到期时刻由进程时钟算出，两者相差毫秒级，
	// 因此按天数四舍五入后再比较。
	days := h.scalar(t, `SELECT round(extract(epoch FROM (expires_at - deleted_at)) / 86400)::int
		FROM recycle_bin WHERE object_type = 'prompt' AND object_id = $1`, promptID)
	if days != domain.RecycleRetentionDays {
		t.Fatalf("保留天数 = %d，期望 %d", days, domain.RecycleRetentionDays)
	}
	if state := h.scalarString(t, `SELECT state FROM recommend_tags WHERE id = $1`, tagID); state != "expired" {
		t.Fatalf("推荐标签状态 = %s，期望 expired（C-14）", state)
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'prompt_delete' AND object_id = $1`,
		promptID) != 1 {
		t.Fatal("删除应写入 prompt_delete 操作日志")
	}
}

func TestAdminRestorePromptReturnsItToLibrary(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "restore@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "待恢复提示词")
	if err := h.prompt.Delete(ctx, operator, promptID, ""); err != nil {
		t.Fatalf("删除提示词失败: %v", err)
	}

	// 管理端的回收站模块复用提示词域的恢复路径。
	if err := h.service.RestoreRecycleEntry(ctx, operator, "prompt", promptID, ""); err != nil {
		t.Fatalf("恢复提示词失败: %v", err)
	}
	if h.scalar(t, `SELECT count(*) FROM prompts WHERE id = $1 AND deleted_at IS NULL`, promptID) != 1 {
		t.Fatal("恢复后提示词应重新可见")
	}
	if h.scalar(t, `SELECT count(*) FROM recycle_bin
		WHERE object_type = 'prompt' AND object_id = $1 AND restored_at IS NOT NULL`, promptID) != 1 {
		t.Fatal("恢复应结清回收站记录")
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'prompt_restore'`) != 1 {
		t.Fatal("恢复应写入操作日志")
	}
}

func TestAdminPurgePromptRejectsLibraryReference(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "purge@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "被题库引用的提示词")
	if err := h.prompt.Delete(ctx, operator, promptID, ""); err != nil {
		t.Fatalf("删除提示词失败: %v", err)
	}

	libraryID := h.seedLibraryWithReference(t, ownerID, promptID)

	err := h.service.PurgeRecycleEntry(ctx, operator, "prompt", promptID, "")
	expectCode(t, err, apperr.CodePromptInUse)

	// 解除引用后可以彻底删除。
	if _, err := h.pool.Exec(ctx,
		`DELETE FROM library_questions WHERE library_id = $1`, libraryID); err != nil {
		t.Fatalf("解除引用失败: %v", err)
	}
	if err := h.service.PurgeRecycleEntry(ctx, operator, "prompt", promptID, ""); err != nil {
		t.Fatalf("解除引用后彻底删除失败: %v", err)
	}
	if h.scalar(t, `SELECT count(*) FROM prompts WHERE id = $1`, promptID) != 0 {
		t.Fatal("彻底删除后提示词行应消失")
	}
	if h.scalar(t, `SELECT count(*) FROM recycle_bin
		WHERE object_type = 'prompt' AND object_id = $1 AND purged_at IS NOT NULL`, promptID) != 1 {
		t.Fatal("彻底删除应结清回收站记录")
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'prompt_purge'`) != 1 {
		t.Fatal("彻底删除应写入操作日志")
	}
}

func TestForceMajorMarkAppliesEveryStepOfMajorModification(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "major@example.com", Verified: true})
	raterID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "rater@example.com", Verified: true})
	promptID := pgtest.SeedPromptWith(t, h.pool, pgtest.PromptOptions{
		UploaderID: ownerID, Title: "将被强制标记的提示词", Body: "原始正文",
	})
	modelID := h.seedModel(t, "强制标记模型", "major-model", "major-provider")
	h.seedRating(t, raterID, promptID, modelID, 4)
	confirmed := h.seedRecommendTag(t, promptID, modelID, "saturation", "confirmed")
	pending := h.seedRecommendTag(t, promptID, modelID, "obsolete", "pending")

	before := h.scalarString(t, `SELECT latest_version_no FROM prompts WHERE id = $1`, promptID)
	if before != "v1.0" {
		t.Fatalf("初始版本 = %s，期望 v1.0", before)
	}

	result, err := h.service.ForceMajorMark(ctx, operator, promptID, "管理员强制标记", "10.0.0.5")
	if err != nil {
		t.Fatalf("强制标记重大修改失败: %v", err)
	}

	// 第 1、2 步：生成新的主版本并成为前台展示版本。
	if result.VersionNo != "v2.0" {
		t.Fatalf("新版本 = %s，期望 v2.0", result.VersionNo)
	}
	if h.scalarString(t, `SELECT latest_version_no FROM prompts WHERE id = $1`, promptID) != "v2.0" {
		t.Fatal("提示词应切换到新版本")
	}
	if h.scalar(t, `SELECT count(*) FROM prompt_versions WHERE prompt_id = $1 AND version_no = 'v2.0' AND is_major`, promptID) != 1 {
		t.Fatal("新版本应标记为重大修改")
	}
	// 第 3、4 步：当前评分清空，历史评分归档。
	if h.scalar(t, `SELECT count(*) FROM ratings WHERE prompt_id = $1 AND deleted_at IS NULL`, promptID) != 0 {
		t.Fatal("重大修改后当前评分应清空")
	}
	if h.scalar(t, `SELECT count(*) FROM prompt_version_ratings WHERE prompt_id = $1`, promptID) != 1 {
		t.Fatal("历史评分应归档到 prompt_version_ratings")
	}
	// 第 5 步：前台提示该提示词已发生重大修改。
	if h.scalar(t, `SELECT count(*) FROM prompts WHERE id = $1 AND major_modified`, promptID) != 1 {
		t.Fatal("提示词应标记为已发生重大修改")
	}
	// 第 6 步：统计快照清空，等待重算。
	if h.scalar(t, `SELECT count(*) FROM prompt_model_stats WHERE prompt_id = $1`, promptID) != 0 {
		t.Fatal("重大修改后应清空统计快照")
	}
	// 第 7 步、C-12：过时与训练饱和标签都转待复核。
	for _, id := range []int64{confirmed, pending} {
		if state := h.scalarString(t, `SELECT state FROM recommend_tags WHERE id = $1`, id); state != "review" {
			t.Fatalf("推荐标签 %d 状态 = %s，期望 review（C-12）", id, state)
		}
	}
	// 内容不变：这是标记而不是编辑。
	if body := h.scalarString(t, `SELECT body FROM prompts WHERE id = $1`, promptID); body != "原始正文" {
		t.Fatalf("正文 = %q，强制标记不应改动内容", body)
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'prompt_major_mark' AND object_id = $1`,
		promptID) != 1 {
		t.Fatal("强制标记应写入 prompt_major_mark 操作日志")
	}
}

func TestForceMajorMarkRejectsRecycledPrompt(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "recycled@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "回收站中的提示词")
	if err := h.prompt.Delete(ctx, operator, promptID, ""); err != nil {
		t.Fatalf("删除提示词失败: %v", err)
	}

	_, err := h.service.ForceMajorMark(ctx, operator, promptID, "", "")
	expectCode(t, err, apperr.CodeInvalidState)
}

// ---------- 推荐标签处置 ----------

func TestReviewRecommendTagsConfirmsAndIgnores(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "tags@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "带推荐标签的提示词")
	confirmedID := h.seedRecommendTag(t, promptID, 0, "obsolete", "pending")
	ignoredID := h.seedRecommendTag(t, promptID, 0, "suspected_saturation", "review")

	if err := h.service.ReviewRecommendTags(ctx, operator, []int64{confirmedID}, admin.RecommendTagReview{
		State: domain.RecommendConfirmed, LabelZh: "已确认文案",
	}, ""); err != nil {
		t.Fatalf("确认推荐标签失败: %v", err)
	}
	if err := h.service.ReviewRecommendTags(ctx, operator, []int64{ignoredID}, admin.RecommendTagReview{
		State: domain.RecommendIgnored,
	}, ""); err != nil {
		t.Fatalf("忽略推荐标签失败: %v", err)
	}

	if state := h.scalarString(t, `SELECT state FROM recommend_tags WHERE id = $1`, confirmedID); state != "confirmed" {
		t.Fatalf("状态 = %s，期望 confirmed", state)
	}
	if label := h.scalarString(t, `SELECT label_zh FROM recommend_tags WHERE id = $1`, confirmedID); label != "已确认文案" {
		t.Fatalf("文案 = %s，期望被改写", label)
	}
	if state := h.scalarString(t, `SELECT state FROM recommend_tags WHERE id = $1`, ignoredID); state != "ignored" {
		t.Fatalf("状态 = %s，期望 ignored", state)
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'recommend_tag_confirm'`) != 1 {
		t.Fatal("确认应写入操作日志")
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'recommend_tag_ignore'`) != 1 {
		t.Fatal("忽略应写入操作日志")
	}

	// 已处置的标签不可再次处置：终态不参与流转（C-13）。
	err := h.service.ReviewRecommendTags(ctx, operator, []int64{confirmedID}, admin.RecommendTagReview{
		State: domain.RecommendIgnored,
	}, "")
	expectCode(t, err, apperr.CodeInvalidState)
}

func TestReviewRecommendTagsRejectsUnknownStateAndEmptyList(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	err := h.service.ReviewRecommendTags(ctx, operator, []int64{1}, admin.RecommendTagReview{
		State: domain.RecommendPending,
	}, "")
	expectCode(t, err, apperr.CodeValidationFailed)

	err = h.service.ReviewRecommendTags(ctx, operator, nil, admin.RecommendTagReview{
		State: domain.RecommendConfirmed,
	}, "")
	expectCode(t, err, apperr.CodeValidationFailed)
}
