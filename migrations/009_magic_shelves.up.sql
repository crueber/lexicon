CREATE TABLE magic_shelf (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    icon            TEXT,
    icon_color      TEXT,
    rules           TEXT NOT NULL DEFAULT '{}',
    sort_field      TEXT NOT NULL DEFAULT 'added_date',
    sort_dir        TEXT NOT NULL DEFAULT 'DESC',
    limit_count     INTEGER,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_magic_shelf_user_id ON magic_shelf(user_id);
