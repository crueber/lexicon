CREATE TABLE user_content_restriction (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    restriction_type TEXT NOT NULL,
    value           TEXT NOT NULL,
    mode            TEXT NOT NULL,
    UNIQUE (user_id, restriction_type, value)
);
