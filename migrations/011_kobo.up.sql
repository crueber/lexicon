CREATE TABLE kobo_device (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       TEXT NOT NULL UNIQUE,
    device_name     TEXT,
    model           TEXT,
    firmware        TEXT,
    last_sync_at    TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE kobo_reading_state (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_file_id    INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    content_id      TEXT NOT NULL,
    status          TEXT,
    percent_read    REAL,
    current_cfi     TEXT,
    rest_of_book_estimate INTEGER,
    time_spent_reading INTEGER,
    last_modified   TEXT,
    UNIQUE(user_id, content_id)
);

CREATE INDEX idx_kobo_device_user ON kobo_device(user_id);
CREATE INDEX idx_kobo_reading_state_user ON kobo_reading_state(user_id);
