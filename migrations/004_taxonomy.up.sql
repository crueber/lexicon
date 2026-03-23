-- Author, series, category, tag, mood + junction tables
CREATE TABLE author (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    bio         TEXT,
    photo_path  TEXT,
    audnexus_id TEXT
);

CREATE TABLE book_author (
    book_id     INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    author_id   INTEGER NOT NULL REFERENCES author(id) ON DELETE CASCADE,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, author_id)
);

CREATE TABLE series (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE book_series (
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    series_id       INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    series_number   REAL,
    PRIMARY KEY (book_id, series_id)
);

CREATE TABLE category (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE book_category (
    book_id     INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES category(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, category_id)
);

CREATE TABLE tag (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE book_tag (
    book_id INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tag(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, tag_id)
);

CREATE TABLE mood (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE book_mood (
    book_id INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    mood_id INTEGER NOT NULL REFERENCES mood(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, mood_id)
);
