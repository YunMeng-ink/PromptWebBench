package admin_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/service/admin"
)

type harness struct {
	pool    *pgxpool.Pool
	service *admin.Service
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pool := pgtest.MigratedDatabase(t)
	return &harness{
		pool:    pool,
		service: admin.New(admin.Deps{Pool: pool, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}),
	}
}

func expectCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望返回错误码 %s，实际成功", code)
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("错误类型 = %T，期望 *apperr.Error（%v）", err, err)
	}
	if appErr.Code != code {
		t.Fatalf("错误码 = %s，期望 %s", appErr.Code, code)
	}
}

func (h *harness) operator(t *testing.T) domain.User {
	t.Helper()
	id := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "operator@example.com", Password: "Operator1!", Role: "admin", Verified: true,
	})
	user, err := h.service.UserByID(context.Background(), id)
	if err != nil {
		t.Fatalf("读取操作人失败: %v", err)
	}
	return user
}

func (h *harness) systemUserID(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := h.pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE is_system`).Scan(&id); err != nil {
		t.Fatalf("读取系统账户失败: %v", err)
	}
	return id
}

func (h *harness) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	return n
}

func (h *harness) user(t *testing.T, id int64) domain.User {
	t.Helper()
	user, err := h.service.UserByID(context.Background(), id)
	if err != nil {
		t.Fatalf("读取账户失败: %v", err)
	}
	return user
}

func TestBanTransfersPromptsToSystemAccount(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	targetID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "spammer@example.com", Password: "Spammer1!", Verified: true,
	})
	first := pgtest.SeedPrompt(t, h.pool, targetID, "被转移的提示词一")
	second := pgtest.SeedPrompt(t, h.pool, targetID, "被转移的提示词二")
	versionBefore := h.user(t, targetID).TokenVersion

	result, err := h.service.BanUser(ctx, operator, targetID, "垃圾广告", "10.0.0.9")
	if err != nil {
		t.Fatalf("封禁失败: %v", err)
	}
	if result.User.Status != domain.StatusBanned {
		t.Fatalf("状态 = %s，期望 banned", result.User.Status)
	}
	if result.TransferredPrompts != 2 {
		t.Fatalf("转移提示词数 = %d，期望 2", result.TransferredPrompts)
	}
	if h.user(t, targetID).TokenVersion <= versionBefore {
		t.Fatal("封禁应提升令牌版本使旧会话失效（C-24）")
	}

	systemID := h.systemUserID(t)
	for _, promptID := range []int64{first, second} {
		var (
			uploader         int64
			originalUploader int64
		)
		if err := h.pool.QueryRow(ctx,
			`SELECT uploader_id, original_uploader_id FROM prompts WHERE id = $1`, promptID).
			Scan(&uploader, &originalUploader); err != nil {
			t.Fatalf("读取提示词失败: %v", err)
		}
		if uploader != systemID {
			t.Fatalf("提示词 %d 的上传者 = %d，期望系统账户 %d", promptID, uploader, systemID)
		}
		if originalUploader != targetID {
			t.Fatalf("提示词 %d 的原上传者 = %d，期望 %d（后台可追溯，C-25）", promptID, originalUploader, targetID)
		}
	}

	if h.count(t, `SELECT count(*) FROM transfer_logs WHERE from_user_id = $1 AND to_user_id = $2`, targetID, systemID) != 2 {
		t.Fatal("每条转移都应留下转移日志")
	}
	if h.count(t, `SELECT count(*) FROM operation_logs WHERE action = 'user_ban'`) != 1 {
		t.Fatal("封禁应写入操作日志")
	}
	if h.count(t, `SELECT count(*) FROM operation_logs WHERE action = 'user_transfer_prompts'`) != 1 {
		t.Fatal("提示词转移应写入操作日志")
	}
}

func TestBanKeepsPromptVisibilityAndDerivedData(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	targetID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "private@example.com", Password: "Private1!", Verified: true,
	})
	promptID := pgtest.SeedPrompt(t, h.pool, targetID, "私有提示词")
	if _, err := h.pool.Exec(ctx, `UPDATE prompts SET visibility = 'private' WHERE id = $1`, promptID); err != nil {
		t.Fatalf("设置可见性失败: %v", err)
	}

	if _, err := h.service.BanUser(ctx, operator, targetID, "", "10.0.0.9"); err != nil {
		t.Fatalf("封禁失败: %v", err)
	}

	var visibility string
	if err := h.pool.QueryRow(ctx, `SELECT visibility FROM prompts WHERE id = $1`, promptID).Scan(&visibility); err != nil {
		t.Fatalf("读取可见性失败: %v", err)
	}
	if visibility != "private" {
		t.Fatalf("可见性 = %s，封禁不应改变公开状态（需求 §15.2）", visibility)
	}
}

func TestBanRejectsSelfSystemAndAlreadyBanned(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	_, err := h.service.BanUser(ctx, operator, operator.ID, "", "")
	expectCode(t, err, apperr.CodeInvalidState)

	_, err = h.service.BanUser(ctx, operator, h.systemUserID(t), "", "")
	expectCode(t, err, apperr.CodeInvalidState)

	targetID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "twice@example.com", Verified: true})
	if _, err := h.service.BanUser(ctx, operator, targetID, "", ""); err != nil {
		t.Fatalf("首次封禁失败: %v", err)
	}
	_, err = h.service.BanUser(ctx, operator, targetID, "", "")
	expectCode(t, err, apperr.CodeInvalidState)
}

func TestBanUnknownUserReportsNotFound(t *testing.T) {
	h := newHarness(t)
	operator := h.operator(t)

	_, err := h.service.BanUser(context.Background(), operator, 999999, "", "")
	expectCode(t, err, apperr.CodeNotFound)
}

func TestUnbanRestoresStatusWithoutReturningPrompts(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	verifiedID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "verified@example.com", Password: "Verified1!", Verified: true,
	})
	promptID := pgtest.SeedPrompt(t, h.pool, verifiedID, "不会转回的提示词")

	if _, err := h.service.BanUser(ctx, operator, verifiedID, "", ""); err != nil {
		t.Fatalf("封禁失败: %v", err)
	}
	unbanned, err := h.service.UnbanUser(ctx, operator, verifiedID, "")
	if err != nil {
		t.Fatalf("解封失败: %v", err)
	}
	if unbanned.Status != domain.StatusNormal {
		t.Fatalf("已验证明细解封后状态 = %s，期望 normal", unbanned.Status)
	}

	systemID := h.systemUserID(t)
	var uploader int64
	if err := h.pool.QueryRow(ctx, `SELECT uploader_id FROM prompts WHERE id = $1`, promptID).Scan(&uploader); err != nil {
		t.Fatalf("读取上传者失败: %v", err)
	}
	if uploader != systemID {
		t.Fatalf("解封后提示词上传者 = %d，期望仍为系统账户 %d（C-25 不自动转回）", uploader, systemID)
	}
	if h.count(t, `SELECT count(*) FROM operation_logs WHERE action = 'user_unban'`) != 1 {
		t.Fatal("解封应写入操作日志")
	}
}

func TestUnbanUnverifiedAccountReturnsToUnverified(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	targetID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "unverified@example.com", Status: string(domain.StatusUnverified),
	})
	if _, err := h.service.BanUser(ctx, operator, targetID, "", ""); err != nil {
		t.Fatalf("封禁失败: %v", err)
	}
	unbanned, err := h.service.UnbanUser(ctx, operator, targetID, "")
	if err != nil {
		t.Fatalf("解封失败: %v", err)
	}
	if unbanned.Status != domain.StatusUnverified {
		t.Fatalf("未验证账户解封后状态 = %s，期望 unverified", unbanned.Status)
	}
}

func TestUnbanRejectsAccountThatIsNotBanned(t *testing.T) {
	h := newHarness(t)
	operator := h.operator(t)
	targetID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "normal@example.com", Verified: true})

	_, err := h.service.UnbanUser(context.Background(), operator, targetID, "")
	expectCode(t, err, apperr.CodeInvalidState)
}

func TestSetDeveloperGrantsAndRevokes(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	targetID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "member@example.com", Verified: true})

	granted, err := h.service.SetDeveloper(ctx, operator, targetID, true, "10.0.0.9")
	if err != nil {
		t.Fatalf("授予开发者身份失败: %v", err)
	}
	if granted.Role != domain.RoleDeveloper {
		t.Fatalf("身份组 = %s，期望 developer", granted.Role)
	}
	if !granted.IsDeveloper() || granted.IsAdmin() {
		t.Fatal("开发者身份判定不正确")
	}

	revoked, err := h.service.SetDeveloper(ctx, operator, targetID, false, "10.0.0.9")
	if err != nil {
		t.Fatalf("撤销开发者身份失败: %v", err)
	}
	if revoked.Role != domain.RoleUser {
		t.Fatalf("身份组 = %s，期望 user", revoked.Role)
	}

	if h.count(t, `SELECT count(*) FROM operation_logs WHERE action = 'developer_grant'`) != 1 {
		t.Fatal("授予身份应写入操作日志")
	}
	if h.count(t, `SELECT count(*) FROM operation_logs WHERE action = 'developer_revoke'`) != 1 {
		t.Fatal("撤销身份应写入操作日志")
	}
}

func TestSetDeveloperRejectsInvalidTargets(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	operator := h.operator(t)

	_, err := h.service.SetDeveloper(ctx, operator, operator.ID, true, "")
	expectCode(t, err, apperr.CodeInvalidState)

	_, err = h.service.SetDeveloper(ctx, operator, h.systemUserID(t), true, "")
	expectCode(t, err, apperr.CodeInvalidState)

	adminID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "otheradmin@example.com", Role: "admin", Verified: true,
	})
	_, err = h.service.SetDeveloper(ctx, operator, adminID, true, "")
	expectCode(t, err, apperr.CodeInvalidState)

	memberID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{Email: "plain@example.com", Verified: true})
	_, err = h.service.SetDeveloper(ctx, operator, memberID, false, "")
	expectCode(t, err, apperr.CodeInvalidState)

	bannedID := pgtest.SeedUser(t, h.pool, pgtest.UserOptions{
		Email: "bannedmember@example.com", Status: string(domain.StatusBanned),
	})
	_, err = h.service.SetDeveloper(ctx, operator, bannedID, true, "")
	expectCode(t, err, apperr.CodeInvalidState)
}
