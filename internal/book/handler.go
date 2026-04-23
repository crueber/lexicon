package book

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/auth"
)

// shelfHandler is the interface for the shelf handler's book-shelves endpoint.
// Defined here at the consumer to keep the interface small.
type shelfHandler interface {
	HandleListShelvesForBook(w http.ResponseWriter, r *http.Request)
}

// Handler handles HTTP requests for book management.
type Handler struct {
	db           *sql.DB
	logger       *slog.Logger
	shelfHandler shelfHandler
	auditSvc     *audit.Service
}

// NewHandler creates a new book Handler.
func NewHandler(db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		db:     db,
		logger: logger,
	}
}

// WithShelfHandler sets the shelf handler for the /api/books/{id}/shelves endpoint.
func (h *Handler) WithShelfHandler(sh shelfHandler) {
	h.shelfHandler = sh
}

// WithAuditService sets the audit service for logging book events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// Routes registers all book routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.handleList)
	r.Get("/{id}", h.handleGet)
	r.Delete("/{id}", h.handleDelete)
	r.Get("/{id}/files", h.handleListFiles)
	r.Get("/{id}/shelves", h.handleListShelves)
}

// handleListShelves handles GET /api/books/{id}/shelves.
func (h *Handler) handleListShelves(w http.ResponseWriter, r *http.Request) {
	if h.shelfHandler == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}
	h.shelfHandler.HandleListShelvesForBook(w, r)
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

// BookMetadataResponse is the JSON representation of book metadata.
type BookMetadataResponse struct {
	Title         *string `json:"title"`
	Subtitle      *string `json:"subtitle"`
	Description   *string `json:"description"`
	Publisher     *string `json:"publisher"`
	PublishDate   *string `json:"publishDate"`
	PageCount     *int64  `json:"pageCount"`
	Language      *string `json:"language"`
	Isbn10        *string `json:"isbn10"`
	Isbn13        *string `json:"isbn13"`
	CoverPath     *string `json:"coverPath"`
	GoogleBooksID *string `json:"googleBooksId"`
	AmazonID      *string `json:"amazonId"`
	GoodreadsID   *string `json:"goodreadsId"`
	HardcoverID   *string `json:"hardcoverId"`
}

// AuthorResponse is the JSON representation of an author.
type AuthorResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// SeriesResponse is the JSON representation of a series entry.
type SeriesResponse struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	SeriesNumber *float64 `json:"seriesNumber"`
}

// CategoryResponse is the JSON representation of a category.
type CategoryResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// TagResponse is the JSON representation of a tag.
type TagResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// BookFileResponse is the JSON representation of a book file.
type BookFileResponse struct {
	ID           int64   `json:"id"`
	Format       string  `json:"format"`
	FileSize     *int64  `json:"fileSize"`
	FilePath     string  `json:"filePath"`
	TrackNumber  *int64  `json:"trackNumber"`
	TrackTitle   *string `json:"trackTitle"`
	DurationSecs *int64  `json:"durationSecs"`
}

// BookDetailResponse is the JSON representation of a book with full details.
type BookDetailResponse struct {
	ID         int64                 `json:"id"`
	LibraryID  int64                 `json:"libraryId"`
	BookType   string                `json:"bookType"`
	FolderPath *string               `json:"folderPath"`
	AddedDate  *string               `json:"addedDate"`
	Metadata   *BookMetadataResponse `json:"metadata"`
	Authors    []AuthorResponse      `json:"authors"`
	Series     []SeriesResponse      `json:"series"`
	Categories []CategoryResponse    `json:"categories"`
	Tags       []TagResponse         `json:"tags"`
	Files      []BookFileResponse    `json:"files"`
}

// handleGet handles GET /api/books/{id}.
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	// Fetch book with metadata in a single query.
	row, err := q.GetBookWithMetadata(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		h.logger.Error("get book with metadata", "book_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Fetch authors.
	authors, err := q.ListBookAuthors(ctx, id)
	if err != nil {
		h.logger.Warn("list book authors", "book_id", id, "error", err)
		authors = nil
	}

	// Fetch series.
	seriesRows, err := q.ListBookSeries(ctx, id)
	if err != nil {
		h.logger.Warn("list book series", "book_id", id, "error", err)
		seriesRows = nil
	}

	// Fetch categories.
	categories, err := q.ListBookCategories(ctx, id)
	if err != nil {
		h.logger.Warn("list book categories", "book_id", id, "error", err)
		categories = nil
	}

	// Fetch tags.
	tags, err := q.ListBookTags(ctx, id)
	if err != nil {
		h.logger.Warn("list book tags", "book_id", id, "error", err)
		tags = nil
	}

	// Fetch files.
	files, err := q.ListBookFiles(ctx, id)
	if err != nil {
		h.logger.Warn("list book files", "book_id", id, "error", err)
		files = nil
	}

	// Assemble response.
	resp := BookDetailResponse{
		ID:         row.ID,
		LibraryID:  row.LibraryID,
		BookType:   row.BookType,
		Authors:    make([]AuthorResponse, 0, len(authors)),
		Series:     make([]SeriesResponse, 0, len(seriesRows)),
		Categories: make([]CategoryResponse, 0, len(categories)),
		Tags:       make([]TagResponse, 0, len(tags)),
		Files:      make([]BookFileResponse, 0, len(files)),
	}

	if row.FolderPath.Valid {
		resp.FolderPath = &row.FolderPath.String
	}
	if row.AddedDate.Valid {
		resp.AddedDate = &row.AddedDate.String
	}

	// Build metadata if any metadata field is present.
	meta := &BookMetadataResponse{}
	hasMetadata := false
	if row.Title.Valid {
		meta.Title = &row.Title.String
		hasMetadata = true
	}
	if row.Subtitle.Valid {
		meta.Subtitle = &row.Subtitle.String
		hasMetadata = true
	}
	if row.Description.Valid {
		meta.Description = &row.Description.String
		hasMetadata = true
	}
	if row.Publisher.Valid {
		meta.Publisher = &row.Publisher.String
		hasMetadata = true
	}
	if row.PublishDate.Valid {
		meta.PublishDate = &row.PublishDate.String
		hasMetadata = true
	}
	if row.PageCount.Valid {
		meta.PageCount = &row.PageCount.Int64
		hasMetadata = true
	}
	if row.Language.Valid {
		meta.Language = &row.Language.String
		hasMetadata = true
	}
	if row.Isbn10.Valid {
		meta.Isbn10 = &row.Isbn10.String
		hasMetadata = true
	}
	if row.Isbn13.Valid {
		meta.Isbn13 = &row.Isbn13.String
		hasMetadata = true
	}
	if row.CoverPath.Valid {
		meta.CoverPath = &row.CoverPath.String
		hasMetadata = true
	}
	if row.GoogleBooksID.Valid {
		meta.GoogleBooksID = &row.GoogleBooksID.String
		hasMetadata = true
	}
	if row.AmazonID.Valid {
		meta.AmazonID = &row.AmazonID.String
		hasMetadata = true
	}
	if row.GoodreadsID.Valid {
		meta.GoodreadsID = &row.GoodreadsID.String
		hasMetadata = true
	}
	if row.HardcoverID.Valid {
		meta.HardcoverID = &row.HardcoverID.String
		hasMetadata = true
	}
	if hasMetadata {
		resp.Metadata = meta
	}

	for _, a := range authors {
		resp.Authors = append(resp.Authors, AuthorResponse{ID: a.ID, Name: a.Name})
	}

	for _, s := range seriesRows {
		sr := SeriesResponse{ID: s.ID, Name: s.Name}
		if s.SeriesNumber.Valid {
			sr.SeriesNumber = &s.SeriesNumber.Float64
		}
		resp.Series = append(resp.Series, sr)
	}

	for _, c := range categories {
		resp.Categories = append(resp.Categories, CategoryResponse{ID: c.ID, Name: c.Name})
	}

	for _, t := range tags {
		resp.Tags = append(resp.Tags, TagResponse{ID: t.ID, Name: t.Name})
	}

	for _, f := range files {
		fr := BookFileResponse{
			ID:       f.ID,
			Format:   f.Format,
			FilePath: f.FilePath,
		}
		if f.FileSize.Valid {
			fr.FileSize = &f.FileSize.Int64
		}
		if f.TrackNumber.Valid {
			fr.TrackNumber = &f.TrackNumber.Int64
		}
		if f.TrackTitle.Valid {
			fr.TrackTitle = &f.TrackTitle.String
		}
		if f.DurationSecs.Valid {
			fr.DurationSecs = &f.DurationSecs.Int64
		}
		resp.Files = append(resp.Files, fr)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleDelete handles DELETE /api/books/{id} (admin only).
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	// Verify the book exists before deleting.
	_, err = q.GetBookByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		h.logger.Error("get book for delete", "book_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := q.DeleteBook(ctx, id); err != nil {
		h.logger.Error("delete book", "book_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		var userID int64
		if principal != nil {
			userID = principal.UserID
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:     &userID,
			Action:     audit.ActionBookDeleted,
			ResourceType: "book",
			ResourceID: &id,
			IPAddress:  ip,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListFiles handles GET /api/books/{id}/files.
func (h *Handler) handleListFiles(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	// Verify the book exists.
	_, err = q.GetBookByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		h.logger.Error("get book for files", "book_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	files, err := q.ListBookFiles(ctx, id)
	if err != nil {
		h.logger.Error("list book files", "book_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]BookFileResponse, 0, len(files))
	for _, f := range files {
		fr := BookFileResponse{
			ID:       f.ID,
			Format:   f.Format,
			FilePath: f.FilePath,
		}
		if f.FileSize.Valid {
			fr.FileSize = &f.FileSize.Int64
		}
		if f.TrackNumber.Valid {
			fr.TrackNumber = &f.TrackNumber.Int64
		}
		if f.TrackTitle.Valid {
			fr.TrackTitle = &f.TrackTitle.String
		}
		if f.DurationSecs.Valid {
			fr.DurationSecs = &f.DurationSecs.Int64
		}
		resp = append(resp, fr)
	}

	writeJSON(w, http.StatusOK, resp)
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
