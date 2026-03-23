-- Library tables
CREATE TABLE library (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    name                    TEXT NOT NULL,
    icon                    TEXT,
    icon_color              TEXT,
    organization_mode       TEXT NOT NULL DEFAULT 'BOOK_PER_FILE',
    file_naming_pattern     TEXT,
    created_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE library_path (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id  INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    UNIQUE (library_id, path)
);

CREATE TABLE library_metadata_source (
    library_id      INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    field_priority  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (library_id, provider)
);

CREATE TABLE user_library_permission (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    library_id  INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, library_id)
);
