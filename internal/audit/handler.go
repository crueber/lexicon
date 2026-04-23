package audit

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// Handler handles HTTP requests for audit logs.
type Handler struct {
	db      *sql.DB
	service *Service
	logger  *slog.Logger
}

// NewHandler creates a new audit Handler.
func NewHandler(db *sql.DB, service *Service, logger *slog.Logger) *Handler {
	return &Handler{db: db, service: service, logger: logger}
}

// HandleListAuditLogs handles GET /api/admin/audit-logs.
// Query params: page, size, action, userId, from, to
func (h *Handler) HandleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	size := 50
	if s := r.URL.Query().Get("size"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 100 {
			size = v
		}
	}

	action := r.URL.Query().Get("action")
	userIDStr := r.URL.Query().Get("userId")
	fromDate := r.URL.Query().Get("from")
	toDate := r.URL.Query().Get("to")
	if toDate != "" {
		toDate = toDate + "T23:59:59"
	}

	q := New(h.db)
	ctx := r.Context()

	offset := (page - 1) * size

	userID := sql.NullInt64{Valid: false}
	coalesceUserID := int64(-1)
	if userIDStr != "" {
		if v, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = sql.NullInt64{Int64: v, Valid: true}
			coalesceUserID = v
		}
	}

	rows, err := q.ListAuditLogsFiltered(ctx, ListAuditLogsFilteredParams{
		Action:      action,
		Column1:     action,
		Column3:     coalesceUserID,
		UserID:      userID,
		Column5:     fromDate,
		CreatedAt:   fromDate,
		Column7:     toDate,
		CreatedAt_2: toDate,
		Limit:       int64(size),
		Offset:      int64(offset),
	})
	if err != nil {
		h.logger.Error("list audit logs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	total, err := q.CountAuditLogsFiltered(ctx, CountAuditLogsFilteredParams{
		Action:      action,
		Column1:     action,
		Column3:     coalesceUserID,
		UserID:      userID,
		Column5:     fromDate,
		CreatedAt:   fromDate,
		Column7:     toDate,
		CreatedAt_2: toDate,
	})
	if err != nil {
		h.logger.Error("count audit logs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := struct {
		Logs  []AuditLog `json:"logs"`
		Total int64      `json:"total"`
		Page  int        `json:"page"`
		Size  int        `json:"size"`
	}{
		Logs:  rows,
		Total: total,
		Page:  page,
		Size:  size,
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
