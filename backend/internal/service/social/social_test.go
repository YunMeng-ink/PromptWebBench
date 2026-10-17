package social

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"

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
	t.Cleanup(cfgStore.Close)
	if err := cfgStore.Refresh(context.Background()); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return &fixture{pool: pool, svc: New(Deps{Pool: pool, Config: cfgStore, Logger: logger}), config: cfgStore}
}

// user 建立一个已验证账户并返回其身份。
func (f *fixture) user(t *testing.T) (int64, domain.Viewer) {
	t.Helper()
	id := pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Verified: true})
	return id, domain.Viewer{UserID: id, Role: domain.RoleUser, Logged: true, Writable: true}
}

// developer 建立一个已验证的开发者账户。
func (f *fixture) developer(t *testing.T) (int64, domain.Viewer) {
	t.Helper()
	id := pgtest.SeedUser(t, f.pool, pgtest.UserOptions{Role: "developer", Verified: true})
	return id, domain.Viewer{UserID: id, Role: domain.RoleDeveloper, Logged: true, Writable: true}
}

// prompt 建立一条公开提示词。
func (f *fixture) prompt(t *testing.T, uploaderID int64, title string) int64 {
	t.Helper()
	return pgtest.SeedPrompt(t, f.pool, uploaderID, title)
}

// setPromptVisibility 修改提示词的可见性与所属考察目的的高风险标记。
func (f *fixture) setPromptVisibility(t *testing.T, promptID int64, visibility string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE prompts SET visibility = $2 WHERE id = $1`, promptID, visibility); err != nil {
		t.Fatalf("修改提示词可见性失败: %v", err)
	}
}

// setPromptHighRisk 把提示词的考察目标为高风险。
func (f *fixture) setPromptHighRisk(t *testing.T, promptID int64) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE purposes SET is_high_risk = true
		WHERE id = (SELECT purpose_id FROM prompts WHERE id = $1)`, promptID); err != nil {
		t.Fatalf("标记高风险考察目的失败: %v", err)
	}
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

// TestCreateFavoriteEnforcesQuota 覆盖需求 §9.2：普通用户最多创建 20 个收藏夹。
func TestCreateFavoriteEnforcesQuota(t *testing.T) {
	f := newFixture(t)
	_, author := f.user(t)
	ctx := context.Background()

	for i := 0; i < domain.FavoriteCountLimitUser; i++ {
		if _, err := f.svc.CreateFavorite(ctx, author, FavoriteInput{
			Name: "收藏夹", Visibility: domain.FavoritePrivate,
		}); err != nil {
			t.Fatalf("创建第 %d 个收藏夹失败: %v", i+1, err)
		}
	}
	_, err := f.svc.CreateFavorite(ctx, author, FavoriteInput{
		Name: "超出配额", Visibility: domain.FavoritePrivate,
	})
	assertCode(t, err, apperr.CodeQuotaExceeded)
}

// TestCreateFavoriteConcurrentQuotaNotExceeded 覆盖 C-21：并发创建不突破配额。
//
// 判定若在事务外完成，多个请求会各自读到未超限的计数并同时写入成功。
// 这里让 10 个请求同时抢 3 个名额，断言恰好 3 个成功。
func TestCreateFavoriteConcurrentQuotaNotExceeded(t *testing.T) {
	f := newFixture(t)
	_, author := f.user(t)
	ctx := context.Background()

	// 先把配额降到 3：直接改配置表的键值。
	if err := f.config.Set(ctx, "favorite_count_limit_user", 3, "测试用", 0); err != nil {
		t.Fatalf("设置配额失败: %v", err)
	}

	const attempts = 10
	var (
		mu        sync.Mutex
		succeeded int
		failures  int
	)
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := f.svc.CreateFavorite(ctx, author, FavoriteInput{
				Name: "并发收藏夹", Visibility: domain.FavoritePrivate,
			})
			mu.Lock()
			if err == nil {
				succeeded++
			} else {
				failures++
			}
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if succeeded != 3 {
		t.Fatalf("并发生成了 %d 个收藏夹，期望恰好 3 个（配额 3，共 %d 次尝试）",
			succeeded, attempts)
	}
	if failures != attempts-3 {
		t.Fatalf("失败次数 = %d，期望 %d", failures, attempts-3)
	}

	// 库里的实际行数也必须等于配额，而不是只让调用方看到正确结果。
	var stored int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM favorites WHERE user_id = $1 AND deleted_at IS NULL`,
		author.UserID).Scan(&stored); err != nil {
		t.Fatalf("统计收藏夹失败: %v", err)
	}
	if stored != 3 {
		t.Fatalf("库中收藏夹数 = %d，期望 3", stored)
	}
}

// TestFavoriteItemQuotaNotExceededUnderConcurrency 覆盖条目上限的并发语义。
func TestFavoriteItemQuotaNotExceededUnderConcurrency(t *testing.T) {
	f := newFixture(t)
	ownerID, owner := f.user(t)
	ctx := context.Background()

	// 把条目上限降到 5，并准备 12 条提示词。
	if err := f.config.Set(ctx, "favorite_item_limit", 5, "测试用", 0); err != nil {
		t.Fatalf("设置条目上限失败: %v", err)
	}
	promptIDs := make([]int64, 0, 12)
	for i := 0; i < 12; i++ {
		promptIDs = append(promptIDs, f.prompt(t, ownerID, "条目测试"))
	}

	favorite, err := f.svc.CreateFavorite(ctx, owner, FavoriteInput{
		Name: "条目配额", Visibility: domain.FavoritePrivate,
	})
	if err != nil {
		t.Fatalf("创建收藏夹失败: %v", err)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, promptID := range promptIDs {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			_ = f.svc.AddItem(ctx, owner, favorite.Entity.ID, id)
		}(promptID)
	}
	close(start)
	wg.Wait()

	var stored int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM favorite_items WHERE favorite_id = $1`,
		favorite.Entity.ID).Scan(&stored); err != nil {
		t.Fatalf("统计条目失败: %v", err)
	}
	if stored > 5 {
		t.Fatalf("收藏夹内条目数 = %d，超过上限 5", stored)
	}

	// 计数列必须与实际条目数一致：条件 UPDATE 未命中时插入必须回滚。
	var counted int32
	if err := f.pool.QueryRow(ctx,
		`SELECT item_count FROM favorites WHERE id = $1`, favorite.Entity.ID).Scan(&counted); err != nil {
		t.Fatalf("读取条目计数失败: %v", err)
	}
	if int64(counted) != stored {
		t.Fatalf("条目计数 = %d，实际条目数 = %d，两者必须一致", counted, stored)
	}
}

// TestFavoriteItemsRecheckVisibility 覆盖开发方案 §6.5 验收标准：
// 收藏夹中被改为私有或高风险的提示词不再展示。
func TestFavoriteItemsRecheckVisibility(t *testing.T) {
	f := newFixture(t)
	ownerID, owner := f.user(t)
	_, visitor := f.user(t)
	ctx := context.Background()

	publicPrompt := f.prompt(t, ownerID, "公开条目")
	privatePrompt := f.prompt(t, ownerID, "将被改为私有")

	favorite, err := f.svc.CreateFavorite(ctx, owner, FavoriteInput{
		Name: "公开收藏夹", Visibility: domain.FavoritePublic,
	})
	if err != nil {
		t.Fatalf("创建收藏夹失败: %v", err)
	}
	for _, id := range []int64{publicPrompt, privatePrompt} {
		if err := f.svc.AddItem(ctx, owner, favorite.Entity.ID, id); err != nil {
			t.Fatalf("加入收藏夹失败: %v", err)
		}
	}

	// 加入时两条都可见。
	page, err := f.svc.ListItems(ctx, visitor, favorite.Entity.ID, 1, 20)
	if err != nil {
		t.Fatalf("读取条目失败: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("加入后可见条目数 = %d，期望 2", len(page.Items))
	}

	// 把其中一条改为私有：它对其他用户不再可见，但仍在夹内。
	f.setPromptVisibility(t, privatePrompt, "private")

	page, err = f.svc.ListItems(ctx, visitor, favorite.Entity.ID, 1, 20)
	if err != nil {
		t.Fatalf("读取条目失败: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("改私有后可见条目数 = %d，期望 1", len(page.Items))
	}
	if page.Items[0].Prompt.ID != publicPrompt {
		t.Fatalf("剩余条目 = %d，期望公开的那条 %d", page.Items[0].Prompt.ID, publicPrompt)
	}
	if page.Total != 1 {
		t.Fatalf("总数 = %d，期望与列表口径一致为 1", page.Total)
	}

	// 所有者本人仍能看到两条。
	own, err := f.svc.ListItems(ctx, owner, favorite.Entity.ID, 1, 20)
	if err != nil {
		t.Fatalf("所有者读取条目失败: %v", err)
	}
	if len(own.Items) != 2 {
		t.Fatalf("所有者可见条目数 = %d，期望 2", len(own.Items))
	}
}

// TestFavoriteItemsHideHighRisk 覆盖 C-20 在收藏夹条目上的表现。
func TestFavoriteItemsHideHighRisk(t *testing.T) {
	f := newFixture(t)
	ownerID, owner := f.developer(t)
	_, ordinary := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "高风险条目")
	favorite, err := f.svc.CreateFavorite(ctx, owner, FavoriteInput{
		Name: "高风险收藏夹", Visibility: domain.FavoritePublic,
	})
	if err != nil {
		t.Fatalf("创建收藏夹失败: %v", err)
	}
	if err := f.svc.AddItem(ctx, owner, favorite.Entity.ID, promptID); err != nil {
		t.Fatalf("加入收藏夹失败: %v", err)
	}

	// 开发者本就能看到高风险内容。
	developerView, err := f.svc.ListItems(ctx, owner, favorite.Entity.ID, 1, 20)
	if err != nil {
		t.Fatalf("开发者读取条目失败: %v", err)
	}
	if len(developerView.Items) != 1 {
		t.Fatalf("开发者可见条目数 = %d，期望 1", len(developerView.Items))
	}

	// 考察目的被标为高风险后，普通用户不再能在公开收藏夹里看到它。
	f.setPromptHighRisk(t, promptID)

	ordinaryView, err := f.svc.ListItems(ctx, ordinary, favorite.Entity.ID, 1, 20)
	if err != nil {
		t.Fatalf("普通用户读取条目失败: %v", err)
	}
	if len(ordinaryView.Items) != 0 {
		t.Fatalf("普通用户可见条目数 = %d，期望 0（高风险内容不可见）", len(ordinaryView.Items))
	}
}

// TestPrivateFavoriteHiddenFromOthers 覆盖需求 §9.3：私有收藏夹仅创建者可见。
func TestPrivateFavoriteHiddenFromOthers(t *testing.T) {
	f := newFixture(t)
	_, owner := f.user(t)
	_, other := f.user(t)
	ctx := context.Background()

	private, err := f.svc.CreateFavorite(ctx, owner, FavoriteInput{
		Name: "私有收藏夹", Visibility: domain.FavoritePrivate,
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if _, err := f.svc.ListItems(ctx, other, private.Entity.ID, 1, 20); err == nil {
		t.Fatal("他人读取私有收藏夹应报错")
	}
	// 游客同样不可见。
	if _, err := f.svc.ListItems(ctx, domain.Guest(), private.Entity.ID, 1, 20); err == nil {
		t.Fatal("游客读取私有收藏夹应报错")
	}
}

// TestPublicFavoriteVisibleToGuest 覆盖开发方案 §6.5 验收标准「游客可查看公开收藏夹」。
func TestPublicFavoriteVisibleToGuest(t *testing.T) {
	f := newFixture(t)
	ownerID, owner := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "游客可见条目")
	public, err := f.svc.CreateFavorite(ctx, owner, FavoriteInput{
		Name: "公开收藏夹", Visibility: domain.FavoritePublic,
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := f.svc.AddItem(ctx, owner, public.Entity.ID, promptID); err != nil {
		t.Fatalf("加入条目失败: %v", err)
	}

	page, err := f.svc.ListItems(ctx, domain.Guest(), public.Entity.ID, 1, 20)
	if err != nil {
		t.Fatalf("游客读取公开收藏夹失败: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("游客可见条目数 = %d，期望 1", len(page.Items))
	}
}

// TestCreateReportUsesEnglishReasonKeys 覆盖 C-31：举报原因存英文枚举键。
func TestCreateReportUsesEnglishReasonKeys(t *testing.T) {
	f := newFixture(t)
	ownerID, _ := f.user(t)
	_, reporter := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "被举报的提示词")

	report, err := f.svc.CreateReport(ctx, reporter, ReportInput{
		ObjectType: domain.ReportObjectPrompt,
		ObjectID:   promptID,
		Reason:     domain.ReportSpam,
		Detail:     "疑似广告",
	})
	if err != nil {
		t.Fatalf("提交举报失败: %v", err)
	}
	if report.Reason != domain.ReportSpam {
		t.Fatalf("举报原因 = %s，期望 %s", report.Reason, domain.ReportSpam)
	}

	// 库里也必须是英文键，中文只留在前端语言包。
	var stored string
	if err := f.pool.QueryRow(ctx,
		`SELECT reason FROM reports WHERE id = $1`, report.ID).Scan(&stored); err != nil {
		t.Fatalf("读取举报失败: %v", err)
	}
	if stored != string(domain.ReportSpam) {
		t.Fatalf("库中举报原因 = %q，期望英文键 %q", stored, domain.ReportSpam)
	}

	// 全部八种原因都必须被数据库的 CHECK 约束接受。每种原因用不同的对象，
	// 否则会被「同一对象只留一条待处理举报」的规则挡下。
	for i, reason := range domain.AllReportReasons() {
		target := f.prompt(t, ownerID, "举报原因校验")
		if _, err := f.svc.CreateReport(ctx, reporter, ReportInput{
			ObjectType: domain.ReportObjectPrompt,
			ObjectID:   target,
			Reason:     reason,
		}); err != nil {
			t.Fatalf("第 %d 种举报原因 %s 无法提交: %v", i+1, reason, err)
		}
	}
}

// TestCreateReportRejectsGuest 覆盖 C-32：只有登录用户可举报。
func TestCreateReportRejectsGuest(t *testing.T) {
	f := newFixture(t)
	ownerID, _ := f.user(t)
	promptID := f.prompt(t, ownerID, "游客不能举报")

	_, err := f.svc.CreateReport(context.Background(), domain.Guest(), ReportInput{
		ObjectType: domain.ReportObjectPrompt,
		ObjectID:   promptID,
		Reason:     domain.ReportSpam,
	})
	assertCode(t, err, apperr.CodeEmailNotVerified)
}

// TestCreateReportRejectsDuplicatePending 覆盖同一用户对同一对象只留一条待处理举报。
func TestCreateReportRejectsDuplicatePending(t *testing.T) {
	f := newFixture(t)
	ownerID, _ := f.user(t)
	_, reporter := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "重复举报")
	if _, err := f.svc.CreateReport(ctx, reporter, ReportInput{
		ObjectType: domain.ReportObjectPrompt, ObjectID: promptID, Reason: domain.ReportSpam,
	}); err != nil {
		t.Fatalf("首次举报失败: %v", err)
	}

	_, err := f.svc.CreateReport(ctx, reporter, ReportInput{
		ObjectType: domain.ReportObjectPrompt, ObjectID: promptID, Reason: domain.ReportIllegal,
	})
	assertCode(t, err, apperr.CodeInvalidState)
}

// TestCreateReportRejectsInvisibleObject 覆盖开发方案 §5.3：对不可见对象举报返回 404。
func TestCreateReportRejectsInvisibleObject(t *testing.T) {
	f := newFixture(t)
	ownerID, _ := f.user(t)
	_, reporter := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "私有提示词")
	f.setPromptVisibility(t, promptID, "private")

	_, err := f.svc.CreateReport(ctx, reporter, ReportInput{
		ObjectType: domain.ReportObjectPrompt, ObjectID: promptID, Reason: domain.ReportSpam,
	})
	assertCode(t, err, apperr.CodeNotFound)
}

// TestCommentReplyNotifiesAuthor 覆盖需求 §12.3 的评论回复通知。
func TestCommentReplyNotifiesAuthor(t *testing.T) {
	f := newFixture(t)
	ownerID, author := f.user(t)
	_, replier := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "评论通知")
	parent, err := f.svc.CreateComment(ctx, author, promptID, nil, "顶层评论")
	if err != nil {
		t.Fatalf("发表评论失败: %v", err)
	}

	parentID := parent.ID
	if _, err := f.svc.CreateComment(ctx, replier, promptID, &parentID, "一条回复"); err != nil {
		t.Fatalf("发表回复失败: %v", err)
	}

	var count int64
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications
		WHERE user_id = $1 AND kind = 'comment_reply' AND deleted_at IS NULL`,
		author.UserID).Scan(&count); err != nil {
		t.Fatalf("统计通知失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("回复后作者收到 %d 条通知，期望 1 条", count)
	}
}

// TestSelfReplyDoesNotNotify 覆盖「不对自己发送通知」。
func TestSelfReplyDoesNotNotify(t *testing.T) {
	f := newFixture(t)
	ownerID, author := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "自回复")
	parent, err := f.svc.CreateComment(ctx, author, promptID, nil, "顶层评论")
	if err != nil {
		t.Fatalf("发表评论失败: %v", err)
	}
	parentID := parent.ID
	if _, err := f.svc.CreateComment(ctx, author, promptID, &parentID, "自己回复自己"); err != nil {
		t.Fatalf("发表回复失败: %v", err)
	}

	var count int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1`, author.UserID).Scan(&count); err != nil {
		t.Fatalf("统计通知失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("自己回复自己产生了 %d 条通知，期望 0 条", count)
	}
}

// TestDeleteCommentWritesAuditLog 覆盖需求 §11.2：评论直接删除但必须记录日志。
func TestDeleteCommentWritesAuditLog(t *testing.T) {
	f := newFixture(t)
	ownerID, author := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "评论删除")
	comment, err := f.svc.CreateComment(ctx, author, promptID, nil, "将被删除")
	if err != nil {
		t.Fatalf("发表评论失败: %v", err)
	}
	if err := f.svc.DeleteComment(ctx, author, comment.ID, "127.0.0.1"); err != nil {
		t.Fatalf("删除评论失败: %v", err)
	}

	var logged int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM operation_logs WHERE action = 'comment_delete' AND object_id = $1`,
		comment.ID).Scan(&logged); err != nil {
		t.Fatalf("统计日志失败: %v", err)
	}
	if logged != 1 {
		t.Fatalf("评论删除产生 %d 条日志，期望 1 条", logged)
	}
}

// TestDeleteOthersCommentForbidden 覆盖越权删除被拒。
func TestDeleteOthersCommentForbidden(t *testing.T) {
	f := newFixture(t)
	ownerID, author := f.user(t)
	_, other := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "越权删除")
	comment, err := f.svc.CreateComment(ctx, author, promptID, nil, "别人的评论")
	if err != nil {
		t.Fatalf("发表评论失败: %v", err)
	}
	assertCode(t, f.svc.DeleteComment(ctx, other, comment.ID, ""), apperr.CodeNotFound)
}

// TestLikeCommentIsIdempotent 覆盖重复点赞与取消点赞的计数一致性。
func TestLikeCommentIsIdempotent(t *testing.T) {
	f := newFixture(t)
	ownerID, author := f.user(t)
	_, liker := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "点赞")
	comment, err := f.svc.CreateComment(ctx, author, promptID, nil, "被点赞的评论")
	if err != nil {
		t.Fatalf("发表评论失败: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := f.svc.LikeComment(ctx, liker, comment.ID); err != nil {
			t.Fatalf("第 %d 次点赞失败: %v", i+1, err)
		}
	}
	if got := f.likeCount(t, comment.ID); got != 1 {
		t.Fatalf("重复点赞 3 次后计数 = %d，期望 1", got)
	}

	// 未点过赞的用户取消点赞不改变计数。
	_, stranger := f.user(t)
	if err := f.svc.UnlikeComment(ctx, stranger, comment.ID); err != nil {
		t.Fatalf("取消点赞失败: %v", err)
	}
	if got := f.likeCount(t, comment.ID); got != 1 {
		t.Fatalf("无关用户取消点赞后计数 = %d，期望仍为 1", got)
	}

	if err := f.svc.UnlikeComment(ctx, liker, comment.ID); err != nil {
		t.Fatalf("取消点赞失败: %v", err)
	}
	if got := f.likeCount(t, comment.ID); got != 0 {
		t.Fatalf("取消点赞后计数 = %d，期望 0", got)
	}
}

// likeCount 读取评论的点赞数。
func (f *fixture) likeCount(t *testing.T, commentID int64) int32 {
	t.Helper()
	var count int32
	if err := f.pool.QueryRow(context.Background(),
		`SELECT like_count FROM comments WHERE id = $1`, commentID).Scan(&count); err != nil {
		t.Fatalf("读取点赞数失败: %v", err)
	}
	return count
}

// TestFollowSelfRejected 覆盖需求 §9.4 与数据库的自关注约束。
func TestFollowSelfRejected(t *testing.T) {
	f := newFixture(t)
	_, author := f.user(t)
	assertCode(t, f.svc.Follow(context.Background(), author, author.UserID), apperr.CodeValidationFailed)
}

// TestFollowIsIdempotent 覆盖重复关注的幂等性。
func TestFollowIsIdempotent(t *testing.T) {
	f := newFixture(t)
	_, follower := f.user(t)
	targetID, _ := f.user(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := f.svc.Follow(ctx, follower, targetID); err != nil {
			t.Fatalf("第 %d 次关注失败: %v", i+1, err)
		}
	}
	var count int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM follows WHERE follower_id = $1 AND followee_id = $2`,
		follower.UserID, targetID).Scan(&count); err != nil {
		t.Fatalf("统计关注失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("重复关注 3 次后关系数 = %d，期望 1", count)
	}
}

// TestAnnouncementWatermarkMarksRead 覆盖 C-33 的水位线机制。
func TestAnnouncementWatermarkMarksRead(t *testing.T) {
	f := newFixture(t)
	_, reader := f.user(t)
	ctx := context.Background()

	// 直接建一条已发布的公告：发布端点在阶段 6 的管理后台。
	var announcementID int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO announcements (title, body, published_at)
		VALUES ('首次公告', '正文', now()) RETURNING id`).Scan(&announcementID); err != nil {
		t.Fatalf("创建公告失败: %v", err)
	}

	page, err := f.svc.ListAnnouncements(ctx, reader, 1, 20)
	if err != nil {
		t.Fatalf("读取公告失败: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("公告数 = %d，期望 1", len(page.Items))
	}
	if page.Items[0].Read {
		t.Fatal("从未读过公告时该条应标记为未读")
	}

	if unread, err := f.svc.UnreadCount(ctx, reader); err != nil {
		t.Fatalf("统计未读数失败: %v", err)
	} else if unread != 1 {
		t.Fatalf("未读数 = %d，期望 1（一条未读公告）", unread)
	}

	if err := f.svc.MarkAnnouncementsRead(ctx, reader); err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}

	page, err = f.svc.ListAnnouncements(ctx, reader, 1, 20)
	if err != nil {
		t.Fatalf("读取公告失败: %v", err)
	}
	if !page.Items[0].Read {
		t.Fatal("标记已读后该条应显示为已读")
	}
	if unread, err := f.svc.UnreadCount(ctx, reader); err != nil {
		t.Fatalf("统计未读数失败: %v", err)
	} else if unread != 0 {
		t.Fatalf("标记已读后未读数 = %d，期望 0", unread)
	}
}

// TestWithdrawnAnnouncementDisappears 覆盖开发方案 §6.5 验收标准：
// 公告撤回后立即从用户侧消失。
func TestWithdrawnAnnouncementDisappears(t *testing.T) {
	f := newFixture(t)
	_, reader := f.user(t)
	ctx := context.Background()

	var id int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO announcements (title, body, published_at)
		VALUES ('将被撤回', '正文', now()) RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("创建公告失败: %v", err)
	}

	before, err := f.svc.ListAnnouncements(ctx, reader, 1, 20)
	if err != nil {
		t.Fatalf("读取公告失败: %v", err)
	}
	if len(before.Items) != 1 {
		t.Fatalf("撤回前公告数 = %d，期望 1", len(before.Items))
	}

	if _, err := f.pool.Exec(ctx,
		`UPDATE announcements SET withdrawn_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("撤回公告失败: %v", err)
	}

	after, err := f.svc.ListAnnouncements(ctx, reader, 1, 20)
	if err != nil {
		t.Fatalf("读取公告失败: %v", err)
	}
	if len(after.Items) != 0 {
		t.Fatalf("撤回后公告数 = %d，期望 0", len(after.Items))
	}
	// 未读数同样必须立即归零，不能残留。
	if unread, err := f.svc.UnreadCount(ctx, reader); err != nil {
		t.Fatalf("统计未读数失败: %v", err)
	} else if unread != 0 {
		t.Fatalf("撤回后未读数 = %d，期望 0", unread)
	}
}

// TestNotificationCenterOperations 覆盖需求 §12.3 的通知中心六项能力。
func TestNotificationCenterOperations(t *testing.T) {
	f := newFixture(t)
	_, reader := f.user(t)
	ctx := context.Background()

	// 直接建三条不同类型的通知。
	for i, kind := range []string{"system", "library_question_deleted", "comment_reply"} {
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO notifications (user_id, kind, title, body)
			VALUES ($1, $2, $3, '正文')`, reader.UserID, kind, "通知"+string(rune('A'+i))); err != nil {
			t.Fatalf("创建通知失败: %v", err)
		}
	}

	// 1. 查看通知
	page, err := f.svc.ListNotifications(ctx, reader, nil, 1, 20)
	if err != nil {
		t.Fatalf("读取通知失败: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("通知数 = %d，期望 3", len(page.Items))
	}

	// 2. 按类型筛选
	filtered, err := f.svc.ListNotifications(ctx, reader,
		[]domain.NotificationKind{domain.NotifyCommentReply}, 1, 20)
	if err != nil {
		t.Fatalf("筛选通知失败: %v", err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].Kind != domain.NotifyCommentReply {
		t.Fatalf("按评论回复筛选得到 %d 条，期望 1 条", len(filtered.Items))
	}

	// 3. 未读数量
	unread, err := f.svc.UnreadCount(ctx, reader)
	if err != nil {
		t.Fatalf("统计未读数失败: %v", err)
	}
	if unread != 3 {
		t.Fatalf("未读数 = %d，期望 3", unread)
	}

	// 4. 标记单条已读
	if err := f.svc.MarkNotificationRead(ctx, reader, page.Items[0].ID); err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}
	unread, err = f.svc.UnreadCount(ctx, reader)
	if err != nil {
		t.Fatalf("统计未读数失败: %v", err)
	}
	if unread != 2 {
		t.Fatalf("标记一条后未读数 = %d，期望 2", unread)
	}

	// 5. 全部标记已读
	if err := f.svc.MarkAllNotificationsRead(ctx, reader); err != nil {
		t.Fatalf("全部标记已读失败: %v", err)
	}
	unread, err = f.svc.UnreadCount(ctx, reader)
	if err != nil {
		t.Fatalf("统计未读数失败: %v", err)
	}
	if unread != 0 {
		t.Fatalf("全部标记后未读数 = %d，期望 0", unread)
	}

	// 6. 删除通知
	if err := f.svc.DeleteNotification(ctx, reader, page.Items[0].ID); err != nil {
		t.Fatalf("删除通知失败: %v", err)
	}
	page, err = f.svc.ListNotifications(ctx, reader, nil, 1, 20)
	if err != nil {
		t.Fatalf("读取通知失败: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("删除一条后通知数 = %d，期望 2", len(page.Items))
	}
	if page.Total != 2 {
		t.Fatalf("删除后总数 = %d，期望 2", page.Total)
	}
}

// TestNotificationIsolation 覆盖越权读取与删除他人通知被拒。
func TestNotificationIsolation(t *testing.T) {
	f := newFixture(t)
	_, owner := f.user(t)
	_, other := f.user(t)
	ctx := context.Background()

	var id int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO notifications (user_id, kind, title) VALUES ($1, 'system', '私有通知')
		RETURNING id`, owner.UserID).Scan(&id); err != nil {
		t.Fatalf("创建通知失败: %v", err)
	}

	// 他人既看不到，也不能标记已读或删除。
	page, err := f.svc.ListNotifications(ctx, other, nil, 1, 20)
	if err != nil {
		t.Fatalf("读取通知失败: %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("他人看到了 %d 条通知，期望 0 条", len(page.Items))
	}
	assertCode(t, f.svc.MarkNotificationRead(ctx, other, id), apperr.CodeNotFound)
	assertCode(t, f.svc.DeleteNotification(ctx, other, id), apperr.CodeNotFound)
}

// TestDeleteFavoriteWritesAuditLog 覆盖需求 §11.2：收藏夹直接删除但必须记录日志。
func TestDeleteFavoriteWritesAuditLog(t *testing.T) {
	f := newFixture(t)
	ownerID, owner := f.user(t)
	ctx := context.Background()

	promptID := f.prompt(t, ownerID, "收藏夹删除")
	favorite, err := f.svc.CreateFavorite(ctx, owner, FavoriteInput{
		Name: "将被删除", Visibility: domain.FavoritePrivate,
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := f.svc.AddItem(ctx, owner, favorite.Entity.ID, promptID); err != nil {
		t.Fatalf("加入条目失败: %v", err)
	}

	if err := f.svc.DeleteFavorite(ctx, owner, favorite.Entity.ID, "127.0.0.1"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	var logged int64
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM operation_logs WHERE action = 'favorite_delete' AND object_id = $1`,
		favorite.Entity.ID).Scan(&logged); err != nil {
		t.Fatalf("统计日志失败: %v", err)
	}
	if logged != 1 {
		t.Fatalf("收藏夹删除产生 %d 条日志，期望 1 条", logged)
	}

	// 条目随收藏夹一并清除。
	items, err := f.svc.ListItems(ctx, owner, favorite.Entity.ID, 1, 20)
	if err == nil {
		t.Fatalf("已删除的收藏夹不应还能读取条目，实际返回 %d 条", len(items.Items))
	}
}

// TestDeveloperFavoriteQuotaDiffers 覆盖需求 §9.2：开发者上限为 30。
func TestDeveloperFavoriteQuotaDiffers(t *testing.T) {
	f := newFixture(t)
	_, developer := f.developer(t)
	ctx := context.Background()

	if err := f.config.Set(ctx, "favorite_count_limit_developer", 3, "测试用", 0); err != nil {
		t.Fatalf("设置配额失败: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := f.svc.CreateFavorite(ctx, developer, FavoriteInput{
			Name: "开发者收藏夹", Visibility: domain.FavoritePrivate,
		}); err != nil {
			t.Fatalf("创建第 %d 个失败: %v", i+1, err)
		}
	}
	_, err := f.svc.CreateFavorite(ctx, developer, FavoriteInput{
		Name: "超出配额", Visibility: domain.FavoritePrivate,
	})
	assertCode(t, err, apperr.CodeQuotaExceeded)
}

// TestPromptCountersFollowCommentAndFavoriteWrites 覆盖冗余计数维护：
// 评论数与收藏数随各自写入同事务增减，详情页与后台热门排序取到真实值。
func TestPromptCountersFollowCommentAndFavoriteWrites(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	uploaderID, author := f.user(t)
	promptID := f.prompt(t, uploaderID, "计数源提示词")

	assertCounters := func(wantComment, wantFavorite int64) {
		t.Helper()
		var c, fav int64
		if err := f.pool.QueryRow(ctx,
			`SELECT comment_count, favorite_count FROM prompts WHERE id = $1`, promptID).Scan(&c, &fav); err != nil {
			t.Fatalf("读取提示词计数失败: %v", err)
		}
		if c != wantComment || fav != wantFavorite {
			t.Fatalf("计数 comment=%d favorite=%d，期望 %d/%d", c, fav, wantComment, wantFavorite)
		}
	}

	comment, err := f.svc.CreateComment(ctx, author, promptID, nil, "顶层评论")
	if err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}
	assertCounters(1, 0)

	if err := f.svc.DeleteComment(ctx, author, comment.ID, "127.0.0.1"); err != nil {
		t.Fatalf("删除评论失败: %v", err)
	}
	assertCounters(0, 0)

	favorite, err := f.svc.CreateFavorite(ctx, author, FavoriteInput{
		Name: "计数收藏夹", Visibility: domain.FavoritePrivate,
	})
	if err != nil {
		t.Fatalf("创建收藏夹失败: %v", err)
	}
	if err := f.svc.AddItem(ctx, author, favorite.Entity.ID, promptID); err != nil {
		t.Fatalf("加入收藏夹失败: %v", err)
	}
	assertCounters(0, 1)

	if err := f.svc.RemoveItem(ctx, author, favorite.Entity.ID, promptID); err != nil {
		t.Fatalf("移除收藏夹条目失败: %v", err)
	}
	assertCounters(0, 0)
}
