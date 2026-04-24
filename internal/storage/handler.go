package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/auth"
	bookpkg "github.com/crueber/lexicon/internal/book"
)

// Handler serves cover images for books.
type Handler struct {
	db                   *sql.DB
	dataDir              string
	logger               *slog.Logger
	fontSvc              *FontService
	auditSvc             *audit.Service
	broadcastBookUpdated func(bookID int64)
}

// NewHandler creates a new storage Handler.
func NewHandler(db *sql.DB, dataDir string, logger *slog.Logger) *Handler {
	h := &Handler{
		db:      db,
		dataDir: dataDir,
		logger:  logger,
	}
	h.fontSvc = NewFontService(db, dataDir, logger)
	return h
}

// WithAuditService sets the audit service for logging storage events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// WithBroadcastBookUpdatedFunc sets the broadcast function for book updates.
func (h *Handler) WithBroadcastBookUpdatedFunc(fn func(bookID int64)) {
	h.broadcastBookUpdated = fn
}

// Routes registers cover serving routes.
// The router is expected to be mounted at /api/books/{id}/cover.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.handleCover)
	r.Get("/thumbnail", h.handleThumbnail)
}

// handleCover serves the full-size cover image for a book.
func (h *Handler) handleCover(w http.ResponseWriter, r *http.Request) {
	h.serveCoverFile(w, r, "cover.jpg")
}

// handleThumbnail serves the thumbnail cover image for a book.
func (h *Handler) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	h.serveCoverFile(w, r, "thumbnail.jpg")
}

// serveCoverFile looks up the book's cover path and serves the given filename
// from the cover directory.
func (h *Handler) serveCoverFile(w http.ResponseWriter, r *http.Request, filename string) {
	idStr := chi.URLParam(r, "id")
	bookID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid book id", http.StatusBadRequest)
		return
	}

	q := bookpkg.New(h.db)
	meta, err := q.GetBookMetadata(r.Context(), bookID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get book metadata", "book_id", bookID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !meta.CoverPath.Valid || meta.CoverPath.String == "" {
		http.NotFound(w, r)
		return
	}

	// The cover_path is relative (e.g., "covers/books/42/cover.jpg").
	// Replace the filename to serve either cover.jpg or thumbnail.jpg.
	coverDir := filepath.Join(h.dataDir, filepath.Dir(meta.CoverPath.String))
	filePath := filepath.Join(coverDir, filename)

	if _, err := os.Stat(filePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("stat cover file", "path", filePath, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, filePath)
}

// HandleUploadCover handles PUT /api/books/{id}/cover.
func (h *Handler) HandleUploadCover(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	// Parse multipart form with 10MB max memory.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("cover")
	if err != nil {
		writeError(w, http.StatusBadRequest, "cover file required")
		return
	}
	defer file.Close()

	// Validate file size (max 10MB).
	if header.Size > 10<<20 {
		writeError(w, http.StatusBadRequest, "cover file too large")
		return
	}

	// Read file into memory.
	data, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		h.logger.Error("read cover upload", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Determine if audiobook for square crop.
	q := bookpkg.New(h.db)
	ctx := r.Context()
	bookRow, err := q.GetBookByID(ctx, bookID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		h.logger.Error("get book for cover upload", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	isAudio := bookRow.BookType == "AUDIOBOOK"

	// Process and save cover.
	coverPath, err := ProcessCover(data, bookID, h.dataDir, isAudio)
	if err != nil {
		h.logger.Error("process cover upload", "error", err)
		writeError(w, http.StatusBadRequest, "invalid image file")
		return
	}

	// Update database.
	if err := q.UpdateBookCover(ctx, bookpkg.UpdateBookCoverParams{
		CoverPath: sql.NullString{String: coverPath, Valid: true},
		BookID:    bookID,
	}); err != nil {
		h.logger.Error("update book cover", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		var userID int64
		if p := authPrincipalFromRequest(r); p != nil {
			userID = p.UserID
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:       &userID,
			Action:       audit.ActionBookCoverUpdated,
			ResourceType: "book",
			ResourceID:   &bookID,
			IPAddress:    ip,
		})
	}

	if h.broadcastBookUpdated != nil {
		h.broadcastBookUpdated(bookID)
	}

	writeJSON(w, http.StatusOK, map[string]string{"coverPath": coverPath})
}

// HandleDeleteCover handles DELETE /api/books/{id}/cover.
func (h *Handler) HandleDeleteCover(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := bookpkg.New(h.db)
	ctx := r.Context()

	meta, err := q.GetBookMetadata(ctx, bookID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		h.logger.Error("get book metadata for cover delete", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if meta.CoverPath.Valid && meta.CoverPath.String != "" {
		coverDir := filepath.Join(h.dataDir, filepath.Dir(meta.CoverPath.String))
		if err := os.RemoveAll(coverDir); err != nil {
			h.logger.Error("delete cover directory", "path", coverDir, "error", err)
			// Non-fatal: continue to clear DB record.
		}
	}

	if err := q.UpdateBookCover(ctx, bookpkg.UpdateBookCoverParams{
		CoverPath: sql.NullString{Valid: false},
		BookID:    bookID,
	}); err != nil {
		h.logger.Error("clear book cover", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		var userID int64
		if p := authPrincipalFromRequest(r); p != nil {
			userID = p.UserID
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:       &userID,
			Action:       audit.ActionBookCoverUpdated,
			ResourceType: "book",
			ResourceID:   &bookID,
			IPAddress:    ip,
		})
	}

	if h.broadcastBookUpdated != nil {
		h.broadcastBookUpdated(bookID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// authPrincipalFromRequest extracts the auth principal from the request context.
func authPrincipalFromRequest(r *http.Request) *auth.Principal {
	return auth.PrincipalFromContext(r.Context())
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
