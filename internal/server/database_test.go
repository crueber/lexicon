package server

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	migrations "github.com/crueber/lexicon/migrations"
)

func TestOpenDatabase(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase(%q) error: %v", dataDir, err)
	}
	defer db.Close()

	// Verify the database file was created.
	dbPath := filepath.Join(dataDir, "lexicon.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("database file not created at %s", dbPath)
	}
}

func TestOpenDatabaseCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "nested", "data")

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase(%q) error: %v", dataDir, err)
	}
	defer db.Close()

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Errorf("data directory not created at %s", dataDir)
	}
}

func TestDatabasePragmas(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "journal_mode", query: "PRAGMA journal_mode", want: "wal"},
		{name: "busy_timeout", query: "PRAGMA busy_timeout", want: "5000"},
		{name: "foreign_keys", query: "PRAGMA foreign_keys", want: "1"},
		{name: "synchronous", query: "PRAGMA synchronous", want: "1"}, // NORMAL = 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if err := db.QueryRow(tt.query).Scan(&got); err != nil {
				t.Fatalf("query %q error: %v", tt.query, err)
			}
			if got != tt.want {
				t.Errorf("%s = %q; want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestRunMigrations(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	logger := newLogger("error", "text")

	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("RunMigrations error: %v", err)
	}

	// Verify all tables from migrations 001-005 exist by querying them.
	tables := []string{
		// 001: users
		"users", "user_permissions", "user_settings", "refresh_tokens", "app_settings",
		// 002: libraries
		"library", "library_path", "library_metadata_source", "user_library_permission",
		// 003: books
		"book", "book_file", "book_metadata", "comic_metadata",
		// 004: taxonomy
		"author", "book_author", "series", "book_series",
		"category", "book_category", "tag", "book_tag", "mood", "book_mood",
		// 005: progress
		"user_book_file_progress", "reading_sessions",
	}
	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			query := "SELECT count(*) FROM " + table
			var count int
			if err := db.QueryRow(query).Scan(&count); err != nil {
				t.Errorf("table %q not accessible: %v", table, err)
			}
		})
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	logger := newLogger("error", "text")

	// Run migrations twice — the second run should be a no-op.
	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("first RunMigrations error: %v", err)
	}
	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("second RunMigrations error: %v", err)
	}
}

func TestMigrationsCreateCorrectSchema(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	logger := newLogger("error", "text")
	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("RunMigrations error: %v", err)
	}

	ctx := context.Background()

	// Insert a user and verify the schema works end-to-end.
	result, err := db.ExecContext(ctx,
		"INSERT INTO users (username, email, name) VALUES (?, ?, ?)",
		"testuser", "test@example.com", "Test User",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}

	// Verify user was created.
	var username string
	if err := db.QueryRowContext(ctx, "SELECT username FROM users WHERE id = ?", userID).Scan(&username); err != nil {
		t.Fatalf("select user: %v", err)
	}
	if username != "testuser" {
		t.Errorf("username = %q; want %q", username, "testuser")
	}

	// Verify foreign key constraint works — insert permissions for the user.
	_, err = db.ExecContext(ctx,
		"INSERT INTO user_permissions (user_id, role) VALUES (?, ?)",
		userID, "ADMIN",
	)
	if err != nil {
		t.Fatalf("insert permissions: %v", err)
	}

	// Verify foreign key constraint rejects invalid user_id.
	_, err = db.ExecContext(ctx,
		"INSERT INTO user_permissions (user_id, role) VALUES (?, ?)",
		99999, "USER",
	)
	if err == nil {
		t.Error("expected foreign key violation for invalid user_id; got nil")
	}

	// Verify cascade delete — deleting user should delete permissions.
	_, err = db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var permCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM user_permissions WHERE user_id = ?", userID).Scan(&permCount); err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permCount != 0 {
		t.Errorf("permissions count after cascade delete = %d; want 0", permCount)
	}
}

func TestDatabasePing(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Errorf("db.Ping() error: %v", err)
	}
}

// openTestDB is a helper that opens a database in a temp directory and runs
// migrations. It returns the database and a cleanup function.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dataDir := t.TempDir()
	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}

	logger := newLogger("error", "text")
	if err := RunMigrations(db, logger); err != nil {
		db.Close()
		t.Fatalf("RunMigrations error: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

// createTestUser inserts a user and returns its ID. Fails the test on error.
func createTestUser(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()
	ctx := context.Background()
	result, err := db.ExecContext(ctx,
		"INSERT INTO users (username) VALUES (?)", username,
	)
	if err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// createTestLibrary inserts a library and returns its ID. Fails the test on error.
func createTestLibrary(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	ctx := context.Background()
	result, err := db.ExecContext(ctx,
		"INSERT INTO library (name) VALUES (?)", name,
	)
	if err != nil {
		t.Fatalf("insert library %q: %v", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

func TestLibraryInsertAndSelect(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	libID := createTestLibrary(t, db, "My Library")

	var name string
	var orgMode string
	err := db.QueryRowContext(ctx,
		"SELECT name, organization_mode FROM library WHERE id = ?", libID,
	).Scan(&name, &orgMode)
	if err != nil {
		t.Fatalf("select library: %v", err)
	}
	if name != "My Library" {
		t.Errorf("library name = %q; want %q", name, "My Library")
	}
	if orgMode != "BOOK_PER_FILE" {
		t.Errorf("organization_mode = %q; want %q", orgMode, "BOOK_PER_FILE")
	}
}

func TestLibraryPathUniqueConstraint(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	libID := createTestLibrary(t, db, "Test Library")

	_, err := db.ExecContext(ctx,
		"INSERT INTO library_path (library_id, path) VALUES (?, ?)", libID, "/books",
	)
	if err != nil {
		t.Fatalf("first insert library_path: %v", err)
	}

	// Duplicate path for same library should fail.
	_, err = db.ExecContext(ctx,
		"INSERT INTO library_path (library_id, path) VALUES (?, ?)", libID, "/books",
	)
	if err == nil {
		t.Error("expected unique constraint violation for duplicate library_path; got nil")
	}
}

func TestDeleteLibraryCascadesToBooks(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	libID := createTestLibrary(t, db, "Cascade Library")

	// Create a book in the library.
	result, err := db.ExecContext(ctx,
		"INSERT INTO book (library_id, book_type) VALUES (?, 'EBOOK')", libID,
	)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	bookID, _ := result.LastInsertId()

	// Create a book file.
	_, err = db.ExecContext(ctx,
		"INSERT INTO book_file (book_id, file_path, format) VALUES (?, '/test.epub', 'EPUB')", bookID,
	)
	if err != nil {
		t.Fatalf("insert book_file: %v", err)
	}

	// Create book metadata.
	_, err = db.ExecContext(ctx,
		"INSERT INTO book_metadata (book_id, title) VALUES (?, 'Test Book')", bookID,
	)
	if err != nil {
		t.Fatalf("insert book_metadata: %v", err)
	}

	// Create library path.
	_, err = db.ExecContext(ctx,
		"INSERT INTO library_path (library_id, path) VALUES (?, '/books')", libID,
	)
	if err != nil {
		t.Fatalf("insert library_path: %v", err)
	}

	// Delete the library — should cascade to books, book_files, book_metadata, library_path.
	_, err = db.ExecContext(ctx, "DELETE FROM library WHERE id = ?", libID)
	if err != nil {
		t.Fatalf("delete library: %v", err)
	}

	// Verify cascades.
	tables := []struct {
		name  string
		query string
	}{
		{"book", "SELECT count(*) FROM book WHERE library_id = ?"},
		{"book_file", "SELECT count(*) FROM book_file WHERE book_id = ?"},
		{"book_metadata", "SELECT count(*) FROM book_metadata WHERE book_id = ?"},
		{"library_path", "SELECT count(*) FROM library_path WHERE library_id = ?"},
	}

	for _, tt := range tables {
		t.Run(tt.name, func(t *testing.T) {
			var count int
			// Use libID for library-scoped tables, bookID for book-scoped.
			id := libID
			if tt.name == "book_file" || tt.name == "book_metadata" {
				id = bookID
			}
			if err := db.QueryRowContext(ctx, tt.query, id).Scan(&count); err != nil {
				t.Fatalf("count %s: %v", tt.name, err)
			}
			if count != 0 {
				t.Errorf("%s count after cascade delete = %d; want 0", tt.name, count)
			}
		})
	}
}

func TestDeleteBookCascadesToTaxonomy(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	libID := createTestLibrary(t, db, "Taxonomy Library")

	result, err := db.ExecContext(ctx,
		"INSERT INTO book (library_id, book_type) VALUES (?, 'EBOOK')", libID,
	)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	bookID, _ := result.LastInsertId()

	// Create taxonomy entries and link them.
	authorResult, err := db.ExecContext(ctx, "INSERT INTO author (name) VALUES ('Author One')")
	if err != nil {
		t.Fatalf("insert author: %v", err)
	}
	authorID, _ := authorResult.LastInsertId()

	_, err = db.ExecContext(ctx,
		"INSERT INTO book_author (book_id, author_id, sort_order) VALUES (?, ?, 0)", bookID, authorID,
	)
	if err != nil {
		t.Fatalf("insert book_author: %v", err)
	}

	seriesResult, err := db.ExecContext(ctx, "INSERT INTO series (name) VALUES ('Test Series')")
	if err != nil {
		t.Fatalf("insert series: %v", err)
	}
	seriesID, _ := seriesResult.LastInsertId()

	_, err = db.ExecContext(ctx,
		"INSERT INTO book_series (book_id, series_id, series_number) VALUES (?, ?, 1.0)", bookID, seriesID,
	)
	if err != nil {
		t.Fatalf("insert book_series: %v", err)
	}

	tagResult, err := db.ExecContext(ctx, "INSERT INTO tag (name) VALUES ('fiction')")
	if err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	tagID, _ := tagResult.LastInsertId()

	_, err = db.ExecContext(ctx,
		"INSERT INTO book_tag (book_id, tag_id) VALUES (?, ?)", bookID, tagID,
	)
	if err != nil {
		t.Fatalf("insert book_tag: %v", err)
	}

	// Delete the book — junction tables should cascade.
	_, err = db.ExecContext(ctx, "DELETE FROM book WHERE id = ?", bookID)
	if err != nil {
		t.Fatalf("delete book: %v", err)
	}

	// Verify junction rows are gone.
	junctions := []struct {
		name  string
		query string
	}{
		{"book_author", "SELECT count(*) FROM book_author WHERE book_id = ?"},
		{"book_series", "SELECT count(*) FROM book_series WHERE book_id = ?"},
		{"book_tag", "SELECT count(*) FROM book_tag WHERE book_id = ?"},
	}

	for _, tt := range junctions {
		t.Run(tt.name, func(t *testing.T) {
			var count int
			if err := db.QueryRowContext(ctx, tt.query, bookID).Scan(&count); err != nil {
				t.Fatalf("count %s: %v", tt.name, err)
			}
			if count != 0 {
				t.Errorf("%s count after cascade delete = %d; want 0", tt.name, count)
			}
		})
	}

	// Verify the taxonomy entries themselves still exist (not cascaded).
	var authorCount, seriesCount, tagCount int
	db.QueryRowContext(ctx, "SELECT count(*) FROM author WHERE id = ?", authorID).Scan(&authorCount)
	db.QueryRowContext(ctx, "SELECT count(*) FROM series WHERE id = ?", seriesID).Scan(&seriesCount)
	db.QueryRowContext(ctx, "SELECT count(*) FROM tag WHERE id = ?", tagID).Scan(&tagCount)

	if authorCount != 1 {
		t.Errorf("author count = %d; want 1 (should not be cascade deleted)", authorCount)
	}
	if seriesCount != 1 {
		t.Errorf("series count = %d; want 1 (should not be cascade deleted)", seriesCount)
	}
	if tagCount != 1 {
		t.Errorf("tag count = %d; want 1 (should not be cascade deleted)", tagCount)
	}
}

func TestBookForeignKeyConstraint(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Inserting a book with a non-existent library_id should fail.
	_, err := db.ExecContext(ctx,
		"INSERT INTO book (library_id, book_type) VALUES (?, 'EBOOK')", 99999,
	)
	if err == nil {
		t.Error("expected foreign key violation for invalid library_id; got nil")
	}
}

func TestUserLibraryPermission(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	userID := createTestUser(t, db, "permuser")
	libID := createTestLibrary(t, db, "Perm Library")

	// Grant access.
	_, err := db.ExecContext(ctx,
		"INSERT INTO user_library_permission (user_id, library_id) VALUES (?, ?)", userID, libID,
	)
	if err != nil {
		t.Fatalf("grant library access: %v", err)
	}

	// Verify access exists.
	var count int
	err = db.QueryRowContext(ctx,
		"SELECT count(*) FROM user_library_permission WHERE user_id = ? AND library_id = ?",
		userID, libID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if count != 1 {
		t.Errorf("permission count = %d; want 1", count)
	}

	// Delete user — should cascade to user_library_permission.
	_, err = db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		t.Fatalf("delete user: %v", err)
	}

	err = db.QueryRowContext(ctx,
		"SELECT count(*) FROM user_library_permission WHERE user_id = ?", userID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count permissions after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("permission count after user delete = %d; want 0", count)
	}
}

func TestProgressAndReadingSessions(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	userID := createTestUser(t, db, "reader")
	libID := createTestLibrary(t, db, "Progress Library")

	// Create book and file.
	bookResult, err := db.ExecContext(ctx,
		"INSERT INTO book (library_id, book_type) VALUES (?, 'EBOOK')", libID,
	)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	bookID, _ := bookResult.LastInsertId()

	fileResult, err := db.ExecContext(ctx,
		"INSERT INTO book_file (book_id, file_path, format) VALUES (?, '/test.epub', 'EPUB')", bookID,
	)
	if err != nil {
		t.Fatalf("insert book_file: %v", err)
	}
	fileID, _ := fileResult.LastInsertId()

	// Insert progress.
	_, err = db.ExecContext(ctx,
		"INSERT INTO user_book_file_progress (user_id, book_file_id, progress, progress_type) VALUES (?, ?, '50%', 'PERCENT')",
		userID, fileID,
	)
	if err != nil {
		t.Fatalf("insert progress: %v", err)
	}

	// Verify progress.
	var progress string
	err = db.QueryRowContext(ctx,
		"SELECT progress FROM user_book_file_progress WHERE user_id = ? AND book_file_id = ?",
		userID, fileID,
	).Scan(&progress)
	if err != nil {
		t.Fatalf("select progress: %v", err)
	}
	if progress != "50%" {
		t.Errorf("progress = %q; want %q", progress, "50%")
	}

	// Insert reading session.
	_, err = db.ExecContext(ctx,
		"INSERT INTO reading_sessions (user_id, book_id, book_file_id, started_at) VALUES (?, ?, ?, datetime('now'))",
		userID, bookID, fileID,
	)
	if err != nil {
		t.Fatalf("insert reading_session: %v", err)
	}

	// Delete book file — progress should cascade, reading_session.book_file_id should be SET NULL.
	_, err = db.ExecContext(ctx, "DELETE FROM book_file WHERE id = ?", fileID)
	if err != nil {
		t.Fatalf("delete book_file: %v", err)
	}

	var progressCount int
	err = db.QueryRowContext(ctx,
		"SELECT count(*) FROM user_book_file_progress WHERE book_file_id = ?", fileID,
	).Scan(&progressCount)
	if err != nil {
		t.Fatalf("count progress: %v", err)
	}
	if progressCount != 0 {
		t.Errorf("progress count after file delete = %d; want 0", progressCount)
	}

	// Reading session should still exist but with NULL book_file_id.
	var sessionBookFileID sql.NullInt64
	err = db.QueryRowContext(ctx,
		"SELECT book_file_id FROM reading_sessions WHERE user_id = ? AND book_id = ?",
		userID, bookID,
	).Scan(&sessionBookFileID)
	if err != nil {
		t.Fatalf("select reading_session: %v", err)
	}
	if sessionBookFileID.Valid {
		t.Errorf("reading_session.book_file_id = %d; want NULL after file delete", sessionBookFileID.Int64)
	}
}

func TestMigrationsRollback(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	logger := newLogger("error", "text")

	// Apply all migrations.
	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("RunMigrations error: %v", err)
	}

	// Roll back all migrations one at a time using the migrate library.
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("create migration source: %v", err)
	}

	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		t.Fatalf("create migration driver: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		t.Fatalf("create migrate instance: %v", err)
	}

	// Step down through all migrations.
	for {
		err := m.Steps(-1)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				break // No more migrations to roll back.
			}
			t.Fatalf("migration step down: %v", err)
		}
	}

	// After rolling back all migrations, only the schema_migrations table should remain.
	// Verify that the application tables no longer exist.
	appTables := []string{"users", "library", "book", "author", "user_book_file_progress"}
	for _, table := range appTables {
		var count int
		err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
		if err == nil {
			t.Errorf("table %q still exists after full rollback", table)
		}
	}
}

func TestComicMetadataInsert(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	libID := createTestLibrary(t, db, "Comic Library")

	result, err := db.ExecContext(ctx,
		"INSERT INTO book (library_id, book_type) VALUES (?, 'COMIC')", libID,
	)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	bookID, _ := result.LastInsertId()

	_, err = db.ExecContext(ctx,
		"INSERT INTO comic_metadata (book_id, web, volume, manga, age_rating) VALUES (?, 'https://example.com', 1, 'YES', 'TEEN')",
		bookID,
	)
	if err != nil {
		t.Fatalf("insert comic_metadata: %v", err)
	}

	var web, manga, ageRating string
	var volume int
	err = db.QueryRowContext(ctx,
		"SELECT web, volume, manga, age_rating FROM comic_metadata WHERE book_id = ?", bookID,
	).Scan(&web, &volume, &manga, &ageRating)
	if err != nil {
		t.Fatalf("select comic_metadata: %v", err)
	}
	if web != "https://example.com" {
		t.Errorf("web = %q; want %q", web, "https://example.com")
	}
	if volume != 1 {
		t.Errorf("volume = %d; want 1", volume)
	}
	if manga != "YES" {
		t.Errorf("manga = %q; want %q", manga, "YES")
	}
	if ageRating != "TEEN" {
		t.Errorf("age_rating = %q; want %q", ageRating, "TEEN")
	}
}

func TestBookMetadataLocking(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	libID := createTestLibrary(t, db, "Lock Library")

	result, err := db.ExecContext(ctx,
		"INSERT INTO book (library_id, book_type) VALUES (?, 'EBOOK')", libID,
	)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	bookID, _ := result.LastInsertId()

	// Insert initial metadata.
	_, err = db.ExecContext(ctx,
		"INSERT INTO book_metadata (book_id, title) VALUES (?, 'Original Title')", bookID,
	)
	if err != nil {
		t.Fatalf("insert book_metadata: %v", err)
	}

	// Lock the title field.
	_, err = db.ExecContext(ctx,
		"UPDATE book_metadata SET title_locked = 1 WHERE book_id = ?", bookID,
	)
	if err != nil {
		t.Fatalf("lock title: %v", err)
	}

	// Upsert with a new title — should be ignored because title is locked.
	_, err = db.ExecContext(ctx, `
		INSERT INTO book_metadata (book_id, title) VALUES (?, 'New Title')
		ON CONFLICT(book_id) DO UPDATE SET
			title = CASE WHEN book_metadata.title_locked = 0 THEN excluded.title ELSE book_metadata.title END
	`, bookID)
	if err != nil {
		t.Fatalf("upsert book_metadata: %v", err)
	}

	var title string
	err = db.QueryRowContext(ctx,
		"SELECT title FROM book_metadata WHERE book_id = ?", bookID,
	).Scan(&title)
	if err != nil {
		t.Fatalf("select title: %v", err)
	}
	if title != "Original Title" {
		t.Errorf("title = %q; want %q (locked field should not change)", title, "Original Title")
	}
}

func TestMoodAndCategoryJunctions(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	libID := createTestLibrary(t, db, "Junction Library")

	bookResult, err := db.ExecContext(ctx,
		"INSERT INTO book (library_id, book_type) VALUES (?, 'EBOOK')", libID,
	)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	bookID, _ := bookResult.LastInsertId()

	// Create mood and category.
	moodResult, err := db.ExecContext(ctx, "INSERT INTO mood (name) VALUES ('dark')")
	if err != nil {
		t.Fatalf("insert mood: %v", err)
	}
	moodID, _ := moodResult.LastInsertId()

	catResult, err := db.ExecContext(ctx, "INSERT INTO category (name) VALUES ('Science Fiction')")
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}
	catID, _ := catResult.LastInsertId()

	// Link them.
	_, err = db.ExecContext(ctx,
		"INSERT INTO book_mood (book_id, mood_id) VALUES (?, ?)", bookID, moodID,
	)
	if err != nil {
		t.Fatalf("insert book_mood: %v", err)
	}

	_, err = db.ExecContext(ctx,
		"INSERT INTO book_category (book_id, category_id) VALUES (?, ?)", bookID, catID,
	)
	if err != nil {
		t.Fatalf("insert book_category: %v", err)
	}

	// Verify.
	var moodCount, catCount int
	db.QueryRowContext(ctx, "SELECT count(*) FROM book_mood WHERE book_id = ?", bookID).Scan(&moodCount)
	db.QueryRowContext(ctx, "SELECT count(*) FROM book_category WHERE book_id = ?", bookID).Scan(&catCount)

	if moodCount != 1 {
		t.Errorf("book_mood count = %d; want 1", moodCount)
	}
	if catCount != 1 {
		t.Errorf("book_category count = %d; want 1", catCount)
	}
}
