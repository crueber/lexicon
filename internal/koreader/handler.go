// Package koreader implements the KOSync protocol for KOReader device sync.
//
// KOReader uses a simple REST API with HTTP Basic Auth. Passwords are sent
// as MD5 hashes by the KOReader client. This package stores those hashes
// directly and compares them on authentication.
package koreader

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/book"
	"github.com/crueber/lexicon/internal/user"
)

// Handler handles KOSync protocol requests.
type Handler struct {
	db       *sql.DB
	logger   *slog.Logger
	auditSvc *audit.Service
}

// Compile-time interface check.
var _ http.Handler = (*Handler)(nil)

// NewHandler creates a new KOReader Handler.
func NewHandler(db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		db:     db,
		logger: logger,
	}
}

// WithAuditService sets the audit service for logging KOReader events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// ServeHTTP implements http.Handler (required for compile-time check).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// Routes registers all KOSync protocol routes.
// No JWT middleware is applied — KOSync uses HTTP Basic Auth with MD5 passwords.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/users/create", h.handleCreateUser)
	r.Get("/users/auth", h.handleAuth)
	r.Put("/syncs/progress", h.handleUpdateProgress)
	r.Get("/syncs/progress/{document}", h.handleGetProgress)
}

// --- KOSync API Endpoints ---

// createUserRequest is the JSON body for POST /kosync/users/create.
type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleCreateUser handles POST /kosync/users/create.
// Registers a new KOReader user. The password field is expected to already
// be MD5-hashed by the KOReader client.
func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	// Check if username already exists.
	_, err := q.GetKOReaderUserByUsername(ctx, req.Username)
	if err == nil {
		writeError(w, http.StatusConflict, "username already registered")
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		h.logger.Error("check koreader username", "username", req.Username, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Try to link to an existing Lexicon user with the same username.
	var linkedUserID sql.NullInt64
	uq := user.New(h.db)
	lexiconUser, err := uq.GetUserByUsername(ctx, req.Username)
	if err == nil {
		linkedUserID = sql.NullInt64{Int64: lexiconUser.ID, Valid: true}
	}
	// If no matching Lexicon user, linkedUserID remains null — that's fine.

	_, err = q.CreateKOReaderUser(ctx, CreateKOReaderUserParams{
		UserID:      linkedUserID,
		Username:    req.Username,
		PasswordMd5: req.Password,
	})
	if err != nil {
		h.logger.Error("create koreader user", "username", req.Username, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("koreader user created", "username", req.Username, "linked_user_id", linkedUserID)

	writeJSON(w, http.StatusCreated, map[string]string{"username": req.Username})
}

// handleAuth handles GET /kosync/users/auth.
// Validates HTTP Basic Auth credentials (username + MD5 password).
func (h *Handler) handleAuth(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authenticate(w, r); !ok {
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"authorized": "OK"})
}

// updateProgressRequest is the JSON body for PUT /kosync/syncs/progress.
type updateProgressRequest struct {
	Document   string  `json:"document"`
	Progress   string  `json:"progress"`
	Percentage float64 `json:"percentage"`
	Device     string  `json:"device"`
	DeviceID   string  `json:"device_id"`
}

// handleUpdateProgress handles PUT /kosync/syncs/progress.
// Upserts reading progress for the authenticated KOReader user.
func (h *Handler) handleUpdateProgress(w http.ResponseWriter, r *http.Request) {
	koUser, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	var req updateProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Document == "" {
		writeError(w, http.StatusBadRequest, "document is required")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	now := time.Now().Unix()

	progress, err := q.UpsertKOReaderProgress(ctx, UpsertKOReaderProgressParams{
		KoreaderUserID: koUser.ID,
		Document:       req.Document,
		Progress:       req.Progress,
		Percentage:     req.Percentage,
		Device:         sql.NullString{String: req.Device, Valid: req.Device != ""},
		DeviceID:       sql.NullString{String: req.DeviceID, Valid: req.DeviceID != ""},
		Timestamp:      now,
	})
	if err != nil {
		h.logger.Error("upsert koreader progress", "document", req.Document, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// If the KOReader user is linked to a Lexicon user, try to sync progress
	// to user_book_file_progress by matching the document filename.
	if koUser.UserID.Valid {
		h.syncProgressToLexicon(ctx, koUser.UserID.Int64, req.Document, req.Progress, req.Percentage)
	}

	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		var userID int64
		if koUser.UserID.Valid {
			userID = koUser.UserID.Int64
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:       &userID,
			Action:       audit.ActionKOReaderSync,
			ResourceType: "koreader",
			IPAddress:    ip,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"document":  progress.Document,
		"timestamp": progress.Timestamp,
	})
}

// handleGetProgress handles GET /kosync/syncs/progress/{document}.
// Returns the stored reading progress for the given document.
func (h *Handler) handleGetProgress(w http.ResponseWriter, r *http.Request) {
	koUser, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	document := chi.URLParam(r, "document")
	if document == "" {
		writeError(w, http.StatusBadRequest, "document is required")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	progress, err := q.GetKOReaderProgress(ctx, GetKOReaderProgressParams{
		KoreaderUserID: koUser.ID,
		Document:       document,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "progress not found")
			return
		}
		h.logger.Error("get koreader progress", "document", document, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := map[string]any{
		"document":   progress.Document,
		"progress":   progress.Progress,
		"percentage": progress.Percentage,
		"device":     progress.Device.String,
		"device_id":  progress.DeviceID.String,
		"timestamp":  progress.Timestamp,
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- Authentication ---

// authenticate validates HTTP Basic Auth credentials against the koreader_user
// table. The password is expected to be MD5-hashed by the KOReader client.
// Returns the authenticated KoreaderUser and true on success, or writes an
// error response and returns false on failure.
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (KoreaderUser, bool) {
	username, password, ok := r.BasicAuth()
	if !ok || username == "" || password == "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="KOSync"`)
		writeError(w, http.StatusUnauthorized, "authentication required")
		return KoreaderUser{}, false
	}

	ctx := r.Context()
	q := New(h.db)

	koUser, err := q.GetKOReaderUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("WWW-Authenticate", `Basic realm="KOSync"`)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return KoreaderUser{}, false
		}
		h.logger.Error("get koreader user", "username", username, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return KoreaderUser{}, false
	}

	if koUser.PasswordMd5 != password {
		w.Header().Set("WWW-Authenticate", `Basic realm="KOSync"`)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return KoreaderUser{}, false
	}

	return koUser, true
}

// --- Progress Sync ---

// syncProgressToLexicon attempts to match the KOReader document filename to a
// book_file record and upsert user_book_file_progress. This is best-effort:
// errors are logged but do not affect the KOSync response.
func (h *Handler) syncProgressToLexicon(ctx context.Context, userID int64, document, progress string, percentage float64) {
	// Try to find a book_file whose file_path contains the document name.
	// We use a LIKE query directly since sqlc doesn't support dynamic LIKE patterns.
	const query = `SELECT id FROM book_file WHERE file_path LIKE '%' || ? || '%' LIMIT 1`

	row := h.db.QueryRowContext(ctx, query, filepath.Base(document))

	var bookFileID int64
	if err := row.Scan(&bookFileID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			h.logger.Error("match koreader document to book file", "document", document, "error", err)
		}
		// No match found — nothing to sync.
		return
	}

	// Determine the progress value to store. Use the KOReader progress string
	// (which is a CFI or position marker) if non-empty, otherwise use percentage.
	progressValue := progress
	if progressValue == "" {
		progressValue = fmt.Sprintf("%.4f", percentage)
	}

	bq := book.New(h.db)
	if err := bq.UpsertProgress(ctx, book.UpsertProgressParams{
		UserID:       userID,
		BookFileID:   bookFileID,
		Progress:     sql.NullString{String: progressValue, Valid: true},
		ProgressType: sql.NullString{String: "koreader", Valid: true},
	}); err != nil {
		h.logger.Error("upsert progress from koreader", "user_id", userID, "book_file_id", bookFileID, "error", err)
	}
}

// --- Helpers ---

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
