CREATE TABLE koreader_user (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER REFERENCES users(id) ON DELETE SET NULL,
    username        TEXT NOT NULL UNIQUE,
    password_md5    TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE koreader_progress (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    koreader_user_id INTEGER NOT NULL REFERENCES koreader_user(id) ON DELETE CASCADE,
    document        TEXT NOT NULL,
    progress        TEXT NOT NULL,
    percentage      REAL NOT NULL DEFAULT 0,
    device          TEXT,
    device_id       TEXT,
    timestamp       INTEGER NOT NULL DEFAULT 0,
    UNIQUE(koreader_user_id, document)
);

CREATE INDEX idx_koreader_progress_user ON koreader_progress(koreader_user_id);
