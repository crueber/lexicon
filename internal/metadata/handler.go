package metadata

import (
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

// Handler handles HTTP requests for metadata operations.
type Handler struct {
	svc      *Service
	logger   *slog.Logger
	auditSvc *audit.Service
}

// NewHandler creates a new metadata Handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{
		svc:    svc,
		logger: logger,
	}
}

// WithAuditService sets the audit service for logging metadata events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// Routes registers all metadata routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/search", h.handleSearch)
	r.Get("/proposals", h.handleListPendingProposals)
	r.Post("/books/{bookId}/proposals", h.handleCreateProposal)
	r.Get("/books/{bookId}/proposals", h.handleListProposals)
	r.Post("/proposals/{id}/accept", h.handleAcceptProposal)
	r.Post("/proposals/{id}/reject", h.handleRejectProposal)
	r.Put("/books/{bookId}/lock", h.handleToggleLock)
}

// AdminRoutes registers admin-only metadata routes.
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/settings/metadata", h.handleGetMetadataSettings)
	r.Put("/settings/metadata", h.handleSaveMetadataSettings)
}

// HandleMergeProposals handles POST /api/metadata/proposals/merge.
func (h *Handler) HandleMergeProposals(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() && !principal.Permissions.CanEditMetadata {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	var req struct {
		BookID      int64   `json:"bookId"`
		ProposalIDs []int64 `json:"proposalIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BookID <= 0 || len(req.ProposalIDs) == 0 {
		writeError(w, http.StatusBadRequest, "bookId and proposalIds are required")
		return
	}

	if err := h.svc.MergeProposals(r.Context(), req.BookID, req.ProposalIDs); err != nil {
		if errors.Is(err, ErrProposalNotFound) {
			writeError(w, http.StatusNotFound, "proposal not found")
			return
		}
		h.logger.Error("merge proposals", "book_id", req.BookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListProviderPriorities handles GET /api/metadata/provider-priorities.
func (h *Handler) HandleListProviderPriorities(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	priorities, err := h.svc.GetProviderPriorities(r.Context())
	if err != nil {
		h.logger.Error("list provider priorities", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	type providerPriorityResponse struct {
		Provider string `json:"provider"`
		Priority int64  `json:"priority"`
	}

	resp := make([]providerPriorityResponse, 0, len(priorities))
	for _, p := range priorities {
		resp = append(resp, providerPriorityResponse{
			Provider: p.ProviderName,
			Priority: p.Priority,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleSetProviderPriority handles PUT /api/metadata/provider-priorities/{provider}.
func (h *Handler) HandleSetProviderPriority(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	provider := chi.URLParam(r, "provider")
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	var req struct {
		Priority int `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Priority < 1 || req.Priority > 10 {
		writeError(w, http.StatusBadRequest, "priority must be between 1 and 10")
		return
	}

	if err := h.svc.SetProviderPriority(r.Context(), provider, req.Priority); err != nil {
		h.logger.Error("set provider priority", "provider", provider, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleSearch handles GET /api/metadata/search?title={}&author={}&isbn={}&bookType={}&libraryId={}.
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := Query{
		Title:    r.URL.Query().Get("title"),
		Author:   r.URL.Query().Get("author"),
		ISBN:     r.URL.Query().Get("isbn"),
		BookType: r.URL.Query().Get("bookType"),
	}

	if libraryIDStr := r.URL.Query().Get("libraryId"); libraryIDStr != "" {
		if libraryID, err := strconv.ParseInt(libraryIDStr, 10, 64); err == nil {
			query.LibraryID = libraryID
		}
	}

	if query.Title == "" && query.Author == "" && query.ISBN == "" {
		writeError(w, http.StatusBadRequest, "at least one of title, author, or isbn is required")
		return
	}

	results, err := h.svc.Search(r.Context(), query)
	if err != nil {
		h.logger.Error("metadata search", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// createProposalRequest is the request body for creating a proposal.
type createProposalRequest struct {
	Provider   string `json:"provider"`
	ProviderID string `json:"providerId"`
}

// handleCreateProposal handles POST /api/metadata/books/{bookId}/proposals.
func (h *Handler) handleCreateProposal(w http.ResponseWriter, r *http.Request) {
	bookID, err := parseBookID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	var req createProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Provider == "" || req.ProviderID == "" {
		writeError(w, http.StatusBadRequest, "provider and providerId are required")
		return
	}

	// Look up the provider.
	p, ok := h.svc.Provider(req.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown provider: %s", req.Provider))
		return
	}

	// Fetch full details from the provider.
	result, err := p.FetchByID(r.Context(), req.ProviderID)
	if err != nil {
		h.logger.Error("fetch by id", "provider", req.Provider, "id", req.ProviderID, "error", err)
		writeError(w, http.StatusBadGateway, "failed to fetch metadata from provider")
		return
	}

	// Save as proposal.
	proposalID, err := h.svc.CreateProposal(r.Context(), bookID, *result)
	if err != nil {
		h.logger.Error("create proposal", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"id": proposalID})
}

// handleListProposals handles GET /api/metadata/books/{bookId}/proposals.
func (h *Handler) handleListProposals(w http.ResponseWriter, r *http.Request) {
	bookID, err := parseBookID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	proposals, err := h.svc.ListProposals(r.Context(), bookID)
	if err != nil {
		h.logger.Error("list proposals", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, proposals)
}

// pendingProposalResponse is the JSON representation of a pending metadata proposal.
type pendingProposalResponse struct {
	ID         int64  `json:"id"`
	BookID     int64  `json:"bookId"`
	BookTitle  string `json:"bookTitle"`
	Provider   string `json:"provider"`
	ProviderID string `json:"providerId"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
}

// handleListPendingProposals handles GET /api/metadata/proposals.
func (h *Handler) handleListPendingProposals(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() && !principal.Permissions.CanEditMetadata {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	q := New(h.svc.db)
	rows, err := q.ListPendingProposals(r.Context())
	if err != nil {
		h.logger.Error("list pending proposals", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]pendingProposalResponse, 0, len(rows))
	for _, row := range rows {
		p := pendingProposalResponse{
			ID:        row.ID,
			BookID:    row.BookID,
			Provider:  row.Provider,
			Status:    row.Status,
			CreatedAt: row.CreatedAt,
		}
		if row.BookTitle.Valid {
			p.BookTitle = row.BookTitle.String
		}
		if row.ProviderID.Valid {
			p.ProviderID = row.ProviderID.String
		}
		resp = append(resp, p)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleAcceptProposal handles POST /api/metadata/proposals/{id}/accept.
func (h *Handler) handleAcceptProposal(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() && !principal.Permissions.CanEditMetadata {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	proposalID, err := parseProposalID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid proposal id")
		return
	}

	if err := h.svc.AcceptProposal(r.Context(), proposalID); err != nil {
		if errors.Is(err, ErrProposalNotFound) {
			writeError(w, http.StatusNotFound, "proposal not found")
			return
		}
		h.logger.Error("accept proposal", "proposal_id", proposalID, "error", err)
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
			UserID:       &userID,
			Action:       audit.ActionBookMetadataUpdated,
			ResourceType: "proposal",
			ResourceID:   &proposalID,
			IPAddress:    ip,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleRejectProposal handles POST /api/metadata/proposals/{id}/reject.
func (h *Handler) handleRejectProposal(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() && !principal.Permissions.CanEditMetadata {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	proposalID, err := parseProposalID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid proposal id")
		return
	}

	if err := h.svc.RejectProposal(r.Context(), proposalID); err != nil {
		if errors.Is(err, ErrProposalNotFound) {
			writeError(w, http.StatusNotFound, "proposal not found")
			return
		}
		h.logger.Error("reject proposal", "proposal_id", proposalID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// lockRequest is the request body for toggling a field lock.
type lockRequest struct {
	Field  string `json:"field"`
	Locked bool   `json:"locked"`
}

// validLockFields is the set of fields that can be locked.
var validLockFields = map[string]string{
	"title":       "title_locked",
	"subtitle":    "subtitle_locked",
	"description": "description_locked",
	"publisher":   "publisher_locked",
	"publishDate": "publish_date_locked",
	"pageCount":   "page_count_locked",
	"language":    "language_locked",
	"isbn10":      "isbn_10_locked",
	"isbn13":      "isbn_13_locked",
	"cover":       "cover_locked",
}

// handleToggleLock handles PUT /api/metadata/books/{bookId}/lock.
func (h *Handler) handleToggleLock(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() && !principal.Permissions.CanEditMetadata {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	bookID, err := parseBookID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	var req lockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	col, ok := validLockFields[req.Field]
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid field: %s", req.Field))
		return
	}

	lockedVal := 0
	if req.Locked {
		lockedVal = 1
	}

	// Use a safe column name from the allowlist (not user input).
	query := fmt.Sprintf("UPDATE book_metadata SET %s = ? WHERE book_id = ?", col)
	if _, err := h.svc.db.ExecContext(r.Context(), query, lockedVal, bookID); err != nil {
		h.logger.Error("toggle field lock", "book_id", bookID, "field", req.Field, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// metadataSettingsResponse is the JSON response for metadata settings.
type metadataSettingsResponse struct {
	GoogleBooksAPIKey string `json:"googleBooksApiKey"`
}

// handleGetMetadataSettings handles GET /api/admin/settings/metadata.
func (h *Handler) handleGetMetadataSettings(w http.ResponseWriter, r *http.Request) {
	apiKey, err := h.svc.GetAppSetting(r.Context(), "google_books_api_key")
	if err != nil {
		h.logger.Error("get metadata settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Mask the API key for display.
	maskedKey := ""
	if len(apiKey) > 4 {
		maskedKey = "****" + apiKey[len(apiKey)-4:]
	} else if apiKey != "" {
		maskedKey = "****"
	}

	writeJSON(w, http.StatusOK, metadataSettingsResponse{
		GoogleBooksAPIKey: maskedKey,
	})
}

// saveMetadataSettingsRequest is the request body for saving metadata settings.
type saveMetadataSettingsRequest struct {
	GoogleBooksAPIKey string `json:"googleBooksApiKey"`
}

// handleSaveMetadataSettings handles PUT /api/admin/settings/metadata.
func (h *Handler) handleSaveMetadataSettings(w http.ResponseWriter, r *http.Request) {
	var req saveMetadataSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.SetAppSetting(r.Context(), "google_books_api_key", req.GoogleBooksAPIKey); err != nil {
		h.logger.Error("save metadata settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Re-register the Google Books provider with the new API key.
	if p, ok := h.svc.providers["google_books"]; ok {
		if gbp, ok := p.(*GoogleBooksProvider); ok {
			gbp.apiKey = req.GoogleBooksAPIKey
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseBookID extracts and validates the bookId URL parameter.
func parseBookID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
}

// parseProposalID extracts and validates the id URL parameter.
func parseProposalID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
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
