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
	"github.com/crueber/lexicon/internal/contentrestriction"
)

// shelfHandler is the interface for the shelf handler's book-shelves endpoint.
// Defined here at the consumer to keep the interface small.
type shelfHandler interface {
	HandleListShelvesForBook(w http.ResponseWriter, r *http.Request)
}

// Handler handles HTTP requests for book management.
type Handler struct {
	db                    *sql.DB
	logger                *slog.Logger
	shelfHandler          shelfHandler
	auditSvc              *audit.Service
	contentRestrictionSvc *contentrestriction.Service
	broadcastBookDeleted  func(bookID int64)
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

// WithContentRestrictionService sets the content restriction service for filtering book listings.
func (h *Handler) WithContentRestrictionService(svc *contentrestriction.Service) {
	h.contentRestrictionSvc = svc
}

// WithBroadcastBookDeletedFunc sets the function called after a book is successfully deleted.
func (h *Handler) WithBroadcastBookDeletedFunc(fn func(bookID int64)) {
	h.broadcastBookDeleted = fn
}

// Routes registers all book routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.handleList)
	r.Get("/duplicates", h.handleListDuplicates)
	r.Post("/duplicates/dismiss", h.handleDismissDuplicate)
	r.Post("/merge", h.handleMergeBooks)
	r.Get("/{id}", h.handleGet)
	r.Delete("/{id}", h.handleDelete)
	r.Get("/{id}/files", h.handleListFiles)
	r.Get("/{id}/shelves", h.handleListShelves)
}

// AuthorRoutes registers author routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) AuthorRoutes(r chi.Router) {
	r.Get("/", h.handleListAuthors)
	r.Get("/{id}", h.handleGetAuthor)
	r.Get("/{id}/books", h.handleListBooksByAuthor)
}

// SeriesRoutes registers series routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) SeriesRoutes(r chi.Router) {
	r.Get("/", h.handleListSeries)
	r.Get("/{id}", h.handleGetSeries)
	r.Get("/{id}/books", h.handleListBooksBySeries)
}

// TaxonomyRoutes registers taxonomy routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) TaxonomyRoutes(r chi.Router) {
	r.Get("/categories", h.handleListCategories)
	r.Get("/tags", h.handleListTags)
	r.Get("/moods", h.handleListMoods)
}

// handleListShelves handles GET /api/books/{id}/shelves.
func (h *Handler) handleListShelves(w http.ResponseWriter, r *http.Request) {
	if h.shelfHandler == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}
	h.shelfHandler.HandleListShelvesForBook(w, r)
}

// handleListDuplicates handles GET /api/books/duplicates?preset={preset}.
func (h *Handler) handleListDuplicates(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	preset := DuplicatePreset(r.URL.Query().Get("preset"))
	if preset == "" {
		preset = PresetModerate
	}
	switch preset {
	case PresetStrict, PresetModerate, PresetLoose, PresetTitleOnly:
		// valid
	default:
		preset = PresetModerate
	}

	groups, err := FindDuplicates(r.Context(), h.db, preset)
	if err != nil {
		h.logger.Error("find duplicates", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, groups)
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

	// Apply content restrictions.
	if h.contentRestrictionSvc != nil && principal != nil {
		bookIDs := make([]int64, len(bookRows))
		for i, b := range bookRows {
			bookIDs[i] = b.ID
		}
		filteredIDs, filterErr := h.contentRestrictionSvc.FilterBookIDs(r.Context(), principal.UserID, principal.IsAdmin(), bookIDs)
		if filterErr != nil {
			h.logger.Error("filter book ids", "error", filterErr)
			// Non-fatal: continue without filtering.
		} else {
			idSet := make(map[int64]struct{}, len(filteredIDs))
			for _, id := range filteredIDs {
				idSet[id] = struct{}{}
			}
			var filtered []ListBooksWithMetadataRow
			for _, b := range bookRows {
				if _, ok := idSet[b.ID]; ok {
					filtered = append(filtered, b)
				}
			}
			bookRows = filtered
			// total remains the unfiltered SQL count; pagination totals may be slightly off for restricted users.
		}
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

	// Apply content restrictions.
	if h.contentRestrictionSvc != nil && principal != nil && !principal.IsAdmin() {
		filteredIDs, filterErr := h.contentRestrictionSvc.FilterBookIDs(ctx, principal.UserID, principal.IsAdmin(), []int64{id})
		if filterErr != nil {
			h.logger.Error("filter book id", "book_id", id, "error", filterErr)
			// Non-fatal: continue without filtering.
		} else if len(filteredIDs) == 0 {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
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

	if h.broadcastBookDeleted != nil {
		h.broadcastBookDeleted(id)
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

// dismissDuplicateRequest is the JSON body for POST /api/books/duplicates/dismiss.
type dismissDuplicateRequest struct {
	BookIDA int64 `json:"bookIdA"`
	BookIDB int64 `json:"bookIdB"`
}

// handleDismissDuplicate handles POST /api/books/duplicates/dismiss.
func (h *Handler) handleDismissDuplicate(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	var req dismissDuplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	q := New(h.db)
	if err := q.DismissDuplicate(r.Context(), DismissDuplicateParams{
		BookIDA: req.BookIDA,
		BookIDB: req.BookIDB,
		DismissedBy: sql.NullInt64{Int64: principal.UserID, Valid: true},
	}); err != nil {
		h.logger.Error("dismiss duplicate", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// mergeBooksRequest is the JSON body for POST /api/books/merge.
type mergeBooksRequest struct {
	SourceID int64 `json:"sourceId"`
	TargetID int64 `json:"targetId"`
}

// handleMergeBooks handles POST /api/books/merge (admin only).
func (h *Handler) handleMergeBooks(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	var req mergeBooksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SourceID == 0 || req.TargetID == 0 || req.SourceID == req.TargetID {
		writeError(w, http.StatusBadRequest, "sourceId and targetId must be different and non-zero")
		return
	}

	ctx := r.Context()

	// Verify both books exist.
	q := New(h.db)
	if _, err := q.GetBookByID(ctx, req.SourceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "source book not found")
			return
		}
		h.logger.Error("get source book", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := q.GetBookByID(ctx, req.TargetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "target book not found")
			return
		}
		h.logger.Error("get target book", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		h.logger.Error("begin merge transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback()

	tq := New(tx)

	// Move book files.
	if _, err := tx.ExecContext(ctx, "UPDATE book_file SET book_id = ? WHERE book_id = ?", req.TargetID, req.SourceID); err != nil {
		h.logger.Error("merge book files", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Move book authors.
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO book_author (book_id, author_id, sort_order) SELECT ?, author_id, sort_order FROM book_author WHERE book_id = ?", req.TargetID, req.SourceID); err != nil {
		h.logger.Error("merge book authors", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_author WHERE book_id = ?", req.SourceID); err != nil {
		h.logger.Error("delete source book authors", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Move book series.
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO book_series (book_id, series_id, series_number) SELECT ?, series_id, series_number FROM book_series WHERE book_id = ?", req.TargetID, req.SourceID); err != nil {
		h.logger.Error("merge book series", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_series WHERE book_id = ?", req.SourceID); err != nil {
		h.logger.Error("delete source book series", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Move book categories.
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO book_category (book_id, category_id) SELECT ?, category_id FROM book_category WHERE book_id = ?", req.TargetID, req.SourceID); err != nil {
		h.logger.Error("merge book categories", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_category WHERE book_id = ?", req.SourceID); err != nil {
		h.logger.Error("delete source book categories", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Move book tags.
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO book_tag (book_id, tag_id) SELECT ?, tag_id FROM book_tag WHERE book_id = ?", req.TargetID, req.SourceID); err != nil {
		h.logger.Error("merge book tags", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_tag WHERE book_id = ?", req.SourceID); err != nil {
		h.logger.Error("delete source book tags", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Move book moods.
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO book_mood (book_id, mood_id) SELECT ?, mood_id FROM book_mood WHERE book_id = ?", req.TargetID, req.SourceID); err != nil {
		h.logger.Error("merge book moods", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_mood WHERE book_id = ?", req.SourceID); err != nil {
		h.logger.Error("delete source book moods", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Move annotations.
	if _, err := tx.ExecContext(ctx, "UPDATE annotation SET book_id = ? WHERE book_id = ?", req.TargetID, req.SourceID); err != nil {
		h.logger.Error("merge annotations", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Move reading sessions.
	if _, err := tx.ExecContext(ctx, "UPDATE reading_sessions SET book_id = ? WHERE book_id = ?", req.TargetID, req.SourceID); err != nil {
		h.logger.Error("merge reading sessions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Delete source book metadata.
	if _, err := tx.ExecContext(ctx, "DELETE FROM book_metadata WHERE book_id = ?", req.SourceID); err != nil {
		h.logger.Error("delete source book metadata", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Delete source book.
	if err := tq.DeleteBook(ctx, req.SourceID); err != nil {
		h.logger.Error("delete source book", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(); err != nil {
		h.logger.Error("commit merge transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:       &principal.UserID,
			Action:       audit.ActionBookDeleted,
			ResourceType: "book",
			ResourceID:   &req.SourceID,
			Details: map[string]any{
				"merged_into": req.TargetID,
			},
			IPAddress: ip,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Authors ---

// AuthorListResponse is the JSON representation of an author with book count.
type AuthorListResponse struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	BookCount  int64  `json:"bookCount"`
}

// handleListAuthors handles GET /api/authors.
func (h *Handler) handleListAuthors(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	q := New(h.db)
	authors, err := q.ListAuthors(r.Context())
	if err != nil {
		h.logger.Error("list authors", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]AuthorListResponse, 0, len(authors))
	for _, a := range authors {
		resp = append(resp, AuthorListResponse{
			ID:        a.ID,
			Name:      a.Name,
			BookCount: a.BookCount,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetAuthor handles GET /api/authors/{id}.
func (h *Handler) handleGetAuthor(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid author id")
		return
	}

	q := New(h.db)
	author, err := q.GetAuthorByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "author not found")
			return
		}
		h.logger.Error("get author", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, AuthorResponse{ID: author.ID, Name: author.Name})
}

// handleListBooksByAuthor handles GET /api/authors/{id}/books.
func (h *Handler) handleListBooksByAuthor(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	authorID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid author id")
		return
	}

	q := New(h.db)
	ctx := r.Context()
	rows, err := q.ListBooksByAuthor(ctx, authorID)
	if err != nil {
		h.logger.Error("list books by author", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	books := make([]BookResponse, 0, len(rows))
	for _, row := range rows {
		authors, authErr := q.ListBookAuthors(ctx, row.ID)
		if authErr != nil {
			h.logger.Warn("list book authors", "book_id", row.ID, "error", authErr)
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

	writeJSON(w, http.StatusOK, books)
}

// --- Series ---

// SeriesListResponse is the JSON representation of a series with book count.
type SeriesListResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int64  `json:"bookCount"`
}

// handleListSeries handles GET /api/series.
func (h *Handler) handleListSeries(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	q := New(h.db)
	series, err := q.ListSeries(r.Context())
	if err != nil {
		h.logger.Error("list series", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]SeriesListResponse, 0, len(series))
	for _, s := range series {
		resp = append(resp, SeriesListResponse{
			ID:        s.ID,
			Name:      s.Name,
			BookCount: s.BookCount,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetSeries handles GET /api/series/{id}.
func (h *Handler) handleGetSeries(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid series id")
		return
	}

	q := New(h.db)
	s, err := q.GetSeriesByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "series not found")
			return
		}
		h.logger.Error("get series", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, SeriesResponse{ID: s.ID, Name: s.Name})
}

// handleListBooksBySeries handles GET /api/series/{id}/books.
func (h *Handler) handleListBooksBySeries(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	seriesID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid series id")
		return
	}

	q := New(h.db)
	ctx := r.Context()
	rows, err := q.ListBooksBySeries(ctx, seriesID)
	if err != nil {
		h.logger.Error("list books by series", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	books := make([]BookResponse, 0, len(rows))
	for _, row := range rows {
		authors, authErr := q.ListBookAuthors(ctx, row.ID)
		if authErr != nil {
			h.logger.Warn("list book authors", "book_id", row.ID, "error", authErr)
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

	writeJSON(w, http.StatusOK, books)
}

// --- Taxonomy ---

// CategoryListResponse is the JSON representation of a category with book count.
type CategoryListResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int64  `json:"bookCount"`
}

// TagListResponse is the JSON representation of a tag with book count.
type TagListResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int64  `json:"bookCount"`
}

// MoodListResponse is the JSON representation of a mood with book count.
type MoodListResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int64  `json:"bookCount"`
}

// handleListCategories handles GET /api/categories.
func (h *Handler) handleListCategories(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	q := New(h.db)
	categories, err := q.ListCategories(r.Context())
	if err != nil {
		h.logger.Error("list categories", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]CategoryListResponse, 0, len(categories))
	for _, c := range categories {
		resp = append(resp, CategoryListResponse{
			ID:        c.ID,
			Name:      c.Name,
			BookCount: c.BookCount,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleListTags handles GET /api/tags.
func (h *Handler) handleListTags(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	q := New(h.db)
	tags, err := q.ListTags(r.Context())
	if err != nil {
		h.logger.Error("list tags", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]TagListResponse, 0, len(tags))
	for _, t := range tags {
		resp = append(resp, TagListResponse{
			ID:        t.ID,
			Name:      t.Name,
			BookCount: t.BookCount,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleListMoods handles GET /api/moods.
func (h *Handler) handleListMoods(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	q := New(h.db)
	moods, err := q.ListMoods(r.Context())
	if err != nil {
		h.logger.Error("list moods", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]MoodListResponse, 0, len(moods))
	for _, m := range moods {
		resp = append(resp, MoodListResponse{
			ID:        m.ID,
			Name:      m.Name,
			BookCount: m.BookCount,
		})
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
