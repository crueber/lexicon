package shelf

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
)

// Handler handles HTTP requests for shelf management.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler creates a new shelf Handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{
		svc:    svc,
		logger: logger,
	}
}

// Routes registers all shelf routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.handleList)
	r.Post("/", h.handleCreate)
	r.Get("/{id}", h.handleGet)
	r.Put("/{id}", h.handleUpdate)
	r.Delete("/{id}", h.handleDelete)
	r.Get("/{id}/books", h.handleListBooks)
	r.Post("/{id}/books", h.handleAddBook)
	r.Delete("/{id}/books/{bookId}", h.handleRemoveBook)
}

// ShelfResponse is the JSON representation of a shelf.
type ShelfResponse struct {
	ID          int64   `json:"id"`
	UserID      int64   `json:"userId"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	IconColor   *string `json:"iconColor"`
	IsPublic    bool    `json:"isPublic"`
	BookCount   *int64  `json:"bookCount,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// ShelfBookResponse is the JSON representation of a book in a shelf.
type ShelfBookResponse struct {
	ID        int64   `json:"id"`
	LibraryID int64   `json:"libraryId"`
	BookType  string  `json:"bookType"`
	Title     *string `json:"title"`
	CoverPath *string `json:"coverPath"`
	AddedAt   string  `json:"addedAt"`
	SortOrder int64   `json:"sortOrder"`
}

func shelfToResponse(sh Shelf) ShelfResponse {
	resp := ShelfResponse{
		ID:        sh.ID,
		UserID:    sh.UserID,
		Name:      sh.Name,
		IsPublic:  sh.IsPublic != 0,
		CreatedAt: sh.CreatedAt,
		UpdatedAt: sh.UpdatedAt,
	}
	if sh.Description.Valid {
		resp.Description = &sh.Description.String
	}
	if sh.Icon.Valid {
		resp.Icon = &sh.Icon.String
	}
	if sh.IconColor.Valid {
		resp.IconColor = &sh.IconColor.String
	}
	return resp
}

// handleList handles GET /api/shelves — list user's shelves with book counts.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	shelves, err := h.svc.ListForUser(r.Context(), principal.UserID)
	if err != nil {
		h.logger.Error("list shelves for user", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]ShelfResponse, 0, len(shelves))
	for _, s := range shelves {
		sr := shelfToResponse(s.Shelf)
		count := s.BookCount
		sr.BookCount = &count
		resp = append(resp, sr)
	}

	writeJSON(w, http.StatusOK, resp)
}

// createShelfRequest is the request body for creating a shelf.
type createShelfRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IconColor   string `json:"iconColor"`
	IsPublic    bool   `json:"isPublic"`
}

// handleCreate handles POST /api/shelves.
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req createShelfRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	sh, err := h.svc.Create(r.Context(), principal.UserID, CreateParams{
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		IconColor:   req.IconColor,
		IsPublic:    req.IsPublic,
	})
	if err != nil {
		h.logger.Error("create shelf", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, shelfToResponse(*sh))
}

// handleGet handles GET /api/shelves/{id}.
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}

	sh, err := h.svc.GetByID(r.Context(), id, principal.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shelf not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("get shelf", "shelf_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, shelfToResponse(*sh))
}

// updateShelfRequest is the request body for updating a shelf.
type updateShelfRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IconColor   string `json:"iconColor"`
	IsPublic    bool   `json:"isPublic"`
}

// handleUpdate handles PUT /api/shelves/{id}.
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}

	var req updateShelfRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := h.svc.Update(r.Context(), id, principal.UserID, UpdateParams{
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		IconColor:   req.IconColor,
		IsPublic:    req.IsPublic,
	}); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shelf not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("update shelf", "shelf_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDelete handles DELETE /api/shelves/{id}.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}

	if err := h.svc.Delete(r.Context(), id, principal.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shelf not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("delete shelf", "shelf_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListBooks handles GET /api/shelves/{id}/books.
func (h *Handler) handleListBooks(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}

	books, err := h.svc.ListBooks(r.Context(), id, principal.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shelf not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("list books in shelf", "shelf_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]ShelfBookResponse, 0, len(books))
	for _, b := range books {
		sbr := ShelfBookResponse{
			ID:        b.ID,
			LibraryID: b.LibraryID,
			BookType:  b.BookType,
			AddedAt:   b.AddedAt,
			SortOrder: b.SortOrder,
		}
		if b.Title.Valid {
			sbr.Title = &b.Title.String
		}
		if b.CoverPath.Valid {
			sbr.CoverPath = &b.CoverPath.String
		}
		resp = append(resp, sbr)
	}

	writeJSON(w, http.StatusOK, resp)
}

// addBookRequest is the request body for adding a book to a shelf.
type addBookRequest struct {
	BookID int64 `json:"bookId"`
}

// handleAddBook handles POST /api/shelves/{id}/books.
func (h *Handler) handleAddBook(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}

	var req addBookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BookID == 0 {
		writeError(w, http.StatusBadRequest, "bookId is required")
		return
	}

	if err := h.svc.AddBook(r.Context(), id, req.BookID, principal.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shelf not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("add book to shelf", "shelf_id", id, "book_id", req.BookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveBook handles DELETE /api/shelves/{id}/books/{bookId}.
func (h *Handler) handleRemoveBook(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}

	bookID, err := parseID(r, "bookId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	if err := h.svc.RemoveBook(r.Context(), id, bookID, principal.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shelf not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("remove book from shelf", "shelf_id", id, "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListShelvesForBook handles GET /api/books/{id}/shelves.
// This is mounted on the book handler but uses the shelf service.
func (h *Handler) HandleListShelvesForBook(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	shelves, err := h.svc.ListShelvesContainingBook(r.Context(), bookID, principal.UserID)
	if err != nil {
		h.logger.Error("list shelves containing book", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]ShelfResponse, 0, len(shelves))
	for _, s := range shelves {
		resp = append(resp, shelfToResponse(s))
	}

	writeJSON(w, http.StatusOK, resp)
}

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
