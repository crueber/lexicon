package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/crueber/lexicon/internal/user"
)

// openTestDB creates a temporary SQLite database with migrations applied.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	// Set pragmas.
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

	// Apply all migrations (avoid importing server package for migrations).
	// The login handler now queries user_library_permission (migration 002).
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

// seedTestUser creates a test user with known credentials and returns the user.
func seedTestUser(t *testing.T, db *sql.DB, username, password, role string) user.User {
	t.Helper()

	ctx := context.Background()
	var u user.User
	var err error

	if role == "ADMIN" {
		u, err = user.CreateAdminUser(ctx, db, user.CreateUserServiceParams{
			Username: username,
			Password: password,
			Name:     "Test " + username,
			Email:    username + "@test.com",
		})
	} else {
		u, err = user.CreateUserWithDefaults(ctx, db, user.CreateUserServiceParams{
			Username: username,
			Password: password,
			Name:     "Test " + username,
			Email:    username + "@test.com",
		})
	}
	if err != nil {
		t.Fatalf("seed test user %q: %v", username, err)
	}
	return u
}

// newTestHandler creates a Handler with a test database and logger.
func newTestHandler(t *testing.T, db *sql.DB) *Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewHandler(db, testSecret, logger)
}

// newTestRouter creates a chi router with auth routes mounted.
func newTestRouter(t *testing.T, h *Handler) *chi.Mux {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/api/auth/login", h.HandleLogin)
	r.Post("/api/auth/refresh", h.HandleRefresh)
	r.Post("/api/auth/logout", h.HandleLogout)
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(testSecret))
		r.Get("/api/auth/me", h.HandleMe)
		r.Patch("/api/auth/me/password", h.HandleChangePassword)
	})
	return r
}

func TestHandleLoginSuccess(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "testuser", "password123", "USER")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	body := `{"username":"testuser","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("accessToken is empty")
	}
	if resp.RefreshToken == "" {
		t.Error("refreshToken is empty")
	}
	if resp.User.Username != "testuser" {
		t.Errorf("user.username = %q; want %q", resp.User.Username, "testuser")
	}
	if resp.User.Role != "USER" {
		t.Errorf("user.role = %q; want %q", resp.User.Role, "USER")
	}
}

func TestHandleLoginAdminSuccess(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "admin", "adminpass", "ADMIN")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	body := `{"username":"admin","password":"adminpass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.User.Role != "ADMIN" {
		t.Errorf("user.role = %q; want %q", resp.User.Role, "ADMIN")
	}
}

func TestHandleLoginWrongPassword(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "testuser", "password123", "USER")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	body := `{"username":"testuser","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleLoginUnknownUser(t *testing.T) {
	db := openTestDB(t)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	body := `{"username":"nonexistent","password":"password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleLoginMissingFields(t *testing.T) {
	db := openTestDB(t)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	body := `{"username":"testuser"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleLoginInvalidJSON(t *testing.T) {
	db := openTestDB(t)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleRefreshSuccess(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "testuser", "password123", "USER")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	// First, login to get a refresh token.
	loginBody := `{"username":"testuser","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d; want %d", loginRec.Code, http.StatusOK)
	}

	var loginResp loginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	// Now refresh.
	refreshBody, _ := json.Marshal(refreshRequest{RefreshToken: loginResp.RefreshToken})
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewBuffer(refreshBody))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshRec := httptest.NewRecorder()
	router.ServeHTTP(refreshRec, refreshReq)

	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d; want %d; body = %s", refreshRec.Code, http.StatusOK, refreshRec.Body.String())
	}

	var refreshResp refreshResponse
	if err := json.NewDecoder(refreshRec.Body).Decode(&refreshResp); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}

	if refreshResp.AccessToken == "" {
		t.Error("new accessToken is empty")
	}
	if refreshResp.RefreshToken == "" {
		t.Error("new refreshToken is empty")
	}

	// Old refresh token should be revoked (using it again should fail).
	refreshBody2, _ := json.Marshal(refreshRequest{RefreshToken: loginResp.RefreshToken})
	refreshReq2 := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewBuffer(refreshBody2))
	refreshReq2.Header.Set("Content-Type", "application/json")
	refreshRec2 := httptest.NewRecorder()
	router.ServeHTTP(refreshRec2, refreshReq2)

	if refreshRec2.Code != http.StatusUnauthorized {
		t.Errorf("reuse old refresh token: status = %d; want %d", refreshRec2.Code, http.StatusUnauthorized)
	}
}

func TestHandleRefreshInvalidToken(t *testing.T) {
	db := openTestDB(t)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	body := `{"refreshToken":"invalid-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleLogoutSuccess(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "testuser", "password123", "USER")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	// Login first.
	loginBody := `{"username":"testuser","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	var loginResp loginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	// Logout.
	logoutBody, _ := json.Marshal(logoutRequest{RefreshToken: loginResp.RefreshToken})
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", bytes.NewBuffer(logoutBody))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Errorf("logout status = %d; want %d", logoutRec.Code, http.StatusOK)
	}

	// Refresh with the revoked token should fail.
	refreshBody, _ := json.Marshal(refreshRequest{RefreshToken: loginResp.RefreshToken})
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewBuffer(refreshBody))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshRec := httptest.NewRecorder()
	router.ServeHTTP(refreshRec, refreshReq)

	if refreshRec.Code != http.StatusUnauthorized {
		t.Errorf("refresh after logout: status = %d; want %d", refreshRec.Code, http.StatusUnauthorized)
	}
}

func TestHandleMeSuccess(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "testuser", "password123", "USER")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	// Login to get an access token.
	loginBody := `{"username":"testuser","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	var loginResp loginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	// Call /me.
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d; want %d; body = %s", meRec.Code, http.StatusOK, meRec.Body.String())
	}

	var meResp userInfo
	if err := json.NewDecoder(meRec.Body).Decode(&meResp); err != nil {
		t.Fatalf("decode me response: %v", err)
	}

	if meResp.Username != "testuser" {
		t.Errorf("username = %q; want %q", meResp.Username, "testuser")
	}
}

func TestHandleMeUnauthorized(t *testing.T) {
	db := openTestDB(t)
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleChangePasswordSuccess(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "testuser", "oldpassword", "USER")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	// Login with old password.
	loginBody := `{"username":"testuser","password":"oldpassword"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	var loginResp loginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	// Change password.
	changeBody := `{"currentPassword":"oldpassword","newPassword":"newpassword123"}`
	changeReq := httptest.NewRequest(http.MethodPatch, "/api/auth/me/password", bytes.NewBufferString(changeBody))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	changeRec := httptest.NewRecorder()
	router.ServeHTTP(changeRec, changeReq)

	if changeRec.Code != http.StatusOK {
		t.Fatalf("change password status = %d; want %d; body = %s", changeRec.Code, http.StatusOK, changeRec.Body.String())
	}

	// Login with new password should succeed.
	loginBody2 := `{"username":"testuser","password":"newpassword123"}`
	loginReq2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody2))
	loginReq2.Header.Set("Content-Type", "application/json")
	loginRec2 := httptest.NewRecorder()
	router.ServeHTTP(loginRec2, loginReq2)

	if loginRec2.Code != http.StatusOK {
		t.Errorf("login with new password: status = %d; want %d", loginRec2.Code, http.StatusOK)
	}

	// Login with old password should fail.
	loginBody3 := `{"username":"testuser","password":"oldpassword"}`
	loginReq3 := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody3))
	loginReq3.Header.Set("Content-Type", "application/json")
	loginRec3 := httptest.NewRecorder()
	router.ServeHTTP(loginRec3, loginReq3)

	if loginRec3.Code != http.StatusUnauthorized {
		t.Errorf("login with old password: status = %d; want %d", loginRec3.Code, http.StatusUnauthorized)
	}
}

func TestHandleChangePasswordWrongCurrent(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "testuser", "password123", "USER")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	// Login.
	loginBody := `{"username":"testuser","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	var loginResp loginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	// Try to change with wrong current password.
	changeBody := `{"currentPassword":"wrongpassword","newPassword":"newpassword123"}`
	changeReq := httptest.NewRequest(http.MethodPatch, "/api/auth/me/password", bytes.NewBufferString(changeBody))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	changeRec := httptest.NewRecorder()
	router.ServeHTTP(changeRec, changeReq)

	if changeRec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", changeRec.Code, http.StatusUnauthorized)
	}
}

func TestHandleChangePasswordTooShort(t *testing.T) {
	db := openTestDB(t)
	seedTestUser(t, db, "testuser", "password123", "USER")
	h := newTestHandler(t, db)
	router := newTestRouter(t, h)

	// Login.
	loginBody := `{"username":"testuser","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	var loginResp loginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	// Try to change to a too-short password.
	changeBody := `{"currentPassword":"password123","newPassword":"short"}`
	changeReq := httptest.NewRequest(http.MethodPatch, "/api/auth/me/password", bytes.NewBufferString(changeBody))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	changeRec := httptest.NewRecorder()
	router.ServeHTTP(changeRec, changeReq)

	if changeRec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", changeRec.Code, http.StatusBadRequest)
	}
}
