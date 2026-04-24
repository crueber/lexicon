package notebook

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
	"github.com/crueber/lexicon/internal/contentrestriction"
)

// Handler handles HTTP requests for the notebook (annotations) endpoints.
type Handler struct {
	db                    *sql.DB
	logger                *slog.Logger
	contentRestrictionSvc *contentrestriction.Service
}

// Compile-time interface check.
var _ http.Handler = (*Handler)(nil)

// NewHandler creates a new notebook Handler.
func NewHandler(db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		db:     db,
		logger: logger,
	}
}

// WithContentRestrictionService sets the content restriction service for filtering notebook annotations.
func (h *Handler) WithContentRestrictionService(svc *contentrestriction.Service) {
	h.contentRestrictionSvc = svc
}

// ServeHTTP implements http.Handler (required for compile-time check).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// ReaderRoutes registers annotation routes that live under /api/reader.
// RequireAuth must already be applied by the caller.
func (h *Handler) ReaderRoutes(r chi.Router) {
	r.Get("/books/{bookId}/annotations", h.handleListAnnotationsForBook)
	r.Post("/books/{bookId}/annotations", h.handleCreateAnnotation)
	r.Put("/books/{bookId}/annotations/{id}", h.handleUpdateAnnotation)
	r.Delete("/books/{bookId}/annotations/{id}", h.handleDeleteAnnotation)
}

// NotebookRoutes registers the notebook listing routes under /api/notebook.
// RequireAuth must already be applied by the caller.
func (h *Handler) NotebookRoutes(r chi.Router) {
	r.Get("/", h.handleListAllAnnotations)
	r.Get("/export", h.handleExportMarkdown)
}

// ---- Request / Response types ----

// annotationResponse is the JSON representation of an annotation.
type annotationResponse struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"userId"`
	BookID     int64   `json:"bookId"`
	BookFileID *int64  `json:"bookFileId"`
	Type       string  `json:"type"`
	CFI        *string `json:"cfi"`
	PageNumber *int64  `json:"pageNumber"`
	Text       *string `json:"text"`
	Note       *string `json:"note"`
	Color      string  `json:"color"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

// annotationWithBookResponse extends annotationResponse with book metadata.
type annotationWithBookResponse struct {
	annotationResponse
	BookTitle *string `json:"bookTitle"`
	CoverPath *string `json:"coverPath"`
}

// createAnnotationRequest is the JSON body for POST /api/reader/books/{bookId}/annotations.
type createAnnotationRequest struct {
	BookFileID *int64  `json:"bookFileId"`
	Type       string  `json:"type"`
	CFI        *string `json:"cfi"`
	PageNumber *int64  `json:"pageNumber"`
	Text       *string `json:"text"`
	Note       *string `json:"note"`
	Color      string  `json:"color"`
}

// updateAnnotationRequest is the JSON body for PUT /api/reader/books/{bookId}/annotations/{id}.
type updateAnnotationRequest struct {
	Note  *string `json:"note"`
	Color string  `json:"color"`
}

// ---- Converters ----

func annotationToResponse(a Annotation) annotationResponse {
	resp := annotationResponse{
		ID:        a.ID,
		UserID:    a.UserID,
		BookID:    a.BookID,
		Type:      a.Type,
		Color:     a.Color,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
	if a.BookFileID.Valid {
		v := a.BookFileID.Int64
		resp.BookFileID = &v
	}
	if a.Cfi.Valid {
		v := a.Cfi.String
		resp.CFI = &v
	}
	if a.PageNumber.Valid {
		v := a.PageNumber.Int64
		resp.PageNumber = &v
	}
	if a.Text.Valid {
		v := a.Text.String
		resp.Text = &v
	}
	if a.Note.Valid {
		v := a.Note.String
		resp.Note = &v
	}
	return resp
}

// ---- Handlers ----

// handleListAnnotationsForBook handles GET /api/reader/books/{bookId}/annotations.
func (h *Handler) handleListAnnotationsForBook(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, err := parseID(r, "bookId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := New(h.db)
	annotations, err := q.ListAnnotationsForBook(r.Context(), ListAnnotationsForBookParams{
		UserID: principal.UserID,
		BookID: bookID,
	})
	if err != nil {
		h.logger.Error("list annotations for book", "book_id", bookID, "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]annotationResponse, 0, len(annotations))
	for _, a := range annotations {
		resp = append(resp, annotationToResponse(a))
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCreateAnnotation handles POST /api/reader/books/{bookId}/annotations.
func (h *Handler) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, err := parseID(r, "bookId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	var req createAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	annotationType := req.Type
	if annotationType == "" {
		annotationType = "HIGHLIGHT"
	}
	color := req.Color
	if color == "" {
		color = "yellow"
	}

	params := CreateAnnotationParams{
		UserID: principal.UserID,
		BookID: bookID,
		Type:   annotationType,
		Color:  color,
	}
	if req.BookFileID != nil {
		params.BookFileID = sql.NullInt64{Int64: *req.BookFileID, Valid: true}
	}
	if req.CFI != nil {
		params.Cfi = sql.NullString{String: *req.CFI, Valid: true}
	}
	if req.PageNumber != nil {
		params.PageNumber = sql.NullInt64{Int64: *req.PageNumber, Valid: true}
	}
	if req.Text != nil {
		params.Text = sql.NullString{String: *req.Text, Valid: true}
	}
	if req.Note != nil {
		params.Note = sql.NullString{String: *req.Note, Valid: true}
	}

	q := New(h.db)
	annotation, err := q.CreateAnnotation(r.Context(), params)
	if err != nil {
		h.logger.Error("create annotation", "book_id", bookID, "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, annotationToResponse(annotation))
}

// handleUpdateAnnotation handles PUT /api/reader/books/{bookId}/annotations/{id}.
func (h *Handler) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	_, err := parseID(r, "bookId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	annotationID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid annotation id")
		return
	}

	var req updateAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	color := req.Color
	if color == "" {
		color = "yellow"
	}

	params := UpdateAnnotationParams{
		ID:     annotationID,
		UserID: principal.UserID,
		Color:  color,
	}
	if req.Note != nil {
		params.Note = sql.NullString{String: *req.Note, Valid: true}
	}

	q := New(h.db)
	if err := q.UpdateAnnotation(r.Context(), params); err != nil {
		h.logger.Error("update annotation", "annotation_id", annotationID, "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteAnnotation handles DELETE /api/reader/books/{bookId}/annotations/{id}.
func (h *Handler) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	_, err := parseID(r, "bookId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	annotationID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid annotation id")
		return
	}

	q := New(h.db)
	if err := q.DeleteAnnotation(r.Context(), DeleteAnnotationParams{
		ID:     annotationID,
		UserID: principal.UserID,
	}); err != nil {
		h.logger.Error("delete annotation", "annotation_id", annotationID, "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListAllAnnotations handles GET /api/notebook.
// Supports optional ?bookId= filter and ?page= / ?limit= pagination.
func (h *Handler) handleListAllAnnotations(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	// Optional bookId filter — if provided, list annotations for that book only.
	if bookIDStr := r.URL.Query().Get("bookId"); bookIDStr != "" {
		bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bookId")
			return
		}

		annotations, err := q.ListAnnotationsForBook(ctx, ListAnnotationsForBookParams{
			UserID: principal.UserID,
			BookID: bookID,
		})
		if err != nil {
			h.logger.Error("list annotations for book (notebook)", "book_id", bookID, "user_id", principal.UserID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// Apply content restrictions.
		if h.contentRestrictionSvc != nil && principal != nil && !principal.IsAdmin() {
			filteredIDs, filterErr := h.contentRestrictionSvc.FilterBookIDs(ctx, principal.UserID, principal.IsAdmin(), []int64{bookID})
			if filterErr != nil {
				h.logger.Error("filter notebook book", "book_id", bookID, "error", filterErr)
				// Non-fatal: continue without filtering.
			} else if len(filteredIDs) == 0 {
				writeJSON(w, http.StatusOK, []annotationResponse{})
				return
			}
		}

		resp := make([]annotationResponse, 0, len(annotations))
		for _, a := range annotations {
			resp = append(resp, annotationToResponse(a))
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Pagination.
	limit := int64(50)
	offset := int64(0)

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 1 {
			offset = (n - 1) * limit
		}
	}

	total, err := q.CountAnnotationsForUser(ctx, principal.UserID)
	if err != nil {
		h.logger.Error("count annotations for user", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := q.ListAllAnnotationsForUser(ctx, ListAllAnnotationsForUserParams{
		UserID: principal.UserID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		h.logger.Error("list all annotations for user", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	items := make([]annotationWithBookResponse, 0, len(rows))
	for _, row := range rows {
		base := annotationToResponse(Annotation{
			ID:         row.ID,
			UserID:     row.UserID,
			BookID:     row.BookID,
			BookFileID: row.BookFileID,
			Type:       row.Type,
			Cfi:        row.Cfi,
			PageNumber: row.PageNumber,
			Text:       row.Text,
			Note:       row.Note,
			Color:      row.Color,
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		})
		item := annotationWithBookResponse{annotationResponse: base}
		if row.BookTitle.Valid {
			v := row.BookTitle.String
			item.BookTitle = &v
		}
		if row.CoverPath.Valid {
			v := row.CoverPath.String
			item.CoverPath = &v
		}
		items = append(items, item)
	}

	// Apply content restrictions.
	if h.contentRestrictionSvc != nil && principal != nil && !principal.IsAdmin() && len(items) > 0 {
		bookIDs := make([]int64, len(items))
		for i, item := range items {
			bookIDs[i] = item.BookID
		}
		filteredIDs, filterErr := h.contentRestrictionSvc.FilterBookIDs(ctx, principal.UserID, principal.IsAdmin(), bookIDs)
		if filterErr != nil {
			h.logger.Error("filter notebook annotations", "error", filterErr)
			// Non-fatal: continue without filtering.
		} else {
			idSet := make(map[int64]struct{}, len(filteredIDs))
			for _, id := range filteredIDs {
				idSet[id] = struct{}{}
			}
			var filtered []annotationWithBookResponse
			for _, item := range items {
				if _, ok := idSet[item.BookID]; ok {
					filtered = append(filtered, item)
				}
			}
			items = filtered
			// total remains the unfiltered SQL count; pagination totals may be slightly off for restricted users.
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"annotations": items,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
	})
}

// handleExportMarkdown handles GET /api/notebook/export.
func (h *Handler) handleExportMarkdown(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	rows, err := q.ListAllAnnotationsForUserExport(ctx, principal.UserID)
	if err != nil {
		h.logger.Error("export markdown list annotations", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Apply content restrictions.
	if h.contentRestrictionSvc != nil && !principal.IsAdmin() && len(rows) > 0 {
		bookIDs := make([]int64, len(rows))
		for i, row := range rows {
			bookIDs[i] = row.BookID
		}
		filteredIDs, filterErr := h.contentRestrictionSvc.FilterBookIDs(ctx, principal.UserID, principal.IsAdmin(), bookIDs)
		if filterErr != nil {
			h.logger.Error("filter notebook export annotations", "error", filterErr)
			// Non-fatal: continue without filtering.
		} else {
			idSet := make(map[int64]struct{}, len(filteredIDs))
			for _, id := range filteredIDs {
				idSet[id] = struct{}{}
			}
			var filtered []ListAllAnnotationsForUserExportRow
			for _, row := range rows {
				if _, ok := idSet[row.BookID]; ok {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
		}
	}

	// Group by book title.
	type bookGroup struct {
		title       string
		annotations []ListAllAnnotationsForUserExportRow
	}
	var groups []bookGroup
	var current *bookGroup
	for _, row := range rows {
		title := row.BookTitle.String
		if title == "" {
			title = fmt.Sprintf("Book %d", row.BookID)
		}
		if current == nil || current.title != title {
			groups = append(groups, bookGroup{title: title, annotations: []ListAllAnnotationsForUserExportRow{row}})
			current = &groups[len(groups)-1]
		} else {
			current.annotations = append(current.annotations, row)
		}
	}

	var b strings.Builder
	b.WriteString("# Notebook Export\n\n")
	for _, g := range groups {
		b.WriteString(fmt.Sprintf("## %s\n\n", g.title))
		for _, a := range g.annotations {
			var ref string
			if a.PageNumber.Valid {
				ref = fmt.Sprintf("Page %d", a.PageNumber.Int64)
			} else if a.Cfi.Valid {
				ref = fmt.Sprintf("CFI: %s", a.Cfi.String)
			} else {
				ref = "Note"
			}

			var text string
			if a.Text.Valid && a.Text.String != "" {
				text = fmt.Sprintf(" \"%s\"", a.Text.String)
			}
			b.WriteString(fmt.Sprintf("- **%s** —%s (%s)\n", ref, text, a.Color))
			if a.Note.Valid && a.Note.String != "" {
				b.WriteString(fmt.Sprintf("  - Note: %s\n", a.Note.String))
			}
			b.WriteString("\n")
		}
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="notebook.md"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

// ---- Helpers ----

// parseID parses a URL parameter as an int64.
func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
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

// Compile-time check: *sql.DB satisfies the DBTX interface.
var _ DBTX = (*sql.DB)(nil)
