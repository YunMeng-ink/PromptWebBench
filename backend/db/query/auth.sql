-- 邮箱验证码、第三方登录绑定与 API Token。

-- name: CreateEmailVerification :one
INSERT INTO email_verifications (user_id, email, code, purpose, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: SupersedeEmailVerifications :exec
UPDATE email_verifications
SET consumed_at = now()
WHERE lower(email) = lower($1) AND purpose = $2 AND consumed_at IS NULL;

-- name: GetActiveEmailVerification :one
SELECT * FROM email_verifications
WHERE lower(email) = lower($1)
  AND purpose = $2
  AND code = $3
  AND consumed_at IS NULL
  AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: ConsumeEmailVerification :exec
UPDATE email_verifications
SET consumed_at = now()
WHERE id = $1;

-- 记录一次验证码校验失败并返回失败计数：达到上限的验证码由调用方作废。
-- 同一（邮箱、用途）只有一个生效验证码（新码申请时旧码作废），子查询 LIMIT 1 只是兜底。
-- name: RecordVerificationFailure :one
UPDATE email_verifications
SET failed_attempts = failed_attempts + 1
WHERE id = (
    SELECT ev.id FROM email_verifications ev
    WHERE lower(ev.email) = lower($1)
      AND ev.purpose = $2
      AND ev.consumed_at IS NULL
      AND ev.expires_at > now()
    ORDER BY ev.created_at DESC
    LIMIT 1
)
RETURNING id, failed_attempts;

-- name: CountRecentEmailVerifications :one
SELECT count(*) FROM email_verifications
WHERE lower(email) = lower($1) AND purpose = $2 AND created_at >= $3;

-- name: GetOAuthAccount :one
SELECT * FROM oauth_accounts
WHERE provider = $1 AND provider_user_id = $2;

-- name: CreateOAuthAccount :one
INSERT INTO oauth_accounts (user_id, provider, provider_user_id, provider_login)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateAPIToken :one
INSERT INTO api_tokens (user_id, token_hash, token_prefix)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByAPITokenHash :one
SELECT u.* FROM api_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1 AND t.revoked_at IS NULL AND t.expires_at > now()
  AND u.deleted_at IS NULL;

-- name: TouchAPIToken :exec
UPDATE api_tokens
SET last_used_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: GetActiveAPIToken :one
SELECT * FROM api_tokens
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeActiveAPITokens :exec
UPDATE api_tokens
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;
