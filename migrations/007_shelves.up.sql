CREATE TABLE shelf (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT,
    icon        TEXT,
    icon_color  TEXT,
    is_public   INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE shelf_book (
    shelf_id    INTEGER NOT NULL REFERENCES shelf(id) ON DELETE CASCADE,
    book_id     INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    added_at    TEXT NOT NULL DEFAULT (datetime('now')),
    sort_order  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (shelf_id, book_id)
);

CREATE INDEX idx_shelf_user_id ON shelf(user_id);
CREATE INDEX idx_shelf_book_shelf_id ON shelf_book(shelf_id);
