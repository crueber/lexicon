CREATE TABLE oidc_session (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    state        TEXT NOT NULL UNIQUE,
    nonce        TEXT NOT NULL,
    redirect_url TEXT,
    user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE oidc_group_mapping (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    group_name     TEXT NOT NULL UNIQUE,
    permission_bit TEXT NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);
