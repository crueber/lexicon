-- Users & Auth tables
CREATE TABLE users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT UNIQUE NOT NULL,
    email           TEXT,
    password_hash   TEXT,
    name            TEXT,
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE user_permissions (
    user_id                 INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    role                    TEXT NOT NULL DEFAULT 'USER',
    can_download            INTEGER NOT NULL DEFAULT 0,
    can_upload              INTEGER NOT NULL DEFAULT 0,
    can_email_send          INTEGER NOT NULL DEFAULT 0,
    can_edit_metadata       INTEGER NOT NULL DEFAULT 0,
    opds_access             INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_settings (
    user_id                     INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    theme                       TEXT DEFAULT 'dark',
    book_cards_per_row          INTEGER DEFAULT 6,
    pdf_reader_setting          TEXT,
    epub_reader_setting         TEXT,
    comic_reader_setting        TEXT,
    audiobook_reader_setting    TEXT,
    sidebar_setting             TEXT,
    dashboard_setting           TEXT
);

CREATE TABLE refresh_tokens (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    device_info TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT NOT NULL,
    revoked     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE app_settings (
    key     TEXT PRIMARY KEY,
    value   TEXT
);
