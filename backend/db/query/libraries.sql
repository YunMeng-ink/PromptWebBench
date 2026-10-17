-- 私有题库、成员、题目、题目标签、题目修订与题库回收站的读写。
--
-- 容量判定一律在事务内完成（C-21）：锁定 → 计数 → 判定 → 写入。
-- 题目读取时联出来源提示词的可见性要素，使「引用题是否对当前身份可见」
-- 能在应用层用与提示词同一套谓词判定（C-20）。

-- 锁键 = 命名空间 << 32 + 对象标识。命名空间：1 收藏夹、2 统计重算、3 题库。
-- 高位置命名空间使不同域的同号对象不再互相阻塞。
-- name: LockLibraryOwner :exec
SELECT pg_advisory_xact_lock((3::bigint << 32) + sqlc.arg(owner_id)::bigint);

-- name: CountLibraries :one
SELECT count(*)::bigint
FROM libraries
WHERE owner_id = $1 AND deleted_at IS NULL;

-- name: InsertLibrary :one
INSERT INTO libraries (owner_id, name, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: InsertLibraryMember :execrows
INSERT INTO library_members (library_id, user_id, role, invited_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (library_id, user_id) DO NOTHING;

-- 邀请协作者按用户名定位账户。封禁账户仍可被查到，由服务层拒绝：
-- 把「不存在」与「已封禁」分成两种错误，前端才能给出可读提示。
-- name: GetUserForLibraryInvite :one
SELECT id, username, status
FROM users
WHERE lower(username) = lower(sqlc.arg(username)) AND deleted_at IS NULL;

-- name: GetLibrary :one
SELECT * FROM libraries
WHERE id = $1 AND deleted_at IS NULL;

-- 回收站中的题库需要按标识直接读到，恢复与彻底删除都以此为入口。
-- name: GetLibraryIncludingDeleted :one
SELECT * FROM libraries WHERE id = $1;

-- name: GetLibraryForUpdate :one
SELECT * FROM libraries
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- 题库名称由所有者修改，描述由所有者与协作者修改：调用方按能力拆出两个入口，
-- 因此这里分别给出只改名称与只改描述的语句，避免协作者借道改到名称。
-- name: UpdateLibraryName :one
UPDATE libraries
SET name = sqlc.arg(name), updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateLibraryDescription :one
UPDATE libraries
SET description = sqlc.arg(description), updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- 删除题库进入回收站（需求 §11.2），由软删除加一条回收站记录两步完成。
-- name: SoftDeleteLibrary :one
UPDATE libraries
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: RestoreLibrary :one
UPDATE libraries
SET deleted_at = NULL, updated_at = now()
WHERE id = $1
RETURNING *;

-- 彻底删除回收站中的题库。deleted_at 条件使并发恢复提交后本语句不命中。
-- name: HardDeleteLibrary :execrows
DELETE FROM libraries WHERE id = $1 AND deleted_at IS NOT NULL;

-- 锁定回收站中的题库行：彻底删除前阻止并发的恢复（C-21）。
-- name: LockLibraryIncludingDeleted :one
SELECT * FROM libraries WHERE id = $1 FOR UPDATE;

-- 用户参与的全部题库，含所有者与协作者两种身份，带出本人在库中的角色。
-- name: ListLibrariesForUser :many
SELECT l.*, m.role AS member_role, u.username AS owner_name
FROM library_members m
JOIN libraries l ON l.id = m.library_id
JOIN users u ON u.id = l.owner_id
WHERE m.user_id = sqlc.arg(user_id) AND l.deleted_at IS NULL
ORDER BY l.updated_at DESC, l.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountLibrariesForUser :one
SELECT count(*)::bigint
FROM library_members m
JOIN libraries l ON l.id = m.library_id
WHERE m.user_id = $1 AND l.deleted_at IS NULL;

-- name: GetLibraryMember :one
SELECT * FROM library_members
WHERE library_id = $1 AND user_id = $2;

-- name: ListLibraryMembers :many
SELECT m.*, u.username, u.avatar_url
FROM library_members m
JOIN users u ON u.id = m.user_id
WHERE m.library_id = $1
ORDER BY m.role, m.id;

-- name: DeleteLibraryMember :one
DELETE FROM library_members
WHERE library_id = $1 AND user_id = $2 AND role = 'collaborator'
RETURNING id;

-- 题目容量计数只计引用题（C-22）：派生题不占题库容量，回收站中的引用题仍占。
-- name: CountLibraryQuotaQuestions :one
SELECT count(*)::bigint
FROM library_questions
WHERE library_id = $1 AND kind = 'reference';

-- name: CountLiveLibraryQuestions :one
SELECT count(*)::bigint
FROM library_questions
WHERE library_id = $1 AND deleted_at IS NULL;

-- name: InsertLibraryQuestion :one
INSERT INTO library_questions (
    library_id, kind, source_prompt_id, source_prompt_version_id,
    title, body, description, category_id, ability_id, difficulty_id,
    sensitive_level, sort_order
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- 引用题插入后同步题库的配额计数。条件 WHERE 让并发添加不会越过容量上限：
-- 即便两个请求同时读到未满，也只有一个能命中 increment。
-- name: IncrementLibraryQuestionCount :one
UPDATE libraries
SET question_count = question_count + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND question_count < sqlc.arg(limit_count)
RETURNING question_count;

-- 只有彻底删除引用题才减少配额计数：软删除的题目仍在回收站中，仍占容量（C-22）。
-- name: DecrementLibraryQuestionCount :exec
UPDATE libraries
SET question_count = GREATEST(question_count - 1, 0), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- 题目的来源提示词字段：引用题的正文取自来源提示词当前版本。
-- 来源提示词软删除后仍联出可见性要素与删除标记：派生题继续按来源可见性复查（C-20），
-- 引用题按删除标记显示「来源不可用」。彻底删除被引用检查阻止，此 JOIN 不会因来源缺失而放行。
-- 题库内全部题目（可按需带上回收站中的题目）。回收站列表与正常列表共用这一条语句，
-- 使两者的可见性判定与字段解析只有一份实现。
-- include_deleted 为真时回收站中的题目一并返回，调用方按 deleted_at 区分。
-- name: ListLibraryQuestions :many
SELECT
    q.*,
    p.title             AS source_title,
    p.body              AS source_body,
    p.description       AS source_description,
    p.category_id       AS source_category_id,
    p.ability_id        AS source_ability_id,
    p.difficulty_id     AS source_difficulty_id,
    p.visibility        AS source_visibility,
    p.sensitive_level   AS source_sensitive_level,
    p.uploader_id       AS source_uploader_id,
    p.latest_version_no AS source_version_no,
    p.major_modified    AS source_major_modified,
    p.deleted_at        AS source_deleted_at,
    COALESCE(pc.is_high_risk, false) AS source_is_high_risk,
    COALESCE(pc.name_zh, dc.name_zh, '') AS category_name_zh,
    COALESCE(pc.name_en, dc.name_en, '') AS category_name_en
FROM library_questions q
LEFT JOIN prompts p ON p.id = q.source_prompt_id
LEFT JOIN categories pc ON pc.id = p.category_id
LEFT JOIN categories dc ON dc.id = q.category_id
WHERE q.library_id = sqlc.arg(library_id)
  AND (sqlc.arg(include_deleted)::boolean OR q.deleted_at IS NULL)
ORDER BY q.sort_order, q.id;

-- name: SoftDeleteLibraryQuestion :one
UPDATE library_questions
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: RestoreLibraryQuestion :one
UPDATE library_questions
SET deleted_at = NULL, updated_at = now()
WHERE id = $1
RETURNING *;

-- 彻底删除回收站中的题目。deleted_at 条件使并发恢复提交后本语句不命中。
-- name: HardDeleteLibraryQuestion :execrows
DELETE FROM library_questions WHERE id = $1 AND deleted_at IS NOT NULL;

-- 按给定顺序重排题目：orders 与 ids 按位置对应，只影响本库内未删除的题目。
-- 用 WITH ORDINALITY 把两个数组按下标配对，避免逐条 UPDATE 在一个事务里往返多次。
-- name: ReorderLibraryQuestions :exec
UPDATE library_questions AS q
SET sort_order = data.ord, updated_at = now()
FROM (
    SELECT t.id AS id, (sqlc.arg(orders)::int[])[t.ordinality] AS ord
    FROM unnest(sqlc.arg(ids)::bigint[]) WITH ORDINALITY AS t(id, ordinality)
) AS data
WHERE q.id = data.id AND q.library_id = sqlc.arg(library_id) AND q.deleted_at IS NULL;

-- name: MaxLibraryQuestionSortOrder :one
SELECT COALESCE(max(sort_order), 0)::int
FROM library_questions
WHERE library_id = $1;

-- 派生题的修改只改自身内容，来源提示词不受影响（C-28）。
-- name: UpdateLibraryQuestionContent :one
UPDATE library_questions
SET title = sqlc.arg(title), body = sqlc.arg(body), description = sqlc.arg(description),
    category_id = sqlc.arg(category_id), ability_id = sqlc.arg(ability_id),
    difficulty_id = sqlc.arg(difficulty_id), sensitive_level = sqlc.arg(sensitive_level),
    updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- 引用题被修改时原地转为派生题：保留来源提示词标识供追溯，正文改存副本。
-- name: DeriveLibraryQuestion :one
UPDATE library_questions
SET kind = 'derived', title = sqlc.arg(title), body = sqlc.arg(body),
    description = sqlc.arg(description), category_id = sqlc.arg(category_id),
    ability_id = sqlc.arg(ability_id), difficulty_id = sqlc.arg(difficulty_id),
    sensitive_level = sqlc.arg(sensitive_level), updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- 只按标识定位题目所属题库，用于把「题目 ID」路由到题库后做成员与角色判定。
-- 内容读取统一走 ListLibraryQuestions，避免出现第二份字段解析。
-- name: GetLibraryQuestionRef :one
SELECT id, library_id, kind, deleted_at
FROM library_questions
WHERE id = $1 AND (sqlc.arg(include_deleted)::boolean OR deleted_at IS NULL);

-- name: InsertLibraryQuestionTag :exec
INSERT INTO library_question_tags (question_id, tag)
VALUES ($1, $2)
ON CONFLICT (question_id, tag) DO NOTHING;

-- name: DeleteLibraryQuestionTag :exec
DELETE FROM library_question_tags
WHERE question_id = $1 AND tag = $2;

-- name: DeleteLibraryQuestionTags :exec
DELETE FROM library_question_tags WHERE question_id = $1;

-- name: ListLibraryQuestionTags :many
SELECT tag FROM library_question_tags
WHERE question_id = $1
ORDER BY created_at, id;

-- name: ListLibraryQuestionTagsByQuestions :many
SELECT question_id, tag FROM library_question_tags
WHERE question_id = ANY(sqlc.arg(question_ids)::bigint[])
ORDER BY question_id, created_at, id;

-- name: InsertLibraryQuestionRevision :exec
INSERT INTO library_question_revisions (question_id, editor_id, before, after, note)
VALUES ($1, $2, $3, $4, $5);

-- name: ListLibraryQuestionRevisions :many
SELECT r.*, u.username AS editor_name
FROM library_question_revisions r
JOIN users u ON u.id = r.editor_id
WHERE r.question_id = $1
ORDER BY r.created_at DESC, r.id DESC;

-- 每道题的最近一次修改说明，供导出时写出派生题目的修改说明（需求 §10.8 末项）。
-- name: ListLatestLibraryQuestionRevisionNotes :many
SELECT DISTINCT ON (question_id) question_id, note
FROM library_question_revisions
WHERE question_id = ANY(sqlc.arg(question_ids)::bigint[])
ORDER BY question_id, created_at DESC, id DESC;

-- 回收站条目：同一对象同时只允许一条未恢复未清除的记录，
-- 由 recycle_bin_live_uniq 唯一索引兜底。
-- name: InsertRecycleBinEntry :one
INSERT INTO recycle_bin (
    object_type, object_id, owner_id, library_id, snapshot, deleted_by, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetLiveRecycleBinEntry :one
SELECT * FROM recycle_bin
WHERE object_type = $1 AND object_id = $2
  AND restored_at IS NULL AND purged_at IS NULL;

-- name: ListLibraryRecycleBin :many
SELECT * FROM recycle_bin
WHERE library_id = sqlc.arg(library_id)::bigint AND object_type = 'library_question'
  AND restored_at IS NULL AND purged_at IS NULL
ORDER BY deleted_at DESC, id DESC;

-- name: ListOwnedLibraryRecycleBin :many
SELECT * FROM recycle_bin
WHERE owner_id = sqlc.arg(owner_id)::bigint AND object_type = 'library'
  AND restored_at IS NULL AND purged_at IS NULL
ORDER BY deleted_at DESC, id DESC;

-- name: MarkRecycleBinRestored :exec
UPDATE recycle_bin
SET restored_at = now()
WHERE id = $1 AND restored_at IS NULL AND purged_at IS NULL;

-- name: MarkRecycleBinPurged :exec
UPDATE recycle_bin
SET purged_at = now()
WHERE object_type = $1 AND object_id = $2 AND restored_at IS NULL AND purged_at IS NULL;

-- 彻底删除题库时，库内题目在回收站中的记录一并结清，
-- 否则库已不存在而题目条目仍挂着，回收站会累积无法恢复的孤儿记录。
-- name: MarkLibraryRecycleBinPurged :exec
UPDATE recycle_bin
SET purged_at = now()
WHERE library_id = sqlc.arg(library_id)::bigint AND object_type = 'library_question'
  AND restored_at IS NULL AND purged_at IS NULL;

-- 回收站自动清理：到期且未恢复的记录（需求 §11.3）。
-- 被题库引用的提示词会长期占住 expires_at 最前的批位、拖住后续条目，
-- 这里直接把它们排除出批次：引用解除后自然回到候选集。清理事务内仍复查引用计数（C-21）。
-- name: ListExpiredRecycleBinEntries :many
SELECT rb.* FROM recycle_bin rb
WHERE rb.restored_at IS NULL AND rb.purged_at IS NULL AND rb.expires_at <= now()
  AND (
    rb.object_type <> 'prompt'
    OR NOT EXISTS (
        SELECT 1 FROM library_questions lq WHERE lq.source_prompt_id = rb.object_id
    )
  )
ORDER BY rb.expires_at
LIMIT sqlc.arg(limit_count);
