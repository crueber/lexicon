package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// setupRoutes registers all HTTP routes on the router.
func (s *Server) setupRoutes() {
	// Health check endpoint.
	s.router.Get("/health", s.handleHealth)

	// API routes will be mounted here in future phases.
	// s.router.Route("/api", func(r chi.Router) { ... })

	// Frontend: proxy to Vite in dev mode, serve embedded files in production.
	if s.cfg.DevMode {
		s.router.Handle("/*", s.viteProxy())
	} else {
		s.router.Handle("/*", s.staticHandler())
	}
}

// handleHealth returns a simple JSON health check response.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// viteProxy returns a reverse proxy to the Vite dev server.
func (s *Server) viteProxy() http.Handler {
	target, err := url.Parse("http://localhost:5173")
	if err != nil {
		// Hardcoded URL; a parse failure here is a programming error.
		panic(fmt.Sprintf("parse vite proxy URL: %v", err))
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.Host = target.Host
	}

	return proxy
}

// staticHandler serves the embedded frontend files, falling back to index.html
// for client-side routing.
func (s *Server) staticHandler() http.Handler {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		s.logger.Error("failed to create sub filesystem for frontend", "error", err)
		return http.NotFoundHandler()
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly.
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if the file exists in the embedded filesystem.
		f, err := distFS.Open(path[1:]) // strip leading slash
		if err != nil {
			// File not found — serve index.html for SPA client-side routing.
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
