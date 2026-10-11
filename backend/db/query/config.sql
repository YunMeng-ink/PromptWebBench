-- config 表的读写。value 为 JSONB，键名与类型语义由调用方按 C-45 解释。

-- name: ListConfig :many
SELECT key, value, secret_ciphertext, is_secret, description, updated_at
FROM config
ORDER BY key;

-- name: GetConfig :one
SELECT key, value, secret_ciphertext, is_secret, description, updated_at
FROM config
WHERE key = $1;

-- name: UpsertConfig :exec
INSERT INTO config (key, value, description, updated_by, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    secret_ciphertext = NULL,
    is_secret = false,
    description = EXCLUDED.description,
    updated_by = EXCLUDED.updated_by,
    updated_at = now();

-- name: UpsertSecretConfig :exec
INSERT INTO config (key, value, secret_ciphertext, is_secret, description, updated_by, updated_at)
VALUES ($1, NULL, $2, true, $3, $4, now())
ON CONFLICT (key) DO UPDATE
SET value = NULL,
    secret_ciphertext = EXCLUDED.secret_ciphertext,
    is_secret = true,
    description = EXCLUDED.description,
    updated_by = EXCLUDED.updated_by,
    updated_at = now();
