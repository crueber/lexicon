package contentrestriction

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
)

// Handler handles HTTP requests for content restrictions.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler creates a new content restriction Handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// HandleList handles GET /api/users/me/content-restrictions.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	restrictions, err := h.service.ListRestrictions(r.Context(), principal.UserID)
	if err != nil {
		h.logger.Error("list restrictions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, restrictions)
}

// HandleCreate handles POST /api/users/me/content-restrictions.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		RestrictionType string `json:"restrictionType"`
		Value           string `json:"value"`
		Mode            string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RestrictionType == "" || req.Value == "" || req.Mode == "" {
		writeError(w, http.StatusBadRequest, "restrictionType, value, and mode are required")
		return
	}

	if req.Mode != ModeExclude && req.Mode != ModeAllowOnly {
		writeError(w, http.StatusBadRequest, "mode must be EXCLUDE or ALLOW_ONLY")
		return
	}

	validTypes := map[string]bool{
		TypeCategory:      true,
		TypeTag:           true,
		TypeMood:          true,
		TypeAgeRating:     true,
		TypeContentRating: true,
	}
	if !validTypes[req.RestrictionType] {
		writeError(w, http.StatusBadRequest, "invalid restriction type")
		return
	}

	if err := h.service.AddRestriction(r.Context(), principal.UserID, req.RestrictionType, req.Value, req.Mode); err != nil {
		h.logger.Error("add restriction", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// HandleDelete handles DELETE /api/users/me/content-restrictions/{id}.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid restriction id")
		return
	}

	if err := h.service.RemoveRestriction(r.Context(), principal.UserID, id); err != nil {
		h.logger.Error("remove restriction", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleUpdate handles PUT /api/users/me/content-restrictions/{id}.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid restriction id")
		return
	}

	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Mode != ModeExclude && req.Mode != ModeAllowOnly {
		writeError(w, http.StatusBadRequest, "mode must be EXCLUDE or ALLOW_ONLY")
		return
	}

	// Update the restriction mode.
	q := New(h.service.db)
	if err := q.UpdateRestrictionMode(r.Context(), UpdateRestrictionModeParams{
		ID:     id,
		UserID: principal.UserID,
		Mode:   req.Mode,
	}); err != nil {
		h.logger.Error("update restriction", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
