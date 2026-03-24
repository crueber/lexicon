-- name: CreateKOReaderUser :one
INSERT INTO koreader_user (user_id, username, password_md5)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetKOReaderUserByUsername :one
SELECT * FROM koreader_user WHERE username = ? LIMIT 1;

-- name: UpsertKOReaderProgress :one
INSERT INTO koreader_progress (koreader_user_id, document, progress, percentage, device, device_id, timestamp)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(koreader_user_id, document) DO UPDATE SET
    progress = excluded.progress,
    percentage = excluded.percentage,
    device = excluded.device,
    device_id = excluded.device_id,
    timestamp = excluded.timestamp
RETURNING *;

-- name: GetKOReaderProgress :one
SELECT * FROM koreader_progress WHERE koreader_user_id = ? AND document = ? LIMIT 1;
