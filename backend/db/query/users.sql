-- users 表的读取与写入。

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE lower(email) = lower($1) AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetSystemUser :one
SELECT * FROM users
WHERE is_system AND deleted_at IS NULL
LIMIT 1;

-- name: UsernameExists :one
SELECT EXISTS (
    SELECT 1 FROM users WHERE lower(username) = lower($1) AND deleted_at IS NULL
) AS exists;

-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, role, status, email_verified_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateUserProfile :one
UPDATE users
SET username = $2, bio = $3, avatar_url = $4, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: UpdateUserPassword :one
UPDATE users
SET password_hash = $2,
    token_version = token_version + 1,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: VerifyUserEmail :one
UPDATE users
SET status = 'normal',
    email_verified_at = now(),
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL AND status = 'unverified'
RETURNING *;

-- name: SetUserStatus :one
UPDATE users
SET status = $2,
    banned_at = $3,
    banned_reason = $4,
    token_version = token_version + 1,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SetUserRole :one
UPDATE users
SET role = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: BumpUserTokenVersion :one
UPDATE users
SET token_version = token_version + 1, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: LockUser :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: TransferPrompts :many
UPDATE prompts
SET uploader_id = sqlc.arg(to_uploader),
    original_uploader_id = COALESCE(original_uploader_id, uploader_id),
    updated_at = now()
WHERE uploader_id = sqlc.arg(from_uploader) AND deleted_at IS NULL
RETURNING id;

-- name: CountUsers :one
SELECT count(*) FROM users WHERE deleted_at IS NULL;
