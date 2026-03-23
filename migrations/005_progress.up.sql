-- Reading progress tables
CREATE TABLE user_book_file_progress (
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_file_id    INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    progress        TEXT,
    progress_type   TEXT,
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, book_file_id)
);

CREATE TABLE reading_sessions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    book_file_id    INTEGER REFERENCES book_file(id) ON DELETE SET NULL,
    start_progress  TEXT,
    end_progress    TEXT,
    started_at      TEXT NOT NULL,
    ended_at        TEXT,
    duration_secs   INTEGER
);
