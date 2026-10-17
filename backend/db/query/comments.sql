-- 评论、点赞与举报的读写。

-- 插入评论或回复。parent_id 不加 ::bigint 转换：加上会把列的可空性抹掉，
-- sqlc 随即把参数映射为非空 int64，顶层评论的 NULL 就写不进去。
-- name: InsertComment :one
INSERT INTO comments (prompt_id, user_id, parent_id, body)
VALUES ($1, $2, sqlc.narg(parent_id), $3)
RETURNING *;

-- name: GetComment :one
SELECT * FROM comments
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteComment :one
UPDATE comments
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- 顶层评论分页，附带作者昵称、当前用户是否点过赞、以及回复数。
-- 回复数用相关子查询而不是聚合：聚合会与分页的 LIMIT 互相干扰。
-- name: ListComments :many
SELECT c.*, u.username AS author_name,
       EXISTS (
           SELECT 1 FROM comment_likes cl
           WHERE cl.comment_id = c.id AND cl.user_id = sqlc.arg(viewer_id)
       )::boolean AS liked,
       (SELECT count(*)::bigint FROM comments r
        WHERE r.parent_id = c.id AND r.deleted_at IS NULL) AS reply_count
FROM comments c
JOIN users u ON u.id = c.user_id
WHERE c.prompt_id = sqlc.arg(prompt_id)
  AND c.parent_id IS NULL
  AND c.deleted_at IS NULL
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountComments :one
SELECT count(*)::bigint
FROM comments
WHERE prompt_id = $1 AND parent_id IS NULL AND deleted_at IS NULL;

-- name: ListReplies :many
SELECT c.*, u.username AS author_name,
       EXISTS (
           SELECT 1 FROM comment_likes cl
           WHERE cl.comment_id = c.id AND cl.user_id = sqlc.arg(viewer_id)
       )::boolean AS liked,
       0::bigint AS reply_count
FROM comments c
JOIN users u ON u.id = c.user_id
WHERE c.parent_id = sqlc.arg(parent_id) AND c.deleted_at IS NULL
ORDER BY c.created_at ASC, c.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountReplies :one
SELECT count(*)::bigint
FROM comments
WHERE parent_id = $1 AND deleted_at IS NULL;

-- 点赞：唯一约束保证同一用户重复点赞只产生一条记录。
-- ON CONFLICT DO NOTHING 在冲突时不返回行，据此判定是否为重复点赞。
-- name: InsertCommentLike :one
INSERT INTO comment_likes (comment_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
RETURNING id;

-- 取消点赞用 :execrows 返回影响行数：0 行说明本来就没点过赞，
-- 此时不能再递减计数，否则计数会与实际点赞记录脱节。
-- name: DeleteCommentLike :execrows
DELETE FROM comment_likes WHERE comment_id = $1 AND user_id = $2;

-- 点赞数与插入同事务，条件 WHERE 让计数不会在并发下越过实际点赞数。
-- name: IncrementCommentLikeCount :one
UPDATE comments
SET like_count = like_count + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING like_count;

-- name: DecrementCommentLikeCount :one
UPDATE comments
SET like_count = GREATEST(like_count - 1, 0), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING like_count;

-- ---------- 举报 ----------

-- name: InsertReport :one
INSERT INTO reports (reporter_id, object_type, object_id, reason, detail)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (reporter_id, object_type, object_id) WHERE status = 'pending' DO NOTHING
RETURNING *;

-- 同一用户对同一对象同时只保留一条待处理举报，避免重复刷单。
-- name: CountPendingReportByReporter :one
SELECT count(*)::bigint
FROM reports
WHERE reporter_id = $1 AND object_type = $2 AND object_id = $3 AND status = 'pending';

-- 举报对象的存在性与可见性在服务层判定：评论与提示词的可见性规则不同，
-- 这里只提供按类型取对象的读取。
-- name: GetCommentForReport :one
SELECT * FROM comments WHERE id = $1 AND deleted_at IS NULL;