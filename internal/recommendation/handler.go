package recommendation

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/crueber/lexicon/internal/auth"
	"github.com/go-chi/chi/v5"
)

// Handler handles HTTP requests for book recommendations.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler creates a new recommendation Handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// HandleSimilarBooks handles GET /api/books/{id}/similar.
func (h *Handler) HandleSimilarBooks(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	bookID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
			if limit > 20 {
				limit = 20
			}
		}
	}

	var libraryIDs []int64
	if p := auth.PrincipalFromContext(r.Context()); p != nil {
		libraryIDs = p.LibraryIDs
	}

	books, err := h.service.FindSimilarBooks(r.Context(), bookID, libraryIDs, limit)
	if err != nil {
		h.logger.Error("find similar books", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, books)
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
