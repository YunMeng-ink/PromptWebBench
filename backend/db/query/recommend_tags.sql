-- recommend_tags 的读写。推荐标签生命周期由 C-12 ~ C-16 规定。

-- 同一「提示词 × 模型 × 标签类型」同时只保留一条未终结的推荐标签，
-- 唯一索引在该状态下约束，重算时冲突即视为标签已存在（C-13）。
--
-- name: InsertRecommendTag :one
INSERT INTO recommend_tags (
    prompt_id, model_id, scope, kind, state, label_zh, label_en,
    confidence, reason, snapshot, triggered_at
)
VALUES (
    sqlc.arg(prompt_id),
    sqlc.arg(model_id),
    sqlc.arg(scope)::text,
    sqlc.arg(kind)::text,
    sqlc.arg(state)::text,
    sqlc.arg(label_zh),
    sqlc.arg(label_en),
    sqlc.arg(confidence)::text,
    sqlc.arg(reason),
    sqlc.arg(snapshot)::jsonb,
    sqlc.arg(triggered_at)::timestamptz
)
ON CONFLICT DO NOTHING
RETURNING *;

-- 详情页展示已确认的推荐标签。忽略、过期与待确认的推荐不进前台。
-- name: ListConfirmedRecommendTags :many
SELECT * FROM recommend_tags
WHERE prompt_id = $1 AND state = 'confirmed'
ORDER BY triggered_at DESC, id DESC;

-- 详情页在管理员身份下展示全部推荐标签，供后台处理（开发方案 §6.7）。
-- name: ListRecommendTagsByPrompt :many
SELECT * FROM recommend_tags
WHERE prompt_id = $1
ORDER BY triggered_at DESC, id DESC;

-- name: ListLiveRecommendTagsByKind :many
SELECT * FROM recommend_tags
WHERE prompt_id = sqlc.arg(prompt_id)
  AND model_id = sqlc.arg(model_id)
  AND kind = ANY(sqlc.arg(kinds)::text[])
  AND state IN ('pending', 'confirmed', 'review');

-- 区分度回升时把已存在的待确认过时标签转已忽略并记录审计日志（C-15）。
-- 条件里带 state = 'pending'：已确认的标签由管理员显式处理，系统不代为改动。
-- name: IgnorePendingTags :many
UPDATE recommend_tags
SET state = 'ignored',
    updated_at = now()
WHERE prompt_id = sqlc.arg(prompt_id)
  AND kind = ANY(sqlc.arg(kinds)::text[])
  AND state = 'pending'
RETURNING *;

-- 提示词被删除或彻底删除时，推荐标签一律转已过期（C-14）。
-- name: ExpireRecommendTagsByPrompt :exec
UPDATE recommend_tags
SET state = 'expired', updated_at = now()
WHERE prompt_id = $1
  AND state IN ('pending', 'confirmed', 'review');

-- 重大修改：过时标签转待复核，训练饱和标签转待复核（C-12）。
-- name: MoveTagsToReview :exec
UPDATE recommend_tags
SET state = 'review', updated_at = now()
WHERE prompt_id = sqlc.arg(prompt_id)
  AND kind = ANY(sqlc.arg(kinds)::text[])
  AND state IN ('pending', 'confirmed');

-- 管理员处置：确认、忽略、修改文案。转移条件与状态由服务层校验。
-- name: UpdateRecommendTag :one
UPDATE recommend_tags
SET state = sqlc.arg(state)::text,
    label_zh = sqlc.arg(label_zh),
    label_en = sqlc.arg(label_en),
    reviewed_by = sqlc.arg(reviewed_by)::bigint,
    reviewed_at = sqlc.arg(reviewed_at)::timestamptz,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetRecommendTag :one
SELECT * FROM recommend_tags WHERE id = $1;

-- 推荐标签批量处理与后台列表（开发方案 §6.7）。
-- name: ListRecommendTagsByState :many
SELECT * FROM recommend_tags
WHERE state = ANY(sqlc.arg(states)::text[])
ORDER BY triggered_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountRecommendTagsByState :one
SELECT count(*)::bigint
FROM recommend_tags
WHERE state = ANY(sqlc.arg(states)::text[]);