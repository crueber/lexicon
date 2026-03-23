package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	s := &Server{
		cfg:    Config{},
		router: setupTestRouter(),
		logger: newLogger("info", "text"),
	}
	s.setupMiddleware()
	s.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /health status = %d; want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("GET /health Content-Type = %q; want %q", contentType, "application/json")
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("GET /health body decode: %v", err)
	}

	if got, want := body["status"], "ok"; got != want {
		t.Errorf("GET /health status = %q; want %q", got, want)
	}
}

func TestRecovererMiddleware(t *testing.T) {
	s := &Server{
		cfg:    Config{},
		router: setupTestRouter(),
		logger: newLogger("error", "text"),
	}
	s.setupMiddleware()

	// Register a handler that panics.
	s.router.Get("/panic", func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GET /panic status = %d; want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCORSDevMode(t *testing.T) {
	s := &Server{
		cfg:    Config{DevMode: true},
		router: setupTestRouter(),
		logger: newLogger("info", "text"),
	}
	s.setupMiddleware()
	s.router.Get("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS Allow-Origin = %q; want %q", got, "*")
	}
}

func TestCORSProdMode(t *testing.T) {
	s := &Server{
		cfg:    Config{DevMode: false},
		router: setupTestRouter(),
		logger: newLogger("info", "text"),
	}
	s.setupMiddleware()
	s.router.Get("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS Allow-Origin = %q; want empty in prod mode", got)
	}
}

func TestCORSPreflightDevMode(t *testing.T) {
	s := &Server{
		cfg:    Config{DevMode: true},
		router: setupTestRouter(),
		logger: newLogger("info", "text"),
	}
	s.setupMiddleware()
	s.router.Get("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("OPTIONS /test status = %d; want %d", rec.Code, http.StatusNoContent)
	}
}
