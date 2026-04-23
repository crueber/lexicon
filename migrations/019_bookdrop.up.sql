CREATE TABLE bookdrop_file (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    original_filename   TEXT NOT NULL,
    file_path           TEXT NOT NULL,
    file_size           INTEGER NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'PENDING',
    extracted_title     TEXT,
    extracted_authors   TEXT,
    extracted_cover_path TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at        TEXT,
    imported_book_id    INTEGER REFERENCES book(id) ON DELETE SET NULL
);

CREATE INDEX idx_bookdrop_file_status ON bookdrop_file(status);
