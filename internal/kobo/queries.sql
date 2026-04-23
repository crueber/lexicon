-- name: UpsertKoboDevice :exec
INSERT INTO kobo_device (user_id, device_id, device_name, model, firmware, last_sync_at)
VALUES (?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(device_id) DO UPDATE SET
    device_name = excluded.device_name,
    model = excluded.model,
    firmware = excluded.firmware,
    last_sync_at = excluded.last_sync_at;

-- name: GetKoboDeviceByDeviceID :one
SELECT * FROM kobo_device WHERE device_id = ? LIMIT 1;

-- name: UpsertKoboReadingState :exec
INSERT INTO kobo_reading_state (user_id, book_file_id, content_id, status, percent_read, current_cfi, last_modified)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, content_id) DO UPDATE SET
    status = excluded.status,
    percent_read = excluded.percent_read,
    current_cfi = excluded.current_cfi,
    last_modified = excluded.last_modified;

-- name: ListKoboReadingStates :many
SELECT * FROM kobo_reading_state WHERE user_id = ?;

-- name: DeleteKoboReadingState :exec
DELETE FROM kobo_reading_state WHERE user_id = ? AND content_id = ?;
