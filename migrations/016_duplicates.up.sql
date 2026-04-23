CREATE TABLE duplicate_dismiss (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id_a   INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    book_id_b   INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    dismissed_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (book_id_a, book_id_b)
);
