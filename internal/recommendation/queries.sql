-- name: UpsertBookVector :exec
INSERT INTO book_vectors (book_id, vector, updated_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT(book_id) DO UPDATE SET
    vector = excluded.vector,
    updated_at = excluded.updated_at;

-- name: GetBookVector :one
SELECT vector FROM book_vectors WHERE book_id = ? LIMIT 1;

-- name: ListAllBookVectors :many
SELECT bv.book_id, bv.vector, b.library_id, b.book_type, b.added_date, bm.title, bm.cover_path
FROM book_vectors bv
JOIN book b ON bv.book_id = b.id
LEFT JOIN book_metadata bm ON b.id = bm.book_id;

-- name: ListAllBooksForVectorRebuild :many
SELECT b.id, b.library_id, b.book_type, b.added_date,
       bm.title, bm.cover_path, bm.language, bm.publisher
FROM book b
LEFT JOIN book_metadata bm ON b.id = bm.book_id;

-- name: DeleteBookVector :exec
DELETE FROM book_vectors WHERE book_id = ?;

-- name: ListBookAuthors :many
SELECT a.name FROM author a JOIN book_author ba ON a.id = ba.author_id WHERE ba.book_id = ? ORDER BY ba.sort_order;

-- name: ListBookSeries :many
SELECT s.name FROM series s JOIN book_series bs ON s.id = bs.series_id WHERE bs.book_id = ?;

-- name: ListBookCategories :many
SELECT c.name FROM category c JOIN book_category bc ON c.id = bc.category_id WHERE bc.book_id = ?;

-- name: ListBookTags :many
SELECT t.name FROM tag t JOIN book_tag bt ON t.id = bt.tag_id WHERE bt.book_id = ?;

-- name: ListAuthorsForBooks :many
SELECT ba.book_id, a.name
FROM book_author ba
JOIN author a ON a.id = ba.author_id
WHERE ba.book_id IN (SELECT book_id FROM book_vectors)
ORDER BY ba.book_id, ba.sort_order;
