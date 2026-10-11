-- 操作日志与提示词转移日志。操作日志只增不改不删（C-43）。

-- name: InsertOperationLog :one
INSERT INTO operation_logs (
    operator_id, operator_name, action, object_type, object_id,
    before_data, after_data, note, ip
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: InsertTransferLog :exec
INSERT INTO transfer_logs (prompt_id, from_user_id, to_user_id, reason, operator_id)
VALUES ($1, $2, $3, $4, $5);
