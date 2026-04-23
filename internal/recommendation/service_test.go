package recommendation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"log/slog"

	_ "modernc.org/sqlite"
)

// openTestDB creates a temporary SQLite database with all required migrations applied.
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
		filepath.Join("..", "..", "migrations", "013_book_vectors.up.sql"),
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
	t.Helper()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// seedBook creates a library, book, optional metadata, and optional vector.
// Returns the book ID.
func seedBook(t *testing.T, db *sql.DB, libraryID int64, title string, authors, categories, tags, series []string, vector *[vectorDim]float32) int64 {
	t.Helper()
	ctx := context.Background()

	res, err := db.ExecContext(ctx, "INSERT INTO book (library_id, book_type) VALUES (?, 'EBOOK')", libraryID)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	bookID, _ := res.LastInsertId()

	if title != "" {
		_, err = db.ExecContext(ctx, "INSERT INTO book_metadata (book_id, title) VALUES (?, ?)", bookID, title)
		if err != nil {
			t.Fatalf("insert book metadata: %v", err)
		}
	}

	for i, a := range authors {
		var authorID int64
		err = db.QueryRowContext(ctx, "SELECT id FROM author WHERE name = ?", a).Scan(&authorID)
		if err == sql.ErrNoRows {
			res, err := db.ExecContext(ctx, "INSERT INTO author (name) VALUES (?)", a)
			if err != nil {
				t.Fatalf("insert author: %v", err)
			}
			authorID, _ = res.LastInsertId()
		} else if err != nil {
			t.Fatalf("select author: %v", err)
		}
		_, err = db.ExecContext(ctx, "INSERT INTO book_author (book_id, author_id, sort_order) VALUES (?, ?, ?)", bookID, authorID, i)
		if err != nil {
			t.Fatalf("insert book_author: %v", err)
		}
	}

	for _, c := range categories {
		var catID int64
		err = db.QueryRowContext(ctx, "SELECT id FROM category WHERE name = ?", c).Scan(&catID)
		if err == sql.ErrNoRows {
			res, err := db.ExecContext(ctx, "INSERT INTO category (name) VALUES (?)", c)
			if err != nil {
				t.Fatalf("insert category: %v", err)
			}
			catID, _ = res.LastInsertId()
		} else if err != nil {
			t.Fatalf("select category: %v", err)
		}
		_, err = db.ExecContext(ctx, "INSERT INTO book_category (book_id, category_id) VALUES (?, ?)", bookID, catID)
		if err != nil {
			t.Fatalf("insert book_category: %v", err)
		}
	}

	for _, tag := range tags {
		var tagID int64
		err = db.QueryRowContext(ctx, "SELECT id FROM tag WHERE name = ?", tag).Scan(&tagID)
		if err == sql.ErrNoRows {
			res, err := db.ExecContext(ctx, "INSERT INTO tag (name) VALUES (?)", tag)
			if err != nil {
				t.Fatalf("insert tag: %v", err)
			}
			tagID, _ = res.LastInsertId()
		} else if err != nil {
			t.Fatalf("select tag: %v", err)
		}
		_, err = db.ExecContext(ctx, "INSERT INTO book_tag (book_id, tag_id) VALUES (?, ?)", bookID, tagID)
		if err != nil {
			t.Fatalf("insert book_tag: %v", err)
		}
	}

	for _, s := range series {
		var seriesID int64
		err = db.QueryRowContext(ctx, "SELECT id FROM series WHERE name = ?", s).Scan(&seriesID)
		if err == sql.ErrNoRows {
			res, err := db.ExecContext(ctx, "INSERT INTO series (name) VALUES (?)", s)
			if err != nil {
				t.Fatalf("insert series: %v", err)
			}
			seriesID, _ = res.LastInsertId()
		} else if err != nil {
			t.Fatalf("select series: %v", err)
		}
		_, err = db.ExecContext(ctx, "INSERT INTO book_series (book_id, series_id) VALUES (?, ?)", bookID, seriesID)
		if err != nil {
			t.Fatalf("insert book_series: %v", err)
		}
	}

	if vector != nil {
		_, err = db.ExecContext(ctx, "INSERT INTO book_vectors (book_id, vector) VALUES (?, ?)", bookID, vectorToBytes(*vector))
		if err != nil {
			t.Fatalf("insert book vector: %v", err)
		}
	}

	return bookID
}

func seedLibrary(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := db.ExecContext(ctx, "INSERT INTO library (name, organization_mode) VALUES (?, 'BOOK_PER_FILE')", name)
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestFindSimilarBooks_EmptyWhenNoSourceVector(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, newTestLogger(t))
	ctx := context.Background()

	libID := seedLibrary(t, db, "Test Lib")
	bookID := seedBook(t, db, libID, "No Vector Book", []string{"Author A"}, nil, nil, nil, nil)

	result, err := svc.FindSimilarBooks(ctx, bookID, nil, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if result == nil {
		t.Fatalf("FindSimilarBooks() result = nil; want empty slice")
	}
	if len(result) != 0 {
		t.Errorf("FindSimilarBooks() len = %d; want 0", len(result))
	}
}

func TestFindSimilarBooks_EmptyWhenNoCandidates(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, newTestLogger(t))
	ctx := context.Background()

	libID := seedLibrary(t, db, "Test Lib")
	vec := HashBookFeatures([]string{"Author A"}, nil, nil, nil, "", "")
	bookID := seedBook(t, db, libID, "Only Book", []string{"Author A"}, nil, nil, nil, &vec)

	result, err := svc.FindSimilarBooks(ctx, bookID, nil, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 0 {
		t.Errorf("FindSimilarBooks() len = %d; want 0", len(result))
	}
}

func TestFindSimilarBooks_LimitClamping(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, newTestLogger(t))
	ctx := context.Background()

	libID := seedLibrary(t, db, "Test Lib")
	sourceVec := HashBookFeatures([]string{"Author A"}, nil, nil, nil, "", "")
	sourceID := seedBook(t, db, libID, "Source", []string{"Author A"}, nil, nil, nil, &sourceVec)

	// Create 25 similar books with distinct authors so no per-author cap hits.
	for i := 0; i < 25; i++ {
		v := HashBookFeatures([]string{fmt.Sprintf("Author %d", i)}, nil, nil, nil, "", "")
		seedBook(t, db, libID, fmt.Sprintf("Book %d", i), []string{fmt.Sprintf("Author %d", i)}, nil, nil, nil, &v)
	}

	// Default limit (0 passed) should clamp to 10.
	result, err := svc.FindSimilarBooks(ctx, sourceID, nil, 0)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 10 {
		t.Errorf("default limit: got %d; want 10", len(result))
	}

	// Explicit limit of 5 should return 5.
	result, err = svc.FindSimilarBooks(ctx, sourceID, nil, 5)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 5 {
		t.Errorf("limit 5: got %d; want 5", len(result))
	}

	// Limit over 20 should clamp to 20.
	result, err = svc.FindSimilarBooks(ctx, sourceID, nil, 50)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 20 {
		t.Errorf("limit 50: got %d; want 20", len(result))
	}
}

func TestFindSimilarBooks_PerAuthorCap(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, newTestLogger(t))
	ctx := context.Background()

	libID := seedLibrary(t, db, "Test Lib")
	sourceVec := HashBookFeatures([]string{"Author A"}, nil, nil, nil, "", "")
	sourceID := seedBook(t, db, libID, "Source", []string{"Author A"}, nil, nil, nil, &sourceVec)

	// Create 5 books by the same primary author.
	for i := 0; i < 5; i++ {
		v := HashBookFeatures([]string{"Author A", fmt.Sprintf("Coauthor %d", i)}, nil, nil, nil, "", "")
		seedBook(t, db, libID, fmt.Sprintf("Book %d", i), []string{"Author A", fmt.Sprintf("Coauthor %d", i)}, nil, nil, nil, &v)
	}

	result, err := svc.FindSimilarBooks(ctx, sourceID, nil, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 3 {
		t.Errorf("per-author cap: got %d; want 3", len(result))
	}

	// Verify all returned books have Author A as primary author.
	for _, b := range result {
		if len(b.Authors) == 0 || b.Authors[0] != "Author A" {
			t.Errorf("expected primary author 'Author A', got %v", b.Authors)
		}
	}
}

func TestFindSimilarBooks_SimilarityRanking(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, newTestLogger(t))
	ctx := context.Background()

	libID := seedLibrary(t, db, "Test Lib")

	// Source book: Author A, Category Fantasy.
	sourceVec := HashBookFeatures([]string{"Author A"}, nil, []string{"Fantasy"}, nil, "", "")
	sourceID := seedBook(t, db, libID, "Source", []string{"Author A"}, []string{"Fantasy"}, nil, nil, &sourceVec)

	// Candidate 1: same author + same category (should be most similar).
	c1Vec := HashBookFeatures([]string{"Author A"}, nil, []string{"Fantasy"}, nil, "", "")
	c1ID := seedBook(t, db, libID, "Candidate 1", []string{"Author A"}, []string{"Fantasy"}, nil, nil, &c1Vec)

	// Candidate 2: same author only.
	c2Vec := HashBookFeatures([]string{"Author A"}, nil, nil, nil, "", "")
	c2ID := seedBook(t, db, libID, "Candidate 2", []string{"Author A"}, nil, nil, nil, &c2Vec)

	// Candidate 3: same category only.
	c3Vec := HashBookFeatures([]string{"Author B"}, nil, []string{"Fantasy"}, nil, "", "")
	c3ID := seedBook(t, db, libID, "Candidate 3", []string{"Author B"}, []string{"Fantasy"}, nil, nil, &c3Vec)

	// Candidate 4: unrelated.
	c4Vec := HashBookFeatures([]string{"Author C"}, nil, []string{"Sci-Fi"}, nil, "", "")
	c4ID := seedBook(t, db, libID, "Candidate 4", []string{"Author C"}, []string{"Sci-Fi"}, nil, nil, &c4Vec)

	result, err := svc.FindSimilarBooks(ctx, sourceID, nil, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 4 {
		t.Fatalf("got %d results; want 4", len(result))
	}

	// Candidate 1 should be first (most similar).
	if result[0].ID != c1ID {
		t.Errorf("first result id = %d; want %d (most similar)", result[0].ID, c1ID)
	}

	// Candidate 4 should be last (least similar).
	if result[3].ID != c4ID {
		t.Errorf("last result id = %d; want %d (least similar)", result[3].ID, c4ID)
	}

	// Candidate 2 and 3 ordering depends on exact weights, but both should be between 1 and 4.
	if result[1].ID != c2ID && result[1].ID != c3ID {
		t.Errorf("second result unexpected id = %d", result[1].ID)
	}
	if result[2].ID != c2ID && result[2].ID != c3ID {
		t.Errorf("third result unexpected id = %d", result[2].ID)
	}
}

func TestFindSimilarBooks_LibraryFiltering(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, newTestLogger(t))
	ctx := context.Background()

	lib1 := seedLibrary(t, db, "Lib 1")
	lib2 := seedLibrary(t, db, "Lib 2")

	sourceVec := HashBookFeatures([]string{"Author A"}, nil, nil, nil, "", "")
	sourceID := seedBook(t, db, lib1, "Source", []string{"Author A"}, nil, nil, nil, &sourceVec)

	c1Vec := HashBookFeatures([]string{"Author A"}, nil, nil, nil, "", "")
	c1ID := seedBook(t, db, lib1, "Lib1 Book", []string{"Author A"}, nil, nil, nil, &c1Vec)

	c2Vec := HashBookFeatures([]string{"Author A"}, nil, nil, nil, "", "")
	c2ID := seedBook(t, db, lib2, "Lib2 Book", []string{"Author A"}, nil, nil, nil, &c2Vec)

	// No filter: should see both candidates.
	result, err := svc.FindSimilarBooks(ctx, sourceID, nil, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 2 {
		t.Errorf("no filter: got %d; want 2", len(result))
	}

	// Filter to lib1 only.
	result, err = svc.FindSimilarBooks(ctx, sourceID, []int64{lib1}, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 1 {
		t.Errorf("lib1 filter: got %d; want 1", len(result))
	}
	if result[0].ID != c1ID {
		t.Errorf("lib1 filter: got id %d; want %d", result[0].ID, c1ID)
	}

	// Filter to lib2 only.
	result, err = svc.FindSimilarBooks(ctx, sourceID, []int64{lib2}, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 1 {
		t.Errorf("lib2 filter: got %d; want 1", len(result))
	}
	if result[0].ID != c2ID {
		t.Errorf("lib2 filter: got id %d; want %d", result[0].ID, c2ID)
	}

	// Filter to non-existent library.
	result, err = svc.FindSimilarBooks(ctx, sourceID, []int64{9999}, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if len(result) != 0 {
		t.Errorf("non-existent filter: got %d; want 0", len(result))
	}
}

func TestFindSimilarBooks_ReturnsEmptySliceNotNil(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, newTestLogger(t))
	ctx := context.Background()

	libID := seedLibrary(t, db, "Test Lib")
	bookID := seedBook(t, db, libID, "No Vector", []string{"Author A"}, nil, nil, nil, nil)

	result, err := svc.FindSimilarBooks(ctx, bookID, nil, 10)
	if err != nil {
		t.Fatalf("FindSimilarBooks() error = %v; want nil", err)
	}
	if result == nil {
		t.Fatalf("FindSimilarBooks() result = nil; want empty slice")
	}
}
