package prompt

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/repo"
)

// fixture 汇集用例需要的构造物，避免每个用例重复建立服务与查询。
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
	return &fixture{pool: pool, svc: New(Deps{Pool: pool, Config: cfgStore, Logger: logger}), config: cfgStore}
}

// purposeID 按 slug 取考察目的 ID。
func (f *fixture) purposeID(t *testing.T, slug string) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM purposes WHERE slug = $1`, slug).Scan(&id); err != nil {
		t.Fatalf("读取考察目的 %s 失败: %v", slug, err)
	}
	return id
}

// capabilityID 按 slug 取考察维度 ID。
func (f *fixture) capabilityID(t *testing.T, slug string) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM capabilities WHERE slug = $1`, slug).Scan(&id); err != nil {
		t.Fatalf("读取考察维度 %s 失败: %v", slug, err)
	}
	return id
}

func (f *fixture) normalUser(t *testing.T) domain.User {
	t.Helper()
	return domain.User{
		ID:     pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Verified: true}),
		Role:   domain.RoleUser,
		Status: domain.StatusNormal,
	}
}

func (f *fixture) developer(t *testing.T) domain.User {
	t.Helper()
	return domain.User{
		ID:     pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Role: "developer", Verified: true}),
		Role:   domain.RoleDeveloper,
		Status: domain.StatusNormal,
	}
}

func (f *fixture) baseInput(t *testing.T, purposeID int64, visibility domain.Visibility) Input {
	return Input{
		Title:        "测试提示词",
		Body:         "请完成以下任务",
		PurposeID:    purposeID,
		CapabilityID: f.capabilityID(t, "reasoning"),
		Visibility:   visibility,
		Sensitive:    domain.SensitiveLow,
		ChangeNote:   "初始版本",
		Description:  "描述说明",
	}
}

// assertAppErrCode 断言服务返回的错误码。
func assertAppErrCode(t *testing.T, err error, want apperr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，实际无错误", want)
	}
	appErr, ok := err.(*apperr.Error)
	if !ok {
		t.Fatalf("期望 apperr.Error，实际 %T: %v", err, err)
	}
	if appErr.Code != want {
		t.Fatalf("错误码 = %s（%s），期望 %s", appErr.Code, appErr.Message, want)
	}
}

func TestCreateRejectsHighRiskPurposeForOrdinaryUser(t *testing.T) {
	// 创建路径拦截：普通用户不能选择高风险考察目的（C-19、需求 §8.7）。
	f := newFixture(t)
	user := f.normalUser(t)

	_, err := f.svc.Create(context.Background(), user,
		f.baseInput(t, f.purposeID(t, "adversarial"), domain.VisibilityPublic))
	assertAppErrCode(t, err, apperr.CodeForbidden)
}

func TestUpdateRejectsHighRiskPurposeForOrdinaryUser(t *testing.T) {
	// 更新路径拦截。这是 C-20 明确要求的回归：需求 §8.7 只提了创建，
	// 若更新路径漏判，普通用户可以先建普通提示词、再改成高风险考察目的绕过限制。
	f := newFixture(t)
	user := f.normalUser(t)
	normal := f.purposeID(t, "capability_boundary")

	created, err := f.svc.Create(context.Background(), user, f.baseInput(t, normal, domain.VisibilityPublic))
	if err != nil {
		t.Fatalf("创建提示词失败: %v", err)
	}

	in := f.baseInput(t, normal, domain.VisibilityPublic)
	in.Title = "改为高风险考察目的"
	in.PurposeID = f.purposeID(t, "adversarial")
	_, err = f.svc.Update(context.Background(), user, created.Prompt.ID, in)
	assertAppErrCode(t, err, apperr.CodeForbidden)

	// 拒绝之后考察目的未被改写，仍是创建时的考察目的。
	detail, err := f.svc.Get(context.Background(), domain.ViewerFor(user), created.Prompt.ID, "")
	if err != nil {
		t.Fatalf("读取提示词失败: %v", err)
	}
	if detail.Prompt.PurposeID != normal {
		t.Errorf("被拒绝的修改不应生效，考察目的 = %d，期望 %d", detail.Prompt.PurposeID, normal)
	}
}

func TestCreateRejectsDeveloperVisibilityForOrdinaryUser(t *testing.T) {
	f := newFixture(t)
	user := f.normalUser(t)

	_, err := f.svc.Create(context.Background(), user,
		f.baseInput(t, f.purposeID(t, "capability_boundary"), domain.VisibilityDeveloper))
	assertAppErrCode(t, err, apperr.CodeForbidden)
}

func TestUpdateRejectsDeveloperVisibilityForOrdinaryUser(t *testing.T) {
	f := newFixture(t)
	user := f.normalUser(t)
	normal := f.purposeID(t, "capability_boundary")

	created, err := f.svc.Create(context.Background(), user, f.baseInput(t, normal, domain.VisibilityPublic))
	if err != nil {
		t.Fatalf("创建提示词失败: %v", err)
	}

	in := f.baseInput(t, normal, domain.VisibilityDeveloper)
	_, err = f.svc.Update(context.Background(), user, created.Prompt.ID, in)
	assertAppErrCode(t, err, apperr.CodeForbidden)
}

func TestDeveloperMayUseHighRiskPurposeAndDeveloperVisibility(t *testing.T) {
	f := newFixture(t)
	dev := f.developer(t)
	jailbreak := f.purposeID(t, "adversarial")

	created, err := f.svc.Create(context.Background(), dev, f.baseInput(t, jailbreak, domain.VisibilityDeveloper))
	if err != nil {
		t.Fatalf("开发者应能选择高风险考察目的与开发者可见: %v", err)
	}
	if !created.Prompt.HighRisk {
		t.Error("创建结果的高风险标记应为真")
	}

	// 开发者对自己的高风险提示词可读，游客与普通用户读到 404（C-20 的直链侧）。
	if _, err := f.svc.Get(context.Background(), domain.ViewerFor(dev), created.Prompt.ID, ""); err != nil {
		t.Errorf("开发者读取高风险提示词失败: %v", err)
	}
	_, err = f.svc.Get(context.Background(), domain.Guest(), created.Prompt.ID, "")
	assertAppErrCode(t, err, apperr.CodeNotFound)

	other := f.normalUser(t)
	_, err = f.svc.Get(context.Background(), domain.ViewerFor(other), created.Prompt.ID, "")
	assertAppErrCode(t, err, apperr.CodeNotFound)
}

func TestHighRiskPublicPromptIsHiddenFromOrdinaryUsers(t *testing.T) {
	// C-20 的完整形态：考察目的高风险 + 可见性 public。开发者先按普通目的创建再改为高风险，
	// 使一条 public 的高风险提示词真实存在；普通用户与游客都必须 404。
	f := newFixture(t)
	dev := f.developer(t)
	normal := f.purposeID(t, "capability_boundary")

	created, err := f.svc.Create(context.Background(), dev, f.baseInput(t, normal, domain.VisibilityPublic))
	if err != nil {
		t.Fatalf("创建提示词失败: %v", err)
	}
	in := f.baseInput(t, normal, domain.VisibilityPublic)
	in.PurposeID = f.purposeID(t, "adversarial")
	if _, err := f.svc.Update(context.Background(), dev, created.Prompt.ID, in); err != nil {
		t.Fatalf("改为高风险考察目的失败: %v", err)
	}

	_, err = f.svc.Get(context.Background(), domain.Guest(), created.Prompt.ID, "")
	assertAppErrCode(t, err, apperr.CodeNotFound)

	_, err = f.svc.Get(context.Background(), domain.ViewerFor(f.normalUser(t)), created.Prompt.ID, "")
	assertAppErrCode(t, err, apperr.CodeNotFound)

	// 列表侧同样不可见：ListScope 的高风险过滤对非开发者生效。
	list, err := f.svc.List(context.Background(), domain.Guest(), repo.PromptListFilter{}, domain.SortLatest, 1, 20)
	if err != nil {
		t.Fatalf("查询公开列表失败: %v", err)
	}
	for _, item := range list.Items {
		if item.Prompt.ID == created.Prompt.ID {
			t.Error("高风险提示词不应出现在游客可见的公开列表中")
		}
	}

	// 开发者可见：其本人本就对该内容有权限。
	devList, err := f.svc.List(context.Background(), domain.ViewerFor(dev), repo.PromptListFilter{}, domain.SortLatest, 1, 20)
	if err != nil {
		t.Fatalf("查询公开列表失败: %v", err)
	}
	found := false
	for _, item := range devList.Items {
		if item.Prompt.ID == created.Prompt.ID {
			found = true
		}
	}
	if !found {
		t.Error("开发者应能在公开列表中看到高风险提示词")
	}
}

func TestNormalEditCreatesMinorVersionAndKeepsRatings(t *testing.T) {
	// 需求 §7.2：普通修改生成新版本、不重置评分、保留历史版本。
	f := newFixture(t)
	user := f.normalUser(t)
	normal := f.purposeID(t, "capability_boundary")

	created, err := f.svc.Create(context.Background(), user, f.baseInput(t, normal, domain.VisibilityPublic))
	if err != nil {
		t.Fatalf("创建提示词失败: %v", err)
	}
	if created.VersionNo != "v1.0" {
		t.Errorf("初始版本号 = %s，期望 v1.0", created.VersionNo)
	}

	in := f.baseInput(t, normal, domain.VisibilityPublic)
	in.Title = "普通修改后的标题"
	result, err := f.svc.Update(context.Background(), user, created.Prompt.ID, in)
	if err != nil {
		t.Fatalf("普通修改失败: %v", err)
	}
	if result.VersionNo != "v1.1" {
		t.Errorf("普通修改后版本号 = %s，期望 v1.1", result.VersionNo)
	}
	if result.MajorChange {
		t.Error("未勾选重大修改时不应标记为重大修改")
	}

	detail, err := f.svc.Get(context.Background(), domain.ViewerFor(user), created.Prompt.ID, "")
	if err != nil {
		t.Fatalf("读取提示词失败: %v", err)
	}
	if len(detail.Versions) != 2 {
		t.Fatalf("版本数 = %d，期望 2（历史版本应保留）", len(detail.Versions))
	}
	if detail.Version.Title != "普通修改后的标题" {
		t.Errorf("当前版本标题 = %q，期望展示最新版本", detail.Version.Title)
	}
	if detail.Prompt.MajorModified {
		t.Error("普通修改不应把提示词标记为已发生重大修改")
	}
}

func TestMajorChangeResetsRatingsAndArchivesHistory(t *testing.T) {
	// 需求 §7.3 的 7 步影响面：生成新版本、前台展示新版本、评分从零开始、
	// 历史评分归档、提示标记、系统重算统计、标签待复核。
	f := newFixture(t)
	user := f.normalUser(t)
	normal := f.purposeID(t, "capability_boundary")

	created, err := f.svc.Create(context.Background(), user, f.baseInput(t, normal, domain.VisibilityPublic))
	if err != nil {
		t.Fatalf("创建提示词失败: %v", err)
	}

	ratingsBefore := seedRatings(t, f.pool, user.ID, created.Prompt.ID, 4)
	if len(ratingsBefore) == 0 {
		t.Fatal("评分准备失败")
	}

	in := f.baseInput(t, normal, domain.VisibilityPublic)
	in.Title = "重大修改后的标题"
	in.IsMajorChange = true
	in.ChangeNote = "任务定义整体重写"

	result, err := f.svc.Update(context.Background(), user, created.Prompt.ID, in)
	if err != nil {
		t.Fatalf("重大修改失败: %v", err)
	}
	if result.VersionNo != "v2.0" {
		t.Errorf("重大修改后版本号 = %s，期望 v2.0（C-27 次版本归零）", result.VersionNo)
	}
	if !result.MajorChange {
		t.Error("结果应标记为重大修改")
	}
	if result.ArchivedRatings != int64(len(ratingsBefore)) {
		t.Errorf("归档评分数 = %d，期望 %d", result.ArchivedRatings, len(ratingsBefore))
	}

	ctx := context.Background()
	// 当前评分从零开始：一条也不剩。
	var activeCount int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM ratings WHERE prompt_id = $1 AND deleted_at IS NULL`,
		created.Prompt.ID).Scan(&activeCount); err != nil {
		t.Fatalf("统计当前评分失败: %v", err)
	}
	if activeCount != 0 {
		t.Errorf("重大修改后当前评分数 = %d，期望 0", activeCount)
	}

	// 历史评分归档完整，且保留原始评分时间与被归档版本号。
	var archived int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM prompt_version_ratings WHERE prompt_id = $1`,
		created.Prompt.ID).Scan(&archived); err != nil {
		t.Fatalf("统计归档评分失败: %v", err)
	}
	if archived != int64(len(ratingsBefore)) {
		t.Errorf("归档表评分数 = %d，期望 %d", archived, len(ratingsBefore))
	}

	var oldVersion string
	if err := f.pool.QueryRow(ctx,
		`SELECT pv.version_no FROM prompt_version_ratings pvr
		 JOIN prompt_versions pv ON pv.id = pvr.prompt_version_id
		 WHERE pvr.prompt_id = $1 LIMIT 1`, created.Prompt.ID).Scan(&oldVersion); err != nil {
		t.Fatalf("读取归档评分所属版本失败: %v", err)
	}
	if oldVersion != "v1.0" {
		t.Errorf("归档评分应指向修改前的版本，实际 %s", oldVersion)
	}

	// 统计快照失效，等待阶段 3 按新版本重算。
	var statsLeft int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM prompt_stats WHERE prompt_id = $1`, created.Prompt.ID).Scan(&statsLeft); err != nil {
		t.Fatalf("统计快照残留检查失败: %v", err)
	}
	if statsLeft != 0 {
		t.Errorf("重大修改后统计快照应被清空，实际残留 %d 行", statsLeft)
	}

	// 前台标记：详情页据此展示提示文案（需求 §7.3 第 5 步）。
	detail, err := f.svc.Get(ctx, domain.ViewerFor(user), created.Prompt.ID, "")
	if err != nil {
		t.Fatalf("读取提示词失败: %v", err)
	}
	if !detail.Prompt.MajorModified {
		t.Error("重大修改后提示词应被标记为已发生重大修改")
	}
	if detail.Version.Title != "重大修改后的标题" {
		t.Error("重大修改后前台应展示新版本")
	}
}

func TestUpdateByNonOwnerReturnsNotFound(t *testing.T) {
	// 无权限对象统一 404，不通过状态码暴露对象存在性（开发方案 §5.3）。
	f := newFixture(t)
	owner := f.normalUser(t)
	normal := f.purposeID(t, "capability_boundary")

	created, err := f.svc.Create(context.Background(), owner, f.baseInput(t, normal, domain.VisibilityPublic))
	if err != nil {
		t.Fatalf("创建提示词失败: %v", err)
	}

	other := f.normalUser(t)
	in := f.baseInput(t, normal, domain.VisibilityPublic)
	_, err = f.svc.Update(context.Background(), other, created.Prompt.ID, in)
	assertAppErrCode(t, err, apperr.CodeNotFound)
}

func TestUnverifiedUserCannotCreatePrompt(t *testing.T) {
	// C-23：未验证账户可浏览但不可上传。
	f := newFixture(t)
	unverified := domain.User{
		ID:     pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Verified: false}),
		Role:   domain.RoleUser,
		Status: domain.StatusUnverified,
	}

	_, err := f.svc.Create(context.Background(), unverified,
		f.baseInput(t, f.purposeID(t, "capability_boundary"), domain.VisibilityPublic))
	assertAppErrCode(t, err, apperr.CodeEmailNotVerified)
}

func TestPrivatePromptQuotaBlocksBeyondLimit(t *testing.T) {
	// 需求 §8.3 与 C-21：额度在事务内判定。
	f := newFixture(t)
	user := f.normalUser(t)
	normal := f.purposeID(t, "capability_boundary")

	limit, err := f.config.GetInt(context.Background(), "private_prompt_quota_user")
	if err != nil {
		t.Fatalf("读取私有提示词额度失败: %v", err)
	}
	if limit != 100 {
		t.Fatalf("私有提示词额度 = %d，需求 §8.3 规定普通用户为 100", limit)
	}

	// 把额度临时调到 2，使用例覆盖「超限」而不必插入 100 条。
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE config SET value = '2'::jsonb WHERE key = 'private_prompt_quota_user'`); err != nil {
		t.Fatalf("调整额度配置失败: %v", err)
	}
	if err := f.config.Refresh(context.Background()); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	for i := 0; i < 2; i++ {
		in := f.baseInput(t, normal, domain.VisibilityPrivate)
		in.Title = fmt.Sprintf("私有提示词 %d", i)
		if _, err := f.svc.Create(context.Background(), user, in); err != nil {
			t.Fatalf("创建第 %d 条私有提示词失败: %v", i+1, err)
		}
	}

	_, err = f.svc.Create(context.Background(), user, f.baseInput(t, normal, domain.VisibilityPrivate))
	assertAppErrCode(t, err, apperr.CodeQuotaExceeded)
}

// TestPrivatePromptQuotaUnderConcurrentCreates 覆盖 C-21 的并发场景：
// 同一用户并发创建私有提示词时，额度判定在锁内串行，不会超卖。
func TestPrivatePromptQuotaUnderConcurrentCreates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	user := f.normalUser(t)
	normal := f.purposeID(t, "capability_boundary")

	if _, err := f.pool.Exec(ctx,
		`UPDATE config SET value = '3'::jsonb WHERE key = 'private_prompt_quota_user'`); err != nil {
		t.Fatalf("调整额度配置失败: %v", err)
	}
	if err := f.config.Refresh(ctx); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	const workers = 8
	results := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := f.baseInput(t, normal, domain.VisibilityPrivate)
			in.Title = fmt.Sprintf("并发私有提示词 %d", i)
			_, results[i] = f.svc.Create(ctx, user, in)
		}(i)
	}
	wg.Wait()

	success := 0
	for _, err := range results {
		if err == nil {
			success++
			continue
		}
		assertAppErrCode(t, err, apperr.CodeQuotaExceeded)
	}
	if success != 3 {
		t.Fatalf("并发创建成功 %d 条，期望恰为额度 3 条", success)
	}
	var count int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM prompts WHERE uploader_id = $1 AND visibility = 'private'`, user.ID).Scan(&count); err != nil {
		t.Fatalf("统计私有提示词失败: %v", err)
	}
	if count != 3 {
		t.Fatalf("库内私有提示词 %d 条，期望 3 条（未超卖）", count)
	}
}

func TestPublicPromptsDoNotConsumePrivateQuota(t *testing.T) {
	// 需求 §8.2：公开提示词不占用用户私有提示词额度。
	f := newFixture(t)
	user := f.normalUser(t)
	normal := f.purposeID(t, "capability_boundary")

	if _, err := f.pool.Exec(context.Background(),
		`UPDATE config SET value = '1'::jsonb WHERE key = 'private_prompt_quota_user'`); err != nil {
		t.Fatalf("调整额度配置失败: %v", err)
	}
	if err := f.config.Refresh(context.Background()); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	for i := 0; i < 3; i++ {
		in := f.baseInput(t, normal, domain.VisibilityPublic)
		in.Title = fmt.Sprintf("公开提示词 %d", i)
		if _, err := f.svc.Create(context.Background(), user, in); err != nil {
			t.Fatalf("创建第 %d 条公开提示词失败: %v", i+1, err)
		}
	}

	in := f.baseInput(t, normal, domain.VisibilityPrivate)
	if _, err := f.svc.Create(context.Background(), user, in); err != nil {
		t.Fatalf("公开提示词不应占用私有额度，创建私有提示词失败: %v", err)
	}
}

func seedRatings(t *testing.T, pool *pgxpool.Pool, userID, promptID int64, modelCount int) []int64 {
	t.Helper()
	ctx := context.Background()

	var versionID int64
	if err := pool.QueryRow(ctx,
		`SELECT current_version_id FROM prompts WHERE id = $1`, promptID).Scan(&versionID); err != nil {
		t.Fatalf("读取当前版本失败: %v", err)
	}

	ids := make([]int64, 0, modelCount)
	for i := 0; i < modelCount; i++ {
		modelKey := fmt.Sprintf("test-model-%d-%d", promptID, i)
		var modelID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO models (name, model_key, provider_key)
			VALUES ($1, $1, 'test')
			RETURNING id`, modelKey).Scan(&modelID); err != nil {
			t.Fatalf("创建测试模型失败: %v", err)
		}
		var ratingID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO ratings (user_id, prompt_id, prompt_version_id, model_id, score)
			VALUES ($1, $2, $3, $4, 4)
			RETURNING id`, userID, promptID, versionID, modelID).Scan(&ratingID); err != nil {
			t.Fatalf("创建测试评分失败: %v", err)
		}
		ids = append(ids, ratingID)
	}
	return ids
}

// assertNoRows 断言查询没有命中任何行。
func assertNoRows(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("期望无结果，实际查到一行")
	}
	if !isNoRows(err) {
		t.Fatalf("期望无结果，实际错误为 %v", err)
	}
}

// TestPurgeAdjustsFavoriteItemCounts 覆盖收藏夹计数校正：提示词彻底删除时
// 随级联清除的收藏条目不再残留 item_count，收藏夹容量不被虚高侵蚀。
func TestPurgeAdjustsFavoriteItemCounts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	user := f.normalUser(t)
	promptID := pgtest.SeedPrompt(t, f.pool, user.ID, "被收藏的提示词")

	var favoriteID int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO favorites (user_id, name, description, visibility)
		VALUES ($1, '计数收藏夹', '', 'private') RETURNING id`, user.ID).Scan(&favoriteID); err != nil {
		t.Fatalf("插入收藏夹失败: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO favorite_items (favorite_id, prompt_id) VALUES ($1, $2)`, favoriteID, promptID); err != nil {
		t.Fatalf("插入收藏条目失败: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`UPDATE favorites SET item_count = 1 WHERE id = $1`, favoriteID); err != nil {
		t.Fatalf("设置条目计数失败: %v", err)
	}

	if err := f.svc.Delete(ctx, user, promptID, "127.0.0.1"); err != nil {
		t.Fatalf("软删除提示词失败: %v", err)
	}
	if err := f.svc.Purge(ctx, user, promptID, "127.0.0.1"); err != nil {
		t.Fatalf("彻底删除提示词失败: %v", err)
	}

	var count int32
	if err := f.pool.QueryRow(ctx,
		`SELECT item_count FROM favorites WHERE id = $1`, favoriteID).Scan(&count); err != nil {
		t.Fatalf("读取收藏夹计数失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("彻底删除后收藏夹计数 = %d，期望 0", count)
	}
}

func TestCreateAndUpdateMultipleCapabilities(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	user := f.normalUser(t)
	purposeID := f.purposeID(t, "capability_boundary")

	reasoning := f.capabilityID(t, "reasoning")
	coding := f.capabilityID(t, "coding")
	toolUse := f.capabilityID(t, "tool_use")
	language := f.capabilityID(t, "language")
	longContext := f.capabilityID(t, "long_context")

	input := f.baseInput(t, purposeID, domain.VisibilityPublic)
	input.CapabilityIDs = []int64{reasoning, coding, toolUse}
	created, err := f.svc.Create(ctx, user, input)
	if err != nil {
		t.Fatalf("创建多维度提示词失败: %v", err)
	}
	if created.Prompt.CapabilityID != reasoning {
		t.Fatalf("主考察维度 = %d，期望首个维度 %d", created.Prompt.CapabilityID, reasoning)
	}

	viewer := domain.ViewerFor(user)
	detail, err := f.svc.Get(ctx, viewer, created.Prompt.ID, "")
	if err != nil {
		t.Fatalf("读取详情失败: %v", err)
	}
	if len(detail.Capabilities) != 3 {
		t.Fatalf("详情考察维度 = %d 个，期望 3 个", len(detail.Capabilities))
	}
	wantOrder := []int64{reasoning, coding, toolUse}
	for index, capability := range detail.Capabilities {
		if capability.ID != wantOrder[index] {
			t.Fatalf("第 %d 个考察维度 = %d，期望 %d", index+1, capability.ID, wantOrder[index])
		}
	}
	if len(detail.Version.CapabilityIDs) != 3 {
		t.Fatalf("版本考察维度 = %v，期望 3 个", detail.Version.CapabilityIDs)
	}

	// 列表筛选命中第二、第三个维度，而不是只看主维度。
	list, err := repo.PromptListPage(ctx, f.pool, viewer,
		repo.PromptListFilter{CapabilityID: &toolUse}, domain.SortLatest, 1, 20)
	if err != nil {
		t.Fatalf("按考察维度筛选失败: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || len(list.Items[0].Prompt.CapabilityIDs) != 3 {
		t.Fatalf("筛选结果 = %+v，期望命中该提示词并带出 3 个维度", list.Items)
	}

	// 更新整体替换维度集合。
	update := input
	update.Title = "多维度提示词（改）"
	update.Body = "更新后的正文"
	update.CapabilityIDs = []int64{language, longContext}
	update.CapabilityID = 0
	if _, err := f.svc.Update(ctx, user, created.Prompt.ID, update); err != nil {
		t.Fatalf("更新多维度提示词失败: %v", err)
	}
	after, err := f.svc.Get(ctx, viewer, created.Prompt.ID, "")
	if err != nil {
		t.Fatalf("读取更新后的详情失败: %v", err)
	}
	if len(after.Capabilities) != 2 || after.Capabilities[0].ID != language || after.Capabilities[1].ID != longContext {
		t.Fatalf("更新后考察维度 = %+v，期望 language、long_context", after.Capabilities)
	}
	if after.Prompt.CapabilityID != language {
		t.Fatalf("更新后主考察维度 = %d，期望 %d", after.Prompt.CapabilityID, language)
	}

	// 六个维度超出上限，创建与更新都应拒绝。
	over := f.baseInput(t, purposeID, domain.VisibilityPublic)
	over.CapabilityIDs = []int64{
		reasoning, coding, toolUse, language, longContext, f.capabilityID(t, "math"),
	}
	_, err = f.svc.Create(ctx, user, over)
	assertAppErrCode(t, err, apperr.CodeInvalidClassification)
}

func isNoRows(err error) bool {
	return err == pgx.ErrNoRows
}
