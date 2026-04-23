package server

import (
	"encoding/json"
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

	// WebSocket endpoint (auth handled inside the handler via token query param).
	s.wsHandler.Routes(s.router)

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

			// Cover serving routes nested under /books/{id}/cover.
			r.Route("/{id}/cover", func(r chi.Router) {
				s.storageHandler.Routes(r)
				r.Put("/", s.storageHandler.HandleUploadCover)
				r.Delete("/", s.storageHandler.HandleDeleteCover)
			})

			// Similar books recommendation.
			r.Get("/{id}/similar", s.recommendationHandler.HandleSimilarBooks)

			// Send book to device via email.
			r.With(auth.RequirePermission("email_send")).Post("/{id}/send", s.emailHandler.HandleSendBook)
		})

		// Author routes (require auth).
		r.Route("/authors", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.bookHandler.AuthorRoutes(r)
		})

		// Series routes (require auth).
		r.Route("/series", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.bookHandler.SeriesRoutes(r)
		})

		// Taxonomy routes (require auth).
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.bookHandler.TaxonomyRoutes(r)
		})

		// Shelf routes (require auth).
		r.Route("/shelves", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.shelfHandler.Routes(r)
		})

		// Magic shelf routes (require auth).
		r.Route("/magic-shelves", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.shelfHandler.MagicRoutes(r)
		})

		// Task routes (require auth; admin-only routes enforced inside Routes()).
		r.Route("/tasks", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.taskHandler.Routes(r)
		})

		// Admin user management routes (require auth + admin).
		r.Route("/admin/users", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			r.Use(auth.RequireAdmin())
			s.userHandler.AdminRoutes(r)
		})

		// Self-service user routes (require auth).
		r.Route("/users", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.userHandler.SelfRoutes(r)
		})

		// Reading stats (require auth).
		r.Route("/users/me/reading-stats", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			r.Get("/", s.readerHandler.HandleReadingStats)
		})

		// Content restriction routes (require auth).
		r.Route("/users/me/content-restrictions", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			r.Get("/", s.contentRestrictionHandler.HandleList)
			r.Post("/", s.contentRestrictionHandler.HandleCreate)
			r.Put("/{id}", s.contentRestrictionHandler.HandleUpdate)
			r.Delete("/{id}", s.contentRestrictionHandler.HandleDelete)
		})

		// Hardcover sync routes (require auth).
		r.Route("/users/me/hardcover", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			r.Get("/", s.hardcoverHandler.HandleGetSettings)
			r.Put("/", s.hardcoverHandler.HandleSaveSettings)
		})

		// Font management routes (require auth).
		r.Route("/fonts", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			r.Get("/", s.storageHandler.HandleListFonts)
			r.Post("/", s.storageHandler.HandleUploadFont)
			r.Delete("/{id}", s.storageHandler.HandleDeleteFont)
			r.Get("/{id}/file", s.storageHandler.HandleServeFont)
		})

		// Reader routes (require auth).
		r.Route("/reader", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.readerHandler.Routes(r)
			// Annotation endpoints live under /api/reader/books/{bookId}/annotations.
			s.notebookHandler.ReaderRoutes(r)
		})

		// Notebook routes (require auth).
		r.Route("/notebook", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.notebookHandler.NotebookRoutes(r)
		})

		// Dashboard routes (require auth).
		r.Route("/dashboard", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.dashboardHandler.Routes(r)
		})

		// Metadata routes (require auth).
		r.Route("/metadata", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.metadataHandler.Routes(r)
		})

		// BookDrop routes (require auth).
		r.Route("/bookdrop/files", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.bookdropHandler.Routes(r)
		})

		// Email provider routes (require auth + admin).
		r.Route("/email/providers", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			r.Use(auth.RequireAdmin())
			s.emailHandler.ProviderRoutes(r)
		})

		// Email recipient routes (require auth).
		r.Route("/email/recipients", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			s.emailHandler.RecipientRoutes(r)
		})

		// Admin metadata settings routes (require auth + admin).
		r.Route("/admin/settings", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			r.Use(auth.RequireAdmin())
			r.Get("/", s.appsettingsHandler.HandleGetSettings)
			r.Put("/", s.appsettingsHandler.HandleSaveSettings)
			s.metadataHandler.AdminRoutes(r)
		})

		// Admin audit log routes (require auth + admin).
		r.Route("/admin/audit-logs", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg.JWTSecret))
			r.Use(auth.RequireAdmin())
			r.Get("/", s.auditHandler.HandleListAuditLogs)
		})
	})

	// OPDS catalog routes (no JWT auth — OPDS uses its own Basic Auth).
	s.router.Route("/opds", func(r chi.Router) {
		s.opdsHandler.Routes(r)
	})

	// Kobo sync routes (no JWT auth — Kobo uses X-Kobo-UserKey token auth).
	s.router.Route("/kobo", func(r chi.Router) {
		s.koboHandler.Routes(r)
	})

	// Kobo token management (requires JWT auth).
	s.router.Route("/api/kobo", func(r chi.Router) {
		r.Use(auth.RequireAuth(s.cfg.JWTSecret))
		s.koboHandler.TokenRoutes(r)
	})

	// KOReader sync routes (no JWT auth — KOSync uses HTTP Basic Auth with MD5 passwords).
	s.router.Route("/kosync", func(r chi.Router) {
		s.koreaderHandler.Routes(r)
	})

	// Frontend: proxy to Vite in dev mode, serve embedded files in production.
	if s.cfg.DevMode {
		s.router.Handle("/*", s.viteProxy())
	} else {
		s.router.Handle("/*", s.staticHandler())
	}
}

// handleHealth returns a JSON health check response, including a database ping.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connectivity when a db is available.
	if s.db != nil {
		if err := s.db.PingContext(r.Context()); err != nil {
			s.logger.Error("health check: database ping failed", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "reason": "database unreachable"})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
