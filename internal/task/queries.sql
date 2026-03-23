-- name: CreateTask :one
INSERT INTO tasks (task_type, status, payload)
VALUES (?, 'QUEUED', ?)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = ? LIMIT 1;

-- name: ListRecentTasks :many
SELECT * FROM tasks ORDER BY created_at DESC LIMIT 50;

-- name: UpdateTaskStatus :exec
UPDATE tasks SET status = ?, updated_at = datetime('now') WHERE id = ?;

-- name: UpdateTaskProgress :exec
UPDATE tasks SET progress = ?, total = ?, message = ?, updated_at = datetime('now') WHERE id = ?;

-- name: StartTask :exec
UPDATE tasks SET status = 'RUNNING', started_at = datetime('now'), updated_at = datetime('now') WHERE id = ?;

-- name: CompleteTask :exec
UPDATE tasks SET status = 'COMPLETED', completed_at = datetime('now'), updated_at = datetime('now') WHERE id = ?;

-- name: FailTask :exec
UPDATE tasks SET status = 'FAILED', error = ?, completed_at = datetime('now'), updated_at = datetime('now') WHERE id = ?;

-- name: CancelTask :exec
UPDATE tasks SET status = 'CANCELLED', updated_at = datetime('now') WHERE id = ? AND status IN ('QUEUED', 'RUNNING');

-- name: MarkInterruptedTasksFailed :exec
UPDATE tasks SET status = 'FAILED', error = 'interrupted by server restart', updated_at = datetime('now')
WHERE status = 'RUNNING';

-- name: GetCronConfig :one
SELECT * FROM task_cron_configuration WHERE task_type = ? LIMIT 1;

-- name: ListCronConfigs :many
SELECT * FROM task_cron_configuration ORDER BY task_type;

-- name: UpsertCronConfig :exec
INSERT INTO task_cron_configuration (task_type, cron_expr, enabled)
VALUES (?, ?, ?)
ON CONFLICT(task_type) DO UPDATE SET cron_expr = excluded.cron_expr, enabled = excluded.enabled;
