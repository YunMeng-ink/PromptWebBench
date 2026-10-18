-- 字典表查询：考察维度、考察目的、难度、标签、模型（开发方案 §5.5 字典组）。

-- name: ListPurposes :many
SELECT * FROM purposes
WHERE is_enabled
ORDER BY sort_order, id;

-- name: GetPurpose :one
SELECT * FROM purposes
WHERE id = $1;

-- name: AdminListPurposes :many
SELECT * FROM purposes
WHERE (sqlc.arg(keyword)::text = ''
       OR name_zh ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR name_en ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR slug ILIKE '%' || sqlc.arg(keyword)::text || '%')
ORDER BY sort_order, id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountPurposes :one
SELECT count(*)::bigint
FROM purposes
WHERE (sqlc.arg(keyword)::text = ''
       OR name_zh ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR name_en ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR slug ILIKE '%' || sqlc.arg(keyword)::text || '%');

-- name: AdminGetPurpose :one
SELECT * FROM purposes WHERE id = $1;

-- name: InsertPurpose :one
INSERT INTO purposes (slug, name_zh, name_en, description_zh, description_en, is_high_risk, is_enabled, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdatePurpose :one
UPDATE purposes
SET name_zh = $2, name_en = $3, description_zh = $4, description_en = $5,
    is_high_risk = $6, is_enabled = $7, sort_order = $8, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeletePurpose :exec
DELETE FROM purposes WHERE id = $1;

-- name: CountPurposeReferences :one
SELECT (SELECT count(*) FROM prompts p WHERE p.purpose_id = $1)
     + (SELECT count(*) FROM prompt_versions v WHERE v.purpose_id = $1)
     + (SELECT count(*) FROM library_questions q WHERE q.purpose_id = $1);

-- name: ListCapabilities :many
SELECT * FROM capabilities
WHERE is_enabled
ORDER BY sort_order, id;

-- name: GetCapability :one
SELECT * FROM capabilities
WHERE id = $1;

-- name: GetCapabilitiesByIDs :many
SELECT * FROM capabilities
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: AdminListCapabilities :many
SELECT * FROM capabilities
WHERE (sqlc.arg(keyword)::text = ''
       OR name_zh ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR name_en ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR slug ILIKE '%' || sqlc.arg(keyword)::text || '%')
ORDER BY sort_order, id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountCapabilities :one
SELECT count(*)::bigint
FROM capabilities
WHERE (sqlc.arg(keyword)::text = ''
       OR name_zh ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR name_en ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR slug ILIKE '%' || sqlc.arg(keyword)::text || '%');

-- name: AdminGetCapability :one
SELECT * FROM capabilities WHERE id = $1;

-- name: InsertCapability :one
INSERT INTO capabilities (slug, name_zh, name_en, group_key, description_zh, description_en, is_enabled, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateCapability :one
UPDATE capabilities
SET name_zh = $2, name_en = $3, group_key = $4,
    description_zh = $5, description_en = $6, is_enabled = $7, sort_order = $8, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteCapability :exec
DELETE FROM capabilities WHERE id = $1;

-- 考察维度可能作为提示词的第二至第五个维度出现，引用计数以关联表为准，
-- 并按提示词与版本去重，避免同一对象被计多次。
-- name: CountCapabilityReferences :one
SELECT (SELECT count(*) FROM prompt_capabilities pc WHERE pc.capability_id = $1)
     + (SELECT count(*) FROM prompt_version_capabilities pvc WHERE pvc.capability_id = $1)
     + (SELECT count(*) FROM prompts p WHERE p.capability_id = $1)
     + (SELECT count(*) FROM prompt_versions v WHERE v.capability_id = $1)
     + (SELECT count(*) FROM library_questions q WHERE q.capability_id = $1)
     + (SELECT count(*) FROM model_capabilities mc WHERE mc.capability_id = $1);

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
ORDER BY kind, priority DESC, id;

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

-- name: ListModelCapabilityIDs :many
SELECT capability_id FROM model_capabilities WHERE model_id = $1;

-- name: ListModelCapabilitiesForModels :many
SELECT model_id, capability_id
FROM model_capabilities
WHERE model_id = ANY(sqlc.arg(model_ids)::bigint[])
ORDER BY model_id, capability_id;

-- name: ReplaceModelCapabilities :exec
DELETE FROM model_capabilities WHERE model_id = $1;

-- name: AttachModelCapability :exec
INSERT INTO model_capabilities (model_id, capability_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;
