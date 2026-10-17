-- prompts 与 prompt_versions 的读取与写入。

-- 锁键 = 命名空间 << 32 + 对象标识。命名空间：1 收藏夹、2 统计重算、3 题库、
-- 4 私有提示词额度。高位置命名空间使不同域的同号对象不再互相阻塞。
-- name: LockPromptOwner :exec
SELECT pg_advisory_xact_lock((4::bigint << 32) + sqlc.arg(owner_id)::bigint);

-- name: InsertPrompt :one
INSERT INTO prompts (
    title, body, description, purpose_id, capability_id, difficulty_id,
    visibility, sensitive_level, uploader_id, original_uploader_id,
    latest_version_no, major_modified
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10, $11)
RETURNING *;

-- name: InsertPromptVersion :one
INSERT INTO prompt_versions (
    prompt_id, version_no, major, minor, title, body, description,
    purpose_id, capability_id, difficulty_id, visibility, sensitive_level,
    is_major, change_note, author_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: SetPromptCurrentVersion :one
UPDATE prompts
SET current_version_id = $2,
    latest_version_no = $3,
    major_modified = major_modified OR $4,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- prompts 表自身冗余保存当前版本的全部字段，供列表筛选与判定直接使用，
-- 不必每次联表 prompt_versions。每次修改都要同步，否则列表与详情会给出不同取值。
-- name: UpdatePromptContent :one
UPDATE prompts
SET title = $2,
    body = $3,
    description = $4,
    purpose_id = $5,
    capability_id = $6,
    difficulty_id = $7,
    visibility = $8,
    sensitive_level = $9,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: GetPrompt :one
SELECT p.*, pu.is_high_risk
FROM prompts p
JOIN purposes pu ON pu.id = p.purpose_id
WHERE p.id = $1 AND p.deleted_at IS NULL;

-- 后台读取提示词：包含回收站中的条目，并带出考察目的与考察维度的中英名称供列表与详情直接展示。
-- name: GetPromptIncludingDeleted :one
SELECT p.*, pu.is_high_risk,
       pu.name_zh AS purpose_name_zh, pu.name_en AS purpose_name_en,
       cap.name_zh AS capability_name_zh, cap.name_en AS capability_name_en
FROM prompts p
JOIN purposes pu ON pu.id = p.purpose_id
JOIN capabilities cap ON cap.id = p.capability_id
WHERE p.id = $1;

-- 恢复回收站中的提示词（需求 §11.3）。
-- name: RestorePrompt :one
UPDATE prompts
SET deleted_at = NULL, updated_at = now()
WHERE id = $1 AND deleted_at IS NOT NULL
RETURNING id;

-- 彻底删除提示词前必须先确认没有题库题目引用它：引用题的外键为
-- ON DELETE SET NULL，而 library_questions 的形状约束要求引用题必须有来源，
-- 直接删除会违反约束。服务层据此返回 prompt_in_use。
-- name: CountLibraryQuestionsByPrompt :one
SELECT count(*)::bigint
FROM library_questions
WHERE source_prompt_id = $1::bigint;

-- 彻底删除回收站中的提示词。deleted_at 条件使并发恢复提交后本语句不命中：
-- 清理事务回滚或跳过，恢复方胜出，杜绝「清理竞态删除在线提示词」。
-- name: HardDeletePrompt :execrows
DELETE FROM prompts WHERE id = $1 AND deleted_at IS NOT NULL;

-- 锁定回收站中的提示词行：彻底删除前阻止并发的恢复与引用写入，
-- 使「引用计数 → 删除」两步与「读提示词 → 插引用」互斥（C-21）。
-- 列与 GetPromptIncludingDeleted 一致，服务层复用同一转换函数。
-- name: LockPromptIncludingDeleted :one
SELECT p.*, pu.is_high_risk,
       pu.name_zh AS purpose_name_zh, pu.name_en AS purpose_name_en,
       cap.name_zh AS capability_name_zh, cap.name_en AS capability_name_en
FROM prompts p
JOIN purposes pu ON pu.id = p.purpose_id
JOIN capabilities cap ON cap.id = p.capability_id
WHERE p.id = $1
FOR UPDATE OF p;

-- name: LockPrompt :one
SELECT p.*, pu.is_high_risk
FROM prompts p
JOIN purposes pu ON pu.id = p.purpose_id
WHERE p.id = $1 AND p.deleted_at IS NULL
FOR UPDATE OF p;

-- name: GetPromptVersion :one
SELECT * FROM prompt_versions
WHERE prompt_id = $1 AND version_no = $2;

-- name: GetLatestPromptVersion :one
SELECT * FROM prompt_versions
WHERE prompt_id = $1
ORDER BY major DESC, minor DESC
LIMIT 1;

-- name: ListPromptVersions :many
SELECT * FROM prompt_versions
WHERE prompt_id = $1
ORDER BY major DESC, minor DESC;

-- name: ReplacePromptTags :exec
DELETE FROM prompt_tags WHERE prompt_id = $1;

-- name: AttachPromptTag :exec
INSERT INTO prompt_tags (prompt_id, tag_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListPromptTagIDs :many
SELECT tag_id FROM prompt_tags WHERE prompt_id = $1;

-- name: ListPromptTags :many
SELECT t.*
FROM tags t
JOIN prompt_tags pt ON pt.tag_id = t.id
WHERE pt.prompt_id = $1
ORDER BY t.priority DESC, t.id;

-- name: SoftDeletePrompt :one
UPDATE prompts
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- 重大修改时把当前评分归档到历史版本表（需求 §7.3 第 4 步）。
-- 先归档再删除：归档表保留原始评分时间与创建时间，统计窗口仍以评分创建时间为准。
-- name: ArchivePromptRatings :many
INSERT INTO prompt_version_ratings (
    source_rating_id, user_id, prompt_id, prompt_version_id, model_id,
    score, edit_count, rated_at, archive_reason
)
SELECT r.id, r.user_id, r.prompt_id, r.prompt_version_id, r.model_id, r.score, r.edit_count,
       r.created_at, sqlc.arg(reason)
FROM ratings r
WHERE r.prompt_id = sqlc.arg(prompt_id) AND r.deleted_at IS NULL
RETURNING id;

-- name: DeletePromptRatingsForMajorChange :many
DELETE FROM ratings
WHERE prompt_id = $1 AND deleted_at IS NULL
RETURNING id;

-- 重大修改使统计快照失效，阶段 3 会按新版本重算（需求 §7.3 第 6 步）。
-- name: DeletePromptStatsForMajorChange :exec
DELETE FROM prompt_stats WHERE prompt_id = $1;

-- name: DeletePromptModelStatsForMajorChange :exec
DELETE FROM prompt_model_stats WHERE prompt_id = $1;

-- 私有提示词额度在事务内统计，未删除的私有提示词计入（需求 §8.3、C-21）。
-- name: CountPrivatePrompts :one
SELECT count(*)::bigint
FROM prompts
WHERE uploader_id = $1
  AND visibility = 'private'
  AND deleted_at IS NULL;

-- 评论数与收藏数的冗余计数：详情页与后台热门排序直接读取，
-- 与对应的评论写入、收藏条目写入同事务维护，保证两者一致。
-- name: IncrementPromptCommentCount :exec
UPDATE prompts SET comment_count = comment_count + 1 WHERE id = $1;

-- name: DecrementPromptCommentCount :exec
UPDATE prompts SET comment_count = GREATEST(comment_count - 1, 0) WHERE id = $1;

-- name: IncrementPromptFavoriteCount :exec
UPDATE prompts SET favorite_count = favorite_count + 1 WHERE id = $1;

-- name: DecrementPromptFavoriteCount :exec
UPDATE prompts SET favorite_count = GREATEST(favorite_count - 1, 0) WHERE id = $1;

-- name: ListOwnPrompts :many
SELECT p.*, pu.is_high_risk
FROM prompts p
JOIN purposes pu ON pu.id = p.purpose_id
WHERE p.uploader_id = $1 AND p.deleted_at IS NULL
ORDER BY p.updated_at DESC, p.id DESC
LIMIT $2 OFFSET $3;

-- name: CountOwnPrompts :one
SELECT count(*)::bigint
FROM prompts
WHERE uploader_id = $1 AND deleted_at IS NULL;