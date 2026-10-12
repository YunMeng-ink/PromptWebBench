package api_test

import (
	"fmt"
	"net/http"
	"testing"

	api "github.com/promptbench/backend/internal/http"
)

// socialPrompt 建立一条公开提示词并返回其 ID。
func socialPrompt(t *testing.T, env *testEnv, client *apiClient, title string) int64 {
	t.Helper()
	created := decodeBody[struct {
		Prompt struct {
			ID int64 `json:"id"`
		} `json:"prompt"`
	}](t, client.post(api.APIPrefix+"/prompts", map[string]any{
		"title": title, "body": "正文", "category_id": 1,
	}))
	return created.Prompt.ID
}

// TestReportOverHTTPUsesEnglishEnumKey 覆盖 C-31 与开发方案 §6.5 验收标准：
// 举报原因以英文枚举键提交并原样返回。
func TestReportOverHTTPUsesEnglishEnumKey(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "report-author@example.com", httpTestPassword)

	reporter := env.newClient(t)
	reporter.prime()
	registerVerifyLogin(t, env, reporter, "report-reporter@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, author, "被举报的提示词")

	// 举报原因以英文键提交并原样返回。
	//
	// 这里只验两个键：举报接口限流为每用户每分钟 5 次，八种键在同一分钟内全部提交
	// 会先触发限流。八种键都能被数据库的 CHECK 约束接受由服务层用例覆盖，
	// 本用例负责证明键在 HTTP 契约上原样往返。
	for i, reason := range []string{"bad_content", "dangerous"} {
		target := socialPrompt(t, env, author, fmt.Sprintf("举报原因 %d", i))
		resp := decodeBody[struct {
			Reason string `json:"reason"`
		}](t, reporter.post(api.APIPrefix+"/reports", map[string]any{
			"object_type": "prompt",
			"object_id":   target,
			"reason":      reason,
		}))
		if resp.Reason != reason {
			t.Fatalf("提交原因 %s 后返回 %q", reason, resp.Reason)
		}
	}

	// 中文原因不在枚举内，被字段校验挡下。
	expectErrorCode(t, reporter.post(api.APIPrefix+"/reports", map[string]any{
		"object_type": "prompt", "object_id": promptID, "reason": "垃圾广告",
	}), http.StatusUnprocessableEntity, "validation_failed")

	// 非法对象类型同样被挡下。
	expectErrorCode(t, reporter.post(api.APIPrefix+"/reports", map[string]any{
		"object_type": "user", "object_id": promptID, "reason": "spam",
	}), http.StatusUnprocessableEntity, "validation_failed")
}

// TestGuestCannotReportOverHTTP 覆盖 C-32：游客不能举报。
//
// 游客没有会话，因此被鉴权中间件挡在 401；这不是权限不足而是未登录。
func TestGuestCannotReportOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "report-guest-author@example.com", httpTestPassword)
	promptID := socialPrompt(t, env, author, "游客不能举报")

	guest := env.newClient(t)
	guest.prime()
	expectErrorCode(t, guest.post(api.APIPrefix+"/reports", map[string]any{
		"object_type": "prompt", "object_id": promptID, "reason": "spam",
	}), http.StatusUnauthorized, "unauthorized")
}

// TestUnverifiedCannotReportOverHTTP 覆盖 C-32 的另一面：
// 已登录但邮箱未验证的账户同样不能举报（C-23 的写入限制）。
func TestUnverifiedCannotReportOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "report-unverified-author@example.com", httpTestPassword)
	promptID := socialPrompt(t, env, author, "未验证账户不能举报")

	unverified := env.newClient(t)
	unverified.prime()
	registerViaHTTP(t, unverified, "report-unverified@example.com", httpTestPassword)
	env.login(t, unverified, "report-unverified@example.com", httpTestPassword)

	expectErrorCode(t, unverified.post(api.APIPrefix+"/reports", map[string]any{
		"object_type": "prompt", "object_id": promptID, "reason": "spam",
	}), http.StatusForbidden, "email_not_verified")
}

// TestFavoriteLifecycleOverHTTP 覆盖收藏夹的创建、加入、列表与移除。
func TestFavoriteLifecycleOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "favorite-life@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, client, "被收藏的提示词")

	created := decodeBody[struct {
		Favorite struct {
			ID         int64  `json:"id"`
			Visibility string `json:"visibility"`
			ItemCount  int32  `json:"item_count"`
		} `json:"favorite"`
	}](t, client.post(api.APIPrefix+"/favorites", map[string]any{
		"name": "我的收藏夹", "visibility": "public",
	}))
	if created.Favorite.ID == 0 {
		t.Fatal("创建收藏夹未返回 ID")
	}

	path := fmt.Sprintf("%s/favorites/%d/items/%d", api.APIPrefix, created.Favorite.ID, promptID)
	expectStatus(t, client.put(path, nil), http.StatusNoContent)

	list := decodeBody[struct {
		Items []struct {
			PromptID int64  `json:"prompt_id"`
			Title    string `json:"title"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, client.get(fmt.Sprintf("%s/favorites/%d/items", api.APIPrefix, created.Favorite.ID)))
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("条目数 = %d（总数 %d），期望 1", len(list.Items), list.Total)
	}
	if list.Items[0].PromptID != promptID {
		t.Fatalf("条目指向提示词 %d，期望 %d", list.Items[0].PromptID, promptID)
	}

	// 重复加入被挡下。
	expectErrorCode(t, client.put(path, nil), http.StatusUnprocessableEntity, "invalid_state")

	// 移除后条目清零。
	expectStatus(t, client.delete(path), http.StatusNoContent)
	empty := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, client.get(fmt.Sprintf("%s/favorites/%d/items", api.APIPrefix, created.Favorite.ID)))
	if empty.Total != 0 {
		t.Fatalf("移除后条目数 = %d，期望 0", empty.Total)
	}
}

// TestGuestCanViewPublicFavorite 覆盖开发方案 §6.5 验收标准「游客可查看公开收藏夹」。
func TestGuestCanViewPublicFavorite(t *testing.T) {
	env := newTestEnv(t)
	owner := env.newClient(t)
	owner.prime()
	user := registerVerifyLogin(t, env, owner, "favorite-public@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, owner, "公开收藏夹条目")
	created := decodeBody[struct {
		Favorite struct {
			ID int64 `json:"id"`
		} `json:"favorite"`
	}](t, owner.post(api.APIPrefix+"/favorites", map[string]any{
		"name": "公开收藏夹", "visibility": "public",
	}))
	expectStatus(t, owner.put(
		fmt.Sprintf("%s/favorites/%d/items/%d", api.APIPrefix, created.Favorite.ID, promptID), nil),
		http.StatusNoContent)

	guest := env.newClient(t)
	guest.prime()

	// 收藏夹列表与条目都对游客可见。
	list := decodeBody[struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}](t, guest.get(fmt.Sprintf("%s/users/%d/favorites", api.APIPrefix, user.ID)))
	if len(list.Items) != 1 {
		t.Fatalf("游客看到 %d 个收藏夹，期望 1 个", len(list.Items))
	}

	items := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, guest.get(fmt.Sprintf("%s/favorites/%d/items", api.APIPrefix, created.Favorite.ID)))
	if items.Total != 1 {
		t.Fatalf("游客看到 %d 条条目，期望 1 条", items.Total)
	}
}

// TestPrivateFavoriteHiddenOverHTTP 覆盖需求 §9.3。
func TestPrivateFavoriteHiddenOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	owner := env.newClient(t)
	owner.prime()
	registerVerifyLogin(t, env, owner, "favorite-private@example.com", httpTestPassword)

	created := decodeBody[struct {
		Favorite struct {
			ID int64 `json:"id"`
		} `json:"favorite"`
	}](t, owner.post(api.APIPrefix+"/favorites", map[string]any{
		"name": "私有收藏夹", "visibility": "private",
	}))

	guest := env.newClient(t)
	guest.prime()
	// 无权限对象统一 404（开发方案 §5.3）。
	expectErrorCode(t, guest.get(
		fmt.Sprintf("%s/favorites/%d/items", api.APIPrefix, created.Favorite.ID)),
		http.StatusNotFound, "not_found")
}

// TestFavoriteItemVisibilityRecheckedOverHTTP 覆盖把条目里的提示词改为私有后，
// 它从他人的收藏夹视图里消失。
func TestFavoriteItemVisibilityRecheckedOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	owner := env.newClient(t)
	owner.prime()
	user := registerVerifyLogin(t, env, owner, "favorite-recheck@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, owner, "将被改为私有")
	created := decodeBody[struct {
		Favorite struct {
			ID int64 `json:"id"`
		} `json:"favorite"`
	}](t, owner.post(api.APIPrefix+"/favorites", map[string]any{
		"name": "复查可见性", "visibility": "public",
	}))
	expectStatus(t, owner.put(
		fmt.Sprintf("%s/favorites/%d/items/%d", api.APIPrefix, created.Favorite.ID, promptID), nil),
		http.StatusNoContent)

	guest := env.newClient(t)
	guest.prime()
	before := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, guest.get(fmt.Sprintf("%s/favorites/%d/items", api.APIPrefix, created.Favorite.ID)))
	if before.Total != 1 {
		t.Fatalf("改私有前游客看到 %d 条，期望 1 条", before.Total)
	}

	// 把提示词改为私有：条目仍在夹内，但对外不再可见。
	expectStatus(t, owner.patch(fmt.Sprintf("%s/prompts/%d", api.APIPrefix, promptID), map[string]any{
		"title": "将被改为私有", "body": "正文", "category_id": 1, "visibility": "private",
	}), http.StatusOK)

	after := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, guest.get(fmt.Sprintf("%s/favorites/%d/items", api.APIPrefix, created.Favorite.ID)))
	if after.Total != 0 {
		t.Fatalf("改私有后游客看到 %d 条，期望 0 条", after.Total)
	}

	// 所有者本人仍能看到。
	own := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, owner.get(fmt.Sprintf("%s/favorites/%d/items", api.APIPrefix, created.Favorite.ID)))
	if own.Total != 1 {
		t.Fatalf("所有者看到 %d 条，期望 1 条", own.Total)
	}
	_ = user
}

// TestCommentLifecycleOverHTTP 覆盖评论的发表、回复、点赞、删除与通知。
func TestCommentLifecycleOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	registerVerifyLogin(t, env, author, "comment-author@example.com", httpTestPassword)

	commenter := env.newClient(t)
	commenter.prime()
	commenterUser := registerVerifyLogin(t, env, commenter, "comment-other@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, author, "评论测试提示词")

	created := decodeBody[struct {
		Comment struct {
			ID        int64  `json:"id"`
			Body      string `json:"body"`
			LikeCount int32  `json:"like_count"`
		} `json:"comment"`
	}](t, commenter.post(fmt.Sprintf("%s/prompts/%d/comments", api.APIPrefix, promptID), map[string]any{
		"body": "第一条评论",
	}))
	if created.Comment.ID == 0 {
		t.Fatal("发表评论未返回 ID")
	}

	// 游客可读评论（需求 §12.1）。
	guest := env.newClient(t)
	guest.prime()
	listed := decodeBody[struct {
		Items []struct {
			ID         int64  `json:"id"`
			AuthorName string `json:"author_name"`
			Body       string `json:"body"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, guest.get(fmt.Sprintf("%s/prompts/%d/comments", api.APIPrefix, promptID)))
	if listed.Total != 1 || len(listed.Items) != 1 {
		t.Fatalf("评论数 = %d（总数 %d），期望 1", len(listed.Items), listed.Total)
	}
	if listed.Items[0].AuthorName == "" {
		t.Fatal("评论列表应带作者昵称")
	}

	// 回复该评论：作者应收到通知。
	expectStatus(t, author.post(fmt.Sprintf("%s/prompts/%d/comments", api.APIPrefix, promptID), map[string]any{
		"body": "一条回复", "parent_id": created.Comment.ID,
	}), http.StatusOK)

	unread := decodeBody[struct {
		Unread int64 `json:"unread"`
	}](t, commenter.get(api.APIPrefix+"/me/notifications/unread-count"))
	if unread.Unread != 1 {
		t.Fatalf("评论被回复后未读数 = %d，期望 1", unread.Unread)
	}

	// 点赞与取消点赞。
	likePath := fmt.Sprintf("%s/comments/%d/like", api.APIPrefix, created.Comment.ID)
	expectStatus(t, author.put(likePath, nil), http.StatusNoContent)
	expectStatus(t, author.put(likePath, nil), http.StatusNoContent)
	liked := decodeBody[struct {
		Items []struct {
			LikeCount int32 `json:"like_count"`
			Liked     bool  `json:"liked"`
		} `json:"items"`
	}](t, author.get(fmt.Sprintf("%s/prompts/%d/comments", api.APIPrefix, promptID)))
	if liked.Items[0].LikeCount != 1 {
		t.Fatalf("重复点赞后计数 = %d，期望 1", liked.Items[0].LikeCount)
	}
	if !liked.Items[0].Liked {
		t.Fatal("点赞后 liked 应为真")
	}

	expectStatus(t, author.delete(likePath), http.StatusNoContent)

	// 删除自己的评论成功；他人删除被拒。
	expectErrorCode(t, author.delete(fmt.Sprintf("%s/comments/%d", api.APIPrefix, created.Comment.ID)),
		http.StatusNotFound, "not_found")
	expectStatus(t, commenter.delete(fmt.Sprintf("%s/comments/%d", api.APIPrefix, created.Comment.ID)),
		http.StatusNoContent)
	_ = commenterUser
}

// TestFollowOverHTTP 覆盖关注与取消关注，以及关注列表。
func TestFollowOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	follower := env.newClient(t)
	follower.prime()
	registerVerifyLogin(t, env, follower, "follow-a@example.com", httpTestPassword)

	target := env.newClient(t)
	target.prime()
	targetUser := registerVerifyLogin(t, env, target, "follow-b@example.com", httpTestPassword)

	path := fmt.Sprintf("%s/me/following/%d", api.APIPrefix, targetUser.ID)
	expectStatus(t, follower.put(path, nil), http.StatusNoContent)
	// 重复关注幂等。
	expectStatus(t, follower.put(path, nil), http.StatusNoContent)

	list := decodeBody[struct {
		Items []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, follower.get(api.APIPrefix+"/me/following"))
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("关注数 = %d（总数 %d），期望 1", len(list.Items), list.Total)
	}
	if list.Items[0].ID != targetUser.ID {
		t.Fatalf("关注的用户 = %d，期望 %d", list.Items[0].ID, targetUser.ID)
	}

	// 自关注被拒：me 返回自己的 ID，用它作为关注目标。
	me := decodeBody[apiUserDTO](t, follower.get(api.APIPrefix+"/me"))
	expectErrorCode(t, follower.put(
		fmt.Sprintf("%s/me/following/%d", api.APIPrefix, me.ID), nil),
		http.StatusUnprocessableEntity, "validation_failed")

	expectStatus(t, follower.delete(path), http.StatusNoContent)
	after := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, follower.get(api.APIPrefix+"/me/following"))
	if after.Total != 0 {
		t.Fatalf("取消关注后关注数 = %d，期望 0", after.Total)
	}
}

// TestNotificationCenterOverHTTP 覆盖开发方案 §6.5 验收标准「通知中心 6 项能力全部可用」。
func TestNotificationCenterOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	author := env.newClient(t)
	author.prime()
	authorUser := registerVerifyLogin(t, env, author, "notify-author@example.com", httpTestPassword)

	other := env.newClient(t)
	other.prime()
	registerVerifyLogin(t, env, other, "notify-other@example.com", httpTestPassword)

	promptID := socialPrompt(t, env, author, "通知中心")

	// 制造两条评论回复通知。
	for i := 0; i < 2; i++ {
		created := decodeBody[struct {
			Comment struct {
				ID int64 `json:"id"`
			} `json:"comment"`
		}](t, other.post(fmt.Sprintf("%s/prompts/%d/comments", api.APIPrefix, promptID), map[string]any{
			"body": fmt.Sprintf("评论 %d", i),
		}))
		expectStatus(t, author.post(fmt.Sprintf("%s/prompts/%d/comments", api.APIPrefix, promptID),
			map[string]any{"body": "回复", "parent_id": created.Comment.ID}), http.StatusOK)
	}

	// 1. 查看通知
	list := decodeBody[struct {
		Items []struct {
			ID   int64  `json:"id"`
			Kind string `json:"kind"`
			Read bool   `json:"read"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, other.get(api.APIPrefix+"/me/notifications"))
	if list.Total != 2 {
		t.Fatalf("通知数 = %d，期望 2", list.Total)
	}
	if list.Items[0].Kind != "comment_reply" {
		t.Fatalf("通知类型 = %s，期望 comment_reply", list.Items[0].Kind)
	}

	// 2. 按类型筛选
	filtered := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, other.get(api.APIPrefix+"/me/notifications?kind=comment_reply"))
	if filtered.Total != 2 {
		t.Fatalf("按 comment_reply 筛选得到 %d 条，期望 2", filtered.Total)
	}
	none := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, other.get(api.APIPrefix+"/me/notifications?kind=system"))
	if none.Total != 0 {
		t.Fatalf("按 system 筛选得到 %d 条，期望 0", none.Total)
	}

	// 3. 未读数量
	unread := decodeBody[struct {
		Unread int64 `json:"unread"`
	}](t, other.get(api.APIPrefix+"/me/notifications/unread-count"))
	if unread.Unread != 2 {
		t.Fatalf("未读数 = %d，期望 2", unread.Unread)
	}

	// 4. 标记单条已读
	expectStatus(t, other.post(
		fmt.Sprintf("%s/me/notifications/%d/read", api.APIPrefix, list.Items[0].ID), nil), http.StatusNoContent)
	unread = decodeBody[struct {
		Unread int64 `json:"unread"`
	}](t, other.get(api.APIPrefix+"/me/notifications/unread-count"))
	if unread.Unread != 1 {
		t.Fatalf("标记一条后未读数 = %d，期望 1", unread.Unread)
	}

	// 5. 全部标记已读
	expectStatus(t, other.post(api.APIPrefix+"/me/notifications/read-all", nil), http.StatusNoContent)
	unread = decodeBody[struct {
		Unread int64 `json:"unread"`
	}](t, other.get(api.APIPrefix+"/me/notifications/unread-count"))
	if unread.Unread != 0 {
		t.Fatalf("全部标记后未读数 = %d，期望 0", unread.Unread)
	}

	// 6. 删除通知
	expectStatus(t, other.delete(
		fmt.Sprintf("%s/me/notifications/%d", api.APIPrefix, list.Items[0].ID)), http.StatusNoContent)
	after := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, other.get(api.APIPrefix+"/me/notifications"))
	if after.Total != 1 {
		t.Fatalf("删除一条后通知数 = %d，期望 1", after.Total)
	}
	_ = authorUser
}

// TestAnnouncementWatermarkOverHTTP 覆盖 C-33：公告撤回后立即从用户侧消失。
func TestAnnouncementWatermarkOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "announce-reader@example.com", httpTestPassword)

	var id int64
	if err := env.pool.QueryRow(t.Context(), `
		INSERT INTO announcements (title, body, published_at)
		VALUES ('端到端公告', '公告正文', now()) RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("创建公告失败: %v", err)
	}

	// 游客也能看到公告。
	guest := env.newClient(t)
	guest.prime()
	visible := decodeBody[struct {
		Items []struct {
			ID   int64 `json:"id"`
			Read bool  `json:"read"`
		} `json:"items"`
		Total int64 `json:"total"`
	}](t, guest.get(api.APIPrefix+"/announcements"))
	if visible.Total != 1 {
		t.Fatalf("游客看到 %d 条公告，期望 1 条", visible.Total)
	}

	// 登录用户看到未读。
	listed := decodeBody[struct {
		Items []struct {
			Read bool `json:"read"`
		} `json:"items"`
	}](t, client.get(api.APIPrefix+"/announcements"))
	if listed.Items[0].Read {
		t.Fatal("从未读过公告时应为未读")
	}

	expectStatus(t, client.post(api.APIPrefix+"/me/announcements/read-all", nil), http.StatusNoContent)
	afterRead := decodeBody[struct {
		Items []struct {
			Read bool `json:"read"`
		} `json:"items"`
	}](t, client.get(api.APIPrefix+"/announcements"))
	if !afterRead.Items[0].Read {
		t.Fatal("标记已读后应显示为已读")
	}

	// 撤回后立即消失，未读数同步归零。
	if _, err := env.pool.Exec(t.Context(),
		`UPDATE announcements SET withdrawn_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("撤回公告失败: %v", err)
	}
	gone := decodeBody[struct {
		Total int64 `json:"total"`
	}](t, guest.get(api.APIPrefix+"/announcements"))
	if gone.Total != 0 {
		t.Fatalf("撤回后仍有 %d 条公告", gone.Total)
	}
	unread := decodeBody[struct {
		Unread int64 `json:"unread"`
	}](t, client.get(api.APIPrefix+"/me/notifications/unread-count"))
	if unread.Unread != 0 {
		t.Fatalf("撤回后未读数 = %d，期望 0", unread.Unread)
	}
}

// TestFavoriteQuotaOverHTTP 覆盖配额错误码。
func TestFavoriteQuotaOverHTTP(t *testing.T) {
	env := newTestEnv(t)
	client := env.newClient(t)
	client.prime()
	registerVerifyLogin(t, env, client, "favorite-quota@example.com", httpTestPassword)

	// 把配额降到 2。
	if _, err := env.pool.Exec(t.Context(),
		`UPDATE config SET value = '2'::jsonb WHERE key = 'favorite_count_limit_user'`); err != nil {
		t.Fatalf("调整配额失败: %v", err)
	}
	if err := env.config.Refresh(t.Context()); err != nil {
		t.Fatalf("刷新配置失败: %v", err)
	}

	for i := 0; i < 2; i++ {
		expectStatus(t, client.post(api.APIPrefix+"/favorites", map[string]any{
			"name": fmt.Sprintf("收藏夹 %d", i),
		}), http.StatusOK)
	}
	expectErrorCode(t, client.post(api.APIPrefix+"/favorites", map[string]any{
		"name": "超出配额",
	}), http.StatusConflict, "quota_exceeded")
}
