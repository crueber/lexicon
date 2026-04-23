-- name: GetBookFileForReader :one
SELECT bf.id, bf.book_id, bf.file_path, bf.format, b.library_id
FROM book_file bf
JOIN book b ON bf.book_id = b.id
WHERE bf.id = ? AND bf.book_id = ?
LIMIT 1;

-- name: GetReadingProgress :one
SELECT user_id, book_file_id, progress, progress_type, updated_at
FROM user_book_file_progress
WHERE user_id = ? AND book_file_id IN (
    SELECT id FROM book_file WHERE book_id = ?
)
ORDER BY updated_at DESC
LIMIT 1;

-- name: UpsertReadingProgress :exec
INSERT INTO user_book_file_progress (user_id, book_file_id, progress, progress_type, updated_at)
VALUES (?, ?, ?, ?, datetime('now'))
ON CONFLICT(user_id, book_file_id) DO UPDATE SET
    progress = excluded.progress,
    progress_type = excluded.progress_type,
    updated_at = excluded.updated_at;

-- name: GetReaderSettings :one
SELECT epub_reader_setting, pdf_reader_setting FROM user_settings WHERE user_id = ?;

-- name: UpsertEpubReaderSetting :exec
INSERT INTO user_settings (user_id, epub_reader_setting)
VALUES (?, ?)
ON CONFLICT(user_id) DO UPDATE SET epub_reader_setting = excluded.epub_reader_setting;

-- name: UpsertPdfReaderSetting :exec
INSERT INTO user_settings (user_id, pdf_reader_setting)
VALUES (?, ?)
ON CONFLICT(user_id) DO UPDATE SET pdf_reader_setting = excluded.pdf_reader_setting;

-- name: GetAudiobookReaderSetting :one
SELECT audiobook_reader_setting FROM user_settings WHERE user_id = ?;

-- name: UpsertAudiobookReaderSetting :exec
INSERT INTO user_settings (user_id, audiobook_reader_setting)
VALUES (?, ?)
ON CONFLICT(user_id) DO UPDATE SET audiobook_reader_setting = excluded.audiobook_reader_setting;

-- name: GetReadingStats :one
SELECT
    COUNT(DISTINCT book_id) as total_books_read,
    COALESCE(SUM(duration_secs), 0) as total_reading_time
FROM reading_sessions
WHERE user_id = ?;

-- name: GetBooksReadThisMonth :one
SELECT COUNT(DISTINCT book_id) as books_read_this_month
FROM reading_sessions
WHERE user_id = ? AND started_at >= datetime('now', 'start of month');
