-- name: CreateBook :one
INSERT INTO book (library_id, folder_path, book_type, added_date)
VALUES (?, ?, ?, datetime('now'))
RETURNING *;

-- name: GetBookByID :one
SELECT * FROM book WHERE id = ? LIMIT 1;

-- name: ListBooksByLibrary :many
SELECT * FROM book WHERE library_id = ? ORDER BY id;

-- name: DeleteBook :exec
DELETE FROM book WHERE id = ?;

-- name: CreateBookFile :one
INSERT INTO book_file (book_id, file_path, format, file_size, fingerprint, track_number, track_title, duration_secs)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetBookFileByID :one
SELECT * FROM book_file WHERE id = ? LIMIT 1;

-- name: GetBookFileByPath :one
SELECT * FROM book_file WHERE file_path = ? LIMIT 1;

-- name: GetBookFileByFingerprint :one
SELECT * FROM book_file WHERE fingerprint = ? LIMIT 1;

-- name: ListBookFiles :many
SELECT * FROM book_file WHERE book_id = ? ORDER BY track_number, file_path;

-- name: UpdateBookFilePath :exec
UPDATE book_file SET file_path = ? WHERE id = ?;

-- name: UpdateBookFileFingerprint :exec
UPDATE book_file SET fingerprint = ?, file_size = ? WHERE id = ?;

-- name: DeleteBookFile :exec
DELETE FROM book_file WHERE id = ?;

-- name: UpsertBookMetadata :exec
INSERT INTO book_metadata (book_id, title, original_title, subtitle, description, publisher, publish_date, page_count, language, isbn_10, isbn_13)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(book_id) DO UPDATE SET
    title = CASE WHEN book_metadata.title_locked = 0 THEN excluded.title ELSE book_metadata.title END,
    original_title = excluded.original_title,
    subtitle = CASE WHEN book_metadata.subtitle_locked = 0 THEN excluded.subtitle ELSE book_metadata.subtitle END,
    description = CASE WHEN book_metadata.description_locked = 0 THEN excluded.description ELSE book_metadata.description END,
    publisher = CASE WHEN book_metadata.publisher_locked = 0 THEN excluded.publisher ELSE book_metadata.publisher END,
    publish_date = CASE WHEN book_metadata.publish_date_locked = 0 THEN excluded.publish_date ELSE book_metadata.publish_date END,
    page_count = CASE WHEN book_metadata.page_count_locked = 0 THEN excluded.page_count ELSE book_metadata.page_count END,
    language = CASE WHEN book_metadata.language_locked = 0 THEN excluded.language ELSE book_metadata.language END,
    isbn_10 = CASE WHEN book_metadata.isbn_10_locked = 0 THEN excluded.isbn_10 ELSE book_metadata.isbn_10 END,
    isbn_13 = CASE WHEN book_metadata.isbn_13_locked = 0 THEN excluded.isbn_13 ELSE book_metadata.isbn_13 END;

-- name: GetBookMetadata :one
SELECT * FROM book_metadata WHERE book_id = ? LIMIT 1;

-- name: UpdateBookCover :exec
UPDATE book_metadata SET cover_path = ?, cover_updated_at = datetime('now') WHERE book_id = ?;

-- name: GetOrCreateAuthor :one
INSERT INTO author (name) VALUES (?) ON CONFLICT(name) DO UPDATE SET name = name RETURNING *;

-- name: LinkBookAuthor :exec
INSERT OR IGNORE INTO book_author (book_id, author_id, sort_order) VALUES (?, ?, ?);

-- name: ListBookAuthors :many
SELECT a.* FROM author a JOIN book_author ba ON a.id = ba.author_id WHERE ba.book_id = ? ORDER BY ba.sort_order;

-- name: GetOrCreateSeries :one
INSERT INTO series (name) VALUES (?) ON CONFLICT(name) DO UPDATE SET name = name RETURNING *;

-- name: LinkBookSeries :exec
INSERT OR IGNORE INTO book_series (book_id, series_id, series_number) VALUES (?, ?, ?);

-- name: ListBookSeries :many
SELECT s.*, bs.series_number FROM series s JOIN book_series bs ON s.id = bs.series_id WHERE bs.book_id = ?;

-- name: GetOrCreateCategory :one
INSERT INTO category (name) VALUES (?) ON CONFLICT(name) DO UPDATE SET name = name RETURNING *;

-- name: LinkBookCategory :exec
INSERT OR IGNORE INTO book_category (book_id, category_id) VALUES (?, ?);

-- name: ListBookCategories :many
SELECT c.* FROM category c JOIN book_category bc ON c.id = bc.category_id WHERE bc.book_id = ?;

-- name: GetOrCreateTag :one
INSERT INTO tag (name) VALUES (?) ON CONFLICT(name) DO UPDATE SET name = name RETURNING *;

-- name: LinkBookTag :exec
INSERT OR IGNORE INTO book_tag (book_id, tag_id) VALUES (?, ?);

-- name: ListBookTags :many
SELECT t.* FROM tag t JOIN book_tag bt ON t.id = bt.tag_id WHERE bt.book_id = ?;

-- name: UpsertProgress :exec
INSERT INTO user_book_file_progress (user_id, book_file_id, progress, progress_type, updated_at)
VALUES (?, ?, ?, ?, datetime('now'))
ON CONFLICT(user_id, book_file_id) DO UPDATE SET
    progress = excluded.progress,
    progress_type = excluded.progress_type,
    updated_at = excluded.updated_at;

-- name: GetBookByFolderPath :one
SELECT * FROM book WHERE library_id = ? AND folder_path = ? LIMIT 1;

-- name: ListBooksWithMetadata :many
SELECT b.id, b.library_id, b.book_type, b.added_date,
       bm.title, bm.cover_path
FROM book b
LEFT JOIN book_metadata bm ON b.id = bm.book_id
WHERE b.library_id = ?
ORDER BY b.id
LIMIT ? OFFSET ?;

-- name: CountBooksByLibrary :one
SELECT COUNT(*) FROM book WHERE library_id = ?;

-- name: GetProgress :one
SELECT * FROM user_book_file_progress WHERE user_id = ? AND book_file_id = ? LIMIT 1;

-- name: GetBookWithMetadata :one
SELECT b.id, b.library_id, b.folder_path, b.book_type, b.created_at, b.added_date,
       bm.title, bm.subtitle, bm.description, bm.publisher, bm.publish_date,
       bm.page_count, bm.language, bm.isbn_10, bm.isbn_13, bm.cover_path,
       bm.google_books_id, bm.amazon_id, bm.goodreads_id, bm.hardcover_id
FROM book b
LEFT JOIN book_metadata bm ON b.id = bm.book_id
WHERE b.id = ? LIMIT 1;

-- name: GetBookWithLibraryID :one
SELECT b.id, b.library_id FROM book b WHERE b.id = ? LIMIT 1;

-- name: ListBooksWithMetadataAndAuthors :many
SELECT b.id, bm.title, bm.isbn_10, bm.isbn_13,
       GROUP_CONCAT(a.name, '|') as author_names
FROM book b
LEFT JOIN book_metadata bm ON b.id = bm.book_id
LEFT JOIN book_author ba ON b.id = ba.book_id
LEFT JOIN author a ON ba.author_id = a.id
GROUP BY b.id;

-- name: DismissDuplicate :exec
INSERT INTO duplicate_dismiss (book_id_a, book_id_b, dismissed_by)
VALUES (?, ?, ?)
ON CONFLICT(book_id_a, book_id_b) DO NOTHING;

-- name: ListBooksWithFilesAndMetadata :many
SELECT
    b.id as book_id,
    bm.title,
    GROUP_CONCAT(a.name, '|') as author_names,
    s.name as series_name,
    bs.series_number as series_number,
    bf.id as file_id,
    bf.file_path,
    (SELECT path FROM library_path WHERE library_id = b.library_id LIMIT 1) as library_path
FROM book b
LEFT JOIN book_metadata bm ON b.id = bm.book_id
LEFT JOIN book_author ba ON b.id = ba.book_id
LEFT JOIN author a ON ba.author_id = a.id
LEFT JOIN book_series bs ON b.id = bs.book_id
LEFT JOIN series s ON bs.series_id = s.id
LEFT JOIN book_file bf ON b.id = bf.book_id
GROUP BY b.id, bf.id;

-- name: ListDismissedDuplicates :many
SELECT book_id_a, book_id_b FROM duplicate_dismiss;
