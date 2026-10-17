package library

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/repo"
	"github.com/promptbench/backend/internal/service/social"
)

type fixture struct {
	pool   *pgxpool.Pool
	svc    *Service
	config *config.Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := pgtest.MigratedDatabase(t)
	cfgStore := config.NewStore(pool, config.DefaultCacheTTL)
	t.Cleanup(cfgStore.Close)
	if err := cfgStore.Refresh(context.Background()); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// 通知走真实的社交域服务，测试随后直接查库核对通知内容，
	// 不引入替身：替身会让「删了题却没通知」这类缺陷在测试里也照样通过。
	notifier := social.New(social.Deps{Pool: pool, Config: cfgStore, Logger: logger})
	return &fixture{
		pool:   pool,
		config: cfgStore,
		svc:    New(Deps{Pool: pool, Config: cfgStore, Logger: logger, Notifier: notifier}),
	}
}

// user 建立一个已验证的普通账户。返回标识、用户名与身份。
func (f *fixture) user(t *testing.T) (int64, string, domain.Viewer) {
	t.Helper()
	name := fmt.Sprintf("u%d", nextSeq())
	id := pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Username: name, Verified: true})
	return id, name, domain.Viewer{UserID: id, Role: domain.RoleUser, Logged: true, Writable: true}
}

// developer 建立一个已验证的开发者账户。
func (f *fixture) developer(t *testing.T) (int64, string, domain.Viewer) {
	t.Helper()
	name := fmt.Sprintf("d%d", nextSeq())
	id := pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Username: name, Role: "developer", Verified: true})
	return id, name, domain.Viewer{UserID: id, Role: domain.RoleDeveloper, Logged: true, Writable: true}
}

// prompt 建立一条提示词，指定上传者、标题、正文与可见性。
func (f *fixture) prompt(t *testing.T, uploaderID int64, title, body, visibility string) int64 {
	t.Helper()
	return pgtest.SeedPromptWith(t, f.pool, pgtest.PromptOptions{
		UploaderID: uploaderID,
		Title:      title,
		Body:       body,
		Visibility: visibility,
	})
}

// highRiskPrompt 建立一条考察目的被标为高风险的公开提示词。
func (f *fixture) highRiskPrompt(t *testing.T, uploaderID int64, title string) int64 {
	t.Helper()
	promptID := f.prompt(t, uploaderID, title, "高风险正文", "public")
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE purposes SET is_high_risk = true
		WHERE id = (SELECT purpose_id FROM prompts WHERE id = $1)`, promptID); err != nil {
		t.Fatalf("标记高风险考察目的失败: %v", err)
	}
	return promptID
}

// library 建立一个题库并返回它。
func (f *fixture) library(t *testing.T, v domain.Viewer, name string) Library {
	t.Helper()
	created, err := f.svc.CreateLibrary(context.Background(), v, LibraryInput{Name: name})
	if err != nil {
		t.Fatalf("创建题库失败: %v", err)
	}
	return created
}

// invite 邀请协作者并返回其身份。
func (f *fixture) invite(t *testing.T, owner domain.Viewer, libraryID, collaboratorID int64, username string) domain.Viewer {
	t.Helper()
	if _, err := f.svc.InviteMember(context.Background(), owner, libraryID, username); err != nil {
		t.Fatalf("邀请协作者失败: %v", err)
	}
	return domain.Viewer{UserID: collaboratorID, Role: domain.RoleUser, Logged: true, Writable: true}
}

var seqCounter struct {
	mu sync.Mutex
	n  int64
}

func nextSeq() int64 {
	seqCounter.mu.Lock()
	defer seqCounter.mu.Unlock()
	seqCounter.n++
	return seqCounter.n
}

func assertCode(t *testing.T, err error, want apperr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，实际无错误", want)
	}
	appErr, ok := err.(*apperr.Error)
	if !ok {
		t.Fatalf("期望 *apperr.Error，实际 %T: %v", err, err)
	}
	if appErr.Code != want {
		t.Fatalf("期望错误码 %s，实际 %s（消息 %q）", want, appErr.Code, appErr.Message)
	}
}

func TestLibraryQuotaIsEnforcedUnderConcurrency(t *testing.T) {
	f := newFixture(t)
	_, _, owner := f.user(t)

	const attempts = 4
	var wg sync.WaitGroup
	results := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := f.svc.CreateLibrary(context.Background(), owner, LibraryInput{
				Name: fmt.Sprintf("并发题库 %d", index),
			})
			results[index] = err
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
			continue
		}
		assertCode(t, err, apperr.CodeQuotaExceeded)
	}
	if succeeded != domain.LibraryQuotaUser {
		t.Fatalf("普通用户题库配额为 %d，实际并发创建成功 %d 个", domain.LibraryQuotaUser, succeeded)
	}

	var stored int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM libraries WHERE owner_id = $1 AND deleted_at IS NULL`,
		owner.UserID).Scan(&stored); err != nil {
		t.Fatalf("统计题库失败: %v", err)
	}
	if stored != int64(domain.LibraryQuotaUser) {
		t.Fatalf("库中题库数量为 %d，期望 %d", stored, domain.LibraryQuotaUser)
	}
}

func TestDeveloperCanCreateThreeLibraries(t *testing.T) {
	f := newFixture(t)
	_, _, owner := f.developer(t)

	for i := 0; i < domain.LibraryQuotaDeveloper; i++ {
		f.library(t, owner, fmt.Sprintf("开发者题库 %d", i))
	}
	_, err := f.svc.CreateLibrary(context.Background(), owner, LibraryInput{Name: "超出配额"})
	assertCode(t, err, apperr.CodeQuotaExceeded)
}

func TestQuestionCapacityIsEnforcedUnderConcurrency(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _, owner := f.developer(t)
	lib := f.library(t, owner, "容量题库")

	// 把容量降到 3：判定逻辑与 150 相同，测试不必插入 150 条数据。
	if err := f.config.Set(ctx, "library_question_limit", 3, "单个私有题库题目上限", 0); err != nil {
		t.Fatalf("设置题目容量失败: %v", err)
	}

	const attempts = 6
	prompts := make([]int64, attempts)
	for i := 0; i < attempts; i++ {
		prompts[i] = f.prompt(t, owner.UserID, fmt.Sprintf("提示词 %d", i), "正文", "public")
	}

	var wg sync.WaitGroup
	results := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &prompts[index]})
			results[index] = err
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
			continue
		}
		assertCode(t, err, apperr.CodeQuotaExceeded)
	}
	if succeeded != 3 {
		t.Fatalf("容量为 3 时并发添加成功 %d 条", succeeded)
	}

	var stored int32
	if err := f.pool.QueryRow(ctx,
		`SELECT question_count FROM libraries WHERE id = $1`, lib.Entity.ID).Scan(&stored); err != nil {
		t.Fatalf("读取题目计数失败: %v", err)
	}
	if stored != 3 {
		t.Fatalf("题库题目计数为 %d，期望 3", stored)
	}
	var live int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM library_questions WHERE library_id = $1 AND deleted_at IS NULL`,
		lib.Entity.ID).Scan(&live); err != nil {
		t.Fatalf("统计题目失败: %v", err)
	}
	if live != 3 {
		t.Fatalf("库中题目为 %d 条，期望 3 条", live)
	}
}

// TestCollaboratorCannotReadDeveloperVisibleReferenceQuestion 是开发方案 §6.6 的专项回归。
//
// 假设题目查询只校验成员身份而不复查来源提示词可见性，普通用户协作者就能读到
// 开发者专属内容的全文，导出接口同样泄漏。这里构造「开发者把开发者可见提示词
// 加入题库并邀请普通用户协作」的场景，断言读取 404、列表为空、导出不含该正文。
func TestCollaboratorCannotReadDeveloperVisibleReferenceQuestion(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _, owner := f.developer(t)
	_, collaboratorName, collaborator := f.user(t)

	lib := f.library(t, owner, "开发者内容题库")
	secret := f.prompt(t, owner.UserID, "开发者专属提示词", "开发者专属正文内容", "developer")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &secret})
	if err != nil {
		t.Fatalf("所有者添加引用题失败: %v", err)
	}
	visible := f.prompt(t, owner.UserID, "公开提示词", "公开正文内容", "public")
	if _, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &visible}); err != nil {
		t.Fatalf("所有者添加公开引用题失败: %v", err)
	}
	collaborator = f.invite(t, owner, lib.Entity.ID, collaborator.UserID, collaboratorName)

	_, err = f.svc.GetQuestion(ctx, collaborator, question.ID)
	assertCode(t, err, apperr.CodeNotFound)

	page, err := f.svc.ListQuestions(ctx, collaborator, lib.Entity.ID, 1, 50)
	if err != nil {
		t.Fatalf("协作者读取题目列表失败: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("协作者可见题目为 %d 条，期望 1 条（只应看到公开提示词那条）", page.Total)
	}
	for _, item := range page.Items {
		if item.ID == question.ID {
			t.Fatalf("协作者读到了开发者可见的引用题 %d", item.ID)
		}
	}

	export, err := f.svc.ExportLibrary(ctx, collaborator, lib.Entity.ID)
	if err != nil {
		t.Fatalf("协作者导出题库失败: %v", err)
	}
	if strings.Contains(export.Content, "开发者专属正文内容") {
		t.Fatalf("导出内容泄漏了开发者专属正文：\n%s", export.Content)
	}

	// 所有者是开发者，自己应当看得到。
	ownerView, err := f.svc.GetQuestion(ctx, owner, question.ID)
	if err != nil {
		t.Fatalf("所有者读取自己的引用题失败: %v", err)
	}
	if ownerView.Body != "开发者专属正文内容" {
		t.Fatalf("所有者读到的正文为 %q", ownerView.Body)
	}

	// 协作者也不能把开发者可见的提示词加进题库。
	_, err = f.svc.AddQuestion(ctx, collaborator, lib.Entity.ID, AddQuestionInput{PromptID: &secret})
	assertCode(t, err, apperr.CodeNotFound)
}

// TestCollaboratorCannotReadHighRiskReferenceQuestion 是 C-20 的题库侧回归：
// 高风险考察目的内容即使可见性为 public，对普通用户一律不可见。
func TestCollaboratorCannotReadHighRiskReferenceQuestion(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _, owner := f.developer(t)
	_, collaboratorName, collaborator := f.user(t)

	lib := f.library(t, owner, "高风险题库")
	risky := f.highRiskPrompt(t, owner.UserID, "高风险提示词")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &risky})
	if err != nil {
		t.Fatalf("所有者添加高风险引用题失败: %v", err)
	}
	collaborator = f.invite(t, owner, lib.Entity.ID, collaborator.UserID, collaboratorName)

	_, err = f.svc.GetQuestion(ctx, collaborator, question.ID)
	assertCode(t, err, apperr.CodeNotFound)

	page, err := f.svc.ListQuestions(ctx, collaborator, lib.Entity.ID, 1, 50)
	if err != nil {
		t.Fatalf("协作者读取题目列表失败: %v", err)
	}
	if page.Total != 0 {
		t.Fatalf("协作者可见高风险题目 %d 条，期望 0 条", page.Total)
	}
	export, err := f.svc.ExportLibrary(ctx, collaborator, lib.Entity.ID)
	if err != nil {
		t.Fatalf("协作者导出题库失败: %v", err)
	}
	if strings.Contains(export.Content, "高风险正文") {
		t.Fatalf("导出内容泄漏了高风险正文：\n%s", export.Content)
	}
}

// TestNonMemberCannotReadLibraryOrQuestions 验证非成员一律 404。
func TestNonMemberCannotReadLibraryOrQuestions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _, owner := f.developer(t)
	_, _, outsider := f.user(t)

	lib := f.library(t, owner, "私有题库")
	promptID := f.prompt(t, owner.UserID, "公开提示词", "正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}

	_, err = f.svc.GetLibrary(ctx, outsider, lib.Entity.ID)
	assertCode(t, err, apperr.CodeNotFound)
	_, err = f.svc.GetQuestion(ctx, outsider, question.ID)
	assertCode(t, err, apperr.CodeNotFound)
	_, err = f.svc.ListQuestions(ctx, outsider, lib.Entity.ID, 1, 20)
	assertCode(t, err, apperr.CodeNotFound)
	_, err = f.svc.ExportLibrary(ctx, outsider, lib.Entity.ID)
	assertCode(t, err, apperr.CodeNotFound)
}

// TestCollaboratorCapabilityListsMatchSpec 逐条核对需求 §10.5 的两份清单（C-46）。
func TestCollaboratorCapabilityListsMatchSpec(t *testing.T) {
	can := domain.CollaboratorCapabilities()
	forbidden := domain.CollaboratorForbiddenCapabilities()
	if len(can) != 11 {
		t.Fatalf("协作者可做的能力为 %d 项，期望 11 项：%v", len(can), can)
	}
	if len(forbidden) != 7 {
		t.Fatalf("协作者不可做的能力为 %d 项，期望 7 项：%v", len(forbidden), forbidden)
	}

	collaborator := domain.LibraryActor{
		Viewer:   domain.Viewer{UserID: 2, Role: domain.RoleUser, Logged: true, Writable: true},
		IsMember: true,
		Role:     domain.LibraryRoleCollaborator,
	}
	for _, capability := range can {
		if !collaborator.Can(capability) {
			t.Fatalf("协作者应当可以做 %s", capability)
		}
	}
	for _, capability := range forbidden {
		if collaborator.Can(capability) {
			t.Fatalf("协作者不应当可以做 %s", capability)
		}
	}

	owner := domain.LibraryActor{
		Viewer:   domain.Viewer{UserID: 1, Role: domain.RoleUser, Logged: true, Writable: true},
		IsMember: true,
		Role:     domain.LibraryRoleOwner,
	}
	for _, capability := range domain.AllLibraryCapabilities() {
		if !owner.Can(capability) {
			t.Fatalf("所有者应当可以做 %s", capability)
		}
	}
}

// TestCollaboratorPermissionBoundaryOverService 用真实调用验证角色边界：
// 可删除任何题目，但改名称、删题库、邀请与移除成员、彻底删除均被拒绝。
func TestCollaboratorPermissionBoundaryOverService(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	_, collaboratorName, collaborator := f.user(t)
	_, otherName, other := f.user(t)

	lib := f.library(t, owner, "协作题库")
	promptID := f.prompt(t, ownerID, "公开提示词", "正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}
	collaborator = f.invite(t, owner, lib.Entity.ID, collaborator.UserID, collaboratorName)

	// 可做：改描述、加题目、贴标签、排序、导出、恢复。
	description := "协作者修改的描述"
	if _, err := f.svc.UpdateLibrary(ctx, collaborator, lib.Entity.ID, LibraryPatch{Description: &description}); err != nil {
		t.Fatalf("协作者修改描述失败: %v", err)
	}
	second, err := f.svc.AddQuestion(ctx, collaborator, lib.Entity.ID, AddQuestionInput{
		Title: "派生题目", Body: "派生正文",
	})
	if err != nil {
		t.Fatalf("协作者添加派生题失败: %v", err)
	}
	if err := f.svc.AddTag(ctx, collaborator, second.ID, "协作标签"); err != nil {
		t.Fatalf("协作者贴标签失败: %v", err)
	}
	if err := f.svc.RemoveTag(ctx, collaborator, second.ID, "协作标签"); err != nil {
		t.Fatalf("协作者移除标签失败: %v", err)
	}
	if err := f.svc.ReorderQuestions(ctx, collaborator, lib.Entity.ID, []int64{second.ID, question.ID}); err != nil {
		t.Fatalf("协作者调整顺序失败: %v", err)
	}
	if _, err := f.svc.ExportLibrary(ctx, collaborator, lib.Entity.ID); err != nil {
		t.Fatalf("协作者导出失败: %v", err)
	}
	if err := f.svc.DeleteQuestion(ctx, collaborator, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("协作者删除题目失败: %v", err)
	}
	if _, err := f.svc.RestoreQuestion(ctx, collaborator, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("协作者恢复题目失败: %v", err)
	}

	// 不可做：改名称、删题库、邀请、移除成员、彻底删除、转移所有权。
	name := "协作者改的名称"
	_, err = f.svc.UpdateLibrary(ctx, collaborator, lib.Entity.ID, LibraryPatch{Name: &name})
	assertCode(t, err, apperr.CodeForbidden)
	err = f.svc.DeleteLibrary(ctx, collaborator, lib.Entity.ID, "127.0.0.1")
	assertCode(t, err, apperr.CodeForbidden)
	_, err = f.svc.InviteMember(ctx, collaborator, lib.Entity.ID, otherName)
	assertCode(t, err, apperr.CodeForbidden)
	err = f.svc.RemoveMember(ctx, collaborator, lib.Entity.ID, other.UserID, "127.0.0.1")
	assertCode(t, err, apperr.CodeForbidden)

	if err := f.svc.DeleteQuestion(ctx, collaborator, second.ID, "127.0.0.1"); err != nil {
		t.Fatalf("协作者删除题目失败: %v", err)
	}
	err = f.svc.PurgeQuestion(ctx, collaborator, second.ID, "127.0.0.1")
	assertCode(t, err, apperr.CodeForbidden)

	// 所有者仍旧可以彻底删除。
	if err := f.svc.PurgeQuestion(ctx, owner, second.ID, "127.0.0.1"); err != nil {
		t.Fatalf("所有者彻底删除题目失败: %v", err)
	}
}

// TestReferenceQuestionFollowsLatestVersionDerivedDoesNot 覆盖 C-28 与需求 §7.4。
func TestReferenceQuestionFollowsLatestVersionDerivedDoesNot(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "版本题库")

	promptID := f.prompt(t, ownerID, "来源提示词", "第一版正文", "public")
	reference, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加引用题失败: %v", err)
	}
	derived, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{
		Title: "派生题目", Body: "派生正文",
	})
	if err != nil {
		t.Fatalf("添加派生题失败: %v", err)
	}

	// 来源提示词发生重大修改：正文更新、版本号推进。
	majorUpdate(t, f, promptID, "第二版正文", "v2.0")

	after, err := f.svc.GetQuestion(ctx, owner, reference.ID)
	if err != nil {
		t.Fatalf("读取引用题失败: %v", err)
	}
	if after.Body != "第二版正文" {
		t.Fatalf("引用题正文为 %q，期望跟随最新版本", after.Body)
	}
	if after.SourceVersionNo != "v2.0" {
		t.Fatalf("引用题来源版本为 %q，期望 v2.0", after.SourceVersionNo)
	}
	if !after.SourceMajorModified {
		t.Fatalf("引用题未标注来源提示词发生过重大修改")
	}

	derivedAfter, err := f.svc.GetQuestion(ctx, owner, derived.ID)
	if err != nil {
		t.Fatalf("读取派生题失败: %v", err)
	}
	if derivedAfter.Body != "派生正文" {
		t.Fatalf("派生题正文为 %q，期望不受来源重大修改影响（C-28）", derivedAfter.Body)
	}
}

// TestDerivedQuestionDoesNotConsumeCapacity 覆盖 C-22。
func TestDerivedQuestionDoesNotConsumeCapacity(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "容量题库")
	if err := f.config.Set(ctx, "library_question_limit", 2, "单个私有题库题目上限", 0); err != nil {
		t.Fatalf("设置容量失败: %v", err)
	}

	prompts := []int64{
		f.prompt(t, ownerID, "提示词一", "正文一", "public"),
		f.prompt(t, ownerID, "提示词二", "正文二", "public"),
	}
	for _, promptID := range prompts {
		if _, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID}); err != nil {
			t.Fatalf("添加引用题失败: %v", err)
		}
	}

	// 引用题已占满容量。
	extra := f.prompt(t, ownerID, "提示词三", "正文三", "public")
	_, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &extra})
	assertCode(t, err, apperr.CodeQuotaExceeded)

	// 派生题不受容量限制（C-22），可以继续添加。
	for i := 0; i < 3; i++ {
		if _, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{
			Title: fmt.Sprintf("派生题 %d", i), Body: "派生正文",
		}); err != nil {
			t.Fatalf("添加派生题失败: %v", err)
		}
	}

	// 把一条引用题移入回收站：它仍占容量（C-22）。
	reference, err := f.svc.ListQuestions(ctx, owner, lib.Entity.ID, 1, 50)
	if err != nil {
		t.Fatalf("读取题目失败: %v", err)
	}
	var referenceID int64
	for _, item := range reference.Items {
		if item.Kind == domain.QuestionReference {
			referenceID = item.ID
			break
		}
	}
	if err := f.svc.DeleteQuestion(ctx, owner, referenceID, "127.0.0.1"); err != nil {
		t.Fatalf("删除引用题失败: %v", err)
	}
	_, err = f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &extra})
	assertCode(t, err, apperr.CodeQuotaExceeded)

	// 彻底删除后容量归还。
	if err := f.svc.PurgeQuestion(ctx, owner, referenceID, "127.0.0.1"); err != nil {
		t.Fatalf("彻底删除引用题失败: %v", err)
	}
	if _, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &extra}); err != nil {
		t.Fatalf("归还容量后添加引用题失败: %v", err)
	}
}

// TestDerivingReferenceQuestionReturnsCapacity 覆盖 C-22 的转换路径：
// 引用题被修改成为派生题后立即归还容量，容量不会被永久占用。
func TestDerivingReferenceQuestionReturnsCapacity(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "转换容量题库")
	if err := f.config.Set(ctx, "library_question_limit", 1, "单个私有题库题目上限", 0); err != nil {
		t.Fatalf("设置容量失败: %v", err)
	}

	promptID := f.prompt(t, ownerID, "转换来源", "来源正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加引用题失败: %v", err)
	}

	readCount := func() int32 {
		t.Helper()
		var count int32
		if err := f.pool.QueryRow(ctx,
			`SELECT question_count FROM libraries WHERE id = $1`, lib.Entity.ID).Scan(&count); err != nil {
			t.Fatalf("读取题目计数失败: %v", err)
		}
		return count
	}
	if readCount() != 1 {
		t.Fatalf("添加引用题后计数 = %d，期望 1", readCount())
	}

	if _, err := f.svc.UpdateQuestion(ctx, owner, question.ID, UpdateQuestionInput{
		Title: "派生化", Body: "派生正文", Note: "转为派生题",
	}); err != nil {
		t.Fatalf("引用题转派生题失败: %v", err)
	}
	if readCount() != 0 {
		t.Fatalf("转换后计数 = %d，期望归还至 0", readCount())
	}

	// 归还后的容量立即可用。
	second := f.prompt(t, ownerID, "第二条引用", "正文二", "public")
	if _, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &second}); err != nil {
		t.Fatalf("归还容量后添加引用题失败: %v", err)
	}
}

// TestSourcePromptDeletionBehaviour 覆盖「删除被引用的提示词（引用检查）」：
// 来源提示词被软删除后，引用题留在库中但标注来源不可用，成员可删除它归还容量；
// 来源提示词被恢复后题目自动恢复可读；来源未被解除引用时，彻底删除在数据库层被拒绝。
func TestSourcePromptDeletionBehaviour(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "引用题库")
	promptID := f.prompt(t, ownerID, "来源提示词", "来源正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加引用题失败: %v", err)
	}

	// 来源提示词被软删除：题目仍可见，但标注来源不可用、没有正文。
	if _, err := f.pool.Exec(ctx, `UPDATE prompts SET deleted_at = now() WHERE id = $1`, promptID); err != nil {
		t.Fatalf("软删除提示词失败: %v", err)
	}
	missing, err := f.svc.GetQuestion(ctx, owner, question.ID)
	if err != nil {
		t.Fatalf("来源软删除后读取引用题失败: %v", err)
	}
	if missing.SourceAvailable {
		t.Fatalf("来源提示词已删除，题目仍标注来源可用")
	}
	if missing.Body != "" {
		t.Fatalf("来源提示词已删除，题目仍带出正文 %q", missing.Body)
	}
	if missing.Title != domain.LibraryQuestionSourceMissing {
		t.Fatalf("题目标题为 %q，期望来源缺失文案", missing.Title)
	}

	// 来源未解除引用时，数据库拒绝彻底删除该提示词（prompt_in_use 的底层保障）。
	if _, err := f.pool.Exec(ctx, `DELETE FROM prompts WHERE id = $1`, promptID); err == nil {
		t.Fatalf("被题库引用的提示词竟被彻底删除")
	}

	// 恢复来源提示词：题目重新可读。
	if _, err := f.pool.Exec(ctx, `UPDATE prompts SET deleted_at = NULL WHERE id = $1`, promptID); err != nil {
		t.Fatalf("恢复提示词失败: %v", err)
	}
	restored, err := f.svc.GetQuestion(ctx, owner, question.ID)
	if err != nil {
		t.Fatalf("来源恢复后读取引用题失败: %v", err)
	}
	if !restored.SourceAvailable || restored.Body != "来源正文" {
		t.Fatalf("来源恢复后题目未恢复可读：%+v", restored)
	}

	// 成员删除该题目后解除引用，此时提示词可以彻底删除。
	if err := f.svc.DeleteQuestion(ctx, owner, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("删除题目失败: %v", err)
	}
	if err := f.svc.PurgeQuestion(ctx, owner, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("彻底删除题目失败: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM prompts WHERE id = $1`, promptID); err != nil {
		t.Fatalf("解除引用后彻底删除提示词失败: %v", err)
	}
}

// TestDerivedQuestionKeepsSourceVisibilityAfterSourceDeletion 覆盖 C-20 的派生题通道：
// 来源提示词软删除后，派生题仍按来源可见性复查，受限内容不对普通协作者放行。
func TestDerivedQuestionKeepsSourceVisibilityAfterSourceDeletion(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "可见性题库")
	_, collaboratorName, collaborator := f.user(t)
	collaborator = f.invite(t, owner, lib.Entity.ID, collaborator.UserID, collaboratorName)

	promptID := f.prompt(t, ownerID, "受限来源", "受限正文", "developer")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加引用题失败: %v", err)
	}
	if _, err := f.svc.GetQuestion(ctx, collaborator, question.ID); err == nil {
		t.Fatal("普通协作者竟能看到开发者可见来源的引用题")
	}

	// 修改引用题使其转为派生题（正文改存库内副本）。
	derived, err := f.svc.UpdateQuestion(ctx, owner, question.ID, UpdateQuestionInput{
		Title: "受限派生题", Body: "受限正文副本", Note: "转为派生题",
	})
	if err != nil {
		t.Fatalf("引用题转派生题失败: %v", err)
	}
	if derived.Kind != domain.QuestionDerived {
		t.Fatalf("题目类型为 %v，期望转为派生题", derived.Kind)
	}
	if _, err := f.svc.GetQuestion(ctx, collaborator, question.ID); err == nil {
		t.Fatal("普通协作者竟能看到受限派生题")
	}

	// 软删除来源提示词：协作者依然不可见（修复前此处因来源缺失而放行）。
	if _, err := f.pool.Exec(ctx, `UPDATE prompts SET deleted_at = now() WHERE id = $1`, promptID); err != nil {
		t.Fatalf("软删除提示词失败: %v", err)
	}
	if _, err := f.svc.GetQuestion(ctx, collaborator, question.ID); err == nil {
		t.Fatal("来源软删除后普通协作者竟能看到受限派生题")
	}
	if _, err := f.svc.GetQuestion(ctx, owner, question.ID); err != nil {
		t.Fatalf("所有者应始终可见: %v", err)
	}

	// 无来源的派生题不受影响，协作者可见。
	plain, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{
		Title: "无来源派生题", Body: "独立正文",
	})
	if err != nil {
		t.Fatalf("添加无来源派生题失败: %v", err)
	}
	if _, err := f.svc.GetQuestion(ctx, collaborator, plain.ID); err != nil {
		t.Fatalf("协作者应能看到无来源派生题: %v", err)
	}
}

// TestExportContainsAllDeclaredElements 核对导出含 14 项要素（需求 §10.8、§10.7）。
func TestExportContainsAllDeclaredElements(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, ownerName, owner := f.developer(t)
	lib := f.library(t, owner, "导出题库")
	description := "导出用的题库描述"
	if _, err := f.svc.UpdateLibrary(ctx, owner, lib.Entity.ID, LibraryPatch{Description: &description}); err != nil {
		t.Fatalf("写入题库描述失败: %v", err)
	}

	promptID := f.prompt(t, ownerID, "引用题来源", "引用题正文", "public")
	if _, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{
		PromptID: &promptID, Tags: []string{"标签甲", "标签乙"},
	}); err != nil {
		t.Fatalf("添加引用题失败: %v", err)
	}
	derived, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{
		Title: "派生题标题", Body: "派生题正文", Description: "派生题说明",
	})
	if err != nil {
		t.Fatalf("添加派生题失败: %v", err)
	}
	if _, err := f.svc.UpdateQuestion(ctx, owner, derived.ID, UpdateQuestionInput{
		Title: "派生题标题", Body: "派生题正文改", Description: "派生题说明", Note: "调整用词",
	}); err != nil {
		t.Fatalf("修改派生题失败: %v", err)
	}

	export, err := f.svc.ExportLibrary(ctx, owner, lib.Entity.ID)
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if export.Filename != "导出题库.md" {
		t.Fatalf("导出文件名为 %q", export.Filename)
	}

	markers := map[string]string{
		"library_name":         "# 导出题库",
		"library_description":  "- 题库描述：" + description,
		"owner_name":           "- 创建者：" + ownerName,
		"exported_at":          "- 导出时间：",
		"question_count":       "- 题目数量：2",
		"question_order":       "## 1. ",
		"question_title":       "引用题来源",
		"question_purpose":     "- 考察目的：",
		"question_capability":  "- 考察维度：",
		"question_tags":        "- 题目标签：标签甲、标签乙",
		"prompt_body":          "### 提示词正文",
		"question_description": "### 题目说明",
		"source_prompt":        "- 来源提示词：#",
		"source_version":       "- 来源版本：v1.0",
		"derivation_note":      "- 修改说明：调整用词",
	}
	declared := domain.LibraryExportElements()
	if len(declared) != 15 {
		t.Fatalf("导出要素声明为 %d 项，期望 15 项", len(declared))
	}
	for _, element := range declared {
		marker, ok := markers[element]
		if !ok {
			t.Fatalf("导出要素 %s 没有对应的核对标记", element)
		}
		if !strings.Contains(export.Content, marker) {
			t.Fatalf("导出内容缺少要素 %s（标记 %q）：\n%s", element, marker, export.Content)
		}
	}
	if len(markers) != len(declared) {
		t.Fatalf("核对标记 %d 个，与声明的 %d 项要素不一致", len(markers), len(declared))
	}
	if !strings.Contains(export.Content, "派生题正文改") {
		t.Fatalf("导出内容缺少派生题正文：\n%s", export.Content)
	}
}

// TestCollaboratorDeletionLogsSixElementsAndNotifiesOwner 覆盖 C-34 与需求 §10.6。
func TestCollaboratorDeletionLogsSixElementsAndNotifiesOwner(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	_, collaboratorName, collaborator := f.user(t)
	lib := f.library(t, owner, "审计题库")
	promptID := f.prompt(t, ownerID, "被删题目的来源", "被删题目的正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}
	collaborator = f.invite(t, owner, lib.Entity.ID, collaborator.UserID, collaboratorName)
	beforeNotifications(t, f, ownerID)

	if err := f.svc.DeleteQuestion(ctx, collaborator, question.ID, "10.0.0.9"); err != nil {
		t.Fatalf("协作者删除题目失败: %v", err)
	}

	// 要素 2 与 5：删除人与删除前快照。
	var deletedBy int64
	var snapshot []byte
	var expiresAt, deletedAt time.Time
	if err := f.pool.QueryRow(ctx, `
		SELECT deleted_by, snapshot, expires_at, deleted_at FROM recycle_bin
		WHERE object_type = 'library_question' AND object_id = $1`, question.ID,
	).Scan(&deletedBy, &snapshot, &expiresAt, &deletedAt); err != nil {
		t.Fatalf("读取回收站记录失败: %v", err)
	}
	if deletedBy != collaborator.UserID {
		t.Fatalf("回收站记录的删除人为 %d，期望 %d", deletedBy, collaborator.UserID)
	}
	if !strings.Contains(string(snapshot), "被删题目的来源") {
		t.Fatalf("删除前快照缺少题目标题：%s", snapshot)
	}
	if !expiresAt.After(deletedAt) {
		t.Fatalf("回收站到期时刻 %s 未晚于删除时刻 %s", expiresAt, deletedAt)
	}

	// 要素 1 与 3、4：操作日志、被删除题目、删除时间。
	var action, objectType, operatorName string
	if err := f.pool.QueryRow(ctx, `
		SELECT action, object_type, operator_name FROM operation_logs
		WHERE object_id = $1 AND action = 'library_question_delete'`, question.ID,
	).Scan(&action, &objectType, &operatorName); err != nil {
		t.Fatalf("读取操作日志失败: %v", err)
	}
	if objectType != "library_question" {
		t.Fatalf("操作日志对象类型为 %q", objectType)
	}
	if operatorName != collaboratorName {
		t.Fatalf("操作日志操作人为 %q，期望 %q", operatorName, collaboratorName)
	}

	// 要素 6：通知题库所有者。
	var kind, title, body string
	var recipient int64
	if err := f.pool.QueryRow(ctx, `
		SELECT user_id, kind, title, body FROM notifications
		WHERE kind = 'library_question_deleted' ORDER BY id DESC LIMIT 1`,
	).Scan(&recipient, &kind, &title, &body); err != nil {
		t.Fatalf("读取删题通知失败: %v", err)
	}
	if recipient != ownerID {
		t.Fatalf("通知收件人为 %d，期望所有者 %d", recipient, ownerID)
	}
	want := domain.NotificationLibraryQuestionDeletedBody(collaboratorName, "审计题库", "被删题目的来源")
	if body != want {
		t.Fatalf("通知正文为 %q，期望 %q", body, want)
	}
}

// TestOwnerDeletionDoesNotNotifySelf 覆盖需求 §10.6 末条。
func TestOwnerDeletionDoesNotNotifySelf(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "自删题库")
	promptID := f.prompt(t, ownerID, "提示词", "正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}
	before := countNotifications(t, f, ownerID)

	if err := f.svc.DeleteQuestion(ctx, owner, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("所有者删除题目失败: %v", err)
	}
	after := countNotifications(t, f, ownerID)
	if after != before {
		t.Fatalf("所有者删除自己的题目后通知数从 %d 变为 %d，不应通知自己", before, after)
	}
	// 操作日志与回收站记录仍然完整。
	var logs int64
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM operation_logs WHERE object_id = $1 AND action = 'library_question_delete'`,
		question.ID).Scan(&logs); err != nil {
		t.Fatalf("统计操作日志失败: %v", err)
	}
	if logs != 1 {
		t.Fatalf("所有者删除题目后有 %d 条操作日志，期望 1 条", logs)
	}
}

// TestLibraryRecycleBinRestoreAndPurge 覆盖需求 §11.2 与 §11.3。
func TestLibraryRecycleBinRestoreAndPurge(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "回收站题库")
	promptID := f.prompt(t, ownerID, "提示词", "正文", "public")
	if _, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID}); err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}

	if err := f.svc.DeleteLibrary(ctx, owner, lib.Entity.ID, "127.0.0.1"); err != nil {
		t.Fatalf("删除题库失败: %v", err)
	}
	_, err := f.svc.GetLibrary(ctx, owner, lib.Entity.ID)
	assertCode(t, err, apperr.CodeNotFound)

	entries, err := f.svc.ListMyLibraryRecycleBin(ctx, owner)
	if err != nil {
		t.Fatalf("读取题库回收站失败: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "回收站题库" {
		t.Fatalf("题库回收站内容为 %+v", entries)
	}
	if entries[0].ExpiresAt.Before(entries[0].DeletedAt) {
		t.Fatalf("回收站到期时刻早于删除时刻")
	}

	restored, err := f.svc.RestoreLibrary(ctx, owner, lib.Entity.ID, "127.0.0.1")
	if err != nil {
		t.Fatalf("恢复题库失败: %v", err)
	}
	if restored.Entity.Name != "回收站题库" {
		t.Fatalf("恢复后的题库名称为 %q", restored.Entity.Name)
	}
	page, err := f.svc.ListQuestions(ctx, owner, lib.Entity.ID, 1, 20)
	if err != nil {
		t.Fatalf("恢复后读取题目失败: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("恢复后可见题目 %d 道，期望 1 道", page.Total)
	}

	// 再次删除后彻底删除。
	if err := f.svc.DeleteLibrary(ctx, owner, lib.Entity.ID, "127.0.0.1"); err != nil {
		t.Fatalf("再次删除题库失败: %v", err)
	}
	if err := f.svc.PurgeLibrary(ctx, owner, lib.Entity.ID, "127.0.0.1"); err != nil {
		t.Fatalf("彻底删除题库失败: %v", err)
	}
	var libraries, questions, liveEntries int64
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM libraries WHERE id = $1`, lib.Entity.ID).Scan(&libraries); err != nil {
		t.Fatalf("统计题库失败: %v", err)
	}
	if libraries != 0 {
		t.Fatalf("彻底删除后题库仍在库中")
	}
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM library_questions WHERE library_id = $1`, lib.Entity.ID).Scan(&questions); err != nil {
		t.Fatalf("统计题目失败: %v", err)
	}
	if questions != 0 {
		t.Fatalf("彻底删除题库后仍残留 %d 条题目", questions)
	}
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM recycle_bin
		WHERE library_id = $1 AND restored_at IS NULL AND purged_at IS NULL`,
		lib.Entity.ID).Scan(&liveEntries); err != nil {
		t.Fatalf("统计回收站记录失败: %v", err)
	}
	if liveEntries != 0 {
		t.Fatalf("彻底删除题库后仍有 %d 条未结清的回收站记录", liveEntries)
	}
}

// TestQuestionRecycleBinVisibilityAndExpiry 验证回收站条目按可见性过滤并带到期时刻。
func TestQuestionRecycleBinVisibilityAndExpiry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	_, collaboratorName, collaborator := f.user(t)
	lib := f.library(t, owner, "回收站题库")

	promptID := f.prompt(t, ownerID, "公开提示词", "公开正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}
	collaborator = f.invite(t, owner, lib.Entity.ID, collaborator.UserID, collaboratorName)
	if err := f.svc.DeleteQuestion(ctx, owner, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("删除题目失败: %v", err)
	}

	entries, err := f.svc.ListRecycleBin(ctx, collaborator, lib.Entity.ID)
	if err != nil {
		t.Fatalf("协作者读取回收站失败: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("回收站条目为 %d 条，期望 1 条", len(entries))
	}
	if entries[0].DeletedBy != ownerID {
		t.Fatalf("回收站删除人为 %d，期望 %d", entries[0].DeletedBy, ownerID)
	}
	if entries[0].ExpiresAt.IsZero() {
		t.Fatalf("回收站条目缺少到期时刻")
	}
	if !entries[0].Question.Deleted {
		t.Fatalf("回收站条目标记应当为已删除")
	}
}

// TestCollaboratorRestoreHonoursConfig 验证 collaborator_restore_allowed 生效（需求 §10.5）。
func TestCollaboratorRestoreHonoursConfig(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	_, collaboratorName, collaborator := f.user(t)
	lib := f.library(t, owner, "配置题库")
	promptID := f.prompt(t, ownerID, "提示词", "正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}
	collaborator = f.invite(t, owner, lib.Entity.ID, collaborator.UserID, collaboratorName)
	if err := f.svc.DeleteQuestion(ctx, owner, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("删除题目失败: %v", err)
	}

	if err := f.config.Set(ctx, "collaborator_restore_allowed", false, "协作者可否恢复题库回收站题目", 0); err != nil {
		t.Fatalf("关闭协作者恢复失败: %v", err)
	}
	_, err = f.svc.RestoreQuestion(ctx, collaborator, question.ID, "127.0.0.1")
	assertCode(t, err, apperr.CodeForbidden)

	if _, err := f.svc.RestoreQuestion(ctx, owner, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("所有者恢复题目失败: %v", err)
	}
}

// TestMembersAndInvitation 验证成员管理与邀请幂等。
func TestMembersAndInvitation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, ownerName, owner := f.developer(t)
	_, collaboratorName, collaborator := f.user(t)
	lib := f.library(t, owner, "成员题库")

	if _, err := f.svc.InviteMember(ctx, owner, lib.Entity.ID, collaboratorName); err != nil {
		t.Fatalf("邀请协作者失败: %v", err)
	}
	_, err := f.svc.InviteMember(ctx, owner, lib.Entity.ID, collaboratorName)
	assertCode(t, err, apperr.CodeInvalidState)
	_, err = f.svc.InviteMember(ctx, owner, lib.Entity.ID, "不存在的用户")
	assertCode(t, err, apperr.CodeNotFound)

	members, err := f.svc.ListMembers(ctx, collaborator, lib.Entity.ID)
	if err != nil {
		t.Fatalf("协作者读取成员列表失败: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("成员数为 %d，期望 2", len(members))
	}
	roles := map[string]domain.LibraryRole{}
	for _, member := range members {
		roles[member.Username] = member.Role
	}
	if roles[ownerName] != domain.LibraryRoleOwner {
		t.Fatalf("所有者角色为 %q", roles[ownerName])
	}
	if roles[collaboratorName] != domain.LibraryRoleCollaborator {
		t.Fatalf("协作者角色为 %q", roles[collaboratorName])
	}

	// 被邀请的协作者会在「我参与的题库」中看到该题库。
	page, err := f.svc.ListMyLibraries(ctx, collaborator, 1, 20)
	if err != nil {
		t.Fatalf("协作者读取题库列表失败: %v", err)
	}
	if page.Total != 1 || page.Items[0].Entity.ID != lib.Entity.ID {
		t.Fatalf("协作者的题库列表为 %+v", page)
	}
	if page.Items[0].Role != domain.LibraryRoleCollaborator {
		t.Fatalf("协作者在列表中的角色为 %q", page.Items[0].Role)
	}

	if err := f.svc.RemoveMember(ctx, owner, lib.Entity.ID, collaborator.UserID, "127.0.0.1"); err != nil {
		t.Fatalf("移除协作者失败: %v", err)
	}
	_, err = f.svc.GetLibrary(ctx, collaborator, lib.Entity.ID)
	assertCode(t, err, apperr.CodeNotFound)
	err = f.svc.RemoveMember(ctx, owner, lib.Entity.ID, collaborator.UserID, "127.0.0.1")
	assertCode(t, err, apperr.CodeNotFound)
}

// TestDerivedQuestionKeepsRevisionHistory 验证派生题保留修改记录、修改人与修改时间（需求 §10.7）。
func TestDerivedQuestionKeepsRevisionHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, ownerName, owner := f.developer(t)
	lib := f.library(t, owner, "修订题库")
	promptID := f.prompt(t, ownerID, "来源提示词", "来源正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加引用题失败: %v", err)
	}

	updated, err := f.svc.UpdateQuestion(ctx, owner, question.ID, UpdateQuestionInput{
		Title: "改写后的标题", Body: "改写后的正文", Description: "说明", Note: "改写以便复用",
	})
	if err != nil {
		t.Fatalf("修改引用题失败: %v", err)
	}
	if updated.Kind != domain.QuestionDerived {
		t.Fatalf("引用题被修改后类型为 %q，期望转为派生题", updated.Kind)
	}
	if updated.SourcePromptID == nil || *updated.SourcePromptID != promptID {
		t.Fatalf("派生题未保留来源提示词标识")
	}
	if updated.SourceVersionNo != "v1.0" {
		t.Fatalf("派生题来源版本为 %q", updated.SourceVersionNo)
	}

	revisions, err := f.svc.ListRevisions(ctx, owner, question.ID)
	if err != nil {
		t.Fatalf("读取修订记录失败: %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("修订记录为 %d 条，期望 1 条", len(revisions))
	}
	if revisions[0].EditorName != ownerName {
		t.Fatalf("修订记录修改人为 %q，期望 %q", revisions[0].EditorName, ownerName)
	}
	if revisions[0].CreatedAt.IsZero() {
		t.Fatalf("修订记录缺少修改时间")
	}
	if revisions[0].Note != "改写以便复用" {
		t.Fatalf("修订记录说明为 %q", revisions[0].Note)
	}
	if !strings.Contains(string(revisions[0].Before), "来源正文") {
		t.Fatalf("修订记录缺少修改前正文：%s", revisions[0].Before)
	}

	// 来源提示词发生重大修改后，派生题内容不受影响（C-28）。
	majorUpdate(t, f, promptID, "来源第二版正文", "v2.0")
	after, err := f.svc.GetQuestion(ctx, owner, question.ID)
	if err != nil {
		t.Fatalf("读取派生题失败: %v", err)
	}
	if after.Body != "改写后的正文" {
		t.Fatalf("派生题正文为 %q，期望保持库内副本", after.Body)
	}
}

// TestPurgeExpiredRecycleBinEntries 覆盖需求 §11.3 的到期自动清除。
func TestPurgeExpiredRecycleBinEntries(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "到期题库")
	promptID := f.prompt(t, ownerID, "提示词", "正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}
	if err := f.svc.DeleteQuestion(ctx, owner, question.ID, "127.0.0.1"); err != nil {
		t.Fatalf("删除题目失败: %v", err)
	}
	// 把到期时刻提前到过去，模拟保留期已过。
	if _, err := f.pool.Exec(ctx, `
		UPDATE recycle_bin SET expires_at = now() - interval '1 day'
		WHERE object_type = 'library_question' AND object_id = $1`, question.ID); err != nil {
		t.Fatalf("调整到期时刻失败: %v", err)
	}

	purged, err := f.svc.PurgeExpired(ctx, 10)
	if err != nil {
		t.Fatalf("清理到期记录失败: %v", err)
	}
	if purged != 1 {
		t.Fatalf("清理了 %d 条到期记录，期望 1 条", purged)
	}
	var questions int64
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM library_questions WHERE id = $1`, question.ID).Scan(&questions); err != nil {
		t.Fatalf("统计题目失败: %v", err)
	}
	if questions != 0 {
		t.Fatalf("到期题目未被彻底清除")
	}
	var count int32
	if err := f.pool.QueryRow(ctx, `SELECT question_count FROM libraries WHERE id = $1`, lib.Entity.ID).Scan(&count); err != nil {
		t.Fatalf("读取题目计数失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("到期清理后题库计数为 %d，期望 0", count)
	}
}

// TestHardDeleteGuardsRequireDeletedState 验证彻底删除语句只作用于回收站中的行：
// 对在线对象影响 0 行，清理竞态下恢复方胜出；锁定读取能命中回收站中的行。
func TestHardDeleteGuardsRequireDeletedState(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	q := repo.New(f.pool)

	ownerID, _, owner := f.developer(t)
	lib := f.library(t, owner, "护栏题库")
	promptID := f.prompt(t, ownerID, "护栏提示词", "正文", "public")
	question, err := f.svc.AddQuestion(ctx, owner, lib.Entity.ID, AddQuestionInput{PromptID: &promptID})
	if err != nil {
		t.Fatalf("添加题目失败: %v", err)
	}

	if rows, err := q.HardDeletePrompt(ctx, promptID); err != nil || rows != 0 {
		t.Fatalf("在线提示词被彻底删除语句命中：rows=%d err=%v", rows, err)
	}
	if rows, err := q.HardDeleteLibraryQuestion(ctx, question.ID); err != nil || rows != 0 {
		t.Fatalf("在线题目被彻底删除语句命中：rows=%d err=%v", rows, err)
	}
	if rows, err := q.HardDeleteLibrary(ctx, lib.Entity.ID); err != nil || rows != 0 {
		t.Fatalf("在线题库被彻底删除语句命中：rows=%d err=%v", rows, err)
	}
	if _, err := q.LockPromptIncludingDeleted(ctx, promptID); err != nil {
		t.Fatalf("锁定在线提示词行失败: %v", err)
	}
	if _, err := q.LockLibraryIncludingDeleted(ctx, lib.Entity.ID); err != nil {
		t.Fatalf("锁定在线题库行失败: %v", err)
	}
}

// TestUnauthenticatedLibraryAccessIsRejected 验证游客无法触达题库。
func TestUnauthenticatedLibraryAccessIsRejected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	guest := domain.Guest()

	_, err := f.svc.CreateLibrary(ctx, guest, LibraryInput{Name: "游客题库"})
	assertCode(t, err, apperr.CodeEmailNotVerified)
	_, err = f.svc.ListMyLibraries(ctx, guest, 1, 20)
	assertCode(t, err, apperr.CodeUnauthorized)
	_, err = f.svc.GetLibrary(ctx, guest, 1)
	assertCode(t, err, apperr.CodeNotFound)
}

// majorUpdate 模拟来源提示词发生重大修改，并把当前版本推进到新版本号。
func majorUpdate(t *testing.T, f *fixture, promptID int64, body, versionNo string) {
	t.Helper()
	ctx := context.Background()
	var purposeID, capabilityID int64
	var uploaderID int64
	if err := f.pool.QueryRow(ctx,
		`SELECT purpose_id, capability_id, uploader_id FROM prompts WHERE id = $1`, promptID).Scan(&purposeID, &capabilityID, &uploaderID); err != nil {
		t.Fatalf("读取提示词失败: %v", err)
	}
	var versionID int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO prompt_versions (
			prompt_id, version_no, major, minor, title, body,
			purpose_id, capability_id, visibility, sensitive_level, is_major, author_id
		) VALUES ($1, $2, 2, 0, '来源第二版', $3, $4, $5, 'public', 'low', true, $6)
		RETURNING id`, promptID, versionNo, body, purposeID, capabilityID, uploaderID).Scan(&versionID); err != nil {
		t.Fatalf("写入新版本失败: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `
		UPDATE prompts SET body = $2, major_modified = true,
			latest_version_no = $3, current_version_id = $4, updated_at = now()
		WHERE id = $1`, promptID, body, versionNo, versionID); err != nil {
		t.Fatalf("更新提示词失败: %v", err)
	}
}

func countNotifications(t *testing.T, f *fixture, userID int64) int64 {
	t.Helper()
	var count int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND deleted_at IS NULL`, userID).Scan(&count); err != nil {
		t.Fatalf("统计通知失败: %v", err)
	}
	return count
}

// beforeNotifications 清空收件人的既有通知，使后续断言只反映本次操作。
func beforeNotifications(t *testing.T, f *fixture, userID int64) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`DELETE FROM notifications WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("清理通知失败: %v", err)
	}
	if count := countNotifications(t, f, userID); count != 0 {
		t.Fatalf("清理后仍有 %d 条通知", count)
	}
}
