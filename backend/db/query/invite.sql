-- 邀请码的读取与占用。占用走条件更新，保证并发下不重复消耗（C-21）。

-- name: GetInviteCodeByCode :one
SELECT * FROM invite_codes
WHERE code = $1;

-- name: ConsumeInviteCode :one
UPDATE invite_codes
SET used_count = used_count + 1,
    updated_at = now()
WHERE id = $1
  AND disabled = false
  AND used_count < max_uses
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: CreateInviteCodeUsage :exec
INSERT INTO invite_code_usages (invite_code_id, user_id)
VALUES ($1, $2);
