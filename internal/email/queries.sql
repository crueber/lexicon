-- name: ListEmailProviders :many
SELECT id, name, host, port, username, password, from_address, use_tls, is_default, created_at FROM email_provider ORDER BY name;

-- name: GetEmailProviderByID :one
SELECT id, name, host, port, username, password, from_address, use_tls, is_default, created_at FROM email_provider WHERE id = ? LIMIT 1;

-- name: GetDefaultEmailProvider :one
SELECT id, name, host, port, username, password, from_address, use_tls, is_default, created_at FROM email_provider WHERE is_default = 1 LIMIT 1;

-- name: CreateEmailProvider :one
INSERT INTO email_provider (name, host, port, username, password, from_address, use_tls, is_default)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, name, host, port, username, password, from_address, use_tls, is_default, created_at;

-- name: UpdateEmailProvider :exec
UPDATE email_provider SET name = ?, host = ?, port = ?, username = ?, password = ?, from_address = ?, use_tls = ?, is_default = ? WHERE id = ?;

-- name: DeleteEmailProvider :exec
DELETE FROM email_provider WHERE id = ?;

-- name: ClearEmailProviderDefault :exec
UPDATE email_provider SET is_default = 0 WHERE is_default = 1;

-- name: ListEmailRecipientsByUser :many
SELECT id, user_id, name, email_address, created_at FROM email_recipient WHERE user_id = ? ORDER BY name, email_address;

-- name: GetEmailRecipientByID :one
SELECT id, user_id, name, email_address, created_at FROM email_recipient WHERE id = ? LIMIT 1;

-- name: CreateEmailRecipient :one
INSERT INTO email_recipient (user_id, name, email_address)
VALUES (?, ?, ?)
RETURNING id, user_id, name, email_address, created_at;

-- name: DeleteEmailRecipient :exec
DELETE FROM email_recipient WHERE id = ?;

-- name: GetBookFileForSend :one
SELECT bf.file_path, bf.format, bm.title
FROM book_file bf
LEFT JOIN book_metadata bm ON bf.book_id = bm.book_id
WHERE bf.book_id = ?
ORDER BY bf.id
LIMIT 1;
