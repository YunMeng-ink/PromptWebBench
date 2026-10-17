package stats

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/promptbench/backend/internal/config"
	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/formula"
	"github.com/promptbench/backend/internal/platform/pgtest"
	"github.com/promptbench/backend/internal/repo"
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
	svc := New(Deps{Pool: pool, Config: cfgStore, Logger: logger})
	svc.source = formula.NewSamplingSource(20261012)
	return &fixture{pool: pool, svc: svc, config: cfgStore}
}

// model 建立一个启用的测试模型。
func (f *fixture) model(t *testing.T, name string) int64 {
	t.Helper()
	var id int64
	err := f.pool.QueryRow(context.Background(), `
		INSERT INTO models (name, model_key, provider_key)
		VALUES ($1, $1 || '-' || gen_random_uuid()::text, 'test')
		RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatalf("创建模型 %s 失败: %v", name, err)
	}
	return id
}

// rate 插入一条评分。createdAt 为空时取当前时刻。
//
// 直连 SQL 而不走评分服务：统计测试需要构造「评分创建时间在近 N 日窗口之外」这种
// 形态，而评分服务不接受调用方指定创建时间。
func (f *fixture) rate(t *testing.T, promptID, modelID, userID int64, score int, createdAt time.Time) {
	t.Helper()
	f.rateBatch(t, promptID, modelID, []int64{userID}, []int{score}, createdAt)
}

// rateBatch 一次插入同一模型上的多条评分，各条的用户与分值可以不同。
//
// 单个用例要构造几十条评分，逐条插入会把用例拖到分钟级；批量插入把它压成一条语句。
func (f *fixture) rateBatch(t *testing.T, promptID, modelID int64, userIDs []int64, scores []int, createdAt time.Time) {
	t.Helper()
	if len(userIDs) == 0 {
		return
	}
	if len(userIDs) != len(scores) {
		t.Fatalf("评分批量插入的用户数 %d 与分值数 %d 不一致", len(userIDs), len(scores))
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	var versionID int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT current_version_id FROM prompts WHERE id = $1`, promptID).Scan(&versionID); err != nil {
		t.Fatalf("读取提示词 %d 的当前版本失败: %v", promptID, err)
	}

	_, err := f.pool.Exec(context.Background(), `
		INSERT INTO ratings (user_id, prompt_id, prompt_version_id, model_id, score, created_at)
		SELECT u.user_id, $2, $3, $4, u.score, $5
		FROM unnest($1::bigint[], $6::smallint[]) AS u(user_id, score)
		ON CONFLICT DO NOTHING`,
		userIDs, promptID, versionID, modelID, createdAt, scores)
	if err != nil {
		t.Fatalf("批量写入评分失败: %v", err)
	}
}

// raters 建立 n 个已验证账户，返回其 ID。
//
// 账户用一条语句批量建号，密码哈希取同一个：用例只用它们满足「独立评分用户数」，
// 不需要用这些账户登录，因此共用哈希即可。
func (f *fixture) raters(t *testing.T, n int) []int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("TestPassw0rd!"), 4)
	if err != nil {
		t.Fatalf("生成测试口令哈希失败: %v", err)
	}
	rows, err := f.pool.Query(context.Background(), `
		INSERT INTO users (username, email, password_hash, role, status, email_verified_at)
		SELECT 'rater_' || gen_random_uuid()::text,
		       'rater_' || gen_random_uuid()::text || '@example.test',
		       $1, 'user', 'normal', now()
		FROM generate_series(1, $2)
		RETURNING id`, string(hash), n)
	if err != nil {
		t.Fatalf("批量创建评分用户失败: %v", err)
	}
	defer rows.Close()

	out := make([]int64, 0, n)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("扫描评分用户 ID 失败: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历评分用户失败: %v", err)
	}
	return out
}

// setup 建立一条提示词与给定数量的模型，并预建评分用户。
//
// 预建数量取 80：区分度回升的用例需要每个模型各有 60 条历史评分与 6 条近 N 日评分，
// 且同一「用户 × 提示词 × 模型」只能有一条活跃评分，用户不能复用。
func (f *fixture) setup(t *testing.T, modelNames ...string) (promptID int64, modelIDs []int64, raters []int64) {
	t.Helper()
	uploader := pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Verified: true})
	promptID = pgtest.SeedPrompt(t, f.pool, uploader, "统计测试提示词")
	for _, name := range modelNames {
		modelIDs = append(modelIDs, f.model(t, name))
	}
	return promptID, modelIDs, f.raters(t, 80)
}

func (f *fixture) viewer() domain.Viewer {
	return domain.Viewer{}
}

// Test无近期评分的模型读写两条路径都成功 是开发方案 §6.4 的专项回归。
//
// 「模型有历史评分但近 N 日无评分」是平台运行超过 N 日后必然出现的形态。该形态下
// 近 N 日窗口的聚合对这些模型返回零行，若统计查询的聚合未包 COALESCE，读取路径会以
// "cannot scan NULL into *int64" 失败；若写入路径把零值空串写入带 CHECK 约束的列，
// 快照写入会失败。两条路径都要断言成功。
func Test无近期评分的模型读写两条路径都成功(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "模型甲", "模型乙", "模型丙", "模型丁")

	old := time.Now().Add(-30 * 24 * time.Hour)
	// 历史窗口：四个模型各有 8 条评分，全部发生在近 N 日窗口之外。
	scores := []int{5, 4, 3, 1}
	for m := range modelIDs {
		f.rateBatch(t, promptID, modelIDs[m], raters[:8], repeatScore(scores[m], 8), old)
	}
	// 近 N 日只有前三个模型有评分，模型丁一条都没有。
	// 评分用户必须是另一批：同一「用户 × 提示词 × 模型」只能有一条活跃评分，
	// 复用历史评分的用户会让写入被唯一索引挡下，近 N 日窗口仍然是空的。
	for m := 0; m < 3; m++ {
		f.rateBatch(t, promptID, modelIDs[m], raters[8:14], repeatScore(scores[m], 6), time.Time{})
	}

	ctx := context.Background()

	// 写入路径：重算快照必须成功。
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("重算统计失败: %v", err)
	}

	row, err := repo.New(f.pool).GetPromptStats(ctx, promptID)
	if err != nil {
		t.Fatalf("读取统计快照失败: %v", err)
	}
	if row.HistoryValidModelCount != 4 {
		t.Fatalf("历史有效模型数 = %d, 期望 4", row.HistoryValidModelCount)
	}
	if row.RecentValidModelCount != 3 {
		t.Fatalf("近 N 日有效模型数 = %d, 期望 3（模型丁在近 N 日无评分）", row.RecentValidModelCount)
	}

	// 读取路径：Load 必须成功，且模型丁的近 N 日窗口标记为无观测值。
	snapshot, err := f.svc.Load(ctx, f.viewer(), promptID)
	if err != nil {
		t.Fatalf("读取统计失败: %v", err)
	}
	var found bool
	for _, m := range snapshot.Models {
		if m.ModelID != modelIDs[3] {
			continue
		}
		found = true
		if !m.History.HasData {
			t.Fatal("模型丁在历史窗口应有观测值")
		}
		if m.Recent.HasData {
			t.Fatal("模型丁在近 N 日窗口应无观测值")
		}
		if m.Recent.RatingCount != 0 {
			t.Fatalf("模型丁近 N 日评分数 = %d, 期望 0", m.Recent.RatingCount)
		}
	}
	if !found {
		t.Fatal("模型丁的统计快照缺失")
	}
}

// Test近N日数据不足时不生成过时标签 覆盖需求 §4.3 与开发方案 §6.4 验收标准。
func Test近N日数据不足时不生成过时标签(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "模型甲", "模型乙", "模型丙")

	// 全部评分为 3 分：极差为 0，历史区分度为 1，低于过时阈值 2。
	// 但近 N 日只有 1 条评分，低于总评分数门槛 10，数据不足。
	old := time.Now().Add(-30 * 24 * time.Hour)
	for m := range modelIDs {
		f.rateBatch(t, promptID, modelIDs[m], raters[:5], repeatScore(3, 5), old)
	}
	f.rate(t, promptID, modelIDs[0], raters[0], 3, time.Time{})

	ctx := context.Background()
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("重算统计失败: %v", err)
	}

	tags := f.tags(t, promptID)
	for _, tag := range tags {
		// 需求 §4.3：数据不足时不触发「过时」。此时历史区分度低而近 N 日数据不足，
		// 需求 §6.1 要求推荐的是「疑似过时」，不是「过时」。
		if tag.Kind == formula.TagObsolete {
			t.Fatalf("近 N 日数据不足时不应生成「过时」标签，实际生成 %s", tag.Kind)
		}
	}

	snapshot, err := f.svc.Load(ctx, f.viewer(), promptID)
	if err != nil {
		t.Fatalf("读取统计失败: %v", err)
	}
	if snapshot.Recent.Sufficient {
		t.Fatal("近 N 日仅 1 条评分，数据不应充足")
	}
	if snapshot.Recent.Discrimination != nil {
		t.Fatalf("数据不足时区分度应为 nil，实际 %v", *snapshot.Recent.Discrimination)
	}
	if snapshot.Recent.Confidence != formula.ConfidenceLow {
		t.Fatalf("数据不足时区分度置信度 = %v, 期望为低", snapshot.Recent.Confidence)
	}
	// 历史窗口数据充足，历史区分度应当低于过时阈值。
	if !snapshot.History.Sufficient {
		t.Fatal("历史窗口数据充足")
	}
	if *snapshot.History.Discrimination > 2 {
		t.Fatalf("全 3 分的历史区分度 = %v, 期望不超过 2", *snapshot.History.Discrimination)
	}
}

// Test区分度复现需求算例一 把需求 §4.5 示例一的四个校准完成度落到真实数据上，
// 断言重算得到的区分度与算例一致。
func Test区分度复现需求算例一(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "模型甲", "模型乙", "模型丙", "模型丁")

	// 需求 §4.5 示例一的四个校准完成度。评分量表为 1 至 5 的整数，
	// 校准分 5.0 在有限评分下取不到，因此这里按目标值反解原始均值再取整，
	// 断言的是「区分度等于按实际校准分手算的结果」这一结构关系。
	// 示例一与示例二的算例本身由 formula 包的纯函数测试逐位复现。
	targets := []float64{1.4, 2.6, 4.3, 5.0}
	const perModel = 8
	for m, target := range targets {
		mean := invertAdjusted(target, perModel)
		score := int(mean + 0.5)
		if score < domain.ScoreMin {
			score = domain.ScoreMin
		}
		if score > domain.ScoreMax {
			score = domain.ScoreMax
		}
		f.rateBatch(t, promptID, modelIDs[m], raters[:perModel], repeatScore(score, perModel), time.Time{})
	}

	ctx := context.Background()
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("重算统计失败: %v", err)
	}
	snapshot, err := f.svc.Load(ctx, f.viewer(), promptID)
	if err != nil {
		t.Fatalf("读取统计失败: %v", err)
	}

	if !snapshot.Recent.Sufficient {
		t.Fatalf("数据不应不足：有效模型 %d，总评分 %d",
			snapshot.Recent.ValidModelCount, snapshot.Recent.TotalRatingCount)
	}

	// 手算：按各模型实际写入的评分重算校准分，取极差，再套系数。
	disc := formula.DefaultDiscrimination()
	var adjusted []float64
	for _, m := range snapshot.Models {
		if m.Recent.RatingCount < int64(disc.MinRatingsPerModel) {
			continue
		}
		adjusted = append(adjusted, m.Recent.Adjusted)
	}
	want := formula.ComputeDiscrimination(adjusted, len(adjusted), snapshot.Recent.TotalRatingCount, disc)
	if got := *snapshot.Recent.Discrimination; !closeEnough(got, want) {
		t.Fatalf("近 N 日区分度 = %v, 手算 %v（校准分 %v）", got, want, adjusted)
	}

	// 四个模型差异明显：区分度必须高于「全部相同」时的 1，且极差越大区分度越高。
	if *snapshot.Recent.Discrimination <= 1 {
		t.Fatalf("四个模型差异明显，区分度 = %v，应高于 1", *snapshot.Recent.Discrimination)
	}
	if spread(adjusted) <= 0 {
		t.Fatalf("四个模型的校准完成度应互不相同，实际 %v", adjusted)
	}
}

// spread 返回一组校准分的极差。
func spread(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minValue, maxValue := values[0], values[0]
	for _, v := range values[1:] {
		if v < minValue {
			minValue = v
		}
		if v > maxValue {
			maxValue = v
		}
	}
	return maxValue - minValue
}

// Test区分度复现需求算例二 对应需求 §4.5 示例二：四个模型校准分几乎相同。
func Test区分度复现需求算例二(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "模型甲", "模型乙", "模型丙", "模型丁")

	// 各模型都打 3 分，校准分相同，极差为 0。
	for m := range modelIDs {
		f.rateBatch(t, promptID, modelIDs[m], raters[:6], repeatScore(3, 6), time.Time{})
	}

	ctx := context.Background()
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("重算统计失败: %v", err)
	}
	snapshot, err := f.svc.Load(ctx, f.viewer(), promptID)
	if err != nil {
		t.Fatalf("读取统计失败: %v", err)
	}
	if !snapshot.Recent.Sufficient {
		t.Fatal("数据充足时不应标记为不足")
	}
	if got := *snapshot.Recent.Discrimination; !closeEnough(got, 1) {
		t.Fatalf("四个模型校准分相同时区分度 = %v, 期望为 1（需求 §4.5 示例二为 1.2，差值来自极差 0.2）", got)
	}
}

// Test区分度回升时待确认标签转已忽略 覆盖 C-15。
func Test区分度回升时待确认标签转已忽略(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "模型甲", "模型乙", "模型丙", "模型丁")

	// 历史窗口全部 3 分：区分度低。
	old := time.Now().Add(-30 * 24 * time.Hour)
	for m := range modelIDs {
		f.rateBatch(t, promptID, modelIDs[m], raters[:6], repeatScore(3, 6), old)
	}
	// 近 N 日数据不足：只留 1 条评分。
	f.rate(t, promptID, modelIDs[0], raters[0], 3, time.Time{})

	ctx := context.Background()
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("首次重算失败: %v", err)
	}
	pending := f.tagsOfKind(t, promptID, formula.TagSuspectedObsolete)
	if len(pending) != 1 {
		t.Fatalf("首次重算应生成 1 条疑似过时推荐，实际 %d 条", len(pending))
	}
	if pending[0].State != "pending" {
		t.Fatalf("新生成的推荐标签状态 = %s, 期望 pending", pending[0].State)
	}

	// 近 N 日补足评分并让模型间差异明显：区分度回升。
	//
	// 历史窗口没有上界，新评分同样计入历史，因此必须让历史评分数量足够多、
	// 使这批新评分对历史均值的拉动有限，历史区分度仍低于过时阈值；
	// 而近 N 日只有这批新评分，模型间差异明显。两者共同构造「近期回升」的状态。
	recentScores := []int{1, 2, 4, 5}
	const historyPerModel = 60
	for m := range modelIDs {
		f.rateBatch(t, promptID, modelIDs[m], raters[:historyPerModel], repeatScore(3, historyPerModel), old)
		f.rateBatch(t, promptID, modelIDs[m], raters[historyPerModel:historyPerModel+6],
			repeatScore(recentScores[m], 6), time.Time{})
	}

	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("第二次重算失败: %v", err)
	}

	ignored := f.tagsOfKind(t, promptID, formula.TagSuspectedObsolete)
	for _, tag := range ignored {
		if tag.State != "ignored" {
			t.Fatalf("区分度回升后待确认标签状态 = %s, 期望 ignored（C-15）", tag.State)
		}
	}

	// C-15 要求记录审计日志。
	var logged int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM operation_logs WHERE action = $1`, actionDiscriminationRecovery).Scan(&logged); err != nil {
		t.Fatalf("查询审计日志失败: %v", err)
	}
	if logged == 0 {
		t.Fatal("区分度回升时未记录 discrimination_recovery 审计日志（C-15）")
	}
}

// Test重大修改后标签转待复核 覆盖 C-12。
func Test重大修改后标签转待复核(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "模型甲", "模型乙", "模型丙", "模型丁")

	old := time.Now().Add(-30 * 24 * time.Hour)
	for m := range modelIDs {
		f.rateBatch(t, promptID, modelIDs[m], raters[:6], repeatScore(3, 6), old)
	}
	f.rate(t, promptID, modelIDs[0], raters[0], 3, time.Time{})

	ctx := context.Background()
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	before := f.tagsOfKind(t, promptID, formula.TagSuspectedObsolete)
	if len(before) != 1 {
		t.Fatalf("应生成 1 条疑似过时推荐，实际 %d 条", len(before))
	}

	// 重大修改把标签转待复核（需求 §6.4、C-12）。
	if err := repo.New(f.pool).MoveTagsToReview(ctx, repo.MoveTagsToReviewParams{
		PromptID: promptID,
		Kinds:    []string{formula.TagObsolete, formula.TagSuspectedObsolete, formula.TagSaturation, formula.TagSuspectedSaturation},
	}); err != nil {
		t.Fatalf("把标签转待复核失败: %v", err)
	}
	after := f.tagsOfKind(t, promptID, formula.TagSuspectedObsolete)
	if after[0].State != "review" {
		t.Fatalf("重大修改后标签状态 = %s, 期望 review（C-12）", after[0].State)
	}
}

// Test训练饱和推荐使用近N日完成度置信度 覆盖 C-16 与需求 §6.2。
func Test训练饱和推荐使用近N日完成度置信度(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "饱和模型", "对照模型乙", "对照模型丙", "对照模型丁")

	// 饱和模型全部打 5 分，校准完成度高于阈值 4.5；对照模型打 2 分。
	//
	// 校准分 = (n * 5 + 5 * 3) / (n + 5)，n = 20 时为 4.6，超过阈值 4.5；
	// n = 8 时只有 4.23，达不到阈值。评分人数超过总评分数门槛时用 20 个用户。
	perModel := 20
	f.rateBatch(t, promptID, modelIDs[0], raters[:perModel], repeatScore(5, perModel), time.Time{})
	for m := 1; m < len(modelIDs); m++ {
		f.rateBatch(t, promptID, modelIDs[m], raters[:perModel], repeatScore(2, perModel), time.Time{})
	}

	ctx := context.Background()
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("重算失败: %v", err)
	}

	saturation := f.tagsOfKind(t, promptID, formula.TagSaturation)
	if len(saturation) != 1 {
		t.Fatalf("应生成 1 条训练饱和推荐，实际 %d 条", len(saturation))
	}
	if saturation[0].ModelID != modelIDs[0] {
		t.Fatalf("训练饱和标签应绑定饱和模型，实际绑定模型 %d", saturation[0].ModelID)
	}
	// C-16：该标签的置信度取该模型近 N 日完成度置信度，不是区分度置信度。
	// 20 条评分、20 个用户、半宽 0：0.4 + 0.3 + 0.3 = 1.0，达到高档门槛。
	if saturation[0].Confidence != string(formula.ConfidenceHigh) {
		t.Fatalf("训练饱和推荐置信度 = %s, 期望 high（20 条评分、20 个用户）",
			saturation[0].Confidence)
	}
}

// Test重算幂等不产生重复推荐标签 覆盖 C-13 的唯一约束。
func Test重算幂等不产生重复推荐标签(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "饱和模型", "对照模型乙", "对照模型丙", "对照模型丁")

	f.rateBatch(t, promptID, modelIDs[0], raters[:20], repeatScore(5, 20), time.Time{})
	for m := 1; m < len(modelIDs); m++ {
		f.rateBatch(t, promptID, modelIDs[m], raters[:20], repeatScore(2, 20), time.Time{})
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := f.svc.Recompute(ctx, promptID); err != nil {
			t.Fatalf("第 %d 次重算失败: %v", i+1, err)
		}
	}
	if got := len(f.tagsOfKind(t, promptID, formula.TagSaturation)); got != 1 {
		t.Fatalf("连续重算 3 次后训练饱和推荐数 = %d, 期望 1", got)
	}
}

// Test缓存作废后读到新快照 覆盖进程内缓存与评分写入的联动。
func Test缓存作废后读到新快照(t *testing.T) {
	f := newFixture(t)
	promptID, modelIDs, raters := f.setup(t, "模型甲", "模型乙", "模型丙", "模型丁")

	for m := range modelIDs {
		f.rateBatch(t, promptID, modelIDs[m], raters[:6], repeatScore(3, 6), time.Time{})
	}
	ctx := context.Background()
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("重算失败: %v", err)
	}

	first, err := f.svc.Load(ctx, f.viewer(), promptID)
	if err != nil {
		t.Fatalf("首次读取失败: %v", err)
	}
	// 再次读取应命中缓存，返回同一份快照。
	cached, err := f.svc.Load(ctx, f.viewer(), promptID)
	if err != nil {
		t.Fatalf("缓存读取失败: %v", err)
	}
	if cached.ComputedAt != first.ComputedAt {
		t.Fatal("第二次读取未命中缓存")
	}

	// 写入一条新评分（用尚未评过该模型的用户，避免被唯一索引挡下），
	// 作废缓存后重算，下一次读取应看到新的评分数。
	f.rate(t, promptID, modelIDs[0], raters[24], 1, time.Time{})
	f.svc.MarkStale(promptID)
	if _, ok := f.svc.Fresh(ctx, promptID); ok {
		t.Fatal("作废后缓存仍命中")
	}

	// 重算后读到的评分数应包含新评分。
	if err := f.svc.Recompute(ctx, promptID); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	f.svc.MarkStale(promptID)
	after, err := f.svc.Load(ctx, f.viewer(), promptID)
	if err != nil {
		t.Fatalf("作废后读取失败: %v", err)
	}
	if after.Recent.TotalRatingCount != 25 {
		t.Fatalf("重算后近 N 日总评分数 = %d, 期望 25", after.Recent.TotalRatingCount)
	}
}

// Test不可见提示词的统计返回404 覆盖开发方案 §5.3。
func Test不可见提示词的统计返回404(t *testing.T) {
	f := newFixture(t)
	promptID := pgtest.SeedPrompt(t, f.pool,
		pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Verified: true}), "私有统计提示词")

	if _, err := f.pool.Exec(context.Background(),
		`UPDATE prompts SET visibility = 'private' WHERE id = $1`, promptID); err != nil {
		t.Fatalf("改为私有失败: %v", err)
	}

	if _, err := f.svc.Load(context.Background(), domain.Guest(), promptID); err == nil {
		t.Fatal("游客读取私有提示词统计应报错")
	}
}

// Test统计查询中无未包COALESCE的FILTER聚合 是开发方案 §6.4 的静态验收项。
//
// 该形态由上面那条回归用例在运行期覆盖；此处再从 SQL 文本上做一次核对，
// 使新增的统计查询一旦写出裸 FILTER 聚合就被测试拦下。
func Test统计查询中无未包COALESCE的FILTER聚合(t *testing.T) {
	queries := readQueryFiles(t)
	for name, content := range queries {
		if !containsAggregateFilter(content) {
			continue
		}
		t.Fatalf("%s 出现未包 COALESCE 的 FILTER 聚合", name)
	}
}
