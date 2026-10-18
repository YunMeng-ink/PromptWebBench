-- rating_images、rating_outputs 与展示区的读写。
--
-- 公开展示只包含「公开且非高风险」提示词下的素材：图片与模型输出跟着评分走，
-- 私有、开发者可见或高风险提示词的素材不进公开展示区（C-18、C-19、C-20）。
-- 管理员列表不在此限，另行提供。

-- name: InsertRatingImage :one
INSERT INTO rating_images (rating_id, user_id, prompt_id, model_id, url, filename)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: InsertRatingOutput :one
INSERT INTO rating_outputs (rating_id, user_id, prompt_id, model_id, content)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (rating_id) WHERE deleted_at IS NULL
DO UPDATE SET content = EXCLUDED.content, created_at = now()
RETURNING *;

-- 展示区按「提供厂商 × 模型」聚合素材：图片与文本输出各自按模型去重计数，
-- 最近提交时间分开取，由服务层比较先后；GREATEST 的多态参数在 sqlc 中
-- 只能推断为 interface{}，分开取时间列后每一列的类型都是确定的。
-- name: ListShowcaseGroups :many
WITH visible_images AS (
    SELECT i.id, i.model_id, i.created_at
    FROM rating_images i
    JOIN prompts p ON p.id = i.prompt_id AND p.deleted_at IS NULL
    JOIN purposes pu ON pu.id = p.purpose_id
    WHERE i.deleted_at IS NULL AND p.visibility = 'public' AND NOT pu.is_high_risk
),
visible_outputs AS (
    SELECT o.id, o.model_id, o.created_at
    FROM rating_outputs o
    JOIN prompts p ON p.id = o.prompt_id AND p.deleted_at IS NULL
    JOIN purposes pu ON pu.id = p.purpose_id
    WHERE o.deleted_at IS NULL AND p.visibility = 'public' AND NOT pu.is_high_risk
)
SELECT m.id   AS model_id,
       m.name AS model_name,
       m.provider_key AS provider_key,
       m.provider_name AS provider_name,
       count(DISTINCT vi.id)::bigint AS image_count,
       count(DISTINCT vo.id)::bigint AS output_count,
       max(vi.created_at)::timestamptz AS last_image_at,
       max(vo.created_at)::timestamptz AS last_output_at
FROM models m
LEFT JOIN visible_images vi ON vi.model_id = m.id
LEFT JOIN visible_outputs vo ON vo.model_id = m.id
WHERE m.deleted_at IS NULL AND m.is_enabled
GROUP BY m.id, m.name, m.provider_key, m.provider_name
HAVING count(DISTINCT vi.id) > 0 OR count(DISTINCT vo.id) > 0
ORDER BY m.provider_key, m.name;

-- 展示区内某个模型的素材列表：图片与文本输出按提交时间混排。
-- 两段查询的列类型必须一致（union 的列类型由各段同名位置决定），
-- 不参与的分支用空串占位，避免 sqlc 把可空列映射为非指针类型。
-- name: ListShowcaseEntries :many
SELECT 'image'::text AS kind, i.id AS entry_id, i.rating_id, i.user_id,
       i.url, ''::text AS content, i.created_at,
       p.id AS prompt_id, p.title AS prompt_title, u.username AS username
FROM rating_images i
JOIN ratings r ON r.id = i.rating_id AND r.deleted_at IS NULL
JOIN prompts p ON p.id = i.prompt_id AND p.deleted_at IS NULL
JOIN purposes pu ON pu.id = p.purpose_id
JOIN users u ON u.id = i.user_id
WHERE i.model_id = $1 AND i.deleted_at IS NULL
  AND p.visibility = 'public' AND NOT pu.is_high_risk
UNION ALL
SELECT 'output'::text AS kind, o.id AS entry_id, o.rating_id, o.user_id,
       ''::text AS url, o.content, o.created_at,
       p.id AS prompt_id, p.title AS prompt_title, u.username AS username
FROM rating_outputs o
JOIN ratings r ON r.id = o.rating_id AND r.deleted_at IS NULL
JOIN prompts p ON p.id = o.prompt_id AND p.deleted_at IS NULL
JOIN purposes pu ON pu.id = p.purpose_id
JOIN users u ON u.id = o.user_id
WHERE o.model_id = $1 AND o.deleted_at IS NULL
  AND p.visibility = 'public' AND NOT pu.is_high_risk
ORDER BY created_at DESC, entry_id DESC
LIMIT $2 OFFSET $3;

-- name: CountShowcaseEntries :one
SELECT ((
    SELECT count(*) FROM rating_images i
      JOIN ratings r ON r.id = i.rating_id AND r.deleted_at IS NULL
      JOIN prompts p ON p.id = i.prompt_id AND p.deleted_at IS NULL
      JOIN purposes pu ON pu.id = p.purpose_id
      WHERE i.model_id = $1 AND i.deleted_at IS NULL
        AND p.visibility = 'public' AND NOT pu.is_high_risk
  ) + (
    SELECT count(*) FROM rating_outputs o
      JOIN ratings r ON r.id = o.rating_id AND r.deleted_at IS NULL
      JOIN prompts p ON p.id = o.prompt_id AND p.deleted_at IS NULL
      JOIN purposes pu ON pu.id = p.purpose_id
      WHERE o.model_id = $1 AND o.deleted_at IS NULL
        AND p.visibility = 'public' AND NOT pu.is_high_risk
  ))::bigint AS total;

-- 管理员图片列表：不过滤提示词可见性，按提交时间倒序分页。
-- name: ListAdminRatingImages :many
SELECT i.id, i.url, i.filename, i.prompt_id, p.title AS prompt_title,
       i.model_id, m.name AS model_name, u.username AS uploader,
       i.created_at
FROM rating_images i
JOIN prompts p ON p.id = i.prompt_id
JOIN models m ON m.id = i.model_id
JOIN users u ON u.id = i.user_id
WHERE i.deleted_at IS NULL
ORDER BY i.created_at DESC, i.id DESC
LIMIT $1 OFFSET $2;

-- name: CountAdminRatingImages :one
SELECT count(*)::bigint FROM rating_images WHERE deleted_at IS NULL;

-- name: ListAdminRatingOutputs :many
SELECT o.id, o.content, o.prompt_id, p.title AS prompt_title,
       o.model_id, m.name AS model_name, u.username AS uploader,
       o.created_at
FROM rating_outputs o
JOIN prompts p ON p.id = o.prompt_id
JOIN models m ON m.id = o.model_id
JOIN users u ON u.id = o.user_id
WHERE o.deleted_at IS NULL
ORDER BY o.created_at DESC, o.id DESC
LIMIT $1 OFFSET $2;

-- name: CountAdminRatingOutputs :one
SELECT count(*)::bigint FROM rating_outputs WHERE deleted_at IS NULL;

-- 删除指定图片（按 ID 精确删除），记下执行删除的管理员。
-- name: SoftDeleteRatingImage :execrows
UPDATE rating_images
SET deleted_at = now(), deleted_by = $2
WHERE id = $1 AND deleted_at IS NULL;

-- 删除指定文本输出（按 ID 精确删除），记下执行删除的管理员。
-- name: SoftDeleteRatingOutput :execrows
UPDATE rating_outputs
SET deleted_at = now(), deleted_by = $2
WHERE id = $1 AND deleted_at IS NULL;

-- 用户在某条评分下的全部图片（数量校验用）。
-- name: ListRatingImagesByRating :many
SELECT * FROM rating_images
WHERE rating_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC;

-- 单条图片的完整信息（权限判定与删除用）。
-- name: GetRatingImage :one
SELECT * FROM rating_images
WHERE id = $1 AND deleted_at IS NULL;

-- 单条文本输出的完整信息（权限判定与删除用）。
-- name: GetRatingOutput :one
SELECT * FROM rating_outputs
WHERE id = $1 AND deleted_at IS NULL;
