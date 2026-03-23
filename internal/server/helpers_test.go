package server

import "github.com/go-chi/chi/v5"

// setupTestRouter creates a fresh chi router for testing.
func setupTestRouter() *chi.Mux {
	return chi.NewRouter()
}
