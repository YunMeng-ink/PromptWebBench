-- users 表查询。阶段 0 只提供启动期所需的读取与创建。

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE lower(email) = lower($1) AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, role, status, email_verified_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;
