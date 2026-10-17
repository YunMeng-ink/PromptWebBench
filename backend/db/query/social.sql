-- 收藏夹、条目与关注的读写。
--
-- 配额判定一律在事务内完成（C-21）：锁定 → 计数 → 判定 → 写入。
-- 事务外计数后插入会让并发请求各自读到未超限的计数，双双写入成功而突破上限。

-- 锁键 = 命名空间 << 32 + 对象标识。命名空间：1 收藏夹、2 统计重算、3 题库。
-- 高位命名空间使不同域的同号对象不再互相阻塞。
-- name: LockFavoriteOwner :exec
SELECT pg_advisory_xact_lock((1::bigint << 32) + sqlc.arg(owner_id)::bigint);

-- name: CountFavorites :one
SELECT count(*)::bigint
FROM favorites
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: InsertFavorite :one
INSERT INTO favorites (user_id, name, description, visibility)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetFavorite :one
SELECT * FROM favorites
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetFavoriteForUpdate :one
SELECT * FROM favorites
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- 收藏夹列表：本人查看全部，他人只看到公开的。
-- 可见性谓词由应用层经 ListScope 派生后作为参数传入，此处只做取值绑定。
-- name: ListFavorites :many
SELECT f.*, u.username AS owner_name
FROM favorites f
JOIN users u ON u.id = f.user_id
WHERE f.deleted_at IS NULL
  AND f.user_id = sqlc.arg(owner_id)
  AND (sqlc.arg(include_private)::boolean OR f.visibility = 'public')
ORDER BY f.updated_at DESC, f.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountVisibleFavorites :one
SELECT count(*)::bigint
FROM favorites f
WHERE f.deleted_at IS NULL
  AND f.user_id = sqlc.arg(owner_id)
  AND (sqlc.arg(include_private)::boolean OR f.visibility = 'public');

-- name: UpdateFavorite :one
UPDATE favorites
SET name = sqlc.arg(name), description = sqlc.arg(description),
    visibility = sqlc.arg(visibility), updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- 初版收藏夹删除为直接删除，但必须记录日志（需求 §11.2）。
-- name: SoftDeleteFavorite :one
UPDATE favorites
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- 条目：同一收藏夹内同一提示词只允许一条，由唯一约束兜底。
-- ON CONFLICT DO NOTHING 在冲突时不返回行，这是「已在夹内」的判定依据。
-- name: InsertFavoriteItem :one
INSERT INTO favorite_items (favorite_id, prompt_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
RETURNING *;

-- 条目计数与插入同事务：条件 WHERE 让计数不会在并发下越过上限。
-- name: IncrementFavoriteItemCount :one
UPDATE favorites
SET item_count = item_count + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND item_count < sqlc.arg(limit_count)
RETURNING item_count;

-- name: DecrementFavoriteItemCount :exec
UPDATE favorites
SET item_count = GREATEST(item_count - 1, 0), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: DeleteFavoriteItem :one
DELETE FROM favorite_items
WHERE favorite_id = $1 AND prompt_id = $2
RETURNING id;

-- name: DeleteFavoriteItems :exec
DELETE FROM favorite_items WHERE favorite_id = $1;

-- 提示词彻底删除前：统计各收藏夹内将随外键级联删除的条目数，用于校正 item_count。
-- name: ListFavoriteItemCountsByPrompt :many
SELECT favorite_id, count(*)::int AS item_count
FROM favorite_items
WHERE prompt_id = $1
GROUP BY favorite_id;

-- name: AdjustFavoriteItemCount :exec
UPDATE favorites
SET item_count = GREATEST(item_count - sqlc.arg(delta)::int, 0), updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- 收藏夹条目列表。可见性复查在应用层完成：条目里的提示词可能已被改为私有或
-- 高风险，此时不能再展示（开发方案 §6.5 验收标准）。
-- 这里取出判定所需的全部列，由 repo.FilterVisibleItems 逐条过 domain.Viewer。
-- name: ListFavoriteItems :many
SELECT fi.id AS item_id,
       fi.created_at AS added_at,
       p.id, p.title, p.description, p.purpose_id, p.capability_id, p.difficulty_id,
       p.visibility, p.sensitive_level, p.uploader_id, p.latest_version_no,
       p.major_modified, p.comment_count, p.favorite_count, p.created_at, p.updated_at,
       pu.name_zh AS purpose_name_zh, pu.name_en AS purpose_name_en, pu.is_high_risk,
       cap.name_zh AS capability_name_zh, cap.name_en AS capability_name_en
FROM favorite_items fi
JOIN prompts p ON p.id = fi.prompt_id
JOIN purposes pu ON pu.id = p.purpose_id
JOIN capabilities cap ON cap.id = p.capability_id
WHERE fi.favorite_id = $1
  AND p.deleted_at IS NULL
ORDER BY fi.created_at DESC, fi.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFavoriteItems :one
SELECT count(*)::bigint
FROM favorite_items fi
JOIN prompts p ON p.id = fi.prompt_id
WHERE fi.favorite_id = $1 AND p.deleted_at IS NULL;

-- name: IsPromptFavorited :one
SELECT EXISTS (
    SELECT 1 FROM favorite_items fi
    JOIN favorites f ON f.id = fi.favorite_id
    WHERE f.user_id = $1 AND fi.prompt_id = $2 AND f.deleted_at IS NULL
)::boolean;

-- ---------- 关注 ----------

-- name: InsertFollow :one
INSERT INTO follows (follower_id, followee_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: DeleteFollow :exec
DELETE FROM follows WHERE follower_id = $1 AND followee_id = $2;

-- name: GetFolloweeProfile :one
SELECT id, username, bio, avatar_url FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: IsFollowing :one
SELECT EXISTS (
    SELECT 1 FROM follows WHERE follower_id = $1 AND followee_id = $2
)::boolean;

-- name: ListFollowing :many
SELECT u.id, u.username, u.bio, u.avatar_url, f.created_at AS followed_at
FROM follows f
JOIN users u ON u.id = f.followee_id
WHERE f.follower_id = $1 AND u.deleted_at IS NULL
ORDER BY f.created_at DESC, u.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFollowing :one
SELECT count(*)::bigint
FROM follows f
JOIN users u ON u.id = f.followee_id
WHERE f.follower_id = $1 AND u.deleted_at IS NULL;