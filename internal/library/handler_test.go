package library_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/crueber/lexicon/internal/auth"
	"github.com/crueber/lexicon/internal/library"
	"github.com/crueber/lexicon/internal/user"
)

const testSecret = "test-secret-key-at-least-32-chars!"

// openTestDB creates a temporary SQLite database with all migrations applied.
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

	// Apply all migrations in order.
	migrations := []string{
		filepath.Join("..", "..", "migrations", "001_users.up.sql"),
		filepath.Join("..", "..", "migrations", "002_libraries.up.sql"),
		filepath.Join("..", "..", "migrations", "003_books.up.sql"),
		filepath.Join("..", "..", "migrations", "004_taxonomy.up.sql"),
		filepath.Join("..", "..", "migrations", "005_progress.up.sql"),
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

// seedAdmin creates an admin user and returns a valid access token.
func seedAdmin(t *testing.T, db *sql.DB) (user.User, string) {
	t.Helper()

	ctx := context.Background()
	u, err := user.CreateAdminUser(ctx, db, user.CreateUserServiceParams{
		Username: "admin",
		Password: "adminpass",
		Name:     "Admin User",
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	principal := auth.Principal{
		UserID:   u.ID,
		Username: u.Username,
		Role:     "ADMIN",
		Permissions: auth.Permissions{
			CanDownload:     true,
			CanUpload:       true,
			CanEmailSend:    true,
			CanEditMetadata: true,
			OPDSAccess:      true,
		},
	}

	token, err := auth.IssueAccessToken(principal, testSecret)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}

	return u, token
}

// seedRegularUser creates a regular user and returns a valid access token.
// If libraryIDs is non-empty, the user is granted access to those libraries.
func seedRegularUser(t *testing.T, db *sql.DB, username string, libraryIDs []int64) (user.User, string) {
	t.Helper()

	ctx := context.Background()
	u, err := user.CreateUserWithDefaults(ctx, db, user.CreateUserServiceParams{
		Username: username,
		Password: "userpass",
		Name:     "Regular User",
	})
	if err != nil {
		t.Fatalf("seed regular user %q: %v", username, err)
	}

	// Grant library access.
	lq := library.New(db)
	for _, lid := range libraryIDs {
		if err := lq.GrantLibraryAccess(ctx, library.GrantLibraryAccessParams{
			UserID:    u.ID,
			LibraryID: lid,
		}); err != nil {
			t.Fatalf("grant library access: %v", err)
		}
	}

	principal := auth.Principal{
		UserID:     u.ID,
		Username:   u.Username,
		Role:       "USER",
		LibraryIDs: libraryIDs,
	}

	token, err := auth.IssueAccessToken(principal, testSecret)
	if err != nil {
		t.Fatalf("issue user token: %v", err)
	}

	return u, token
}

// newTestRouter creates a chi router with library routes mounted.
func newTestRouter(t *testing.T, db *sql.DB) *chi.Mux {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := library.NewService(db, logger)
	scanner := library.NewScanner(db, t.TempDir(), logger)
	h := library.NewHandler(svc, scanner, logger)

	r := chi.NewRouter()
	r.Route("/api/libraries", func(r chi.Router) {
		r.Use(auth.RequireAuth(testSecret))
		h.Routes(r)
	})

	return r
}

// createLibraryViaAPI creates a library using the API and returns the response.
func createLibraryViaAPI(t *testing.T, router *chi.Mux, token string, req library.CreateLibraryRequest) library.LibraryResponse {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal create request: %v", err)
	}

	httpReq := httptest.NewRequest(http.MethodPost, "/api/libraries", bytes.NewBuffer(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create library: status = %d; want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp library.LibraryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	return resp
}

// --- Tests ---

func TestListLibraries_AdminSeesAll(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	// Create two libraries.
	createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Library A",
		OrganizationMode: "BOOK_PER_FILE",
	})
	createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Library B",
		OrganizationMode: "BOOK_PER_FOLDER",
	})

	// Admin lists libraries.
	req := httptest.NewRequest(http.MethodGet, "/api/libraries", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var libs []library.LibraryResponse
	if err := json.NewDecoder(rec.Body).Decode(&libs); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(libs) != 2 {
		t.Errorf("got %d libraries; want 2", len(libs))
	}
}

func TestListLibraries_UserSeesOnlyPermitted(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	// Create two libraries.
	libA := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Library A",
		OrganizationMode: "BOOK_PER_FILE",
	})
	createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Library B",
		OrganizationMode: "BOOK_PER_FILE",
	})

	// Create a user with access only to Library A.
	_, userToken := seedRegularUser(t, db, "regularuser", []int64{libA.ID})

	req := httptest.NewRequest(http.MethodGet, "/api/libraries", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var libs []library.LibraryResponse
	if err := json.NewDecoder(rec.Body).Decode(&libs); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(libs) != 1 {
		t.Errorf("got %d libraries; want 1", len(libs))
	}
	if len(libs) > 0 && libs[0].ID != libA.ID {
		t.Errorf("got library id %d; want %d", libs[0].ID, libA.ID)
	}
}

func TestListLibraries_Unauthenticated(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/libraries", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCreateLibrary_AdminSucceeds(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	icon := "book"
	iconColor := "#ff0000"
	pattern := "{title}"

	body, _ := json.Marshal(library.CreateLibraryRequest{
		Name:              "My Library",
		Icon:              &icon,
		IconColor:         &iconColor,
		OrganizationMode:  "BOOK_PER_FILE",
		FileNamingPattern: &pattern,
		Paths:             []string{"/books/fiction", "/books/nonfiction"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/libraries", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp library.LibraryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Name != "My Library" {
		t.Errorf("name = %q; want %q", resp.Name, "My Library")
	}
	if resp.Icon == nil || *resp.Icon != icon {
		t.Errorf("icon = %v; want %q", resp.Icon, icon)
	}
	if resp.IconColor == nil || *resp.IconColor != iconColor {
		t.Errorf("iconColor = %v; want %q", resp.IconColor, iconColor)
	}
	if resp.OrganizationMode != "BOOK_PER_FILE" {
		t.Errorf("organizationMode = %q; want %q", resp.OrganizationMode, "BOOK_PER_FILE")
	}
	if len(resp.Paths) != 2 {
		t.Errorf("paths count = %d; want 2", len(resp.Paths))
	}
}

func TestCreateLibrary_NonAdminForbidden(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	_, userToken := seedRegularUser(t, db, "regularuser", nil)

	body, _ := json.Marshal(library.CreateLibraryRequest{
		Name:             "My Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/libraries", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCreateLibrary_ValidationErrors(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	tests := []struct {
		name string
		body library.CreateLibraryRequest
		want int
	}{
		{
			name: "empty name",
			body: library.CreateLibraryRequest{OrganizationMode: "BOOK_PER_FILE"},
			want: http.StatusBadRequest,
		},
		{
			name: "invalid organization mode",
			body: library.CreateLibraryRequest{Name: "Test", OrganizationMode: "INVALID"},
			want: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/libraries", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d; want %d; body = %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestGetLibrary_AdminAccess(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/libraries/%d", created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp library.LibraryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID != created.ID {
		t.Errorf("id = %d; want %d", resp.ID, created.ID)
	}
}

func TestGetLibrary_UserWithAccess(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	_, userToken := seedRegularUser(t, db, "regularuser", []int64{created.ID})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/libraries/%d", created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGetLibrary_UserWithoutAccess(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	// User has no access to this library.
	_, userToken := seedRegularUser(t, db, "regularuser", nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/libraries/%d", created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

func TestGetLibrary_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/libraries/99999", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateLibrary_AdminSucceeds(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Original Name",
		OrganizationMode: "BOOK_PER_FILE",
	})

	updateBody, _ := json.Marshal(library.UpdateLibraryRequest{
		Name:             "Updated Name",
		OrganizationMode: "BOOK_PER_FOLDER",
	})

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/libraries/%d", created.ID), bytes.NewBuffer(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Verify the update by fetching the library.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/libraries/%d", created.ID), nil)
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var updated library.LibraryResponse
	if err := json.NewDecoder(getRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode get response: %v", err)
	}

	if updated.Name != "Updated Name" {
		t.Errorf("name = %q; want %q", updated.Name, "Updated Name")
	}
	if updated.OrganizationMode != "BOOK_PER_FOLDER" {
		t.Errorf("organizationMode = %q; want %q", updated.OrganizationMode, "BOOK_PER_FOLDER")
	}
}

func TestUpdateLibrary_NonAdminForbidden(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	_, userToken := seedRegularUser(t, db, "regularuser", []int64{created.ID})

	updateBody, _ := json.Marshal(library.UpdateLibraryRequest{
		Name:             "Hacked Name",
		OrganizationMode: "BOOK_PER_FILE",
	})

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/libraries/%d", created.ID), bytes.NewBuffer(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteLibrary_AdminSucceeds(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "To Delete",
		OrganizationMode: "BOOK_PER_FILE",
	})

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/libraries/%d", created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Verify it's gone.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/libraries/%d", created.ID), nil)
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete: status = %d; want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteLibrary_NonAdminForbidden(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	_, userToken := seedRegularUser(t, db, "regularuser", []int64{created.ID})

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/libraries/%d", created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteLibrary_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	req := httptest.NewRequest(http.MethodDelete, "/api/libraries/99999", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAddPath_AdminSucceeds(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	addBody, _ := json.Marshal(library.AddPathRequest{Path: "/books/new"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/libraries/%d/paths", created.ID), bytes.NewBuffer(addBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var pathResp library.PathResponse
	if err := json.NewDecoder(rec.Body).Decode(&pathResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if pathResp.Path != "/books/new" {
		t.Errorf("path = %q; want %q", pathResp.Path, "/books/new")
	}
	if pathResp.ID == 0 {
		t.Error("path id is 0")
	}
}

func TestAddPath_NonAdminForbidden(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	_, userToken := seedRegularUser(t, db, "regularuser", []int64{created.ID})

	addBody, _ := json.Marshal(library.AddPathRequest{Path: "/books/new"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/libraries/%d/paths", created.ID), bytes.NewBuffer(addBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAddPath_EmptyPath(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	addBody, _ := json.Marshal(library.AddPathRequest{Path: ""})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/libraries/%d/paths", created.ID), bytes.NewBuffer(addBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRemovePath_AdminSucceeds(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
		Paths:            []string{"/books/fiction"},
	})

	if len(created.Paths) == 0 {
		t.Fatal("expected at least one path in created library")
	}
	pathID := created.Paths[0].ID

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/libraries/%d/paths/%d", created.ID, pathID), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Verify path is gone.
	listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/libraries/%d/paths", created.ID), nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	var paths []library.PathResponse
	if err := json.NewDecoder(listRec.Body).Decode(&paths); err != nil {
		t.Fatalf("decode paths response: %v", err)
	}

	if len(paths) != 0 {
		t.Errorf("got %d paths; want 0", len(paths))
	}
}

func TestRemovePath_NonAdminForbidden(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
		Paths:            []string{"/books/fiction"},
	})

	_, userToken := seedRegularUser(t, db, "regularuser", []int64{created.ID})

	pathID := created.Paths[0].ID
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/libraries/%d/paths/%d", created.ID, pathID), nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

func TestListPaths_AuthenticatedUser(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
		Paths:            []string{"/books/a", "/books/b"},
	})

	_, userToken := seedRegularUser(t, db, "regularuser", []int64{created.ID})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/libraries/%d/paths", created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var paths []library.PathResponse
	if err := json.NewDecoder(rec.Body).Decode(&paths); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("got %d paths; want 2", len(paths))
	}
}

func TestScan_ReturnsOK(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/libraries/%d/scan", created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp library.ScanResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Library has no paths, so nothing should be scanned.
	if resp.BooksAdded != 0 {
		t.Errorf("booksAdded = %d; want 0", resp.BooksAdded)
	}
}

func TestScan_UserWithAccess_Forbidden(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Test Library",
		OrganizationMode: "BOOK_PER_FILE",
	})

	_, userToken := seedRegularUser(t, db, "regularuser", []int64{created.ID})

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/libraries/%d/scan", created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Scan is admin-only; regular users should receive 403.
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteLibrary_CascadesPathDeletion(t *testing.T) {
	db := openTestDB(t)
	_, adminToken := seedAdmin(t, db)
	router := newTestRouter(t, db)

	// Create library with paths.
	created := createLibraryViaAPI(t, router, adminToken, library.CreateLibraryRequest{
		Name:             "Library With Paths",
		OrganizationMode: "BOOK_PER_FILE",
		Paths:            []string{"/books/a", "/books/b"},
	})

	if len(created.Paths) != 2 {
		t.Fatalf("expected 2 paths; got %d", len(created.Paths))
	}

	// Delete the library.
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/libraries/%d", created.ID), nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d; want %d", delRec.Code, http.StatusNoContent)
	}

	// Verify library is gone (paths cascade via FK).
	q := library.New(db)
	_, err := q.GetLibraryByID(context.Background(), created.ID)
	if err == nil {
		t.Error("expected error getting deleted library; got nil")
	}
}
