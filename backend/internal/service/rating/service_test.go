package rating

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/promptbench/backend/internal/apperr"
	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/platform/pgtest"
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
	if err := cfgStore.Refresh(context.Background()); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return &fixture{pool: pool, svc: New(Deps{Pool: pool, Config: cfgStore, Logger: logger}), config: cfgStore}
}

func (f *fixture) user(t *testing.T) domain.User {
	t.Helper()
	return domain.User{
		ID:     pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Verified: true}),
		Role:   domain.RoleUser,
		Status: domain.StatusNormal,
	}
}

// promptAndModel 建立一条提示词与一个模型，返回二者的 ID。
func (f *fixture) promptAndModel(t *testing.T, uploaderID int64) (promptID, modelID int64) {
	t.Helper()
	promptID = pgtest.SeedPrompt(t, f.pool, uploaderID, "被评分的提示词")

	var categoryID int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT current_version_id FROM prompts WHERE id = $1`, promptID).Scan(&categoryID); err != nil {
		t.Fatalf("读取当前版本失败: %v", err)
	}

	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO models (name, model_key, provider_key)
		 VALUES ('测试模型', 'test-model-' || $1::text, 'test') RETURNING id`,
		fmt.Sprint(promptID)).Scan(&modelID); err != nil {
		t.Fatalf("创建测试模型失败: %v", err)
	}
	return promptID, modelID
}

func assertCode(t *testing.T, err error, want apperr.Code) {
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

func TestSubmitRejectsOutOfScaleScores(t *testing.T) {
	// C-01、需求 §3.2：量表为 1-5 整数，空值不构成评分。
	f := newFixture(t)
	user := f.user(t)

	// 合法分值与非法分值走不同的提示词，避免「已评过分」掩盖量表校验。
	for _, score := range []int{0, -1, 6, 100} {
		promptID, modelID := f.promptAndModel(t, user.ID)
		_, err := f.svc.Submit(context.Background(), user,
			SubmitInput{PromptID: promptID, ModelID: modelID, Score: score}, "")
		assertCode(t, err, apperr.CodeValidationFailed)
	}

	for score := domain.ScoreMin; score <= domain.ScoreMax; score++ {
		promptID, modelID := f.promptAndModel(t, user.ID)
		if _, err := f.svc.Submit(context.Background(), user,
			SubmitInput{PromptID: promptID, ModelID: modelID, Score: score}, ""); err != nil {
			t.Errorf("分数 %d 应被接受，实际 %v", score, err)
		}
	}
}

func TestSubmitRejectsDuplicateForSamePromptAndModel(t *testing.T) {
	f := newFixture(t)
	user := f.user(t)
	promptID, modelID := f.promptAndModel(t, user.ID)

	if _, err := f.svc.Submit(context.Background(), user,
		SubmitInput{PromptID: promptID, ModelID: modelID, Score: 4}, ""); err != nil {
		t.Fatalf("首次提交失败: %v", err)
	}
	_, err := f.svc.Submit(context.Background(), user,
		SubmitInput{PromptID: promptID, ModelID: modelID, Score: 5}, "")
	assertCode(t, err, apperr.CodeRatingExists)
}

func TestEditLimitStopsAtFourthAttempt(t *testing.T) {
	// 需求 §3.4：单条评分最多修改 3 次，第 4 次被拒。
	f := newFixture(t)
	user := f.user(t)
	promptID, modelID := f.promptAndModel(t, user.ID)

	if _, err := f.svc.Submit(context.Background(), user,
		SubmitInput{PromptID: promptID, ModelID: modelID, Score: 1}, ""); err != nil {
		t.Fatalf("提交失败: %v", err)
	}

	for i := 1; i <= 3; i++ {
		updated, err := f.svc.Update(context.Background(), user,
			SubmitInput{PromptID: promptID, ModelID: modelID, Score: i + 1}, "")
		if err != nil {
			t.Fatalf("第 %d 次修改应被允许，实际 %v", i, err)
		}
		if updated.EditCount != int32(i) {
			t.Errorf("第 %d 次修改后 edit_count = %d，期望 %d", i, updated.EditCount, i)
		}
	}

	_, err := f.svc.Update(context.Background(), user,
		SubmitInput{PromptID: promptID, ModelID: modelID, Score: 5}, "")
	assertCode(t, err, apperr.CodeRatingEditLimitExceeded)

	// 被拒之后评分未被改写。
	var score int16
	var editCount int32
	if err := f.pool.QueryRow(context.Background(),
		`SELECT score, edit_count FROM ratings WHERE prompt_id = $1 AND model_id = $2`,
		promptID, modelID).Scan(&score, &editCount); err != nil {
		t.Fatalf("读取评分失败: %v", err)
	}
	if score != 4 || editCount != 3 {
		t.Errorf("被拒的修改不应生效，实际 score=%d edit_count=%d", score, editCount)
	}
}

func TestDailyEditLimitResetsAtUTCMidnight(t *testing.T) {
	// 需求 §3.4 与 C-03：每日修改上限 10 次，按 UTC 自然日重置。
	// 用例把上限临时调到 2，使边界行为可在少量操作内验证。
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.pool.Exec(ctx,
		`UPDATE config SET value = '2'::jsonb WHERE key = 'rating_daily_edit_limit'`); err != nil {
		t.Fatalf("调整日限额失败: %v", err)
	}
	if err := f.config.Refresh(ctx); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	promptIDs := make([]int64, 0, 4)
	modelIDs := make([]int64, 0, 4)
	for i := 0; i < 4; i++ {
		author := f.user(t)
		promptID, modelID := f.promptAndModel(t, author.ID)
		promptIDs = append(promptIDs, promptID)
		modelIDs = append(modelIDs, modelID)
	}

	// 同一账户对 3 条不同提示词评分，才能在一天内触发日限额。
	rater := f.user(t)
	for i := range promptIDs {
		if _, err := f.svc.Submit(ctx, rater, SubmitInput{
			PromptID: promptIDs[i], ModelID: modelIDs[i], Score: 3}, ""); err != nil {
			t.Fatalf("提交第 %d 条评分失败: %v", i+1, err)
		}
	}

	// 当日改 2 次后，第 3 次被日限额拦下。
	for i := 0; i < 2; i++ {
		if _, err := f.svc.Update(ctx, rater, SubmitInput{
			PromptID: promptIDs[i], ModelID: modelIDs[i], Score: 4}, ""); err != nil {
			t.Fatalf("当日第 %d 次修改失败: %v", i+1, err)
		}
	}
	_, err := f.svc.Update(ctx, rater, SubmitInput{
		PromptID: promptIDs[2], ModelID: modelIDs[2], Score: 4}, "")
	assertCode(t, err, apperr.CodeQuotaExceeded)

	// 把已产生的修改日志移到 UTC 次日，使新窗口的计数归零。日志时间决定日限额
	// 计数，移动时钟本身不会改变既有记录的落点。
	if _, err := f.pool.Exec(ctx,
		`UPDATE rating_change_logs SET created_at = '2026-10-11T09:00:00Z'
		 WHERE user_id = $1 AND action = 'update'`, rater.ID); err != nil {
		t.Fatalf("调整变更日志时间失败: %v", err)
	}

	// 时钟移到 UTC 次日，同一操作重新放行。时间源注入而非等待，
	// 使边界行为可确定地验证。用第 4 条评分承载这次修改，
	// 避免第 3 条的记录触及单条修改上限，使两种限制互相干扰。
	f.svc.now = func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }
	if _, err := f.svc.Update(ctx, rater, SubmitInput{
		PromptID: promptIDs[3], ModelID: modelIDs[3], Score: 4}, ""); err != nil {
		t.Errorf("UTC 次日应重新放行修改，实际 %v", err)
	}
}

func TestDailyLimitWindowIsUTCNaturalDay(t *testing.T) {
	// C-03：窗口为 [当日 00:00, 次日 00:00) 且以 UTC 切分。
	// 同一条评分变更日志，落在窗口内计入、落在窗口外不计入。
	f := newFixture(t)
	user := f.user(t)
	ctx := context.Background()
	promptID, modelID := f.promptAndModel(t, user.ID)

	if _, err := f.svc.Submit(ctx, user, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 3}, ""); err != nil {
		t.Fatalf("提交失败: %v", err)
	}

	var ratingID int64
	if err := f.pool.QueryRow(ctx,
		`SELECT id FROM ratings WHERE prompt_id = $1 AND model_id = $2`, promptID, modelID).Scan(&ratingID); err != nil {
		t.Fatalf("读取评分失败: %v", err)
	}

	if _, err := f.pool.Exec(ctx,
		`INSERT INTO rating_change_logs (rating_id, user_id, prompt_id, model_id, action)
		 VALUES ($1, $2, $3, $4, 'update')`, ratingID, user.ID, promptID, modelID); err != nil {
		t.Fatalf("写入变更日志失败: %v", err)
	}

	// 窗口左边界本身计入当日。
	f.svc.now = func() time.Time { return time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC) }
	if _, err := f.pool.Exec(ctx,
		`UPDATE rating_change_logs SET created_at = '2026-10-11T00:00:00Z'`); err != nil {
		t.Fatalf("调整日志时间失败: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`UPDATE config SET value = '1'::jsonb WHERE key = 'rating_daily_edit_limit'`); err != nil {
		t.Fatalf("调整日限额失败: %v", err)
	}
	if err := f.config.Refresh(ctx); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	// 日限额 1，日志时间恰为窗口起点，计数已达上限 → 被拒。
	_, err := f.svc.Update(ctx, user, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 5}, "")
	assertCode(t, err, apperr.CodeQuotaExceeded)

	// 同一条日志移到前一日窗口之外，计数归零 → 放行。
	if _, err := f.pool.Exec(ctx,
		`UPDATE rating_change_logs SET created_at = '2026-10-10T23:59:59Z'`); err != nil {
		t.Fatalf("调整日志时间失败: %v", err)
	}
	if _, err := f.svc.Update(ctx, user, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 5}, ""); err != nil {
		t.Errorf("窗口之外的日志不应计入当日限额，实际 %v", err)
	}
}

// TestDailyEditLimitUnderConcurrentUpdates 覆盖每日限额的并发场景：
// 同一用户并发修改评分时，计数与写入在锁内串行，成功次数不超过限额。
func TestDailyEditLimitUnderConcurrentUpdates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	user := f.user(t)

	if _, err := f.pool.Exec(ctx,
		`UPDATE config SET value = '2'::jsonb WHERE key = 'rating_daily_edit_limit'`); err != nil {
		t.Fatalf("调整日限额失败: %v", err)
	}
	if err := f.config.Refresh(ctx); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	promptID, modelID := f.promptAndModel(t, user.ID)
	if _, err := f.svc.Submit(ctx, user, SubmitInput{
		PromptID: promptID, ModelID: modelID, Score: 1}, ""); err != nil {
		t.Fatalf("首次评分失败: %v", err)
	}

	const workers = 6
	results := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = f.svc.Update(ctx, user, SubmitInput{
				PromptID: promptID, ModelID: modelID, Score: 2 + i%3}, "")
		}(i)
	}
	wg.Wait()

	success := 0
	for _, err := range results {
		if err == nil {
			success++
			continue
		}
		assertCode(t, err, apperr.CodeQuotaExceeded)
	}
	if success != 2 {
		t.Fatalf("并发修改成功 %d 次，期望恰为每日限额 2 次", success)
	}
	var updates int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM rating_change_logs WHERE prompt_id = $1 AND action = 'update'`, promptID).Scan(&updates); err != nil {
		t.Fatalf("统计修改日志失败: %v", err)
	}
	if updates != 2 {
		t.Fatalf("修改日志 %d 条，期望 2 条（未超卖）", updates)
	}
}

func TestDeleteRespectsDailyLimit(t *testing.T) {
	// 需求 §3.4：每日删除上限 5 次。
	f := newFixture(t)
	user := f.user(t)
	ctx := context.Background()

	if _, err := f.pool.Exec(ctx,
		`UPDATE config SET value = '1'::jsonb WHERE key = 'rating_daily_delete_limit'`); err != nil {
		t.Fatalf("调整日限额失败: %v", err)
	}
	if err := f.config.Refresh(ctx); err != nil {
		t.Fatalf("刷新配置缓存失败: %v", err)
	}

	var ids []int64
	var modelIDs []int64
	for i := 0; i < 2; i++ {
		promptID, modelID := f.promptAndModel(t, user.ID)
		ids = append(ids, promptID)
		modelIDs = append(modelIDs, modelID)
		if _, err := f.svc.Submit(ctx, user, SubmitInput{
			PromptID: promptID, ModelID: modelID, Score: 3}, ""); err != nil {
			t.Fatalf("提交失败: %v", err)
		}
	}

	if err := f.svc.Delete(ctx, user, ids[0], modelIDs[0], ""); err != nil {
		t.Fatalf("首次删除失败: %v", err)
	}
	err := f.svc.Delete(ctx, user, ids[1], modelIDs[1], "")
	assertCode(t, err, apperr.CodeQuotaExceeded)

	// 删除后重新评分视为新评分，可再次删除，因为新评分对应新的删除记录上限判定。
}

func TestConcurrentSubmitKeepsExactlyOneRating(t *testing.T) {
	// 评分唯一性由部分唯一索引与 ON CONFLICT DO NOTHING 保证（开发方案 §3.5）。
	f := newFixture(t)
	user := f.user(t)
	promptID, modelID := f.promptAndModel(t, user.ID)

	const workers = 8
	var wg sync.WaitGroup
	codes := make([]apperr.Code, workers)
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, err := f.svc.Submit(context.Background(), user,
				SubmitInput{PromptID: promptID, ModelID: modelID, Score: 3}, "")
			var appErr *apperr.Error
			switch {
			case err == nil:
				codes[idx] = ""
			case asAppError(err, &appErr):
				codes[idx] = appErr.Code
			default:
				codes[idx] = apperr.Code("unexpected:" + err.Error())
			}
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded, conflicts := 0, 0
	for _, code := range codes {
		switch code {
		case "":
			succeeded++
		case apperr.CodeRatingExists:
			conflicts++
		default:
			t.Errorf("并发提交出现非预期错误码 %q", code)
		}
	}
	if succeeded != 1 {
		t.Errorf("并发提交成功次数 = %d，期望恰好 1", succeeded)
	}
	if conflicts != workers-1 {
		t.Errorf("并发提交冲突次数 = %d，期望 %d", conflicts, workers-1)
	}

	var count int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ratings WHERE prompt_id = $1 AND model_id = $2 AND deleted_at IS NULL`,
		promptID, modelID).Scan(&count); err != nil {
		t.Fatalf("统计评分失败: %v", err)
	}
	if count != 1 {
		t.Errorf("库中活跃评分数 = %d，期望 1", count)
	}
}

func TestConcurrentEditNeverExceedsPerRecordLimit(t *testing.T) {
	// 条件 UPDATE 保证单条修改次数上限不被并发突破（开发方案 §3.5）。
	f := newFixture(t)
	user := f.user(t)
	ctx := context.Background()
	promptID, modelID := f.promptAndModel(t, user.ID)

	if _, err := f.svc.Submit(ctx, user, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 1}, ""); err != nil {
		t.Fatalf("提交失败: %v", err)
	}

	const workers = 6
	var wg sync.WaitGroup
	start := make(chan struct{})
	allowed := make([]bool, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, err := f.svc.Update(context.Background(), user,
				SubmitInput{PromptID: promptID, ModelID: modelID, Score: idx % 5}, "")
			allowed[idx] = err == nil
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, ok := range allowed {
		if ok {
			successes++
		}
	}
	if successes > 3 {
		t.Errorf("并发修改成功次数 = %d，超过单条上限 3", successes)
	}

	var editCount int32
	if err := f.pool.QueryRow(ctx,
		`SELECT edit_count FROM ratings WHERE prompt_id = $1 AND model_id = $2`,
		promptID, modelID).Scan(&editCount); err != nil {
		t.Fatalf("读取评分失败: %v", err)
	}
	if editCount > 3 {
		t.Errorf("并发修改后 edit_count = %d，超过上限 3", editCount)
	}
}

func TestRatingChangeLogRecordsEveryAction(t *testing.T) {
	// 需求 §3.4「是否记录日志：是」。日限额也以本表为计数依据。
	f := newFixture(t)
	user := f.user(t)
	ctx := context.Background()
	promptID, modelID := f.promptAndModel(t, user.ID)

	if _, err := f.svc.Submit(ctx, user, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 2}, ""); err != nil {
		t.Fatalf("提交失败: %v", err)
	}
	if _, err := f.svc.Update(ctx, user, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 4}, "ip"); err != nil {
		t.Fatalf("修改失败: %v", err)
	}
	if err := f.svc.Delete(ctx, user, promptID, modelID, "ip"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	rows, err := f.pool.Query(ctx,
		`SELECT action, ip FROM rating_change_logs WHERE prompt_id = $1 ORDER BY id`, promptID)
	if err != nil {
		t.Fatalf("查询变更日志失败: %v", err)
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var action, ip string
		if err := rows.Scan(&action, &ip); err != nil {
			t.Fatalf("扫描变更日志失败: %v", err)
		}
		actions = append(actions, action)
	}
	want := []string{"create", "update", "delete"}
	if len(actions) != len(want) {
		t.Fatalf("变更日志条数 = %d（%v），期望 %d 条", len(actions), actions, len(want))
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Errorf("第 %d 条日志动作 = %s，期望 %s", i+1, actions[i], want[i])
		}
	}
}

func TestUnverifiedUserCannotRate(t *testing.T) {
	// C-23：未验证账户可登录、可浏览，但不可评分。
	f := newFixture(t)
	owner := f.user(t)
	promptID, modelID := f.promptAndModel(t, owner.ID)

	unverified := domain.User{
		ID:     pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Verified: false}),
		Role:   domain.RoleUser,
		Status: domain.StatusUnverified,
	}
	_, err := f.svc.Submit(context.Background(), unverified,
		SubmitInput{PromptID: promptID, ModelID: modelID, Score: 3}, "")
	assertCode(t, err, apperr.CodeEmailNotVerified)
}

func TestRatingInvisiblePromptIsRejected(t *testing.T) {
	// 私有提示词不可被他人评分（需求 §8.3）。判定与详情页共用同一组谓词。
	f := newFixture(t)
	owner := f.user(t)
	other := f.user(t)
	ctx := context.Background()

	promptID := pgtest.SeedPrompt(t, f.pool, owner.ID, "私有提示词")
	if _, err := f.pool.Exec(ctx,
		`UPDATE prompts SET visibility = 'private' WHERE id = $1`, promptID); err != nil {
		t.Fatalf("改为私有失败: %v", err)
	}
	var versionID int64
	if err := f.pool.QueryRow(ctx,
		`SELECT current_version_id FROM prompts WHERE id = $1`, promptID).Scan(&versionID); err != nil {
		t.Fatalf("读取当前版本失败: %v", err)
	}
	var modelID int64
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO models (name, model_key, provider_key)
		 VALUES ('私有测试模型', 'private-test-model', 'test') RETURNING id`).Scan(&modelID); err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	_, err := f.svc.Submit(ctx, other, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 3}, "")
	assertCode(t, err, apperr.CodeNotFound)

	// 上传者本人可以评分自己的私有提示词。
	if _, err := f.svc.Submit(ctx, owner, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 3}, ""); err != nil {
		t.Errorf("上传者应能评分自己的私有提示词，实际 %v", err)
	}
	_ = versionID
}

func TestListByPromptReturnsMeansAndMine(t *testing.T) {
	f := newFixture(t)
	owner := f.user(t)
	rater := f.user(t)
	ctx := context.Background()
	promptID, modelID := f.promptAndModel(t, owner.ID)

	if _, err := f.svc.Submit(ctx, rater, SubmitInput{PromptID: promptID, ModelID: modelID, Score: 5}, ""); err != nil {
		t.Fatalf("提交失败: %v", err)
	}

	items, err := f.svc.ListByPrompt(ctx, domain.ViewerFor(rater), promptID)
	if err != nil {
		t.Fatalf("查询评分汇总失败: %v", err)
	}
	var found bool
	for _, item := range items {
		if item.ModelID != modelID {
			continue
		}
		found = true
		if item.RatingCount != 1 {
			t.Errorf("评分数 = %d，期望 1", item.RatingCount)
		}
		if item.MeanScore != 5 {
			t.Errorf("平均分 = %v，期望 5", item.MeanScore)
		}
		if item.MyScore == nil || *item.MyScore != 5 {
			t.Errorf("当前用户的评分应返回自身取值，实际 %v", item.MyScore)
		}
	}
	if !found {
		t.Fatal("评分汇总中缺少刚提交的模型")
	}

	// 游客不返回任何人的评分。
	guestItems, err := f.svc.ListByPrompt(ctx, domain.Guest(), promptID)
	if err != nil {
		t.Fatalf("查询评分汇总失败: %v", err)
	}
	for _, item := range guestItems {
		if item.MyScore != nil {
			t.Error("游客不应看到任何人的评分")
		}
	}
}

func TestListByPromptToleratesModelsWithoutRatings(t *testing.T) {
	// 开发方案 §6.4 专项回归：「模型有历史评分但近 N 日无评分」是必然出现的形态。
	// 此处取其最简单的等价形态——模型完全没有评分——验证读取路径不失败：
	// LEFT JOIN 后 avg 返回 NULL，未包 COALESCE 时扫描会以
	// "cannot scan NULL into *float64" 失败，详情页 500 且不自愈。
	f := newFixture(t)
	owner := f.user(t)
	promptID, ratedModelID := f.promptAndModel(t, owner.ID)
	ctx := context.Background()

	// 再建一个从未被评分的模型，它在该提示词上没有任何观测值。
	var unratedModelID int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO models (name, model_key, provider_key)
		VALUES ('未被评分的模型', 'never-rated-model', 'test')
		RETURNING id`).Scan(&unratedModelID); err != nil {
		t.Fatalf("创建未被评分的模型失败: %v", err)
	}

	if _, err := f.svc.Submit(ctx, owner, SubmitInput{
		PromptID: promptID, ModelID: ratedModelID, Score: 4}, ""); err != nil {
		t.Fatalf("提交评分失败: %v", err)
	}

	items, err := f.svc.ListByPrompt(ctx, domain.ViewerFor(owner), promptID)
	if err != nil {
		t.Fatalf("读取评分汇总失败: %v", err)
	}

	seen := map[int64]PromptModelRating{}
	for _, item := range items {
		seen[item.ModelID] = item
	}
	if len(seen) != 2 {
		t.Fatalf("评分汇总条目数 = %d，期望 2（已评分与未评分两个模型都要出现）", len(seen))
	}

	unrated, ok := seen[unratedModelID]
	if !ok {
		t.Fatal("未被评分的模型应出现在汇总中，用户才能对它评分")
	}
	if unrated.RatingCount != 0 {
		t.Errorf("未评分模型的评分数 = %d，期望 0", unrated.RatingCount)
	}
	if unrated.MeanScore != 0 {
		t.Errorf("未评分模型的均值 = %v，期望 0", unrated.MeanScore)
	}
	if unrated.MyScore != nil {
		t.Error("未评分模型不应带有当前用户的评分")
	}
}

// TestListMineExcludesHiddenPrompts 覆盖 C-20：我的评分列表复查提示词可见性，
// 已删除或改为受限的提示词不再带出标题。
func TestListMineExcludesHiddenPrompts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	user := f.user(t)

	publicPrompt, publicModel := f.promptAndModel(t, user.ID)
	if _, err := f.svc.Submit(ctx, user, SubmitInput{
		PromptID: publicPrompt, ModelID: publicModel, Score: 3}, ""); err != nil {
		t.Fatalf("提交评分失败: %v", err)
	}

	hiddenPrompt := pgtest.SeedPrompt(t, f.pool, user.ID, "将被隐藏的提示词")
	var hiddenModel int64
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO models (name, model_key, provider_key) VALUES ('隐藏模型', 'hidden-model', 'test') RETURNING id`).Scan(&hiddenModel); err != nil {
		t.Fatalf("插入模型失败: %v", err)
	}
	if _, err := f.svc.Submit(ctx, user, SubmitInput{
		PromptID: hiddenPrompt, ModelID: hiddenModel, Score: 4}, ""); err != nil {
		t.Fatalf("提交评分失败: %v", err)
	}

	// 改为开发者可见：普通用户复查失败，标题不再带出（总数仍含未删除的评分）。
	if _, err := f.pool.Exec(ctx,
		`UPDATE prompts SET visibility = 'developer' WHERE id = $1`, hiddenPrompt); err != nil {
		t.Fatalf("修改可见性失败: %v", err)
	}
	items, total, err := f.svc.ListMine(ctx, domain.ViewerFor(user), 1, 20)
	if err != nil {
		t.Fatalf("查询我的评分失败: %v", err)
	}
	if total != 2 || len(items) != 1 || items[0].PromptTitle != "被评分的提示词" {
		t.Fatalf("受限提示词不应带出标题：total=%d items=%+v", total, items)
	}

	// 软删除隐藏提示词：总数与条目都只含公开提示词。
	if _, err := f.pool.Exec(ctx,
		`UPDATE prompts SET deleted_at = now() WHERE id = $1`, hiddenPrompt); err != nil {
		t.Fatalf("软删除提示词失败: %v", err)
	}
	items, total, err = f.svc.ListMine(ctx, domain.ViewerFor(user), 1, 20)
	if err != nil {
		t.Fatalf("查询我的评分失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].PromptTitle != "被评分的提示词" {
		t.Fatalf("已删除提示词不应出现在列表：total=%d items=%+v", total, items)
	}
}

func asAppError(err error, target **apperr.Error) bool {
	appErr, ok := err.(*apperr.Error)
	if ok {
		*target = appErr
	}
	return ok
}
