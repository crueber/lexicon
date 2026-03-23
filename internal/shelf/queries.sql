-- name: CreateShelf :one
INSERT INTO shelf (user_id, name, description, icon, icon_color, is_public)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetShelfByID :one
SELECT * FROM shelf WHERE id = ? LIMIT 1;

-- name: ListShelvesForUser :many
SELECT * FROM shelf WHERE user_id = ? ORDER BY name;

-- name: UpdateShelf :exec
UPDATE shelf SET name = ?, description = ?, icon = ?, icon_color = ?, is_public = ?,
    updated_at = datetime('now')
WHERE id = ? AND user_id = ?;

-- name: DeleteShelf :exec
DELETE FROM shelf WHERE id = ? AND user_id = ?;

-- name: AddBookToShelf :exec
INSERT OR IGNORE INTO shelf_book (shelf_id, book_id, sort_order)
VALUES (?, ?, (SELECT COALESCE(MAX(sb2.sort_order), 0) + 1 FROM shelf_book sb2 WHERE sb2.shelf_id = ?));

-- name: RemoveBookFromShelf :exec
DELETE FROM shelf_book WHERE shelf_id = ? AND book_id = ?;

-- name: ListBooksInShelf :many
SELECT b.id, b.library_id, b.book_type, b.added_date,
       bm.title, bm.cover_path,
       sb.sort_order, sb.added_at
FROM shelf_book sb
JOIN book b ON b.id = sb.book_id
LEFT JOIN book_metadata bm ON bm.book_id = b.id
WHERE sb.shelf_id = ?
ORDER BY sb.sort_order, sb.added_at;

-- name: CountBooksInShelf :one
SELECT COUNT(*) FROM shelf_book WHERE shelf_id = ?;

-- name: IsBookInShelf :one
SELECT COUNT(*) FROM shelf_book WHERE shelf_id = ? AND book_id = ?;

-- name: ListShelvesContainingBook :many
SELECT s.* FROM shelf s
JOIN shelf_book sb ON sb.shelf_id = s.id
WHERE sb.book_id = ? AND s.user_id = ?;
