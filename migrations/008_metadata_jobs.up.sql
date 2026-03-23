CREATE TABLE metadata_fetch_job (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    provider        TEXT,
    error           TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE metadata_proposal (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    provider_id     TEXT,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    data            TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_metadata_proposal_book_id ON metadata_proposal(book_id);
