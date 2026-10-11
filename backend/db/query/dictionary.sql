-- 字典表查询：类别、能力维度、难度、标签、模型（开发方案 §5.5 字典组）。

-- name: ListCategories :many
SELECT * FROM categories
WHERE is_enabled
ORDER BY sort_order, id;

-- name: GetCategory :one
SELECT * FROM categories
WHERE id = $1;

-- name: ListAbilities :many
SELECT * FROM abilities
WHERE is_enabled
ORDER BY sort_order, id;

-- name: GetAbility :one
SELECT * FROM abilities
WHERE id = $1;

-- name: ListDifficulties :many
SELECT * FROM difficulties
WHERE is_enabled
ORDER BY level, id;

-- name: GetDifficulty :one
SELECT * FROM difficulties
WHERE id = $1;

-- name: ListTags :many
SELECT * FROM tags
WHERE is_enabled
ORDER BY priority DESC, id;

-- name: GetTagsByIDs :many
SELECT * FROM tags
WHERE id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY priority DESC, id;

-- name: ListModels :many
SELECT * FROM models
WHERE is_enabled AND deleted_at IS NULL
ORDER BY name, id;

-- name: GetModel :one
SELECT * FROM models
WHERE id = $1 AND deleted_at IS NULL;