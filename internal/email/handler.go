package email

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/auth"
)

// Handler handles HTTP requests for email send-to-device.
type Handler struct {
	svc      *Service
	logger   *slog.Logger
	auditSvc *audit.Service
}

// NewHandler creates a new email Handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// WithAuditService sets the audit service for logging email events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// ProviderRoutes registers provider management routes on the given router.
// RequireAuth and RequireAdmin must already be applied by the caller.
func (h *Handler) ProviderRoutes(r chi.Router) {
	r.Get("/", h.handleListProviders)
	r.Post("/", h.handleCreateProvider)
	r.Put("/{id}", h.handleUpdateProvider)
	r.Delete("/{id}", h.handleDeleteProvider)
	r.Post("/{id}/test", h.handleTestProvider)
}

// RecipientRoutes registers recipient management routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) RecipientRoutes(r chi.Router) {
	r.Get("/", h.handleListRecipients)
	r.Post("/", h.handleCreateRecipient)
	r.Delete("/{id}", h.handleDeleteRecipient)
}

// --- Provider handlers ---

// ProviderResponse is the JSON representation of an email provider.
type ProviderResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int64  `json:"port"`
	Username    string `json:"username"`
	FromAddress string `json:"fromAddress"`
	UseTLS      bool   `json:"useTls"`
	IsDefault   bool   `json:"isDefault"`
	CreatedAt   string `json:"createdAt"`
}

func toProviderResponse(p EmailProvider) ProviderResponse {
	return ProviderResponse{
		ID:          p.ID,
		Name:        p.Name,
		Host:        p.Host,
		Port:        p.Port,
		Username:    p.Username,
		FromAddress: p.FromAddress,
		UseTLS:      p.UseTls == 1,
		IsDefault:   p.IsDefault == 1,
		CreatedAt:   p.CreatedAt,
	}
}

func (h *Handler) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.svc.ListProviders(r.Context())
	if err != nil {
		h.logger.Error("list providers", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]ProviderResponse, len(providers))
	for i, p := range providers {
		resp[i] = toProviderResponse(p)
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateProviderRequest is the JSON body for POST /api/email/providers.
type CreateProviderRequest struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int64  `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromAddress string `json:"fromAddress"`
	UseTLS      bool   `json:"useTls"`
	IsDefault   bool   `json:"isDefault"`
}

func (h *Handler) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req CreateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Host == "" || req.Username == "" || req.Password == "" || req.FromAddress == "" {
		writeError(w, http.StatusBadRequest, "name, host, username, password, and fromAddress are required")
		return
	}
	if req.Port == 0 {
		req.Port = 587
	}

	provider, err := h.svc.CreateProvider(r.Context(), CreateProviderParams{
		Name:        req.Name,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		Password:    req.Password,
		FromAddress: req.FromAddress,
		UseTLS:      req.UseTLS,
		IsDefault:   req.IsDefault,
	})
	if err != nil {
		h.logger.Error("create provider", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toProviderResponse(provider))
}

// UpdateProviderRequest is the JSON body for PUT /api/email/providers/{id}.
type UpdateProviderRequest struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int64  `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromAddress string `json:"fromAddress"`
	UseTLS      bool   `json:"useTls"`
	IsDefault   bool   `json:"isDefault"`
}

func (h *Handler) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req UpdateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Host == "" || req.Username == "" || req.Password == "" || req.FromAddress == "" {
		writeError(w, http.StatusBadRequest, "name, host, username, password, and fromAddress are required")
		return
	}
	if req.Port == 0 {
		req.Port = 587
	}

	if err := h.svc.UpdateProvider(r.Context(), id, UpdateProviderParams{
		Name:        req.Name,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		Password:    req.Password,
		FromAddress: req.FromAddress,
		UseTLS:      req.UseTLS,
		IsDefault:   req.IsDefault,
	}); err != nil {
		if err.Error() == "provider not found" {
			writeError(w, http.StatusNotFound, "provider not found")
			return
		}
		h.logger.Error("update provider", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteProvider(r.Context(), id); err != nil {
		if err.Error() == "provider not found" {
			writeError(w, http.StatusNotFound, "provider not found")
			return
		}
		h.logger.Error("delete provider", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.TestProvider(r.Context(), id); err != nil {
		if err.Error() == "provider not found" {
			writeError(w, http.StatusNotFound, "provider not found")
			return
		}
		h.logger.Error("test provider", "provider_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send test email")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Recipient handlers ---

// RecipientResponse is the JSON representation of an email recipient.
type RecipientResponse struct {
	ID           int64   `json:"id"`
	Name         *string `json:"name"`
	EmailAddress string  `json:"emailAddress"`
	CreatedAt    string  `json:"createdAt"`
}

func toRecipientResponse(r EmailRecipient) RecipientResponse {
	resp := RecipientResponse{
		ID:           r.ID,
		EmailAddress: r.EmailAddress,
		CreatedAt:    r.CreatedAt,
	}
	if r.Name.Valid {
		resp.Name = &r.Name.String
	}
	return resp
}

func (h *Handler) handleListRecipients(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	recipients, err := h.svc.ListRecipients(r.Context(), principal.UserID)
	if err != nil {
		h.logger.Error("list recipients", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]RecipientResponse, len(recipients))
	for i, rec := range recipients {
		resp[i] = toRecipientResponse(rec)
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateRecipientRequest is the JSON body for POST /api/email/recipients.
type CreateRecipientRequest struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
}

func (h *Handler) handleCreateRecipient(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req CreateRecipientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.EmailAddress == "" {
		writeError(w, http.StatusBadRequest, "emailAddress is required")
		return
	}

	var namePtr *string
	if req.Name != "" {
		namePtr = &req.Name
	}

	recipient, err := h.svc.CreateRecipient(r.Context(), CreateRecipientParams{
		UserID:       principal.UserID,
		Name:         namePtr,
		EmailAddress: req.EmailAddress,
	})
	if err != nil {
		h.logger.Error("create recipient", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toRecipientResponse(recipient))
}

func (h *Handler) handleDeleteRecipient(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteRecipient(r.Context(), principal.UserID, id); err != nil {
		if err.Error() == "recipient not found" {
			writeError(w, http.StatusNotFound, "recipient not found")
			return
		}
		h.logger.Error("delete recipient", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Send book handler ---

// SendBookRequest is the JSON body for POST /api/books/{id}/send.
type SendBookRequest struct {
	RecipientIDs []int64 `json:"recipientIds"`
	ProviderID   int64   `json:"providerId"`
}

// HandleSendBook handles POST /api/books/{id}/send.
func (h *Handler) HandleSendBook(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req SendBookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.RecipientIDs) == 0 {
		writeError(w, http.StatusBadRequest, "recipientIds is required")
		return
	}

	if err := h.svc.SendBook(r.Context(), bookID, req.RecipientIDs, req.ProviderID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "book or provider not found")
			return
		}
		if err.Error() == "book file not found" || err.Error() == "email provider not found" || err.Error() == "no default email provider configured" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.HasPrefix(err.Error(), "recipient") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error("send book", "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send book")
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
			Action:       audit.ActionBookSent,
			ResourceType: "book",
			ResourceID:   &bookID,
			IPAddress:    ip,
			Details: map[string]any{
				"recipient_ids": req.RecipientIDs,
				"provider_id":   req.ProviderID,
			},
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

func parseID(w http.ResponseWriter, r *http.Request, param string) (int64, bool) {
	raw := chi.URLParam(r, param)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+param)
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
