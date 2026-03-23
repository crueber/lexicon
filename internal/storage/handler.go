package storage

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"

	bookpkg "github.com/crueber/lexicon/internal/book"
)

// Handler serves cover images for books.
type Handler struct {
	db      *sql.DB
	dataDir string
	logger  *slog.Logger
}

// NewHandler creates a new storage Handler.
func NewHandler(db *sql.DB, dataDir string, logger *slog.Logger) *Handler {
	return &Handler{
		db:      db,
		dataDir: dataDir,
		logger:  logger,
	}
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
