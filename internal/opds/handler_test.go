package opds

import (
	"context"
	"database/sql"
	"encoding/xml"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/crueber/lexicon/internal/user"
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
		filepath.Join("..", "..", "migrations", "006_tasks.up.sql"),
		filepath.Join("..", "..", "migrations", "007_shelves.up.sql"),
		filepath.Join("..", "..", "migrations", "008_metadata_jobs.up.sql"),
		filepath.Join("..", "..", "migrations", "009_magic_shelves.up.sql"),
		filepath.Join("..", "..", "migrations", "010_annotations.up.sql"),
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

// newTestHandler creates an OPDS Handler with a test database and silent logger.
func newTestHandler(t *testing.T, db *sql.DB) *Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewHandler(db, logger)
}

// newTestRouter creates a chi router with OPDS routes mounted.
func newTestRouter(t *testing.T, h *Handler) *chi.Mux {
	t.Helper()
	r := chi.NewRouter()
	r.Route("/opds", func(r chi.Router) {
		h.Routes(r)
	})
	return r
}

// seedAdminUser creates an admin user with OPDS access.
func seedAdminUser(t *testing.T, db *sql.DB) user.User {
	t.Helper()
	u, err := user.CreateAdminUser(context.Background(), db, user.CreateUserServiceParams{
		Username: "admin",
		Password: "adminpass",
		Name:     "Admin User",
	})
	if err != nil {
		t.Fatalf("seed admin user: %v", err)
	}
	return u
}

// seedUserWithOPDS creates a regular user with OPDS access enabled.
func seedUserWithOPDS(t *testing.T, db *sql.DB) user.User {
	t.Helper()
	u, err := user.CreateUserWithDefaults(context.Background(), db, user.CreateUserServiceParams{
		Username: "opdsuser",
		Password: "opdspass",
		Name:     "OPDS User",
	})
	if err != nil {
		t.Fatalf("seed opds user: %v", err)
	}

	// Grant OPDS access.
	q := user.New(db)
	if err := q.UpsertUserPermissions(context.Background(), user.UpsertUserPermissionsParams{
		UserID:     u.ID,
		Role:       "USER",
		OpdsAccess: 1,
	}); err != nil {
		t.Fatalf("grant opds access: %v", err)
	}

	return u
}

// seedUserWithoutOPDS creates a regular user without OPDS access.
func seedUserWithoutOPDS(t *testing.T, db *sql.DB) user.User {
	t.Helper()
	u, err := user.CreateUserWithDefaults(context.Background(), db, user.CreateUserServiceParams{
		Username: "noaccess",
		Password: "noaccesspass",
		Name:     "No Access User",
	})
	if err != nil {
		t.Fatalf("seed no-access user: %v", err)
	}
	return u
}

// TestMissingAuthReturns401 verifies that requests without credentials get 401.
func TestMissingAuthReturns401(t *testing.T) {
	db := openTestDB(t)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	tests := []struct {
		name string
		path string
	}{
		{"root", "/opds/"},
		{"books", "/opds/books"},
		{"libraries", "/opds/libraries"},
		{"shelves", "/opds/shelves"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("path %s: status = %d; want %d", tt.path, rec.Code, http.StatusUnauthorized)
			}

			wwwAuth := rec.Header().Get("WWW-Authenticate")
			if !strings.Contains(wwwAuth, "Basic") {
				t.Errorf("path %s: WWW-Authenticate = %q; want Basic realm", tt.path, wwwAuth)
			}
		})
	}
}

// TestInvalidCredentialsReturns401 verifies that wrong credentials get 401.
func TestInvalidCredentialsReturns401(t *testing.T) {
	db := openTestDB(t)
	seedAdminUser(t, db)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	tests := []struct {
		name     string
		username string
		password string
	}{
		{"wrong password", "admin", "wrongpassword"},
		{"unknown user", "nobody", "password"},
		{"empty password", "admin", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/opds/", nil)
			if tt.username != "" || tt.password != "" {
				req.SetBasicAuth(tt.username, tt.password)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d; want %d (username=%q)", rec.Code, http.StatusUnauthorized, tt.username)
			}
		})
	}
}

// TestUserWithoutOPDSAccessReturns403 verifies that users without opds_access get 403.
func TestUserWithoutOPDSAccessReturns403(t *testing.T) {
	db := openTestDB(t)
	seedUserWithoutOPDS(t, db)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/opds/", nil)
	req.SetBasicAuth("noaccess", "noaccesspass")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

// TestRootCatalogReturnsValidXML verifies the root feed returns valid Atom XML.
func TestRootCatalogReturnsValidXML(t *testing.T) {
	db := openTestDB(t)
	seedAdminUser(t, db)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/opds/", nil)
	req.SetBasicAuth("admin", "adminpass")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/atom+xml") {
		t.Errorf("Content-Type = %q; want application/atom+xml", ct)
	}

	var feed Feed
	if err := xml.NewDecoder(rec.Body).Decode(&feed); err != nil {
		t.Fatalf("decode XML: %v", err)
	}

	if feed.Title == "" {
		t.Error("feed.Title is empty")
	}

	if len(feed.Entries) == 0 {
		t.Error("feed.Entries is empty; want at least one navigation entry")
	}
}

// TestBookListingReturnsEntries verifies that the books feed returns entries.
func TestBookListingReturnsEntries(t *testing.T) {
	db := openTestDB(t)
	seedAdminUser(t, db)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	// Create a library and a book.
	_, err := db.Exec(`INSERT INTO library (name, organization_mode) VALUES ('Test Library', 'BOOK_FOLDER')`)
	if err != nil {
		t.Fatalf("create library: %v", err)
	}

	var libraryID int64
	if err := db.QueryRow(`SELECT id FROM library WHERE name = 'Test Library'`).Scan(&libraryID); err != nil {
		t.Fatalf("get library id: %v", err)
	}

	_, err = db.Exec(`INSERT INTO book (library_id, book_type) VALUES (?, 'EBOOK')`, libraryID)
	if err != nil {
		t.Fatalf("create book: %v", err)
	}

	var bookID int64
	if err := db.QueryRow(`SELECT id FROM book WHERE library_id = ?`, libraryID).Scan(&bookID); err != nil {
		t.Fatalf("get book id: %v", err)
	}

	_, err = db.Exec(`INSERT INTO book_metadata (book_id, title) VALUES (?, 'Test Book')`, bookID)
	if err != nil {
		t.Fatalf("create book metadata: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/opds/books", nil)
	req.SetBasicAuth("admin", "adminpass")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var feed Feed
	if err := xml.NewDecoder(rec.Body).Decode(&feed); err != nil {
		t.Fatalf("decode XML: %v", err)
	}

	if len(feed.Entries) == 0 {
		t.Error("feed.Entries is empty; want at least one book entry")
	}

	if feed.Entries[0].Title != "Test Book" {
		t.Errorf("entry title = %q; want %q", feed.Entries[0].Title, "Test Book")
	}
}

// TestOPDSUserWithAccessCanBrowse verifies that a regular user with opds_access can browse.
func TestOPDSUserWithAccessCanBrowse(t *testing.T) {
	db := openTestDB(t)
	seedUserWithOPDS(t, db)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/opds/", nil)
	req.SetBasicAuth("opdsuser", "opdspass")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestLibrariesFeedReturnsValidXML verifies the libraries navigation feed.
func TestLibrariesFeedReturnsValidXML(t *testing.T) {
	db := openTestDB(t)
	seedAdminUser(t, db)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	// Create a library.
	_, err := db.Exec(`INSERT INTO library (name, organization_mode) VALUES ('My Library', 'BOOK_FOLDER')`)
	if err != nil {
		t.Fatalf("create library: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/opds/libraries", nil)
	req.SetBasicAuth("admin", "adminpass")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var feed Feed
	if err := xml.NewDecoder(rec.Body).Decode(&feed); err != nil {
		t.Fatalf("decode XML: %v", err)
	}

	if len(feed.Entries) == 0 {
		t.Error("feed.Entries is empty; want at least one library entry")
	}

	if feed.Entries[0].Title != "My Library" {
		t.Errorf("entry title = %q; want %q", feed.Entries[0].Title, "My Library")
	}
}
