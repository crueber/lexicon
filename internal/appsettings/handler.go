// Package appsettings provides admin runtime settings management.
package appsettings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/auth"
)

// Service provides runtime app settings operations.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewService creates a new appsettings Service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// ListSettings returns all app_settings as a map.
func (s *Service) ListSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM app_settings ORDER BY key")
	if err != nil {
		return nil, fmt.Errorf("list app settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan app setting: %w", err)
		}
		settings[key] = value
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return settings, nil
}

// SaveSettings upserts multiple app settings.
func (s *Service) SaveSettings(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		_, err := s.db.ExecContext(ctx,
			"INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
			key, value,
		)
		if err != nil {
			return fmt.Errorf("save app setting %q: %w", key, err)
		}
	}
	return nil
}

// Handler handles HTTP requests for runtime app settings.
type Handler struct {
	svc      *Service
	logger   *slog.Logger
	auditSvc *audit.Service
}

// NewHandler creates a new appsettings Handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{
		svc:    svc,
		logger: logger,
	}
}

// WithAuditService sets the audit service for logging admin events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// HandleGetSettings handles GET /api/admin/settings.
func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	settings, err := h.svc.ListSettings(r.Context())
	if err != nil {
		h.logger.Error("list settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// HandleSaveSettings handles PUT /api/admin/settings.
func (h *Handler) HandleSaveSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.SaveSettings(r.Context(), req); err != nil {
		h.logger.Error("save settings", "error", err)
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
			Action:       audit.ActionAdminAction,
			ResourceType: "app_settings",
			IPAddress:    ip,
		})
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
