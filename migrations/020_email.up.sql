CREATE TABLE email_provider (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    host        TEXT NOT NULL,
    port        INTEGER NOT NULL DEFAULT 587,
    username    TEXT NOT NULL,
    password    TEXT NOT NULL,
    from_address TEXT NOT NULL,
    use_tls     INTEGER NOT NULL DEFAULT 1,
    is_default  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE email_recipient (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT,
    email_address TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
