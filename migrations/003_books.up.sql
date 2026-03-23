-- Book tables
CREATE TABLE book (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id          INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    folder_path         TEXT,
    book_type           TEXT NOT NULL DEFAULT 'EBOOK',
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    added_date          TEXT
);

CREATE TABLE book_file (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    file_path       TEXT NOT NULL UNIQUE,
    format          TEXT NOT NULL,
    file_size       INTEGER,
    fingerprint     TEXT,
    track_number    INTEGER,
    track_title     TEXT,
    duration_secs   INTEGER,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE book_metadata (
    book_id             INTEGER PRIMARY KEY REFERENCES book(id) ON DELETE CASCADE,
    title               TEXT,
    original_title      TEXT,
    subtitle            TEXT,
    description         TEXT,
    publisher           TEXT,
    publish_date        TEXT,
    page_count          INTEGER,
    language            TEXT,
    isbn_10             TEXT,
    isbn_13             TEXT,
    google_books_id     TEXT,
    amazon_id           TEXT,
    goodreads_id        TEXT,
    hardcover_id        TEXT,
    audible_id          TEXT,
    comicvine_id        TEXT,
    cover_path          TEXT,
    cover_updated_at    TEXT,
    title_locked            INTEGER NOT NULL DEFAULT 0,
    subtitle_locked         INTEGER NOT NULL DEFAULT 0,
    description_locked      INTEGER NOT NULL DEFAULT 0,
    publisher_locked        INTEGER NOT NULL DEFAULT 0,
    publish_date_locked     INTEGER NOT NULL DEFAULT 0,
    page_count_locked       INTEGER NOT NULL DEFAULT 0,
    language_locked         INTEGER NOT NULL DEFAULT 0,
    isbn_10_locked          INTEGER NOT NULL DEFAULT 0,
    isbn_13_locked          INTEGER NOT NULL DEFAULT 0,
    cover_locked            INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE comic_metadata (
    book_id         INTEGER PRIMARY KEY REFERENCES book(id) ON DELETE CASCADE,
    web             TEXT,
    volume          INTEGER,
    black_and_white INTEGER DEFAULT 0,
    manga           TEXT,
    characters      TEXT,
    teams           TEXT,
    locations       TEXT,
    age_rating      TEXT,
    story_arc       TEXT,
    series_group    TEXT,
    alternate_series    TEXT,
    alternate_number    REAL,
    alternate_count     INTEGER,
    count               INTEGER,
    review              TEXT,
    type                TEXT,
    community_rating    REAL
);
