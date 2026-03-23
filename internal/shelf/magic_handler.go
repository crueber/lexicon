package shelf

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
)

// MagicShelfResponse is the JSON representation of a magic shelf.
type MagicShelfResponse struct {
	ID          int64   `json:"id"`
	UserID      int64   `json:"userId"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	IconColor   *string `json:"iconColor"`
	Rules       string  `json:"rules"`
	SortField   string  `json:"sortField"`
	SortDir     string  `json:"sortDir"`
	LimitCount  *int64  `json:"limitCount"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// MagicShelfBookResponse is the JSON representation of a book in a magic shelf result.
type MagicShelfBookResponse struct {
	ID        int64   `json:"id"`
	LibraryID int64   `json:"libraryId"`
	BookType  string  `json:"bookType"`
	Title     *string `json:"title"`
	CoverPath *string `json:"coverPath"`
	AddedDate *string `json:"addedDate"`
}

func magicShelfToResponse(ms MagicShelf) MagicShelfResponse {
	resp := MagicShelfResponse{
		ID:        ms.ID,
		UserID:    ms.UserID,
		Name:      ms.Name,
		Rules:     ms.Rules,
		SortField: ms.SortField,
		SortDir:   ms.SortDir,
		CreatedAt: ms.CreatedAt,
		UpdatedAt: ms.UpdatedAt,
	}
	if ms.Description.Valid {
		resp.Description = &ms.Description.String
	}
	if ms.Icon.Valid {
		resp.Icon = &ms.Icon.String
	}
	if ms.IconColor.Valid {
		resp.IconColor = &ms.IconColor.String
	}
	if ms.LimitCount.Valid {
		resp.LimitCount = &ms.LimitCount.Int64
	}
	return resp
}

// MagicRoutes registers all magic shelf routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) MagicRoutes(r chi.Router) {
	r.Get("/", h.handleMagicList)
	r.Post("/", h.handleMagicCreate)
	r.Get("/{id}", h.handleMagicGet)
	r.Put("/{id}", h.handleMagicUpdate)
	r.Delete("/{id}", h.handleMagicDelete)
	r.Get("/{id}/books", h.handleMagicBooks)
	r.Get("/{id}/count", h.handleMagicCount)
}

// handleMagicList handles GET /api/magic-shelves.
func (h *Handler) handleMagicList(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	q := New(h.svc.db)
	shelves, err := q.ListMagicShelvesForUser(r.Context(), principal.UserID)
	if err != nil {
		h.logger.Error("list magic shelves", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]MagicShelfResponse, 0, len(shelves))
	for _, ms := range shelves {
		resp = append(resp, magicShelfToResponse(ms))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createMagicShelfRequest is the request body for creating a magic shelf.
type createMagicShelfRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IconColor   string `json:"iconColor"`
	Rules       string `json:"rules"`
	SortField   string `json:"sortField"`
	SortDir     string `json:"sortDir"`
	LimitCount  *int64 `json:"limitCount"`
}

// handleMagicCreate handles POST /api/magic-shelves.
func (h *Handler) handleMagicCreate(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req createMagicShelfRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Default rules to empty group if not provided.
	if req.Rules == "" {
		req.Rules = `{"operator":"AND","rules":[]}`
	}

	// Default sort settings.
	if req.SortField == "" {
		req.SortField = "added_date"
	}
	if req.SortDir == "" {
		req.SortDir = "DESC"
	}

	q := New(h.svc.db)
	ms, err := q.CreateMagicShelf(r.Context(), CreateMagicShelfParams{
		UserID: principal.UserID,
		Name:   req.Name,
		Description: sql.NullString{
			String: req.Description,
			Valid:  req.Description != "",
		},
		Icon: sql.NullString{
			String: req.Icon,
			Valid:  req.Icon != "",
		},
		IconColor: sql.NullString{
			String: req.IconColor,
			Valid:  req.IconColor != "",
		},
		Rules:     req.Rules,
		SortField: req.SortField,
		SortDir:   req.SortDir,
		LimitCount: sql.NullInt64{
			Int64: func() int64 {
				if req.LimitCount != nil {
					return *req.LimitCount
				}
				return 0
			}(),
			Valid: req.LimitCount != nil,
		},
	})
	if err != nil {
		h.logger.Error("create magic shelf", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, magicShelfToResponse(ms))
}

// handleMagicGet handles GET /api/magic-shelves/{id}.
func (h *Handler) handleMagicGet(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid magic shelf id")
		return
	}

	q := New(h.svc.db)
	ms, err := q.GetMagicShelfByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "magic shelf not found")
			return
		}
		h.logger.Error("get magic shelf", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if ms.UserID != principal.UserID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	writeJSON(w, http.StatusOK, magicShelfToResponse(ms))
}

// updateMagicShelfRequest is the request body for updating a magic shelf.
type updateMagicShelfRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IconColor   string `json:"iconColor"`
	Rules       string `json:"rules"`
	SortField   string `json:"sortField"`
	SortDir     string `json:"sortDir"`
	LimitCount  *int64 `json:"limitCount"`
}

// handleMagicUpdate handles PUT /api/magic-shelves/{id}.
func (h *Handler) handleMagicUpdate(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid magic shelf id")
		return
	}

	var req updateMagicShelfRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Default rules to empty group if not provided.
	if req.Rules == "" {
		req.Rules = `{"operator":"AND","rules":[]}`
	}

	// Default sort settings.
	if req.SortField == "" {
		req.SortField = "added_date"
	}
	if req.SortDir == "" {
		req.SortDir = "DESC"
	}

	q := New(h.svc.db)

	// Verify ownership.
	ms, err := q.GetMagicShelfByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "magic shelf not found")
			return
		}
		h.logger.Error("get magic shelf for update", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ms.UserID != principal.UserID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	if err := q.UpdateMagicShelf(r.Context(), UpdateMagicShelfParams{
		Name: req.Name,
		Description: sql.NullString{
			String: req.Description,
			Valid:  req.Description != "",
		},
		Icon: sql.NullString{
			String: req.Icon,
			Valid:  req.Icon != "",
		},
		IconColor: sql.NullString{
			String: req.IconColor,
			Valid:  req.IconColor != "",
		},
		Rules:     req.Rules,
		SortField: req.SortField,
		SortDir:   req.SortDir,
		LimitCount: sql.NullInt64{
			Int64: func() int64 {
				if req.LimitCount != nil {
					return *req.LimitCount
				}
				return 0
			}(),
			Valid: req.LimitCount != nil,
		},
		ID:     id,
		UserID: principal.UserID,
	}); err != nil {
		h.logger.Error("update magic shelf", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleMagicDelete handles DELETE /api/magic-shelves/{id}.
func (h *Handler) handleMagicDelete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid magic shelf id")
		return
	}

	q := New(h.svc.db)

	// Verify ownership.
	ms, err := q.GetMagicShelfByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "magic shelf not found")
			return
		}
		h.logger.Error("get magic shelf for delete", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ms.UserID != principal.UserID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	if err := q.DeleteMagicShelf(r.Context(), DeleteMagicShelfParams{
		ID:     id,
		UserID: principal.UserID,
	}); err != nil {
		h.logger.Error("delete magic shelf", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleMagicBooks handles GET /api/magic-shelves/{id}/books.
func (h *Handler) handleMagicBooks(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid magic shelf id")
		return
	}

	q := New(h.svc.db)
	ms, err := q.GetMagicShelfByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "magic shelf not found")
			return
		}
		h.logger.Error("get magic shelf for books", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ms.UserID != principal.UserID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	// Get the user's accessible library IDs.
	libraryIDs, err := h.svc.getUserLibraryIDs(r.Context(), principal)
	if err != nil {
		h.logger.Error("get user library ids", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	books, err := EvaluateMagicShelf(r.Context(), h.svc.db, ms, libraryIDs)
	if err != nil {
		h.logger.Error("evaluate magic shelf", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]MagicShelfBookResponse, 0, len(books))
	for _, b := range books {
		br := MagicShelfBookResponse{
			ID:        b.ID,
			LibraryID: b.LibraryID,
			BookType:  b.BookType,
		}
		if b.Title.Valid {
			br.Title = &b.Title.String
		}
		if b.CoverPath.Valid {
			br.CoverPath = &b.CoverPath.String
		}
		if b.AddedDate.Valid {
			br.AddedDate = &b.AddedDate.String
		}
		resp = append(resp, br)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleMagicCount handles GET /api/magic-shelves/{id}/count.
func (h *Handler) handleMagicCount(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid magic shelf id")
		return
	}

	q := New(h.svc.db)
	ms, err := q.GetMagicShelfByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "magic shelf not found")
			return
		}
		h.logger.Error("get magic shelf for count", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ms.UserID != principal.UserID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	// Get the user's accessible library IDs.
	libraryIDs, err := h.svc.getUserLibraryIDs(r.Context(), principal)
	if err != nil {
		h.logger.Error("get user library ids for count", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	count, err := CountMagicShelf(r.Context(), h.svc.db, ms, libraryIDs)
	if err != nil {
		h.logger.Error("count magic shelf", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}
