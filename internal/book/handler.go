package book

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
)

// Handler handles HTTP requests for book management.
type Handler struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewHandler creates a new book Handler.
func NewHandler(db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		db:     db,
		logger: logger,
	}
}

// Routes registers all book routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.handleList)
}

// BookResponse is the JSON representation of a book with basic metadata.
type BookResponse struct {
	ID        int64    `json:"id"`
	LibraryID int64    `json:"libraryId"`
	BookType  string   `json:"bookType"`
	Title     *string  `json:"title"`
	Authors   []string `json:"authors"`
	CoverPath *string  `json:"coverPath"`
	AddedDate *string  `json:"addedDate"`
}

// ListBooksResponse is the JSON response for GET /api/books.
type ListBooksResponse struct {
	Books []BookResponse `json:"books"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

// handleList handles GET /api/books?libraryId={id}&page={n}&size={n}.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	libraryIDStr := r.URL.Query().Get("libraryId")
	if libraryIDStr == "" {
		writeError(w, http.StatusBadRequest, "libraryId is required")
		return
	}

	libraryID, err := strconv.ParseInt(libraryIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid libraryId")
		return
	}

	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
			page = v
		}
	}

	size := 20
	if sizeStr := r.URL.Query().Get("size"); sizeStr != "" {
		if v, err := strconv.Atoi(sizeStr); err == nil && v > 0 && v <= 100 {
			size = v
		}
	}

	offset := int64((page - 1) * size)

	q := New(h.db)

	total, err := q.CountBooksByLibrary(r.Context(), libraryID)
	if err != nil {
		h.logger.Error("count books by library", "library_id", libraryID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := q.ListBooksWithMetadata(r.Context(), ListBooksWithMetadataParams{
		LibraryID: libraryID,
		Limit:     int64(size),
		Offset:    offset,
	})
	if err != nil {
		h.logger.Error("list books with metadata", "library_id", libraryID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	books := make([]BookResponse, 0, len(rows))
	for _, row := range rows {
		// Fetch authors for each book.
		authors, authErr := q.ListBookAuthors(r.Context(), row.ID)
		if authErr != nil {
			h.logger.Warn("list book authors", "book_id", row.ID, "error", authErr)
			// Non-fatal: continue with empty authors.
		}

		authorNames := make([]string, len(authors))
		for i, a := range authors {
			authorNames[i] = a.Name
		}

		br := BookResponse{
			ID:        row.ID,
			LibraryID: row.LibraryID,
			BookType:  row.BookType,
			Authors:   authorNames,
		}

		if row.Title.Valid {
			br.Title = &row.Title.String
		}
		if row.CoverPath.Valid {
			br.CoverPath = &row.CoverPath.String
		}
		if row.AddedDate.Valid {
			br.AddedDate = &row.AddedDate.String
		}

		books = append(books, br)
	}

	writeJSON(w, http.StatusOK, ListBooksResponse{
		Books: books,
		Total: total,
		Page:  page,
		Size:  size,
	})
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
