package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAuthValidToken(t *testing.T) {
	p := Principal{
		UserID:   1,
		Username: "testuser",
		Role:     "USER",
	}

	tokenString, err := IssueAccessToken(p, testSecret)
	if err != nil {
		t.Fatalf("IssueAccessToken() error: %v", err)
	}

	var gotPrincipal *Principal
	handler := RequireAuth(testSecret)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotPrincipal = PrincipalFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}

	if gotPrincipal == nil {
		t.Fatal("principal not set in context")
	}
	if gotPrincipal.UserID != p.UserID {
		t.Errorf("UserID = %d; want %d", gotPrincipal.UserID, p.UserID)
	}
	if gotPrincipal.Username != p.Username {
		t.Errorf("Username = %q; want %q", gotPrincipal.Username, p.Username)
	}
}

func TestRequireAuthMissingHeader(t *testing.T) {
	handler := RequireAuth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthInvalidToken(t *testing.T) {
	handler := RequireAuth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthBadFormat(t *testing.T) {
	handler := RequireAuth(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdminWithAdmin(t *testing.T) {
	p := Principal{
		UserID:   1,
		Username: "admin",
		Role:     "ADMIN",
	}

	tokenString, err := IssueAccessToken(p, testSecret)
	if err != nil {
		t.Fatalf("IssueAccessToken() error: %v", err)
	}

	called := false
	handler := RequireAuth(testSecret)(RequireAdmin()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Error("handler was not called for admin user")
	}
}

func TestRequireAdminWithNonAdmin(t *testing.T) {
	p := Principal{
		UserID:   2,
		Username: "user",
		Role:     "USER",
	}

	tokenString, err := IssueAccessToken(p, testSecret)
	if err != nil {
		t.Fatalf("IssueAccessToken() error: %v", err)
	}

	handler := RequireAuth(testSecret)(RequireAdmin()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireAdminNoPrincipal(t *testing.T) {
	handler := RequireAdmin()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequirePermission(t *testing.T) {
	tests := []struct {
		name       string
		principal  Principal
		perm       string
		wantStatus int
	}{
		{
			name: "admin has all permissions",
			principal: Principal{
				UserID: 1, Username: "admin", Role: "ADMIN",
			},
			perm:       "download",
			wantStatus: http.StatusOK,
		},
		{
			name: "user with permission",
			principal: Principal{
				UserID: 2, Username: "user", Role: "USER",
				Permissions: Permissions{CanDownload: true},
			},
			perm:       "download",
			wantStatus: http.StatusOK,
		},
		{
			name: "user without permission",
			principal: Principal{
				UserID: 3, Username: "user2", Role: "USER",
				Permissions: Permissions{CanDownload: false},
			},
			perm:       "download",
			wantStatus: http.StatusForbidden,
		},
		{
			name: "upload permission",
			principal: Principal{
				UserID: 4, Username: "uploader", Role: "USER",
				Permissions: Permissions{CanUpload: true},
			},
			perm:       "upload",
			wantStatus: http.StatusOK,
		},
		{
			name: "unknown permission denied",
			principal: Principal{
				UserID: 5, Username: "user3", Role: "USER",
			},
			perm:       "nonexistent",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenString, err := IssueAccessToken(tt.principal, testSecret)
			if err != nil {
				t.Fatalf("IssueAccessToken() error: %v", err)
			}

			handler := RequireAuth(testSecret)(RequirePermission(tt.perm)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", "Bearer "+tokenString)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d; want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestPrincipalFromContextNil(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := PrincipalFromContext(req.Context())
	if p != nil {
		t.Errorf("PrincipalFromContext() = %v; want nil", p)
	}
}
