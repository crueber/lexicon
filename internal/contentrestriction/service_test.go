package contentrestriction

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	schema := `
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    email TEXT,
    password_hash TEXT,
    name TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE library (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    book_type TEXT NOT NULL DEFAULT 'EBOOK',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE book (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    folder_path TEXT,
    book_type TEXT NOT NULL DEFAULT 'EBOOK',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    added_date TEXT
);
CREATE TABLE category (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);
CREATE TABLE book_category (
    book_id INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES category(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, category_id)
);
CREATE TABLE tag (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);
CREATE TABLE book_tag (
    book_id INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    tag_id INTEGER NOT NULL REFERENCES tag(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, tag_id)
);
CREATE TABLE mood (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);
CREATE TABLE book_mood (
    book_id INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    mood_id INTEGER NOT NULL REFERENCES mood(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, mood_id)
);
CREATE TABLE comic_metadata (
    book_id INTEGER PRIMARY KEY REFERENCES book(id) ON DELETE CASCADE,
    web TEXT,
    volume INTEGER,
    black_and_white INTEGER DEFAULT 0,
    manga TEXT,
    characters TEXT,
    teams TEXT,
    locations TEXT,
    age_rating TEXT,
    story_arc TEXT,
    series_group TEXT,
    alternate_series TEXT,
    alternate_number REAL,
    alternate_count INTEGER,
    count INTEGER,
    review TEXT,
    type TEXT,
    community_rating REAL
);
CREATE TABLE user_content_restriction (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    restriction_type TEXT NOT NULL,
    value TEXT NOT NULL,
    mode TEXT NOT NULL,
    UNIQUE (user_id, restriction_type, value)
);
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestListRestrictionsEmpty(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	restrictions, err := svc.ListRestrictions(ctx, 1)
	if err != nil {
		t.Fatalf("ListRestrictions error: %v", err)
	}
	if len(restrictions) != 0 {
		t.Fatalf("got %d restrictions; want 0", len(restrictions))
	}
}

func TestListRestrictionsWithData(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO users (username) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := svc.AddRestriction(ctx, 1, TypeCategory, "Explicit", ModeExclude); err != nil {
		t.Fatalf("AddRestriction error: %v", err)
	}

	restrictions, err := svc.ListRestrictions(ctx, 1)
	if err != nil {
		t.Fatalf("ListRestrictions error: %v", err)
	}
	if len(restrictions) != 1 {
		t.Fatalf("got %d restrictions; want 1", len(restrictions))
	}
	r := restrictions[0]
	if r.RestrictionType != TypeCategory {
		t.Errorf("restrictionType = %q; want %q", r.RestrictionType, TypeCategory)
	}
	if r.Value != "Explicit" {
		t.Errorf("value = %q; want %q", r.Value, "Explicit")
	}
	if r.Mode != ModeExclude {
		t.Errorf("mode = %q; want %q", r.Mode, ModeExclude)
	}
}

func TestAddRestriction(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO users (username) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := svc.AddRestriction(ctx, 1, TypeTag, "Violence", ModeExclude); err != nil {
		t.Fatalf("AddRestriction error: %v", err)
	}

	q := New(db)
	rows, err := q.ListRestrictionsByUser(ctx, 1)
	if err != nil {
		t.Fatalf("ListRestrictionsByUser error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows; want 1", len(rows))
	}
	if rows[0].RestrictionType != TypeTag {
		t.Errorf("restrictionType = %q; want %q", rows[0].RestrictionType, TypeTag)
	}
}

func TestRemoveRestriction(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO users (username) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := svc.AddRestriction(ctx, 1, TypeMood, "Dark", ModeExclude); err != nil {
		t.Fatalf("AddRestriction error: %v", err)
	}

	restrictions, _ := svc.ListRestrictions(ctx, 1)
	if len(restrictions) != 1 {
		t.Fatalf("expected 1 restriction before removal")
	}

	if err := svc.RemoveRestriction(ctx, 1, restrictions[0].ID); err != nil {
		t.Fatalf("RemoveRestriction error: %v", err)
	}

	restrictions, err = svc.ListRestrictions(ctx, 1)
	if err != nil {
		t.Fatalf("ListRestrictions error: %v", err)
	}
	if len(restrictions) != 0 {
		t.Fatalf("got %d restrictions after removal; want 0", len(restrictions))
	}
}

func TestFilterBookIDsExclude(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO users (username) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = db.Exec(`INSERT INTO library (name, path) VALUES (?, ?)`, "lib", "/tmp/lib")
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	_, err = db.Exec(`INSERT INTO book (library_id) VALUES (?)`, 1)
	if err != nil {
		t.Fatalf("insert book 1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO book (library_id) VALUES (?)`, 1)
	if err != nil {
		t.Fatalf("insert book 2: %v", err)
	}
	_, err = db.Exec(`INSERT INTO category (name) VALUES (?)`, "Explicit")
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}
	_, err = db.Exec(`INSERT INTO book_category (book_id, category_id) VALUES (?, ?)`, 1, 1)
	if err != nil {
		t.Fatalf("insert book_category: %v", err)
	}

	if err := svc.AddRestriction(ctx, 1, TypeCategory, "Explicit", ModeExclude); err != nil {
		t.Fatalf("AddRestriction error: %v", err)
	}

	bookIDs := []int64{1, 2}
	filtered, err := svc.FilterBookIDs(ctx, 1, false, bookIDs)
	if err != nil {
		t.Fatalf("FilterBookIDs error: %v", err)
	}
	if len(filtered) != 1 || filtered[0] != 2 {
		t.Fatalf("got %v; want [2]", filtered)
	}
}

func TestFilterBookIDsAllowOnly(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO users (username) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = db.Exec(`INSERT INTO library (name, path) VALUES (?, ?)`, "lib", "/tmp/lib")
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	_, err = db.Exec(`INSERT INTO book (library_id) VALUES (?)`, 1)
	if err != nil {
		t.Fatalf("insert book 1: %v", err)
	}
	_, err = db.Exec(`INSERT INTO book (library_id) VALUES (?)`, 1)
	if err != nil {
		t.Fatalf("insert book 2: %v", err)
	}
	_, err = db.Exec(`INSERT INTO tag (name) VALUES (?)`, "Safe")
	if err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	_, err = db.Exec(`INSERT INTO book_tag (book_id, tag_id) VALUES (?, ?)`, 1, 1)
	if err != nil {
		t.Fatalf("insert book_tag: %v", err)
	}

	if err := svc.AddRestriction(ctx, 1, TypeTag, "Safe", ModeAllowOnly); err != nil {
		t.Fatalf("AddRestriction error: %v", err)
	}

	bookIDs := []int64{1, 2}
	filtered, err := svc.FilterBookIDs(ctx, 1, false, bookIDs)
	if err != nil {
		t.Fatalf("FilterBookIDs error: %v", err)
	}
	if len(filtered) != 1 || filtered[0] != 1 {
		t.Fatalf("got %v; want [1]", filtered)
	}
}

func TestFilterBookIDsAdminBypass(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO users (username) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := svc.AddRestriction(ctx, 1, TypeCategory, "Explicit", ModeExclude); err != nil {
		t.Fatalf("AddRestriction error: %v", err)
	}

	bookIDs := []int64{1, 2}
	filtered, err := svc.FilterBookIDs(ctx, 1, true, bookIDs)
	if err != nil {
		t.Fatalf("FilterBookIDs error: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("got %d; want 2", len(filtered))
	}
}

func TestFilterBookIDsNoRestrictions(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	bookIDs := []int64{1, 2, 3}
	filtered, err := svc.FilterBookIDs(ctx, 1, false, bookIDs)
	if err != nil {
		t.Fatalf("FilterBookIDs error: %v", err)
	}
	if len(filtered) != 3 {
		t.Fatalf("got %d; want 3", len(filtered))
	}
}
