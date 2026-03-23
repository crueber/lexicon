package book

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

// listParams holds the validated parameters for listing books.
type listParams struct {
	libraryID int64
	page      int
	size      int
	format    string
	bookType  string
	sortBy    string
	sortDir   string
}

// listBooksFiltered executes a safe parameterized query for filtered/sorted book listing.
// It builds the query dynamically but only uses ? placeholders for user-supplied values —
// sort column and direction are validated against an allowlist before interpolation.
func (h *Handler) listBooksFiltered(ctx context.Context, p listParams) ([]ListBooksWithMetadataRow, int64, error) {
	// Validate and map sortBy to a safe column expression.
	sortCol := "b.id"
	switch p.sortBy {
	case "title":
		sortCol = "LOWER(COALESCE(bm.title, ''))"
	case "addedDate":
		sortCol = "b.added_date"
	}

	// Validate sortDir against an allowlist.
	sortDir := "DESC"
	if p.sortDir == "ASC" {
		sortDir = "ASC"
	}

	// Build WHERE clause with parameterized placeholders.
	where := "b.library_id = ?"
	args := []any{p.libraryID}

	if p.bookType != "" {
		where += " AND b.book_type = ?"
		args = append(args, p.bookType)
	}

	if p.format != "" {
		where += " AND EXISTS (SELECT 1 FROM book_file bf WHERE bf.book_id = b.id AND bf.format = ?)"
		args = append(args, p.format)
	}

	// Count query.
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM book b LEFT JOIN book_metadata bm ON b.id = bm.book_id WHERE %s`, where)
	var total int64
	if err := h.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count books: %w", err)
	}

	// Data query — sortCol and sortDir are validated against allowlists above, not user input.
	offset := int64((p.page - 1) * p.size)
	dataQuery := fmt.Sprintf(`
		SELECT b.id, b.library_id, b.book_type, b.added_date, bm.title, bm.cover_path
		FROM book b
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		WHERE %s
		ORDER BY %s %s
		LIMIT ? OFFSET ?`, where, sortCol, sortDir)

	// Copy args to avoid aliasing the underlying array when appending LIMIT/OFFSET.
	dataArgs := make([]any, len(args), len(args)+2)
	copy(dataArgs, args)
	dataArgs = append(dataArgs, int64(p.size), offset)
	rows, err := h.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	var items []ListBooksWithMetadataRow
	for rows.Next() {
		var i ListBooksWithMetadataRow
		if err := rows.Scan(&i.ID, &i.LibraryID, &i.BookType, &i.AddedDate, &i.Title, &i.CoverPath); err != nil {
			return nil, 0, fmt.Errorf("scan book row: %w", err)
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, fmt.Errorf("close rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return items, total, nil
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

// handleList handles GET /api/books?libraryId={id}&page={n}&size={n}&format={f}&bookType={t}&sortBy={s}&sortDir={d}.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
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

	// Optional filter/sort parameters.
	format := r.URL.Query().Get("format")
	bookType := r.URL.Query().Get("bookType")
	sortBy := r.URL.Query().Get("sortBy")
	sortDir := r.URL.Query().Get("sortDir")

	// Validate bookType against known values; ignore unknown values.
	switch bookType {
	case "EBOOK", "AUDIOBOOK", "COMIC":
		// valid
	default:
		bookType = ""
	}

	// Validate sortBy against known values; default to addedDate.
	switch sortBy {
	case "title", "addedDate":
		// valid
	default:
		sortBy = "addedDate"
	}

	// Validate sortDir; default to DESC.
	switch sortDir {
	case "ASC", "DESC":
		// valid
	default:
		sortDir = "DESC"
	}

	params := listParams{
		libraryID: libraryID,
		page:      page,
		size:      size,
		format:    format,
		bookType:  bookType,
		sortBy:    sortBy,
		sortDir:   sortDir,
	}

	bookRows, total, err := h.listBooksFiltered(r.Context(), params)
	if err != nil {
		h.logger.Error("list books filtered", "library_id", libraryID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	q := New(h.db)
	books := make([]BookResponse, 0, len(bookRows))
	for _, row := range bookRows {
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
