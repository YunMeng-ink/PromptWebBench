-- 通知与公告的读写。
--
-- 公告分发采用「共享条目 + 每用户水位线」（C-33）：公告只存一份，
-- 已读状态由 users.last_read_announcement_at 标记，不逐用户扇出。
-- 撤回后共享条目立即从用户侧消失，不留残余通知。

-- name: InsertNotification :one
INSERT INTO notifications (user_id, kind, title, body, link, payload)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- 通知列表与筛选。kinds 为空数组时不过滤类型。
-- name: ListNotifications :many
SELECT * FROM notifications
WHERE user_id = sqlc.arg(user_id)
  AND deleted_at IS NULL
  AND (
      sqlc.arg(kinds)::text[] = '{}'::text[]
      OR kind = ANY(sqlc.arg(kinds)::text[])
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountNotifications :one
SELECT count(*)::bigint
FROM notifications
WHERE user_id = sqlc.arg(user_id)
  AND deleted_at IS NULL
  AND (
      sqlc.arg(kinds)::text[] = '{}'::text[]
      OR kind = ANY(sqlc.arg(kinds)::text[])
  );

-- name: CountUnreadNotifications :one
SELECT count(*)::bigint
FROM notifications
WHERE user_id = $1 AND deleted_at IS NULL AND read_at IS NULL;

-- 标记单条已读。条件里带 user_id，避免越权读改他人通知。
-- name: MarkNotificationRead :one
UPDATE notifications
SET read_at = now()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications
SET read_at = now()
WHERE user_id = $1 AND deleted_at IS NULL AND read_at IS NULL;

-- name: SoftDeleteNotification :one
UPDATE notifications
SET deleted_at = now()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING id;

-- ---------- 公告 ----------

-- name: InsertAnnouncement :one
INSERT INTO announcements (title, body, published_by, published_at)
VALUES ($1, $2, $3, now())
RETURNING *;

-- 未撤回的公告按发布时间倒序。撤回后该条立即从本查询结果中消失（C-33）。
-- name: ListLiveAnnouncements :many
SELECT * FROM announcements
WHERE withdrawn_at IS NULL AND published_at IS NOT NULL
ORDER BY published_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- 未撤回的公告总条数，用于分页。
-- name: CountLiveAnnouncements :one
SELECT count(*)::bigint
FROM announcements
WHERE withdrawn_at IS NULL AND published_at IS NOT NULL;

-- 未读公告数：只统计发布时间晚于水位线的条目。
-- 没有水位线的账户（从未读过公告）看到全部公告为未读。
-- name: CountUnreadAnnouncements :one
SELECT count(*)::bigint
FROM announcements a
WHERE a.withdrawn_at IS NULL
  AND a.published_at IS NOT NULL
  AND (
      (SELECT u.last_read_announcement_at FROM users u WHERE u.id = $1) IS NULL
      OR a.published_at > (SELECT u.last_read_announcement_at FROM users u WHERE u.id = $1)
  );

-- 已读全部公告：把水位线推到当前时刻。窗口边界由应用层给出，
-- SQL 中不出现依赖会话时区的表达式（开发方案 §3.4）。
-- name: AdvanceAnnouncementWatermark :exec
UPDATE users
SET last_read_announcement_at = sqlc.arg(read_at)::timestamptz, updated_at = now()
WHERE id = sqlc.arg(user_id);

-- name: GetAnnouncementWatermark :one
SELECT last_read_announcement_at FROM users WHERE id = $1;