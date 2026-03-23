-- name: GetUserByID :one
SELECT * FROM users WHERE id = ? LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ? LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, name)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListUsers :many
SELECT * FROM users ORDER BY username;

-- name: UpdateUser :exec
UPDATE users SET email = ?, name = ?, enabled = ? WHERE id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: GetUserPermissions :one
SELECT * FROM user_permissions WHERE user_id = ?;

-- name: UpsertUserPermissions :exec
INSERT INTO user_permissions (user_id, role, can_download, can_upload, can_email_send, can_edit_metadata, opds_access)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
    role = excluded.role,
    can_download = excluded.can_download,
    can_upload = excluded.can_upload,
    can_email_send = excluded.can_email_send,
    can_edit_metadata = excluded.can_edit_metadata,
    opds_access = excluded.opds_access;

-- name: GetUserSettings :one
SELECT * FROM user_settings WHERE user_id = ?;

-- name: UpsertUserSettings :exec
INSERT INTO user_settings (user_id, theme, book_cards_per_row)
VALUES (?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
    theme = excluded.theme,
    book_cards_per_row = excluded.book_cards_per_row;

-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (user_id, token_hash, device_info, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens WHERE token_hash = ? AND revoked = 0;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked = 1 WHERE token_hash = ?;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens SET revoked = 1 WHERE user_id = ?;

-- name: GetAppSetting :one
SELECT value FROM app_settings WHERE key = ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ? WHERE id = ?;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens WHERE expires_at < datetime('now') OR revoked = 1;

-- name: UpsertAppSetting :exec
INSERT INTO app_settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- name: ListUserLibraryIDs :many
SELECT library_id FROM user_library_permission WHERE user_id = ?;

-- name: GetUserSettingsFull :one
SELECT * FROM user_settings WHERE user_id = ?;

-- name: UpdateEpubReaderSetting :exec
UPDATE user_settings SET epub_reader_setting = ? WHERE user_id = ?;

-- name: UpdatePdfReaderSetting :exec
UPDATE user_settings SET pdf_reader_setting = ? WHERE user_id = ?;

-- name: UpdateDashboardSetting :exec
UPDATE user_settings SET dashboard_setting = ? WHERE user_id = ?;
