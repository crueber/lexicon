-- name: CreateBookdropFile :one
INSERT INTO bookdrop_file (original_filename, file_path, file_size, status, extracted_title, extracted_authors, extracted_cover_path)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetBookdropFileByID :one
SELECT * FROM bookdrop_file WHERE id = ? LIMIT 1;

-- name: ListPendingBookdropFiles :many
SELECT * FROM bookdrop_file WHERE status = 'PENDING' ORDER BY created_at DESC;

-- name: UpdateBookdropFileStatus :exec
UPDATE bookdrop_file SET status = ?, processed_at = datetime('now'), imported_book_id = ? WHERE id = ?;

-- name: CountBookdropFilesByPath :one
SELECT COUNT(*) FROM bookdrop_file WHERE file_path = ?;

-- name: DeleteBookdropFile :exec
DELETE FROM bookdrop_file WHERE id = ?;
