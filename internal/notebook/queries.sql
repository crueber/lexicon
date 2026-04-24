-- name: CreateAnnotation :one
INSERT INTO annotation (user_id, book_id, book_file_id, type, cfi, page_number, text, note, color)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAnnotationByID :one
SELECT * FROM annotation WHERE id = ? LIMIT 1;

-- name: ListAnnotationsForBook :many
SELECT * FROM annotation WHERE user_id = ? AND book_id = ?
ORDER BY created_at DESC;

-- name: ListAnnotationsForFile :many
SELECT * FROM annotation WHERE user_id = ? AND book_file_id = ?
ORDER BY created_at DESC;

-- name: UpdateAnnotation :exec
UPDATE annotation SET note = ?, color = ?, updated_at = datetime('now')
WHERE id = ? AND user_id = ?;

-- name: DeleteAnnotation :exec
DELETE FROM annotation WHERE id = ? AND user_id = ?;

-- name: ListAllAnnotationsForUser :many
SELECT a.*, bm.title as book_title, bm.cover_path
FROM annotation a
JOIN book_metadata bm ON bm.book_id = a.book_id
WHERE a.user_id = ?
ORDER BY a.created_at DESC
LIMIT ? OFFSET ?;

-- name: ListAllAnnotationsForUserExport :many
SELECT a.*, bm.title as book_title
FROM annotation a
JOIN book_metadata bm ON bm.book_id = a.book_id
WHERE a.user_id = ?
ORDER BY bm.title, a.created_at DESC;

-- name: CountAnnotationsForUser :one
SELECT COUNT(*) FROM annotation WHERE user_id = ?;
