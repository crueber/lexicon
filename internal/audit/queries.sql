-- name: CreateAuditLog :one
INSERT INTO audit_log (user_id, username, action, resource_type, resource_id, details, ip_address, country)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListAuditLogsFiltered :many
-- Parameters: 1=action_check, 2=action, 3=user_id_check, 4=user_id,
--             5=from_check, 6=from_date, 7=to_check, 8=to_date
SELECT * FROM audit_log
WHERE (? = '' OR action = ?)
  AND (CAST(COALESCE(?, -1) AS INTEGER) = -1 OR user_id = ?)
  AND (? = '' OR created_at >= ?)
  AND (? = '' OR created_at <= ?)
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountAuditLogsFiltered :one
-- Parameters: 1=action_check, 2=action, 3=user_id_check, 4=user_id,
--             5=from_check, 6=from_date, 7=to_check, 8=to_date
SELECT COUNT(*) FROM audit_log
WHERE (? = '' OR action = ?)
  AND (CAST(COALESCE(?, -1) AS INTEGER) = -1 OR user_id = ?)
  AND (? = '' OR created_at >= ?)
  AND (? = '' OR created_at <= ?);

-- name: DeleteOldAuditLogs :exec
DELETE FROM audit_log WHERE created_at <= datetime('now', ?);
