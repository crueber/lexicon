package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/go-chi/chi/v5"
)

// Principal is a minimal representation of the authenticated user, extracted
// from the request context. Defined here to avoid an import cycle with the
// auth package (which imports user).
type Principal struct {
	UserID   int64
	Username string
	Role     string
}

// IsAdmin returns true if the principal has the ADMIN role.
func (p *Principal) IsAdmin() bool {
	return p.Role == "ADMIN"
}

// PrincipalExtractor is a function that extracts the authenticated principal
// from a request context. Injected at construction time to avoid import cycles.
type PrincipalExtractor func(ctx context.Context) *Principal

// LibraryAccessSetter is a function that sets library access for a user within
// a transaction. Injected at construction time to avoid import cycles.
type LibraryAccessSetter func(ctx context.Context, db *sql.DB, userID int64, libraryIDs []int64) error

// Handler handles HTTP requests for user management.
type Handler struct {
	db               *sql.DB
	svc              *Service
	logger           *slog.Logger
	getPrincipal     PrincipalExtractor
	setLibraryAccess LibraryAccessSetter
	auditSvc         *audit.Service
}

// NewHandler creates a new user Handler.
// getPrincipal extracts the authenticated principal from a request context.
// setLibraryAccess replaces all library access for a user.
func NewHandler(db *sql.DB, svc *Service, logger *slog.Logger, getPrincipal PrincipalExtractor, setLibraryAccess LibraryAccessSetter) *Handler {
	return &Handler{
		db:               db,
		svc:              svc,
		logger:           logger,
		getPrincipal:     getPrincipal,
		setLibraryAccess: setLibraryAccess,
	}
}

// WithAuditService sets the audit service for logging user events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// AdminRoutes registers admin user management routes on the given router.
// RequireAuth and RequireAdmin must already be applied by the caller.
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/", h.handleListUsers)
	r.Post("/", h.handleCreateUser)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.handleGetUser)
		r.Put("/", h.handleUpdateUser)
		r.Delete("/", h.handleDeleteUser)
		r.Put("/password", h.handleResetPassword)
		r.Put("/permissions", h.handleSetPermissions)
		r.Put("/libraries", h.handleSetLibraries)
	})
}

// SelfRoutes registers self-service user routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) SelfRoutes(r chi.Router) {
	r.Get("/me", h.handleGetMe)
	r.Patch("/me", h.handleUpdateMe)
	r.Patch("/me/password", h.handleChangeMyPassword)
	r.Get("/me/settings", h.handleGetMySettings)
	r.Put("/me/settings", h.handleUpdateMySettings)
}

// --- Response types ---

// UserResponse is the JSON representation of a user.
type UserResponse struct {
	ID          int64                `json:"id"`
	Username    string               `json:"username"`
	Email       *string              `json:"email,omitempty"`
	Name        *string              `json:"name,omitempty"`
	Enabled     bool                 `json:"enabled"`
	CreatedAt   string               `json:"createdAt"`
	Permissions *PermissionsResponse `json:"permissions,omitempty"`
	LibraryIDs  []int64              `json:"libraryIds,omitempty"`
}

// PermissionsResponse is the JSON representation of user permissions.
type PermissionsResponse struct {
	Role            string `json:"role"`
	CanDownload     bool   `json:"canDownload"`
	CanUpload       bool   `json:"canUpload"`
	CanEmailSend    bool   `json:"canEmailSend"`
	CanEditMetadata bool   `json:"canEditMetadata"`
	OPDSAccess      bool   `json:"opdsAccess"`
}

// SettingsResponse is the JSON representation of user settings.
type SettingsResponse struct {
	Theme           *string `json:"theme,omitempty"`
	BookCardsPerRow *int64  `json:"bookCardsPerRow,omitempty"`
}

// toUserResponse converts a User model to a UserResponse.
func toUserResponse(u User) UserResponse {
	resp := UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		Enabled:   u.Enabled == 1,
		CreatedAt: u.CreatedAt,
	}
	if u.Email.Valid {
		resp.Email = &u.Email.String
	}
	if u.Name.Valid {
		resp.Name = &u.Name.String
	}
	return resp
}

// toPermissionsResponse converts a UserPermission model to a PermissionsResponse.
func toPermissionsResponse(p UserPermission) *PermissionsResponse {
	return &PermissionsResponse{
		Role:            p.Role,
		CanDownload:     p.CanDownload == 1,
		CanUpload:       p.CanUpload == 1,
		CanEmailSend:    p.CanEmailSend == 1,
		CanEditMetadata: p.CanEditMetadata == 1,
		OPDSAccess:      p.OpdsAccess == 1,
	}
}

// --- Admin handlers ---

// handleListUsers handles GET /api/admin/users.
func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := New(h.db)

	users, err := q.ListUsers(ctx)
	if err != nil {
		h.logger.Error("list users", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]UserResponse, 0, len(users))
	for _, u := range users {
		ur := toUserResponse(u)

		perms, err := q.GetUserPermissions(ctx, u.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			h.logger.Error("get user permissions", "user_id", u.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err == nil {
			ur.Permissions = toPermissionsResponse(perms)
		}

		resp = append(resp, ur)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCreateUser handles POST /api/admin/users.
func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if req.Role == "" {
		req.Role = "USER"
	}
	if req.Role != "ADMIN" && req.Role != "USER" {
		writeError(w, http.StatusBadRequest, "role must be ADMIN or USER")
		return
	}

	ctx := r.Context()

	var u User
	var err error
	if req.Role == "ADMIN" {
		u, err = CreateAdminUser(ctx, h.db, CreateUserServiceParams{
			Username: req.Username,
			Password: req.Password,
			Name:     req.Name,
			Email:    req.Email,
		})
	} else {
		u, err = CreateUserWithDefaults(ctx, h.db, CreateUserServiceParams{
			Username: req.Username,
			Password: req.Password,
			Name:     req.Name,
			Email:    req.Email,
		})
	}
	if err != nil {
		h.logger.Error("create user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	q := New(h.db)
	perms, err := q.GetUserPermissions(ctx, u.ID)
	if err != nil {
		h.logger.Error("get permissions after create", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := toUserResponse(u)
	resp.Permissions = toPermissionsResponse(perms)

	h.logger.Info("admin created user", "username", u.Username, "role", req.Role)
	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:     &u.ID,
			Username:   u.Username,
			Action:     audit.ActionUserCreated,
			IPAddress:  ip,
			Details:    map[string]any{"role": req.Role},
		})
	}
	writeJSON(w, http.StatusCreated, resp)
}

// handleGetUser handles GET /api/admin/users/{id}.
func (h *Handler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	u, err := q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("get user by id", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := toUserResponse(u)

	perms, err := q.GetUserPermissions(ctx, u.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.logger.Error("get user permissions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err == nil {
		resp.Permissions = toPermissionsResponse(perms)
	}

	libraryIDs, err := q.ListUserLibraryIDs(ctx, u.ID)
	if err != nil {
		h.logger.Error("list user library ids", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp.LibraryIDs = libraryIDs

	writeJSON(w, http.StatusOK, resp)
}

// handleUpdateUser handles PUT /api/admin/users/{id}.
func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Name    *string `json:"name"`
		Email   *string `json:"email"`
		Enabled *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	// Fetch current user to preserve unchanged fields.
	u, err := q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("get user for update", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	params := UpdateUserParams{
		ID:      id,
		Email:   u.Email,
		Name:    u.Name,
		Enabled: u.Enabled,
	}

	if req.Name != nil {
		params.Name = sql.NullString{String: *req.Name, Valid: *req.Name != ""}
	}
	if req.Email != nil {
		params.Email = sql.NullString{String: *req.Email, Valid: *req.Email != ""}
	}
	if req.Enabled != nil {
		if *req.Enabled {
			params.Enabled = 1
		} else {
			params.Enabled = 0
		}
	}

	if err := q.UpdateUser(ctx, params); err != nil {
		h.logger.Error("update user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := q.GetUserByID(ctx, id)
	if err != nil {
		h.logger.Error("get updated user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:    &id,
			Action:    audit.ActionUserUpdated,
			IPAddress: ip,
		})
	}
	writeJSON(w, http.StatusOK, toUserResponse(updated))
}

// handleDeleteUser handles DELETE /api/admin/users/{id}.
func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// Prevent self-deletion.
	p := h.getPrincipal(r.Context())
	if p != nil && p.UserID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	if err := q.DeleteUser(ctx, id); err != nil {
		h.logger.Error("delete user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("admin deleted user", "user_id", id)
	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:    &id,
			Action:    audit.ActionUserDeleted,
			IPAddress: ip,
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleResetPassword handles PUT /api/admin/users/{id}/password.
func (h *Handler) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		h.logger.Error("hash password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	if err := q.UpdateUserPassword(ctx, UpdateUserPasswordParams{
		PasswordHash: sql.NullString{String: hash, Valid: true},
		ID:           id,
	}); err != nil {
		h.logger.Error("reset user password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Revoke all refresh tokens so the user must re-login.
	if err := h.svc.RevokeAllUserRefreshTokens(ctx, id); err != nil {
		h.logger.Error("revoke refresh tokens after password reset", "error", err)
		// Non-fatal: password was changed, tokens will expire naturally.
	}

	h.logger.Info("admin reset user password", "user_id", id)
	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:    &id,
			Action:    audit.ActionUserUpdated,
			IPAddress: ip,
			Details:   map[string]any{"field": "password"},
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSetPermissions handles PUT /api/admin/users/{id}/permissions.
func (h *Handler) handleSetPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Role            string `json:"role"`
		CanDownload     bool   `json:"canDownload"`
		CanUpload       bool   `json:"canUpload"`
		CanEmailSend    bool   `json:"canEmailSend"`
		CanEditMetadata bool   `json:"canEditMetadata"`
		OPDSAccess      bool   `json:"opdsAccess"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Role != "ADMIN" && req.Role != "USER" {
		writeError(w, http.StatusBadRequest, "role must be ADMIN or USER")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	params := UpsertUserPermissionsParams{
		UserID: id,
		Role:   req.Role,
	}
	if req.CanDownload {
		params.CanDownload = 1
	}
	if req.CanUpload {
		params.CanUpload = 1
	}
	if req.CanEmailSend {
		params.CanEmailSend = 1
	}
	if req.CanEditMetadata {
		params.CanEditMetadata = 1
	}
	if req.OPDSAccess {
		params.OpdsAccess = 1
	}

	if err := q.UpsertUserPermissions(ctx, params); err != nil {
		h.logger.Error("set user permissions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	perms, err := q.GetUserPermissions(ctx, id)
	if err != nil {
		h.logger.Error("get permissions after update", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("admin set user permissions", "user_id", id, "role", req.Role)
	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:    &id,
			Action:    audit.ActionUserUpdated,
			IPAddress: ip,
			Details:   map[string]any{"field": "permissions", "role": req.Role},
		})
	}
	writeJSON(w, http.StatusOK, toPermissionsResponse(perms))
}

// handleSetLibraries handles PUT /api/admin/users/{id}/libraries.
func (h *Handler) handleSetLibraries(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		LibraryIDs []int64 `json:"libraryIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.LibraryIDs == nil {
		req.LibraryIDs = []int64{}
	}

	ctx := r.Context()

	if err := h.setLibraryAccess(ctx, h.db, id, req.LibraryIDs); err != nil {
		h.logger.Error("set user library access", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("admin set user library access", "user_id", id, "library_count", len(req.LibraryIDs))
	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:    &id,
			Action:    audit.ActionUserUpdated,
			IPAddress: ip,
			Details:   map[string]any{"field": "libraries", "count": len(req.LibraryIDs)},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"libraryIds": req.LibraryIDs})
}

// --- Self-service handlers ---

// handleGetMe handles GET /api/users/me.
func (h *Handler) handleGetMe(w http.ResponseWriter, r *http.Request) {
	p := h.getPrincipal(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	u, err := q.GetUserByID(ctx, p.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("get user for /me", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := toUserResponse(u)

	perms, err := q.GetUserPermissions(ctx, u.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.logger.Error("get permissions for /me", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err == nil {
		resp.Permissions = toPermissionsResponse(perms)
	}

	libraryIDs, err := q.ListUserLibraryIDs(ctx, u.ID)
	if err != nil {
		h.logger.Error("list library ids for /me", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp.LibraryIDs = libraryIDs

	writeJSON(w, http.StatusOK, resp)
}

// handleUpdateMe handles PATCH /api/users/me.
func (h *Handler) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	p := h.getPrincipal(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		Name  *string `json:"name"`
		Email *string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	u, err := q.GetUserByID(ctx, p.UserID)
	if err != nil {
		h.logger.Error("get user for update me", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	params := UpdateUserParams{
		ID:      p.UserID,
		Email:   u.Email,
		Name:    u.Name,
		Enabled: u.Enabled,
	}

	if req.Name != nil {
		params.Name = sql.NullString{String: *req.Name, Valid: *req.Name != ""}
	}
	if req.Email != nil {
		params.Email = sql.NullString{String: *req.Email, Valid: *req.Email != ""}
	}

	if err := q.UpdateUser(ctx, params); err != nil {
		h.logger.Error("update me", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := q.GetUserByID(ctx, p.UserID)
	if err != nil {
		h.logger.Error("get updated user for me", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(updated))
}

// handleChangeMyPassword handles PATCH /api/users/me/password.
func (h *Handler) handleChangeMyPassword(w http.ResponseWriter, r *http.Request) {
	p := h.getPrincipal(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current and new passwords are required")
		return
	}
	if len(req.NewPassword) < 6 {
		writeError(w, http.StatusBadRequest, "new password must be at least 6 characters")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	u, err := q.GetUserByID(ctx, p.UserID)
	if err != nil {
		h.logger.Error("get user for password change", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !u.PasswordHash.Valid || u.PasswordHash.String == "" {
		writeError(w, http.StatusBadRequest, "no password set for this account")
		return
	}

	if err := VerifyPassword(u.PasswordHash.String, req.CurrentPassword); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		h.logger.Error("hash new password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := q.UpdateUserPassword(ctx, UpdateUserPasswordParams{
		PasswordHash: sql.NullString{String: newHash, Valid: true},
		ID:           p.UserID,
	}); err != nil {
		h.logger.Error("update my password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("user changed own password", "user_id", p.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGetMySettings handles GET /api/users/me/settings.
func (h *Handler) handleGetMySettings(w http.ResponseWriter, r *http.Request) {
	p := h.getPrincipal(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	settings, err := q.GetUserSettings(ctx, p.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return empty settings if none exist.
			writeJSON(w, http.StatusOK, SettingsResponse{})
			return
		}
		h.logger.Error("get user settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := SettingsResponse{}
	if settings.Theme.Valid {
		resp.Theme = &settings.Theme.String
	}
	if settings.BookCardsPerRow.Valid {
		resp.BookCardsPerRow = &settings.BookCardsPerRow.Int64
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleUpdateMySettings handles PUT /api/users/me/settings.
func (h *Handler) handleUpdateMySettings(w http.ResponseWriter, r *http.Request) {
	p := h.getPrincipal(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		Theme           *string `json:"theme"`
		BookCardsPerRow *int64  `json:"bookCardsPerRow"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	q := New(h.db)

	// Fetch current settings to preserve unchanged fields.
	current, err := q.GetUserSettings(ctx, p.UserID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.logger.Error("get current settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	params := UpsertUserSettingsParams{
		UserID: p.UserID,
	}

	// Preserve existing values if not overridden.
	if err == nil {
		params.Theme = current.Theme
		params.BookCardsPerRow = current.BookCardsPerRow
	}

	if req.Theme != nil {
		params.Theme = sql.NullString{String: *req.Theme, Valid: true}
	}
	if req.BookCardsPerRow != nil {
		if *req.BookCardsPerRow < 2 || *req.BookCardsPerRow > 8 {
			writeError(w, http.StatusBadRequest, "bookCardsPerRow must be between 2 and 8")
			return
		}
		params.BookCardsPerRow = sql.NullInt64{Int64: *req.BookCardsPerRow, Valid: true}
	}

	if err := q.UpsertUserSettings(ctx, params); err != nil {
		h.logger.Error("update user settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := q.GetUserSettings(ctx, p.UserID)
	if err != nil {
		h.logger.Error("get updated settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := SettingsResponse{}
	if updated.Theme.Valid {
		resp.Theme = &updated.Theme.String
	}
	if updated.BookCardsPerRow.Valid {
		resp.BookCardsPerRow = &updated.BookCardsPerRow.Int64
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- Helpers ---

// parseID extracts and parses the {id} URL parameter as int64.
func parseID(r *http.Request) (int64, error) {
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
