CREATE TABLE tasks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    task_type       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'QUEUED',
    progress        INTEGER DEFAULT 0,
    total           INTEGER DEFAULT 0,
    message         TEXT,
    error           TEXT,
    payload         TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    started_at      TEXT,
    completed_at    TEXT
);

CREATE TABLE task_cron_configuration (
    task_type   TEXT PRIMARY KEY,
    cron_expr   TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1
);
