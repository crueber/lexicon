package bookdrop

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/crueber/lexicon/internal/library"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			t.Fatalf("set pragma %q: %v", pragma, err)
		}
	}

	db.SetMaxOpenConns(1)

	migrations := []string{
		filepath.Join("..", "..", "migrations", "001_users.up.sql"),
		filepath.Join("..", "..", "migrations", "002_libraries.up.sql"),
		filepath.Join("..", "..", "migrations", "003_books.up.sql"),
		filepath.Join("..", "..", "migrations", "004_taxonomy.up.sql"),
		filepath.Join("..", "..", "migrations", "005_progress.up.sql"),
		filepath.Join("..", "..", "migrations", "019_bookdrop.up.sql"),
	}

	for _, m := range migrations {
		schema, err := os.ReadFile(m)
		if err != nil {
			db.Close()
			t.Fatalf("read migration %s: %v", m, err)
		}
		if _, err := db.Exec(string(schema)); err != nil {
			db.Close()
			t.Fatalf("apply migration %s: %v", m, err)
		}
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func newTestLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func createTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}

func TestService_ListPending(t *testing.T) {
	db := openTestDB(t)
	logger := newTestLogger(t)
	scanner := library.NewScanner(db, t.TempDir(), logger)
	svc := NewService(db, scanner, logger)
	ctx := context.Background()

	// Initially empty.
	files, err := svc.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending() error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("len(files) = %d; want 0", len(files))
	}

	// Insert a pending file directly.
	q := New(db)
	_, err = q.CreateBookdropFile(ctx, CreateBookdropFileParams{
		OriginalFilename: "test.epub",
		FilePath:         "/tmp/test.epub",
		FileSize:         1024,
		Status:           "PENDING",
	})
	if err != nil {
		t.Fatalf("CreateBookdropFile() error: %v", err)
	}

	files, err = svc.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending() error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("len(files) = %d; want 1", len(files))
	}
	if files[0].OriginalFilename != "test.epub" {
		t.Errorf("filename = %q; want %q", files[0].OriginalFilename, "test.epub")
	}
}

func TestService_RejectFile(t *testing.T) {
	db := openTestDB(t)
	logger := newTestLogger(t)
	scanner := library.NewScanner(db, t.TempDir(), logger)
	svc := NewService(db, scanner, logger)
	ctx := context.Background()

	dropDir := t.TempDir()
	filePath := filepath.Join(dropDir, "reject.epub")
	createTestFile(t, filePath, []byte("fake epub"))

	q := New(db)
	record, err := q.CreateBookdropFile(ctx, CreateBookdropFileParams{
		OriginalFilename: "reject.epub",
		FilePath:         filePath,
		FileSize:         100,
		Status:           "PENDING",
	})
	if err != nil {
		t.Fatalf("CreateBookdropFile() error: %v", err)
	}

	if err := svc.RejectFile(ctx, record.ID); err != nil {
		t.Fatalf("RejectFile() error: %v", err)
	}

	// File should be deleted.
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("expected file to be deleted after reject")
	}

	// Status should be REJECTED.
	updated, err := q.GetBookdropFileByID(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetBookdropFileByID() error: %v", err)
	}
	if updated.Status != "REJECTED" {
		t.Errorf("status = %q; want %q", updated.Status, "REJECTED")
	}
}

func TestService_RejectFile_NotFound(t *testing.T) {
	db := openTestDB(t)
	logger := newTestLogger(t)
	scanner := library.NewScanner(db, t.TempDir(), logger)
	svc := NewService(db, scanner, logger)
	ctx := context.Background()

	err := svc.RejectFile(ctx, 99999)
	if err == nil {
		t.Fatal("RejectFile() with nonexistent id should return error")
	}
}

func TestService_ImportFile(t *testing.T) {
	db := openTestDB(t)
	dataDir := t.TempDir()
	logger := newTestLogger(t)
	scanner := library.NewScanner(db, dataDir, logger)
	svc := NewService(db, scanner, logger)
	ctx := context.Background()

	// Create a library.
	lq := library.New(db)
	lib, err := lq.CreateLibrary(ctx, library.CreateLibraryParams{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})
	if err != nil {
		t.Fatalf("CreateLibrary() error: %v", err)
	}

	libPath := t.TempDir()
	_, err = lq.CreateLibraryPath(ctx, library.CreateLibraryPathParams{
		LibraryID: lib.ID,
		Path:      libPath,
	})
	if err != nil {
		t.Fatalf("CreateLibraryPath() error: %v", err)
	}

	// Create a drop file.
	dropDir := t.TempDir()
	filePath := filepath.Join(dropDir, "import.epub")
	createTestFile(t, filePath, []byte("fake epub content for import"))

	q := New(db)
	record, err := q.CreateBookdropFile(ctx, CreateBookdropFileParams{
		OriginalFilename: "import.epub",
		FilePath:         filePath,
		FileSize:         100,
		Status:           "PENDING",
	})
	if err != nil {
		t.Fatalf("CreateBookdropFile() error: %v", err)
	}

	bookID, err := svc.ImportFile(ctx, record.ID, lib.ID)
	if err != nil {
		t.Fatalf("ImportFile() error: %v", err)
	}

	if bookID == 0 {
		t.Error("ImportFile() returned bookID = 0")
	}

	// Status should be IMPORTED.
	updated, err := q.GetBookdropFileByID(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetBookdropFileByID() error: %v", err)
	}
	if updated.Status != "IMPORTED" {
		t.Errorf("status = %q; want %q", updated.Status, "IMPORTED")
	}
	if !updated.ImportedBookID.Valid || updated.ImportedBookID.Int64 != bookID {
		t.Errorf("imported_book_id = %v; want %d", updated.ImportedBookID, bookID)
	}
}

func TestService_ImportFile_NotPending(t *testing.T) {
	db := openTestDB(t)
	logger := newTestLogger(t)
	scanner := library.NewScanner(db, t.TempDir(), logger)
	svc := NewService(db, scanner, logger)
	ctx := context.Background()

	q := New(db)
	record, err := q.CreateBookdropFile(ctx, CreateBookdropFileParams{
		OriginalFilename: "imported.epub",
		FilePath:         "/tmp/imported.epub",
		FileSize:         100,
		Status:           "IMPORTED",
	})
	if err != nil {
		t.Fatalf("CreateBookdropFile() error: %v", err)
	}

	_, err = svc.ImportFile(ctx, record.ID, 1)
	if err == nil {
		t.Fatal("ImportFile() with non-pending status should return error")
	}
}
