-- ratings 与 rating_change_logs 的读写。
--
-- 每日限额一律在事务内统计，窗口边界由应用层按 UTC 自然日计算后作为参数传入（C-03、
-- 开发方案 §3.4）：SQL 中不出现 date_trunc 与任何依赖会话时区的表达式。

-- name: CreateRating :one
INSERT INTO ratings (user_id, prompt_id, prompt_version_id, model_id, score)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetRating :one
SELECT * FROM ratings
WHERE user_id = $1 AND prompt_id = $2 AND model_id = $3 AND deleted_at IS NULL;

-- name: LockRating :one
SELECT * FROM ratings
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- 单条修改次数上限由条件 UPDATE 保证（开发方案 §3.5）：
-- 即使并发修改同时读到 edit_count 未超限，也只有一个事务能命中该行。
-- name: UpdateRatingScore :one
UPDATE ratings
SET score = $2, edit_count = edit_count + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND edit_count < sqlc.arg(max_edits)
RETURNING *;

-- name: SoftDeleteRating :one
UPDATE ratings
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- 每日修改与删除次数统计，窗口为 [day_start, day_end)。
-- name: CountRatingChangesInDay :one
SELECT count(*)::bigint
FROM rating_change_logs
WHERE user_id = $1
  AND action = sqlc.arg(action)
  AND created_at >= sqlc.arg(day_start)
  AND created_at < sqlc.arg(day_end);

-- name: InsertRatingChangeLog :one
INSERT INTO rating_change_logs (
    rating_id, user_id, prompt_id, model_id, action,
    before_score, after_score, before_edit_count, after_edit_count, ip
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- 详情页的模型评分列表：同一模型多条评分取最新一条，展示聚合均值。
--
-- 均值必须包 COALESCE（开发方案 §6.4 专项回归）：LEFT JOIN 后没有任何评分时
-- avg 返回 NULL，sqlc 会把该列映射为非空 float64，扫描即以
-- "cannot scan NULL into *float64" 失败，详情页 500 且不自愈。
-- 无评分的模型均值为 0，评分数为 0 表明「尚无评分」。
-- name: ListPromptModelRatings :many
SELECT m.id AS model_id, m.name AS model_name,
       count(r.id)::bigint                AS rating_count,
       count(DISTINCT r.user_id)::bigint  AS user_count,
       COALESCE(avg(r.score), 0)::float8  AS mean_score
FROM models m
LEFT JOIN ratings r
       ON r.model_id = m.id
      AND r.prompt_id = $1
      AND r.deleted_at IS NULL
WHERE m.deleted_at IS NULL AND m.is_enabled
GROUP BY m.id, m.name
ORDER BY m.name;

-- 当前用户的评分：详情页展示「我已评过的模型」。
-- name: ListMyRatingsForPrompt :many
SELECT * FROM ratings
WHERE user_id = $1 AND prompt_id = $2 AND deleted_at IS NULL
ORDER BY model_id;

-- name: ListMyRatings :many
SELECT r.*, p.title AS prompt_title
FROM ratings r
JOIN prompts p ON p.id = r.prompt_id
WHERE r.user_id = $1 AND r.deleted_at IS NULL
ORDER BY r.created_at DESC, r.id DESC
LIMIT $2 OFFSET $3;

-- name: CountMyRatings :one
SELECT count(*)::bigint
FROM ratings
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: CountPromptRatings :one
SELECT count(*)::bigint
FROM ratings
WHERE prompt_id = $1 AND deleted_at IS NULL;