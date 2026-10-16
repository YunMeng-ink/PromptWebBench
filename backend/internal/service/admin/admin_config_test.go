package admin_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/oplog"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/security"
	"github.com/promptbench/backend/internal/service/admin"
)

// ---------- 模块 11：系统设置 ----------

func TestListSettingsCoversDeclaredKeysAndShowsSources(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()

	items, err := h.service.ListSettings(ctx)
	if err != nil {
		t.Fatalf("读取配置项失败: %v", err)
	}
	if len(items) != len(domain.ConfigSpecs()) {
		t.Fatalf("配置项 = %d 项，期望 %d 项", len(items), len(domain.ConfigSpecs()))
	}

	// 种子数据里的 26 项参数来源是 config 表；SMTP 分组由环境变量兜底。
	byKey := make(map[string]admin.ConfigValue, len(items))
	for _, item := range items {
		byKey[item.Key] = item
	}
	bayes, ok := byKey["bayes_prior"]
	if !ok || bayes.Source != "config" || bayes.Effective == nil {
		t.Fatalf("bayes_prior = %+v，期望来源为 config 且有生效取值", bayes)
	}
	port, ok := byKey["smtp_port"]
	if !ok || port.Source != "env" {
		t.Fatalf("smtp_port = %+v，期望来源为环境变量", port)
	}
	password, ok := byKey["smtp_password"]
	if !ok || !password.Secret {
		t.Fatalf("smtp_password = %+v，期望是密钥项", password)
	}
	if password.Value != nil {
		t.Fatal("密钥项不应回显取值")
	}
}

func TestSaveSettingValidatesAndPersists(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	if err := h.service.SaveSetting(ctx, operator, "bayes_prior", json.RawMessage(`4.2`), ""); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}
	value, err := h.config.GetFloat(ctx, "bayes_prior")
	if err != nil || value != 4.2 {
		t.Fatalf("生效取值 = %v（%v），期望 4.2", value, err)
	}

	// 越界、非整数、枚举外取值与未登记键一律拒绝。
	expectCode(t, h.service.SaveSetting(ctx, operator, "bayes_prior", json.RawMessage(`99`), ""), apperr.CodeValidationFailed)
	expectCode(t, h.service.SaveSetting(ctx, operator, "recycle_retention_days", json.RawMessage(`1.5`), ""), apperr.CodeValidationFailed)
	expectCode(t, h.service.SaveSetting(ctx, operator, "registration_policy", json.RawMessage(`"nope"`), ""), apperr.CodeValidationFailed)
	expectCode(t, h.service.SaveSetting(ctx, operator, "email_domain_whitelist", json.RawMessage(`["not a domain"]`), ""), apperr.CodeValidationFailed)
	expectCode(t, h.service.SaveSetting(ctx, operator, "not_registered", json.RawMessage(`1`), ""), apperr.CodeValidationFailed)

	// 合法取值：枚举、白名单与布尔。
	if err := h.service.SaveSetting(ctx, operator, "registration_policy", json.RawMessage(`"invite_only"`), ""); err != nil {
		t.Fatalf("保存注册策略失败: %v", err)
	}
	policy, err := h.service.RegistrationPolicy(ctx)
	if err != nil || policy != domain.RegistrationInviteOnly {
		t.Fatalf("注册策略 = %s（%v），期望 invite_only", policy, err)
	}
	if err := h.service.SaveSetting(ctx, operator, "email_domain_whitelist", json.RawMessage(`["@example.com","other.test"]`), ""); err != nil {
		t.Fatalf("保存白名单失败: %v", err)
	}
	whitelist, err := h.config.GetStringSlice(ctx, "email_domain_whitelist")
	if err != nil || len(whitelist) != 2 || whitelist[0] != "example.com" {
		t.Fatalf("白名单 = %v（%v），期望去掉前导 @", whitelist, err)
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'config_update'`) != 3 {
		t.Fatal("成功保存的配置项都应写入 config_update 操作日志")
	}
}

func TestSaveSecretSettingEncryptsAndRequiresMasterKey(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	if err := h.service.SaveSetting(ctx, operator, "smtp_password", json.RawMessage(`"s3cret-value"`), ""); err != nil {
		t.Fatalf("保存密钥配置失败: %v", err)
	}
	var (
		isSecret bool
		cipher   string
	)
	if err := h.pool.QueryRow(ctx,
		`SELECT is_secret, secret_ciphertext FROM config WHERE key = 'smtp_password'`).
		Scan(&isSecret, &cipher); err != nil {
		t.Fatalf("读取密钥配置失败: %v", err)
	}
	if !isSecret {
		t.Fatal("密钥配置项应标记 is_secret")
	}
	if strings.Contains(cipher, "s3cret-value") {
		t.Fatal("密文中不应出现明文")
	}
	plain, err := security.NewSecretBox(testMasterKey).Open(cipher)
	if err != nil || plain != "s3cret-value" {
		t.Fatalf("解密结果 = %q（%v），期望还原明文", plain, err)
	}
	// 日志只记录「密钥被更新」，不记录取值。
	var after string
	if err := h.pool.QueryRow(ctx,
		`SELECT after_data::text FROM operation_logs WHERE action = 'config_update' AND object_type = 'config'`).
		Scan(&after); err != nil {
		t.Fatalf("读取操作日志失败: %v", err)
	}
	if strings.Contains(after, "s3cret-value") {
		t.Fatal("操作日志不应包含密钥明文")
	}

	// 未配置主密钥时拒绝保存密钥类配置项。
	noKey := admin.New(admin.Deps{Pool: h.pool, Config: h.config, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	expectCode(t,
		noKey.SaveSetting(ctx, operator, "smtp_password", json.RawMessage(`"another"`), ""),
		apperr.CodeSecretUnavailable)
}

func TestSendTestEmailUsesEffectiveSMTPConfig(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	if err := h.service.SendTestEmail(ctx, operator, "admin@example.com", "10.0.0.7"); err != nil {
		t.Fatalf("发送测试邮件失败: %v", err)
	}
	messages := h.mailer.Messages()
	if len(messages) != 1 || messages[0].To[0] != "admin@example.com" {
		t.Fatalf("收到的邮件 = %+v，期望发往 admin@example.com", messages)
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'email_test'`) != 1 {
		t.Fatal("测试邮件应写入 email_test 操作日志")
	}

	expectCode(t, h.service.SendTestEmail(ctx, operator, "not-an-address", ""), apperr.CodeValidationFailed)
}

// ---------- 模块 7：注册与邀请码管理 ----------

func TestCreateInviteCodesBatch(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	codes, err := h.service.CreateInviteCodes(ctx, operator, admin.InviteInput{
		Kind: "developer", Count: 3, MaxUses: 2, ExpiresInDays: 7, Note: "批量生成",
	}, "")
	if err != nil {
		t.Fatalf("生成邀请码失败: %v", err)
	}
	if len(codes) != 3 {
		t.Fatalf("生成数量 = %d，期望 3", len(codes))
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code.Code] {
			t.Fatalf("邀请码 %s 重复生成", code.Code)
		}
		seen[code.Code] = true
		if code.Kind != "developer" || code.MaxUses != 2 || !code.ExpiresAt.Valid {
			t.Fatalf("邀请码 = %+v，字段与输入不一致", code)
		}
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'invite_code_generate'`) != 1 {
		t.Fatal("生成邀请码应写入操作日志")
	}

	list, err := h.service.ListInviteCodes(ctx, "developer", "", 1, 2)
	if err != nil {
		t.Fatalf("查询邀请码失败: %v", err)
	}
	if len(list.Items) != 2 || list.Total != 3 {
		t.Fatalf("分页结果 = %d 条 / 共 %d，期望 2 条 / 共 3", len(list.Items), list.Total)
	}

	if _, err := h.service.SetInviteCodeDisabled(ctx, codes[0].ID, true); err != nil {
		t.Fatalf("禁用邀请码失败: %v", err)
	}
	disabled, err := h.service.ListInviteCodes(ctx, "", "true", 1, 20)
	if err != nil || disabled.Total != 1 {
		t.Fatalf("禁用筛选 = 共 %d 条（%v），期望 1", disabled.Total, err)
	}
}

func TestInviteInputValidation(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	_, err := h.service.CreateInviteCodes(ctx, operator, admin.InviteInput{Kind: "nope", Count: 1, MaxUses: 1}, "")
	expectCode(t, err, apperr.CodeValidationFailed)

	_, err = h.service.CreateInviteCodes(ctx, operator, admin.InviteInput{Kind: "normal", Count: 0, MaxUses: 1}, "")
	expectCode(t, err, apperr.CodeValidationFailed)

	_, err = h.service.CreateInviteCodes(ctx, operator, admin.InviteInput{
		Kind: "normal", Count: domain.InviteBatchLimit + 1, MaxUses: 1,
	}, "")
	expectCode(t, err, apperr.CodeValidationFailed)

	_, err = h.service.CreateInviteCodes(ctx, operator, admin.InviteInput{Kind: "normal", Count: 1, MaxUses: 0}, "")
	expectCode(t, err, apperr.CodeValidationFailed)
}

// ---------- 模块 9：公告管理 ----------

func TestAnnouncementPublishEditAndWithdraw(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	created, err := h.service.CreateAnnouncement(ctx, operator, admin.AnnouncementInput{
		Title: "平台公告", Body: "正文",
	}, "10.0.0.8")
	if err != nil {
		t.Fatalf("发布公告失败: %v", err)
	}
	if admin.AnnouncementState(created) != "live" {
		t.Fatalf("新建公告状态 = %s，期望 live", admin.AnnouncementState(created))
	}

	updated, err := h.service.UpdateAnnouncement(ctx, created.ID, admin.AnnouncementInput{
		Title: "平台公告（修订）", Body: "正文二",
	})
	if err != nil {
		t.Fatalf("修改公告失败: %v", err)
	}
	if updated.Title != "平台公告（修订）" {
		t.Fatalf("标题 = %s，期望被修改", updated.Title)
	}

	withdrawn, err := h.service.WithdrawAnnouncement(ctx, operator, created.ID, "")
	if err != nil {
		t.Fatalf("撤回公告失败: %v", err)
	}
	if admin.AnnouncementState(withdrawn) != "withdrawn" {
		t.Fatalf("撤回后状态 = %s，期望 withdrawn", admin.AnnouncementState(withdrawn))
	}
	// 撤回后立即从用户侧消失（C-33）：用户侧列表按未撤回过滤。
	live, err := h.service.ListAnnouncements(ctx, "live", 1, 20)
	if err != nil || live.Total != 0 {
		t.Fatalf("未撤回公告 = %d 条（%v），期望 0", live.Total, err)
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action IN ('announcement_publish','announcement_withdraw')`) != 2 {
		t.Fatal("发布与撤回都应写入操作日志")
	}
	_, err = h.service.WithdrawAnnouncement(ctx, operator, created.ID, "")
	expectCode(t, err, apperr.CodeInvalidState)
}

func TestAnnouncementValidationRejectsEmptyTitle(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	_, err := h.service.CreateAnnouncement(ctx, operator, admin.AnnouncementInput{Title: "   "}, "")
	expectCode(t, err, apperr.CodeValidationFailed)
}

// ---------- 模块 6：举报管理 ----------

func TestHandleReportDeletesPromptAndBansAuthor(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	authorID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "author@example.com", Verified: true})
	reporterID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "reporter@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, authorID, "被举报的提示词")

	var reportID int64
	if err := h.pool.QueryRow(ctx, `
		INSERT INTO reports (reporter_id, object_type, object_id, reason, detail)
		VALUES ($1, 'prompt', $2, 'illegal', '细节')
		RETURNING id`, reporterID, promptID).Scan(&reportID); err != nil {
		t.Fatalf("写入举报失败: %v", err)
	}

	if err := h.service.HandleReport(ctx, operator, reportID, admin.ReportHandleInput{
		Status: domain.ReportResolved, Note: "内容违规", DeleteContent: true, BanAuthor: true,
	}, "10.0.0.9"); err != nil {
		t.Fatalf("处置举报失败: %v", err)
	}

	if h.scalar(t, `SELECT count(*) FROM prompts WHERE id = $1 AND deleted_at IS NOT NULL`, promptID) != 1 {
		t.Fatal("处置应删除被举报的提示词")
	}
	if h.scalar(t, `SELECT count(*) FROM recycle_bin WHERE object_type = 'prompt' AND object_id = $1`, promptID) != 1 {
		t.Fatal("被删除的提示词应进入回收站")
	}
	if status := h.scalarString(t, `SELECT status FROM users WHERE id = $1`, authorID); status != "banned" {
		t.Fatalf("作者状态 = %s，期望 banned", status)
	}
	if h.scalar(t, `SELECT count(*) FROM prompts WHERE id = $1 AND uploader_id != $2`, promptID, authorID) != 0 {
		t.Fatal("封禁后其提示词应转移到系统账户")
	}
	status := h.scalarString(t, `SELECT status FROM reports WHERE id = $1`, reportID)
	note := h.scalarString(t, `SELECT handle_note FROM reports WHERE id = $1`, reportID)
	if status != "resolved" || note != "内容违规" {
		t.Fatalf("举报状态 = %s / 备注 = %s，期望 resolved / 内容违规", status, note)
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'report_handle'`) != 1 {
		t.Fatal("处置举报应写入操作日志")
	}
}

func TestHandleReportCommentDeletesComment(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "cowner@example.com", Verified: true})
	commenterID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "commenter@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "带评论的提示词")

	var commentID int64
	if err := h.pool.QueryRow(ctx, `
		INSERT INTO comments (prompt_id, user_id, body) VALUES ($1, $2, '被举报的评论')
		RETURNING id`, promptID, commenterID).Scan(&commentID); err != nil {
		t.Fatalf("写入评论失败: %v", err)
	}
	var reportID int64
	if err := h.pool.QueryRow(ctx, `
		INSERT INTO reports (reporter_id, object_type, object_id, reason)
		VALUES ($1, 'comment', $2, 'attack') RETURNING id`, ownerID, commentID).Scan(&reportID); err != nil {
		t.Fatalf("写入举报失败: %v", err)
	}

	if err := h.service.HandleReport(ctx, operator, reportID, admin.ReportHandleInput{
		Status: domain.ReportIgnored, Note: "已处理", DeleteContent: true,
	}, ""); err != nil {
		t.Fatalf("处置评论举报失败: %v", err)
	}
	if h.scalar(t, `SELECT count(*) FROM comments WHERE id = $1 AND deleted_at IS NOT NULL`, commentID) != 1 {
		t.Fatal("处置应删除被举报的评论")
	}
	if h.scalar(t, `SELECT count(*) FROM operation_logs WHERE action = 'comment_delete'`) != 1 {
		t.Fatal("删除评论应写入 comment_delete 操作日志（需求 §11.2）")
	}
	// 未要求封禁作者时不动账户。
	if status := h.scalarString(t, `SELECT status FROM users WHERE id = $1`, commenterID); status != "normal" {
		t.Fatalf("评论人状态 = %s，期望 normal", status)
	}
}

// ---------- 模块 10：回收站管理 ----------

func TestRecycleBinListsAllTypesAndRefusesLibraryActions(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "recycleowner@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "进入回收站的提示词")
	if err := h.prompt.Delete(ctx, operator, promptID, ""); err != nil {
		t.Fatalf("删除提示词失败: %v", err)
	}
	libraryID := h.seedLibraryWithReference(t, ownerID, promptID)
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO recycle_bin (object_type, object_id, owner_id, snapshot, expires_at)
		VALUES ('library', $1, $2, '{"name":"被删除的题库"}', now() + interval '14 days')`,
		libraryID, ownerID); err != nil {
		t.Fatalf("写入题库回收站记录失败: %v", err)
	}

	list, err := h.service.ListRecycleBin(ctx, "", 1, 20)
	if err != nil {
		t.Fatalf("查询回收站失败: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("回收站对象 = %d 条，期望 2 条", list.Total)
	}
	titles := map[string]string{}
	for _, item := range list.Items {
		titles[item.ObjectType] = admin.RecycleEntryTitle(item.Snapshot)
	}
	if titles["prompt"] != "进入回收站的提示词" || titles["library"] != "被删除的题库" {
		t.Fatalf("回收站标题 = %v，期望从快照中取出", titles)
	}

	// 题库与题库题目由所有者在前台处置，管理端拒绝。
	err = h.service.RestoreRecycleEntry(ctx, operator, "library", libraryID, "")
	expectCode(t, err, apperr.CodePermissionDenied)
	err = h.service.PurgeRecycleEntry(ctx, operator, "library_question", 1, "")
	expectCode(t, err, apperr.CodePermissionDenied)
	err = h.service.PurgeRecycleEntry(ctx, operator, "unknown", 1, "")
	expectCode(t, err, apperr.CodeValidationFailed)
}

func TestPurgeExpiredPromptsSkipsReferencedPrompt(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "expired@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "到期但被引用的提示词")
	if err := h.prompt.Delete(ctx, operator, promptID, ""); err != nil {
		t.Fatalf("删除提示词失败: %v", err)
	}
	libraryID := h.seedLibraryWithReference(t, ownerID, promptID)

	// 把到期时刻提前，模拟保留期已过。
	if _, err := h.pool.Exec(ctx, `
		UPDATE recycle_bin SET expires_at = now() - interval '1 hour'
		WHERE object_type = 'prompt' AND object_id = $1`, promptID); err != nil {
		t.Fatalf("调整到期时刻失败: %v", err)
	}

	purged, err := h.service.PurgeExpiredPrompts(ctx, 10)
	if err != nil {
		t.Fatalf("清理到期记录失败: %v", err)
	}
	if purged != 0 {
		t.Fatalf("被引用的提示词不应被清理，实际清理 %d 条", purged)
	}
	if h.scalar(t, `SELECT count(*) FROM prompts WHERE id = $1`, promptID) != 1 {
		t.Fatal("引用未解除时提示词行应保留")
	}

	// 解除引用后，下一次清理成功。
	if _, err := h.pool.Exec(ctx, `DELETE FROM library_questions WHERE library_id = $1`, libraryID); err != nil {
		t.Fatalf("解除引用失败: %v", err)
	}
	purged, err = h.service.PurgeExpiredPrompts(ctx, 10)
	if err != nil {
		t.Fatalf("清理到期记录失败: %v", err)
	}
	if purged != 1 {
		t.Fatalf("清理条数 = %d，期望 1", purged)
	}
	if h.scalar(t, `SELECT count(*) FROM prompts WHERE id = $1`, promptID) != 0 {
		t.Fatal("清理后提示词行应消失")
	}
}

// ---------- 模块 8：操作日志 ----------

func TestOperationLogsCoverDeclaredActionsAndFilter(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	// 声明表与数据库 CHECK 约束必须一致：逐条写入，任一条不合法都会失败。
	for _, action := range oplog.AllActions() {
		if _, err := h.pool.Exec(ctx, `
			INSERT INTO operation_logs (operator_id, operator_name, action) VALUES ($1, '声明校验', $2)`,
			operator.ID, action); err != nil {
			t.Fatalf("操作类型 %s 未被数据库接受: %v", action, err)
		}
	}

	page, err := h.service.ListOperationLogs(ctx, admin.OplogFilter{Action: "user_ban"}, 1, 20)
	if err != nil {
		t.Fatalf("查询操作日志失败: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("按类型筛选 = %d 条，期望 1", page.Total)
	}

	_, err = h.service.ListOperationLogs(ctx, admin.OplogFilter{Action: "not_an_action"}, 1, 20)
	expectCode(t, err, apperr.CodeValidationFailed)

	// 结束时刻必须晚于起始时刻。
	since := time.Now().UTC()
	until := since.Add(-time.Hour)
	_, err = h.service.ListOperationLogs(ctx, admin.OplogFilter{Since: &since, Until: &until}, 1, 20)
	expectCode(t, err, apperr.CodeValidationFailed)
}

// ---------- 分页契约 ----------

func TestAdminListsPaginate(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "page" + string(rune('a'+i)) + "@example.com", Verified: true})
	}

	all, err := h.service.ListUsers(ctx, admin.UserFilter{}, 1, 100)
	if err != nil {
		t.Fatalf("查询用户列表失败: %v", err)
	}
	page, err := h.service.ListUsers(ctx, admin.UserFilter{}, 1, 2)
	if err != nil {
		t.Fatalf("查询用户列表失败: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("每页条数 = %d，期望 2", len(page.Items))
	}
	if page.Total != all.Total {
		t.Fatalf("分页总数 = %d，期望与全量一致 %d", page.Total, all.Total)
	}
	if page.Page != 1 || page.PageSize != 2 {
		t.Fatalf("分页元信息 = %d/%d，期望 1/2", page.Page, page.PageSize)
	}
}

// ---------- 只读子列表 ----------

// TestAdminReadOnlySubListsResolve 覆盖各模块的只读子列表：它们只做查询与分页，
// 但每一条都必须能在有空数据与有数据两种形态下正常工作。
func TestAdminReadOnlySubListsResolve(t *testing.T) {
	h := newFullHarness(t)
	ctx := context.Background()

	ownerID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "sublist@example.com", Verified: true})
	promptID := pgtest.SeedPrompt(t, h.pool, ownerID, "子列表提示词")
	modelID := h.seedModel(t, "子列表模型", "sublist-model", "sublist-lab")
	h.seedRating(t, ownerID, promptID, modelID, 4)

	if _, err := h.pool.Exec(ctx, `
		INSERT INTO comments (prompt_id, user_id, body) VALUES ($1, $2, '一条评论')`,
		promptID, ownerID); err != nil {
		t.Fatalf("写入评论失败: %v", err)
	}
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO reports (reporter_id, object_type, object_id, reason)
		VALUES ($1, 'prompt', $2, 'spam')`, ownerID, promptID); err != nil {
		t.Fatalf("写入举报失败: %v", err)
	}
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO libraries (owner_id, name, description) VALUES ($1, '子列表题库', '')`,
		ownerID); err != nil {
		t.Fatalf("写入题库失败: %v", err)
	}
	if _, err := h.service.ImportModels(ctx, h.operator(t), admin.ModelImportOptions{}, ""); err != nil {
		t.Fatalf("导入模型失败: %v", err)
	}

	if ratings, err := h.service.ListUserRatings(ctx, ownerID, 1, 20); err != nil || ratings.Total != 1 {
		t.Fatalf("用户评分列表 = %d 条（%v），期望 1 条", ratings.Total, err)
	}
	if comments, err := h.service.ListUserComments(ctx, ownerID, 1, 20); err != nil || comments.Total != 1 {
		t.Fatalf("用户评论列表 = %d 条（%v），期望 1 条", comments.Total, err)
	}
	if libraries, err := h.service.ListUserLibraries(ctx, ownerID, 1, 20); err != nil || libraries.Total != 1 {
		t.Fatalf("用户题库列表 = %d 条（%v），期望 1 条", libraries.Total, err)
	}
	if reports, err := h.service.ListUserReports(ctx, ownerID, 1, 20); err != nil || reports.Total != 1 {
		t.Fatalf("用户举报列表 = %d 条（%v），期望 1 条", reports.Total, err)
	}
	if empty, err := h.service.ListUserRatings(ctx, 999999, 1, 20); err == nil {
		t.Fatalf("查询不存在的账户应报错，实际返回 %+v", empty)
	}

	if prompts, err := h.service.ListModelPrompts(ctx, modelID, 1, 20); err != nil || prompts.Total != 1 {
		t.Fatalf("模型关联提示词 = %d 条（%v），期望 1 条", prompts.Total, err)
	}
	// 导入的模型各带一份原始快照；手工添加的模型没有快照。
	imported, err := h.repo.AdminGetModelByIdentity(ctx, repo.AdminGetModelByIdentityParams{
		ModelKey: "alpha-one", ProviderKey: "alpha",
	})
	if err != nil {
		t.Fatalf("查询导入模型失败: %v", err)
	}
	if snapshots, err := h.service.ListModelSnapshots(ctx, imported.ID, 1, 20); err != nil || snapshots.Total != 1 {
		t.Fatalf("模型快照 = %d 条（%v），期望 1 条", snapshots.Total, err)
	}
	if snapshots, err := h.service.ListModelSnapshots(ctx, modelID, 1, 20); err != nil || snapshots.Total != 0 {
		t.Fatalf("手工模型的快照 = %d 条（%v），期望 0 条", snapshots.Total, err)
	}
	if logs, err := h.service.ListModelImportLogs(ctx, "import", 1, 20); err != nil || logs.Total != 3 {
		t.Fatalf("导入记录 = %d 条（%v），期望 3 条", logs.Total, err)
	}

	// 有评分但尚未算过统计时，完成度列表为空而不是报错。
	stats, err := h.service.ListModelStats(ctx, modelID, 1, 20)
	if err != nil {
		t.Fatalf("模型完成度统计失败: %v", err)
	}
	if stats.Total != 0 {
		t.Fatalf("完成度条目 = %d 条，期望 0 条（快照尚未计算）", stats.Total)
	}

	// 各模块的列表都带分页元信息。
	if users, err := h.service.ListUsers(ctx, admin.UserFilter{}, 2, 2); err != nil || users.Page != 2 {
		t.Fatalf("用户列表分页 = %d（%v），期望第 2 页", users.Page, err)
	}
	if tags, err := h.service.ListTags(ctx, "", 1, 5); err != nil || tags.PageSize != 5 {
		t.Fatalf("标签列表分页 = %d（%v），期望每页 5 条", tags.PageSize, err)
	}
	if invites, err := h.service.ListInviteCodes(ctx, "", "", 1, 5); err != nil || invites.Total != 0 {
		t.Fatalf("邀请码列表 = %d 条（%v），期望 0 条", invites.Total, err)
	}
	if announcements, err := h.service.ListAnnouncements(ctx, "", 1, 5); err != nil || announcements.Total != 0 {
		t.Fatalf("公告列表 = %d 条（%v），期望 0 条", announcements.Total, err)
	}
	if reports, err := h.service.ListReports(ctx, admin.ReportFilter{Status: "pending"}, 1, 5); err != nil || reports.Total != 1 {
		t.Fatalf("举报列表 = %d 条（%v），期望 1 条", reports.Total, err)
	}
	if recommend, err := h.service.ListRecommendTags(ctx, []string{"pending"}, "", 1, 5); err != nil || recommend.Total != 0 {
		t.Fatalf("推荐标签列表 = %d 条（%v），期望 0 条", recommend.Total, err)
	}
	if recycled, err := h.service.ListRecycleBin(ctx, "", 1, 5); err != nil || recycled.Total != 0 {
		t.Fatalf("回收站列表 = %d 条（%v），期望 0 条", recycled.Total, err)
	}
}

// ---------- 路由契约 ----------

// 操作日志只增不改不删（C-43）的写路由检查在接口层完成，
// 因为路由清单只有接口层可见（见 internal/http/admin_api_test.go）。
