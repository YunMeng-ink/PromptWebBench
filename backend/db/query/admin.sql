-- 管理后台的读取与写入（需求第 17 章、C-42 的 11 个模块）。
--
-- 约定：
--   * 后台可见全部对象，因此本文件的查询一律不带可见性谓词；
--     可见性判定（domain.Viewer）只服务于前台，后台的准入由 admin 中间件把关。
--   * 可空的数值列一律 COALESCE 后强制转为 float8，可空的计数一律 COALESCE 后转 bigint，
--     避免 sqlc 生成无法扫描 NULL 的类型（开发方案 §6.4 的 COALESCE 纪律）。
--   * 用于「留空即不筛选」的文本参数显式转 ::text，使 sqlc 按非空 string 生成参数。

-- ============================================================
-- 模块 1：统计仪表盘
-- ============================================================

-- name: AdminDashboardCounts :one
SELECT
    (SELECT count(*)::bigint FROM users WHERE deleted_at IS NULL) AS user_total,
    (SELECT count(*)::bigint FROM users
      WHERE deleted_at IS NULL AND created_at >= sqlc.arg(today_start)::timestamptz) AS user_today,
    (SELECT count(*)::bigint FROM users WHERE deleted_at IS NULL AND role = 'user') AS user_role_user,
    (SELECT count(*)::bigint FROM users WHERE deleted_at IS NULL AND role = 'developer') AS user_role_developer,
    (SELECT count(*)::bigint FROM users WHERE deleted_at IS NULL AND status = 'banned') AS user_banned,
    (SELECT count(*)::bigint FROM prompts WHERE deleted_at IS NULL) AS prompt_total,
    (SELECT count(*)::bigint FROM prompts WHERE deleted_at IS NULL AND visibility = 'public') AS prompt_public,
    (SELECT count(*)::bigint FROM prompts WHERE deleted_at IS NULL AND visibility = 'private') AS prompt_private,
    (SELECT count(*)::bigint FROM prompts WHERE deleted_at IS NULL AND visibility = 'developer') AS prompt_developer,
    (SELECT count(*)::bigint FROM prompts
      WHERE deleted_at IS NULL AND created_at >= sqlc.arg(today_start)::timestamptz) AS prompt_today,
    (SELECT count(*)::bigint FROM ratings WHERE deleted_at IS NULL) AS rating_total,
    (SELECT count(*)::bigint FROM comments WHERE deleted_at IS NULL) AS comment_total,
    (SELECT count(*)::bigint FROM reports WHERE status = 'pending') AS report_pending,
    (SELECT count(*)::bigint FROM recycle_bin
      WHERE restored_at IS NULL AND purged_at IS NULL) AS recycle_total,
    (SELECT count(*)::bigint FROM models WHERE deleted_at IS NULL) AS model_total,
    (SELECT count(*)::bigint FROM invite_codes) AS invite_total,
    (SELECT count(*)::bigint FROM recommend_tags WHERE state = 'pending') AS recommend_pending;

-- 热门提示词按「收藏数 + 评论数」降序取前若干条，再以 ID 升序保证取值确定。
-- name: AdminHotPrompts :many
SELECT p.id, p.title, p.visibility, p.comment_count, p.favorite_count, p.created_at
FROM prompts p
WHERE p.deleted_at IS NULL
ORDER BY (p.favorite_count + p.comment_count) DESC, p.id ASC
LIMIT sqlc.arg(limit_count);

-- name: AdminRecentPrompts :many
SELECT p.id, p.title, p.visibility, p.comment_count, p.favorite_count, p.created_at
FROM prompts p
WHERE p.deleted_at IS NULL
ORDER BY p.created_at DESC, p.id DESC
LIMIT sqlc.arg(limit_count);

-- ============================================================
-- 模块 2：提示词管理
-- ============================================================

-- 后台提示词列表：不取正文，避免把长文本拉进列表响应。
-- 回收站中的提示词默认也在结果内，由 deleted 参数区分（'' 表示不限）。
-- name: AdminListPrompts :many
SELECT p.id, p.title, p.description, p.category_id, p.ability_id, p.difficulty_id,
       p.visibility, p.sensitive_level, p.uploader_id, p.latest_version_no,
       p.major_modified, p.comment_count, p.favorite_count, p.created_at, p.updated_at,
       c.name_zh AS category_name_zh, c.name_en AS category_name_en, c.is_high_risk,
       COALESCE(u.username, '') AS uploader_name,
       (p.deleted_at IS NOT NULL)::boolean AS deleted
FROM prompts p
JOIN categories c ON c.id = p.category_id
LEFT JOIN users u ON u.id = p.uploader_id
WHERE (sqlc.arg(keyword)::text = ''
       OR p.title ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR p.description ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR p.body ILIKE '%' || sqlc.arg(keyword)::text || '%')
  AND (sqlc.arg(visibility)::text = '' OR p.visibility = sqlc.arg(visibility)::text)
  AND (sqlc.arg(deleted)::text = ''
       OR (sqlc.arg(deleted)::text = 'true' AND p.deleted_at IS NOT NULL)
       OR (sqlc.arg(deleted)::text = 'false' AND p.deleted_at IS NULL))
  AND (sqlc.narg(category_id)::bigint IS NULL OR p.category_id = sqlc.narg(category_id)::bigint)
  AND (sqlc.narg(uploader_id)::bigint IS NULL OR p.uploader_id = sqlc.narg(uploader_id)::bigint)
ORDER BY p.updated_at DESC, p.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountPrompts :one
SELECT count(*)::bigint
FROM prompts p
WHERE (sqlc.arg(keyword)::text = ''
       OR p.title ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR p.description ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR p.body ILIKE '%' || sqlc.arg(keyword)::text || '%')
  AND (sqlc.arg(visibility)::text = '' OR p.visibility = sqlc.arg(visibility)::text)
  AND (sqlc.arg(deleted)::text = ''
       OR (sqlc.arg(deleted)::text = 'true' AND p.deleted_at IS NOT NULL)
       OR (sqlc.arg(deleted)::text = 'false' AND p.deleted_at IS NULL))
  AND (sqlc.narg(category_id)::bigint IS NULL OR p.category_id = sqlc.narg(category_id)::bigint)
  AND (sqlc.narg(uploader_id)::bigint IS NULL OR p.uploader_id = sqlc.narg(uploader_id)::bigint);

-- name: DetachPromptTag :exec
DELETE FROM prompt_tags WHERE prompt_id = $1 AND tag_id = $2;

-- name: AdminListPromptRatings :many
SELECT r.id, r.user_id, u.username AS user_name, r.model_id, m.name AS model_name,
       r.score, r.edit_count, r.created_at, r.updated_at,
       (r.deleted_at IS NOT NULL)::boolean AS deleted
FROM ratings r
JOIN users u ON u.id = r.user_id
JOIN models m ON m.id = r.model_id
WHERE r.prompt_id = sqlc.arg(prompt_id)
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountPromptRatings :one
SELECT count(*)::bigint FROM ratings WHERE prompt_id = $1;

-- name: AdminListPromptComments :many
SELECT c.id, c.user_id, u.username AS author_name, c.parent_id, c.body, c.like_count,
       c.created_at, (c.deleted_at IS NOT NULL)::boolean AS deleted
FROM comments c
JOIN users u ON u.id = c.user_id
WHERE c.prompt_id = sqlc.arg(prompt_id)
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountPromptComments :one
SELECT count(*)::bigint FROM comments WHERE prompt_id = $1;

-- ============================================================
-- 模块 3：模型管理
-- ============================================================

-- name: AdminListModels :many
SELECT m.*,
       (SELECT count(*)::bigint FROM ratings r WHERE r.model_id = m.id AND r.deleted_at IS NULL) AS rating_count,
       (SELECT count(*)::bigint FROM prompts p
         JOIN ratings r2 ON r2.prompt_id = p.id
        WHERE r2.model_id = m.id AND r2.deleted_at IS NULL AND p.deleted_at IS NULL) AS prompt_count
FROM models m
WHERE (sqlc.arg(keyword)::text = ''
       OR m.name ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR m.model_key ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR m.provider_name ILIKE '%' || sqlc.arg(keyword)::text || '%')
  AND (sqlc.arg(include_deleted)::boolean OR m.deleted_at IS NULL)
ORDER BY m.provider_name, m.name, m.id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountModels :one
SELECT count(*)::bigint
FROM models m
WHERE (sqlc.arg(keyword)::text = ''
       OR m.name ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR m.model_key ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR m.provider_name ILIKE '%' || sqlc.arg(keyword)::text || '%')
  AND (sqlc.arg(include_deleted)::boolean OR m.deleted_at IS NULL);

-- name: AdminGetModel :one
SELECT * FROM models WHERE id = $1;

-- 重复判断基于「模型标识 + 模型厂商」（需求 §13.4），与 models_identity_active_uniq
-- 唯一索引的列一致。软删除的模型不参与判断，与索引条件保持一致。
-- name: AdminGetModelByIdentity :one
SELECT * FROM models
WHERE model_key = $1 AND provider_key = $2 AND deleted_at IS NULL;

-- 导入预览与执行都按身份列比对，一次取出全部在用模型，避免逐条查询。
-- name: ListModelIdentities :many
SELECT id, name, model_key, provider_key, provider_name
FROM models
WHERE deleted_at IS NULL
ORDER BY id;

-- name: InsertModel :one
INSERT INTO models (
    name, model_key, provider_key, provider_name, input_modalities, aliases,
    description, release_date, context_length, is_enabled, ability_tags, note, import_source
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    sqlc.narg(release_date)::date, sqlc.narg(context_length)::bigint,
    $8, $9, $10, $11
)
RETURNING *;

-- name: UpdateModel :one
UPDATE models
SET name = $2,
    model_key = $3,
    provider_key = $4,
    provider_name = $5,
    input_modalities = $6,
    aliases = $7,
    description = $8,
    release_date = sqlc.narg(release_date)::date,
    context_length = sqlc.narg(context_length)::bigint,
    is_enabled = $9,
    ability_tags = $10,
    note = $11,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SetModelEnabled :one
UPDATE models
SET is_enabled = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteModel :one
UPDATE models
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- 删除模型前的引用检查：存在有效评分即视为被引用（错误码 model_in_use）。
-- name: CountModelRatings :one
SELECT count(*)::bigint FROM ratings WHERE model_id = $1 AND deleted_at IS NULL;

-- name: InsertModelSnapshot :one
INSERT INTO model_snapshots (model_id, source, raw)
VALUES ($1, $2, $3)
RETURNING id;

-- name: ListModelSnapshots :many
SELECT id, model_id, source, raw, created_at
FROM model_snapshots
WHERE model_id = $1
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminListModelPrompts :many
SELECT DISTINCT p.id, p.title, p.visibility, p.created_at
FROM prompts p
JOIN ratings r ON r.prompt_id = p.id
WHERE r.model_id = sqlc.arg(model_id)
  AND r.deleted_at IS NULL
  AND p.deleted_at IS NULL
ORDER BY p.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountModelPrompts :one
SELECT count(DISTINCT p.id)::bigint
FROM prompts p
JOIN ratings r ON r.prompt_id = p.id
WHERE r.model_id = sqlc.arg(model_id) AND r.deleted_at IS NULL AND p.deleted_at IS NULL;

-- 模型的完成度统计：逐提示词给出双窗口计数与校准完成度。
-- name: AdminListModelStats :many
SELECT pms.prompt_id, p.title,
       pms.history_rating_count, COALESCE(pms.history_adjusted, 0)::float8 AS history_adjusted,
       pms.recent_rating_count, COALESCE(pms.recent_adjusted, 0)::float8 AS recent_adjusted,
       pms.computed_at
FROM prompt_model_stats pms
JOIN prompts p ON p.id = pms.prompt_id
WHERE pms.model_id = sqlc.arg(model_id)
ORDER BY pms.prompt_id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountModelStats :one
SELECT count(*)::bigint FROM prompt_model_stats WHERE model_id = $1;

-- name: InsertModelImportLog :one
INSERT INTO model_import_logs (model_id, action, provider_key, model_key, payload, operator_id)
VALUES (sqlc.narg(model_id)::bigint, $1, $2, $3, $4, sqlc.narg(operator_id)::bigint)
RETURNING id;

-- name: AdminListModelImportLogs :many
SELECT l.id, l.model_id, l.action, l.provider_key, l.model_key, l.payload, l.created_at,
       COALESCE(u.username, '') AS operator_name
FROM model_import_logs l
LEFT JOIN users u ON u.id = l.operator_id
WHERE (sqlc.arg(action)::text = '' OR l.action = sqlc.arg(action)::text)
ORDER BY l.created_at DESC, l.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountModelImportLogs :one
SELECT count(*)::bigint
FROM model_import_logs l
WHERE (sqlc.arg(action)::text = '' OR l.action = sqlc.arg(action)::text);

-- ============================================================
-- 模块 4：用户管理
-- ============================================================

-- name: AdminListUsers :many
SELECT *, (SELECT count(*)::bigint FROM prompts p WHERE p.uploader_id = u.id) AS prompt_count
FROM users u
WHERE (sqlc.arg(keyword)::text = ''
       OR u.username ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR u.email ILIKE '%' || sqlc.arg(keyword)::text || '%')
  AND (sqlc.arg(role)::text = '' OR u.role = sqlc.arg(role)::text)
  AND (sqlc.arg(status)::text = '' OR u.status = sqlc.arg(status)::text)
ORDER BY u.created_at DESC, u.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountUsers :one
SELECT count(*)::bigint
FROM users u
WHERE (sqlc.arg(keyword)::text = ''
       OR u.username ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR u.email ILIKE '%' || sqlc.arg(keyword)::text || '%')
  AND (sqlc.arg(role)::text = '' OR u.role = sqlc.arg(role)::text)
  AND (sqlc.arg(status)::text = '' OR u.status = sqlc.arg(status)::text);

-- 后台查看某用户的评分、评论、题库与举报记录，四类各一页。
-- name: AdminListUserRatings :many
SELECT r.id, r.prompt_id, p.title AS prompt_title, r.model_id, m.name AS model_name,
       r.score, r.edit_count, r.created_at, (r.deleted_at IS NOT NULL)::boolean AS deleted
FROM ratings r
JOIN prompts p ON p.id = r.prompt_id
JOIN models m ON m.id = r.model_id
WHERE r.user_id = sqlc.arg(user_id)
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountUserRatings :one
SELECT count(*)::bigint FROM ratings WHERE user_id = $1;

-- name: AdminListUserComments :many
SELECT c.id, c.prompt_id, p.title AS prompt_title, c.parent_id, c.body, c.created_at,
       (c.deleted_at IS NOT NULL)::boolean AS deleted
FROM comments c
JOIN prompts p ON p.id = c.prompt_id
WHERE c.user_id = sqlc.arg(user_id)
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountUserComments :one
SELECT count(*)::bigint FROM comments WHERE user_id = $1;

-- 后台查看用户题库时包含回收站中的题库：管理员需要看到完整的归属关系。
-- name: AdminListUserLibraries :many
SELECT l.id, l.name, l.description, l.question_count,
       (l.deleted_at IS NOT NULL)::boolean AS deleted, l.created_at, l.updated_at
FROM libraries l
WHERE l.owner_id = sqlc.arg(user_id)
ORDER BY l.created_at DESC, l.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountUserLibraries :one
SELECT count(*)::bigint FROM libraries WHERE owner_id = $1;

-- name: AdminListUserReports :many
SELECT r.id, r.object_type, r.object_id, r.reason, r.status, r.detail, r.created_at
FROM reports r
WHERE r.reporter_id = sqlc.arg(user_id)
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountUserReports :one
SELECT count(*)::bigint FROM reports WHERE reporter_id = $1;

-- ============================================================
-- 模块 5：标签管理与推荐标签
-- ============================================================

-- name: AdminListTags :many
SELECT t.*,
       (SELECT count(*)::bigint FROM prompt_tags pt WHERE pt.tag_id = t.id) AS usage_count
FROM tags t
WHERE (sqlc.arg(keyword)::text = ''
       OR t.name_zh ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR t.name_en ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR t.slug ILIKE '%' || sqlc.arg(keyword)::text || '%')
ORDER BY t.priority DESC, t.id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountTags :one
SELECT count(*)::bigint
FROM tags t
WHERE (sqlc.arg(keyword)::text = ''
       OR t.name_zh ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR t.name_en ILIKE '%' || sqlc.arg(keyword)::text || '%'
       OR t.slug ILIKE '%' || sqlc.arg(keyword)::text || '%');

-- name: AdminGetTag :one
SELECT * FROM tags WHERE id = $1;

-- name: InsertTag :one
INSERT INTO tags (slug, name_zh, name_en, color, description, priority, is_enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateTag :one
UPDATE tags
SET slug = $2, name_zh = $3, name_en = $4, color = $5, description = $6,
    priority = $7, is_enabled = $8, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1;

-- name: CountTagPrompts :one
SELECT count(*)::bigint FROM prompt_tags WHERE tag_id = $1;

-- 推荐标签的后台列表带上提示词标题与模型名称，管理员不必逐条跳转详情页。
-- name: AdminListRecommendTags :many
SELECT rt.*, p.title AS prompt_title, COALESCE(m.name, '') AS model_name
FROM recommend_tags rt
JOIN prompts p ON p.id = rt.prompt_id
LEFT JOIN models m ON m.id = rt.model_id
WHERE (array_length(sqlc.arg(states)::text[], 1) IS NULL OR rt.state = ANY(sqlc.arg(states)::text[]))
  AND (sqlc.arg(kind)::text = '' OR rt.kind = sqlc.arg(kind)::text)
ORDER BY rt.triggered_at DESC, rt.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountRecommendTags :one
SELECT count(*)::bigint
FROM recommend_tags rt
WHERE (array_length(sqlc.arg(states)::text[], 1) IS NULL OR rt.state = ANY(sqlc.arg(states)::text[]))
  AND (sqlc.arg(kind)::text = '' OR rt.kind = sqlc.arg(kind)::text);

-- ============================================================
-- 模块 6：举报管理
-- ============================================================

-- 举报列表连带给出被举报对象的摘要：提示词举报给出标题，评论举报给出正文与所属提示词。
-- name: AdminListReports :many
SELECT r.*, u.username AS reporter_name,
       COALESCE(h.username, '') AS handler_name,
       COALESCE(p.title, '') AS prompt_title,
       COALESCE(c.body, '') AS comment_body,
       COALESCE(c.prompt_id, 0)::bigint AS comment_prompt_id
FROM reports r
JOIN users u ON u.id = r.reporter_id
LEFT JOIN users h ON h.id = r.handler_id
LEFT JOIN prompts p ON r.object_type = 'prompt' AND p.id = r.object_id
LEFT JOIN comments c ON r.object_type = 'comment' AND c.id = r.object_id
WHERE (sqlc.arg(object_type)::text = '' OR r.object_type = sqlc.arg(object_type)::text)
  AND (sqlc.arg(status)::text = '' OR r.status = sqlc.arg(status)::text)
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountReports :one
SELECT count(*)::bigint
FROM reports r
WHERE (sqlc.arg(object_type)::text = '' OR r.object_type = sqlc.arg(object_type)::text)
  AND (sqlc.arg(status)::text = '' OR r.status = sqlc.arg(status)::text);

-- name: AdminGetReport :one
SELECT * FROM reports WHERE id = $1;

-- name: HandleReport :one
UPDATE reports
SET status = $2,
    handle_note = $3,
    handler_id = sqlc.narg(handler_id)::bigint,
    handled_at = now(),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetCommentIncludingDeleted :one
SELECT * FROM comments WHERE id = $1;

-- ============================================================
-- 模块 7：注册与邀请码管理
-- ============================================================

-- name: InsertInviteCode :one
INSERT INTO invite_codes (code, kind, created_by, expires_at, max_uses, note)
VALUES ($1, $2, sqlc.narg(created_by)::bigint, sqlc.narg(expires_at)::timestamptz, $3, $4)
RETURNING *;

-- name: AdminListInviteCodes :many
SELECT ic.*, COALESCE(u.username, '') AS creator_name
FROM invite_codes ic
LEFT JOIN users u ON u.id = ic.created_by
WHERE (sqlc.arg(kind)::text = '' OR ic.kind = sqlc.arg(kind)::text)
  AND (sqlc.arg(disabled)::text = '' OR ic.disabled = (sqlc.arg(disabled)::text = 'true'))
ORDER BY ic.created_at DESC, ic.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountInviteCodes :one
SELECT count(*)::bigint
FROM invite_codes ic
WHERE (sqlc.arg(kind)::text = '' OR ic.kind = sqlc.arg(kind)::text)
  AND (sqlc.arg(disabled)::text = '' OR ic.disabled = (sqlc.arg(disabled)::text = 'true'));

-- name: AdminGetInviteCode :one
SELECT * FROM invite_codes WHERE id = $1;

-- name: SetInviteCodeDisabled :one
UPDATE invite_codes
SET disabled = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: AdminListInviteCodeUsages :many
SELECT u.id, u.user_id, COALESCE(us.username, '') AS username, u.used_at
FROM invite_code_usages u
LEFT JOIN users us ON us.id = u.user_id
WHERE u.invite_code_id = sqlc.arg(invite_code_id)
ORDER BY u.used_at DESC, u.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountInviteCodeUsages :one
SELECT count(*)::bigint FROM invite_code_usages WHERE invite_code_id = $1;

-- ============================================================
-- 模块 8：操作日志（只增不改不删，C-43 只提供读取）
-- ============================================================

-- name: AdminListOperationLogs :many
SELECT * FROM operation_logs
WHERE (sqlc.arg(action)::text = '' OR action = sqlc.arg(action)::text)
  AND (sqlc.arg(object_type)::text = '' OR object_type = sqlc.arg(object_type)::text)
  AND (sqlc.arg(operator)::text = '' OR operator_name ILIKE '%' || sqlc.arg(operator)::text || '%')
  AND (sqlc.narg(since)::timestamptz IS NULL OR created_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(until)::timestamptz IS NULL OR created_at < sqlc.narg(until)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountOperationLogs :one
SELECT count(*)::bigint FROM operation_logs
WHERE (sqlc.arg(action)::text = '' OR action = sqlc.arg(action)::text)
  AND (sqlc.arg(object_type)::text = '' OR object_type = sqlc.arg(object_type)::text)
  AND (sqlc.arg(operator)::text = '' OR operator_name ILIKE '%' || sqlc.arg(operator)::text || '%')
  AND (sqlc.narg(since)::timestamptz IS NULL OR created_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(until)::timestamptz IS NULL OR created_at < sqlc.narg(until)::timestamptz);

-- ============================================================
-- 模块 9：公告管理
-- ============================================================

-- 后台列表包含已撤回的公告：管理员需要看到完整的发布历史。
-- name: AdminListAnnouncements :many
SELECT a.*, COALESCE(u.username, '') AS publisher_name
FROM announcements a
LEFT JOIN users u ON u.id = a.published_by
WHERE (sqlc.arg(state)::text = ''
       OR (sqlc.arg(state)::text = 'live' AND a.published_at IS NOT NULL AND a.withdrawn_at IS NULL)
       OR (sqlc.arg(state)::text = 'withdrawn' AND a.withdrawn_at IS NOT NULL))
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountAnnouncements :one
SELECT count(*)::bigint
FROM announcements a
WHERE (sqlc.arg(state)::text = ''
       OR (sqlc.arg(state)::text = 'live' AND a.published_at IS NOT NULL AND a.withdrawn_at IS NULL)
       OR (sqlc.arg(state)::text = 'withdrawn' AND a.withdrawn_at IS NOT NULL));

-- name: AdminGetAnnouncement :one
SELECT * FROM announcements WHERE id = $1;

-- 公告的建立复用 notifications.sql 的 InsertAnnouncement：创建即发布（需求 §17.8「发布公告」）。
-- name: UpdateAnnouncement :one
UPDATE announcements
SET title = $2, body = $3, updated_at = now()
WHERE id = $1 AND withdrawn_at IS NULL
RETURNING *;

-- name: WithdrawAnnouncement :one
UPDATE announcements
SET withdrawn_at = now(), updated_at = now()
WHERE id = $1 AND published_at IS NOT NULL AND withdrawn_at IS NULL
RETURNING *;

-- ============================================================
-- 模块 10：回收站管理
-- ============================================================

-- name: AdminListRecycleBin :many
SELECT rb.*, COALESCE(u.username, '') AS owner_name,
       COALESCE(d.username, '') AS deleted_by_name
FROM recycle_bin rb
LEFT JOIN users u ON u.id = rb.owner_id
LEFT JOIN users d ON d.id = rb.deleted_by
WHERE rb.restored_at IS NULL AND rb.purged_at IS NULL
  AND (sqlc.arg(object_type)::text = '' OR rb.object_type = sqlc.arg(object_type)::text)
ORDER BY rb.deleted_at DESC, rb.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: AdminCountRecycleBin :one
SELECT count(*)::bigint
FROM recycle_bin rb
WHERE rb.restored_at IS NULL AND rb.purged_at IS NULL
  AND (sqlc.arg(object_type)::text = '' OR rb.object_type = sqlc.arg(object_type)::text);

-- name: GetLiveRecycleBinEntryByObject :one
SELECT * FROM recycle_bin
WHERE object_type = sqlc.arg(object_type)
  AND object_id = sqlc.arg(object_id)
  AND restored_at IS NULL
  AND purged_at IS NULL;

-- ============================================================
-- 模块 11：系统设置
-- ============================================================

-- name: AdminListConfig :many
SELECT key, value, is_secret, description, updated_by, updated_at
FROM config
ORDER BY key;

-- name: AdminGetConfig :one
SELECT key, value, secret_ciphertext, is_secret, description, updated_at
FROM config
WHERE key = $1;
