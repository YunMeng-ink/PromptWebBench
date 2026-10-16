package prompt

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// auditRows 读取指定动作的审计记录。
func auditRows(t *testing.T, f *fixture, action string) []repo.OperationLog {
	t.Helper()
	rows, err := repo.New(f.pool).AdminListOperationLogs(context.Background(), repo.AdminListOperationLogsParams{
		Action:     action,
		LimitCount: 10,
	})
	if err != nil {
		t.Fatalf("读取审计日志失败: %v", err)
	}
	return rows
}

// Test高风险内容访问写入审计日志 验证 G-12：高风险类别内容的每次访问都留痕，
// 授予与拒绝两种结果分别记录。
func Test高风险内容访问写入审计日志(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dev := f.developer(t)
	normal := f.normalUser(t)

	created, err := f.svc.Create(ctx, dev, baseInput(f.categoryID(t, "jailbreak"), domain.VisibilityPublic))
	if err != nil {
		t.Fatalf("创建高风险提示词失败: %v", err)
	}
	promptID := created.Prompt.ID

	// 开发者读取：授予，记录 granted。
	if _, err := f.svc.Get(ctx, domain.ViewerFor(dev), promptID, "10.0.0.1"); err != nil {
		t.Fatalf("开发者读取高风险提示词失败: %v", err)
	}
	// 普通用户读取：拒绝，记录 denied。
	if _, err := f.svc.Get(ctx, domain.ViewerFor(normal), promptID, "10.0.0.2"); !isNotFound(err) {
		t.Fatalf("普通用户读取高风险提示词应 404，实际 %v", err)
	}

	rows := auditRows(t, f, "sensitive_content_access")
	if len(rows) != 2 {
		t.Fatalf("审计记录 = %d 条，期望 2 条", len(rows))
	}

	// 列表按时间倒序：第一条是普通用户的拒绝记录，第二条是开发者的授予记录。
	assertAuditRow(t, rows[0], promptID, "denied", usernameOf(t, f, normal.ID))
	assertAuditRow(t, rows[1], promptID, "granted", usernameOf(t, f, dev.ID))
}

// usernameOf 从数据库读取账户昵称。测试里手工构造的 domain.User 不带昵称，
// 而审计记录存的是数据库中的昵称。
func usernameOf(t *testing.T, f *fixture, userID int64) string {
	t.Helper()
	var name string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT username FROM users WHERE id = $1`, userID).Scan(&name); err != nil {
		t.Fatalf("读取账户 %d 昵称失败: %v", userID, err)
	}
	return name
}

// Test非高风险内容不写审计 确认审计只针对高风险类别内容（G-12），
// 普通内容的访问不产生记录。
func Test非高风险内容不写审计(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	user := f.normalUser(t)

	created, err := f.svc.Create(ctx, user, baseInput(f.categoryID(t, "reasoning"), domain.VisibilityPublic))
	if err != nil {
		t.Fatalf("创建普通提示词失败: %v", err)
	}
	if _, err := f.svc.Get(ctx, domain.ViewerFor(user), created.Prompt.ID, "10.0.0.3"); err != nil {
		t.Fatalf("读取普通提示词失败: %v", err)
	}

	if rows := auditRows(t, f, "sensitive_content_access"); len(rows) != 0 {
		t.Fatalf("普通内容产生了 %d 条审计记录，期望 0 条", len(rows))
	}
}

// assertAuditRow 断言一条审计记录的要素。
func assertAuditRow(t *testing.T, row repo.OperationLog, promptID int64, wantResult, wantOperator string) {
	t.Helper()
	if !row.ObjectID.Valid || row.ObjectID.Int64 != promptID {
		t.Fatalf("审计记录对象 = %v，期望提示词 %d", row.ObjectID, promptID)
	}
	if row.OperatorName != wantOperator {
		t.Fatalf("审计记录操作人 = %q，期望 %q", row.OperatorName, wantOperator)
	}
	var payload struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(row.AfterData, &payload); err != nil {
		t.Fatalf("解析审计记录快照失败: %v", err)
	}
	if payload.Result != wantResult {
		t.Fatalf("审计记录结果 = %q，期望 %q", payload.Result, wantResult)
	}
}

// isNotFound 判定错误是否为 not_found 码。
func isNotFound(err error) bool {
	return err != nil && err.Error() == "对象不存在"
}
