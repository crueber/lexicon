package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crueber/lexicon/internal/user"
)

func TestRemoteAuthMiddleware_PrincipalInjection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Create a test user.
	u, err := user.CreateUserWithDefaults(ctx, db, user.CreateUserServiceParams{
		Username: "remoteuser",
		Password: "password123",
		Name:     "Remote User",
		Email:    "remote@test.com",
	})
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	cfg := RemoteAuthConfig{
		Enabled:    true,
		UserHeader: "X-Remote-User",
		AutoCreate: false,
	}

	var gotPrincipal *Principal
	handler := RemoteAuthMiddleware(db, cfg, nil, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotPrincipal = PrincipalFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", "remoteuser")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotPrincipal == nil {
		t.Fatal("principal not injected into context")
	}
	if gotPrincipal.UserID != u.ID {
		t.Errorf("UserID = %d; want %d", gotPrincipal.UserID, u.ID)
	}
	if gotPrincipal.Username != "remoteuser" {
		t.Errorf("Username = %q; want %q", gotPrincipal.Username, "remoteuser")
	}
}

func TestRemoteAuthMiddleware_MissingHeader(t *testing.T) {
	db := openTestDB(t)

	cfg := RemoteAuthConfig{
		Enabled:    true,
		UserHeader: "X-Remote-User",
		AutoCreate: false,
	}

	called := false
	handler := RemoteAuthMiddleware(db, cfg, nil, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		if PrincipalFromContext(r.Context()) != nil {
			t.Error("principal should not be set when header is missing")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestRemoteAuthMiddleware_Disabled(t *testing.T) {
	db := openTestDB(t)

	cfg := RemoteAuthConfig{
		Enabled:    false,
		UserHeader: "X-Remote-User",
		AutoCreate: false,
	}

	called := false
	handler := RemoteAuthMiddleware(db, cfg, nil, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		if PrincipalFromContext(r.Context()) != nil {
			t.Error("principal should not be set when disabled")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", "remoteuser")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestRemoteAuthMiddleware_AutoCreate(t *testing.T) {
	db := openTestDB(t)

	cfg := RemoteAuthConfig{
		Enabled:    true,
		UserHeader: "X-Remote-User",
		AutoCreate: true,
	}

	var gotPrincipal *Principal
	handler := RemoteAuthMiddleware(db, cfg, nil, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotPrincipal = PrincipalFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", "newremoteuser")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotPrincipal == nil {
		t.Fatal("principal not injected for auto-created user")
	}
	if gotPrincipal.Username != "newremoteuser" {
		t.Errorf("Username = %q; want %q", gotPrincipal.Username, "newremoteuser")
	}
}

func TestRemoteAuthMiddleware_DisabledUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, err := user.CreateUserWithDefaults(ctx, db, user.CreateUserServiceParams{
		Username: "disableduser",
		Password: "password123",
		Name:     "Disabled User",
	})
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	// Disable the user.
	q := user.New(db)
	if err := q.UpdateUser(ctx, user.UpdateUserParams{
		ID:      u.ID,
		Email:   u.Email,
		Name:    u.Name,
		Enabled: 0,
	}); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	cfg := RemoteAuthConfig{
		Enabled:    true,
		UserHeader: "X-Remote-User",
		AutoCreate: false,
	}

	called := false
	handler := RemoteAuthMiddleware(db, cfg, nil, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		if PrincipalFromContext(r.Context()) != nil {
			t.Error("principal should not be set for disabled user")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", "disableduser")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}
