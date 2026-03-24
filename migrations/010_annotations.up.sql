CREATE TABLE annotation (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    book_file_id    INTEGER REFERENCES book_file(id) ON DELETE SET NULL,
    type            TEXT NOT NULL DEFAULT 'HIGHLIGHT',
    cfi             TEXT,
    page_number     INTEGER,
    text            TEXT,
    note            TEXT,
    color           TEXT NOT NULL DEFAULT 'yellow',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_annotation_user_book ON annotation(user_id, book_id);
CREATE INDEX idx_annotation_book_file ON annotation(book_file_id);
