CREATE TABLE provider_priority (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_name TEXT NOT NULL UNIQUE,
    priority    INTEGER NOT NULL DEFAULT 5,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
