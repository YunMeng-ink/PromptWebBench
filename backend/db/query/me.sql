-- 个人数据导出与账号注销（G-08、G-09）。

-- ---------- G-08：个人数据导出 ----------

-- name: ExportMyPrompts :many
SELECT p.id, p.title, p.body, p.description, p.visibility, p.sensitive_level,
       p.latest_version_no, p.created_at, p.updated_at,
       pu.name_zh AS purpose_name_zh, pu.name_en AS purpose_name_en,
       cap.name_zh AS capability_name_zh, cap.name_en AS capability_name_en
FROM prompts p
JOIN purposes pu ON pu.id = p.purpose_id
JOIN capabilities cap ON cap.id = p.capability_id
WHERE p.uploader_id = $1 AND p.deleted_at IS NULL
ORDER BY p.created_at ASC, p.id ASC;

-- name: ExportMyRatings :many
SELECT r.id, r.prompt_id, p.title AS prompt_title, m.name AS model_name, r.score,
       r.edit_count, r.created_at, r.updated_at
FROM ratings r
JOIN prompts p ON p.id = r.prompt_id
JOIN models m ON m.id = r.model_id
WHERE r.user_id = $1 AND r.deleted_at IS NULL
ORDER BY r.created_at ASC, r.id ASC;

-- name: ExportMyComments :many
SELECT c.id, c.prompt_id, p.title AS prompt_title, c.parent_id, c.body,
       c.created_at
FROM comments c
JOIN prompts p ON p.id = c.prompt_id
WHERE c.user_id = $1 AND c.deleted_at IS NULL
ORDER BY c.created_at ASC, c.id ASC;

-- ---------- G-09：账号注销 ----------

-- 软删账户并匿名化：昵称统一改为「已注销用户」（唯一索引带 deleted_at IS NULL
-- 条件，注销后昵称不再参与唯一性），邮箱改为不可投递地址以释放原邮箱占用。
-- 注销后无法登录、无法找回；原昵称与邮箱经审计日志留痕。
-- name: SoftDeleteUser :one
UPDATE users
SET username = '已注销用户',
    email = 'deleted_' || id::text || '@deleted.invalid',
    password_hash = NULL,
    bio = '',
    avatar_url = '',
    status = 'banned',
    banned_at = now(),
    banned_reason = 'account_deleted',
    token_version = token_version + 1,
    deleted_at = now(),
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND NOT is_system
RETURNING id;

-- 注销时清空凭据类附属数据：第三方绑定与验证码在账户软删后不再有意义，
-- 留着会让同一个人重新注册时撞上指向已注销账户的绑定。
-- name: DeleteUserOAuthAccounts :exec
DELETE FROM oauth_accounts WHERE user_id = $1;

-- name: DeleteUserEmailVerifications :exec
DELETE FROM email_verifications WHERE user_id = $1;

-- 注销时软删该用户名下的收藏夹与题库：保留行与归属记录供管理员追溯，
-- 停用其访问价值。
-- name: SoftDeleteUserFavorites :exec
UPDATE favorites
SET deleted_at = now(), updated_at = now()
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteUserLibraries :exec
UPDATE libraries
SET deleted_at = now(), updated_at = now()
WHERE owner_id = $1 AND deleted_at IS NULL;

-- 注销时移除该用户的关注关系：指向已注销账户的关注行没有展示价值。
-- name: DeleteUserFollows :exec
DELETE FROM follows WHERE follower_id = $1 OR followee_id = $1;

-- 注销前统计活跃管理员数量，用于阻止最后一个管理员注销自己：
-- 注销后无人能进入后台，平台只有重启才会按 ADMIN_INIT_* 重建初始管理员。
-- name: CountActiveAdmins :one
SELECT count(*)::bigint FROM users
WHERE role = 'admin' AND deleted_at IS NULL;