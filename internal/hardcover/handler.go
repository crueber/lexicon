package hardcover

import (
	"encoding/json"
	"net/http"

	"github.com/crueber/lexicon/internal/auth"
)

// Handler handles HTTP requests for Hardcover sync.
type Handler struct {
	service *Service
}

// NewHandler creates a new Hardcover Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// HandleGetSettings handles GET /api/users/me/hardcover.
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	settings, err := h.service.GetSettings(r.Context(), principal.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if settings == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// HandleSaveSettings handles PUT /api/users/me/hardcover.
func (h *Handler) HandleSaveSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		APIKey  string `json:"apiKey"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.SaveSettings(r.Context(), principal.UserID, req.APIKey, req.Enabled); err != nil {
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
