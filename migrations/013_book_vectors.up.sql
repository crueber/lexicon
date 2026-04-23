CREATE TABLE book_vectors (
    book_id     INTEGER PRIMARY KEY REFERENCES book(id) ON DELETE CASCADE,
    vector      BLOB NOT NULL,          -- 128 x float32 = 512 bytes
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
