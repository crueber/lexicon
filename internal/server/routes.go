package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/crueber/lexicon/internal/auth"
	"github.com/go-chi/chi/v5"
)

// setupRoutes registers all HTTP routes on the router.
func (s *Server) setupRoutes() {
	// Health check endpoint.
	s.router.Get("/health", s.handleHealth)

	// API routes.
	s.router.Route("/api", func(r chi.Router) {
		// Auth routes (public).
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.authHandler.HandleLogin)
			r.Post("/refresh", s.authHandler.HandleRefresh)
			r.Post("/logout", s.authHandler.HandleLogout)

			// Protected auth routes.
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAuth(s.cfg.JWTSecret))
				r.Get("/me", s.authHandler.HandleMe)
				r.Patch("/me/password", s.authHandler.HandleChangePassword)
			})
		})

		// Library routes (require auth; admin-only routes enforced inside Routes()).
		r.Route("/libraries", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.libraryHandler.Routes(r)
		})

		// Book routes (require auth).
		r.Route("/books", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.bookHandler.Routes(r)
		})
	})

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
// If the Vite dev server is not reachable, it returns a helpful HTML page
// instead of a raw 502 Bad Gateway.
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

	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, viteNotRunningPage)
	}

	return proxy
}

// viteNotRunningPage is the HTML shown when the Vite dev server is unreachable.
const viteNotRunningPage = `<!DOCTYPE html>
<html><head><title>Lexicon - Dev Mode</title></head>
<body style="background:#1a1a2e;color:#e0e0e0;font-family:system-ui;display:flex;justify-content:center;align-items:center;height:100vh;margin:0">
<div style="text-align:center;max-width:500px">
<h1>Vite Dev Server Not Running</h1>
<p>The Go backend is running in dev mode but the Vite frontend dev server is not reachable at localhost:5173.</p>
<p>Run <code style="background:#2d2d44;padding:2px 8px;border-radius:4px">make run-frontend</code> in another terminal.</p>
<p style="color:#888;font-size:0.9em">Or run <code style="background:#2d2d44;padding:2px 8px;border-radius:4px">make build</code> to build a production binary with embedded frontend.</p>
</div></body></html>`

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
