package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/user"
)

// Handler handles authentication HTTP endpoints.
type Handler struct {
	db       *sql.DB
	secret   string
	logger   *slog.Logger
	auditSvc *audit.Service
}

// NewHandler creates a new auth Handler.
func NewHandler(db *sql.DB, secret string, logger *slog.Logger) *Handler {
	return &Handler{
		db:     db,
		secret: secret,
		logger: logger,
	}
}

// WithAuditService sets the audit service for logging auth events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// loginRequest is the JSON body for POST /api/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse is the JSON body returned from POST /api/auth/login.
type loginResponse struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	User         userInfo `json:"user"`
}

// userInfo is the safe user representation returned in auth responses.
type userInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Name     string `json:"name,omitempty"`
	Role     string `json:"role"`
}

// refreshRequest is the JSON body for POST /api/auth/refresh.
type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// refreshResponse is the JSON body returned from POST /api/auth/refresh.
type refreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// logoutRequest is the JSON body for POST /api/auth/logout.
type logoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// changePasswordRequest is the JSON body for PATCH /api/auth/me/password.
type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// HandleLogin handles POST /api/auth/login.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	ctx := r.Context()
	q := user.New(h.db)

	// Look up user by username.
	u, err := q.GetUserByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		h.logger.Error("get user by username", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Check if user is enabled.
	if u.Enabled == 0 {
		writeError(w, http.StatusUnauthorized, "account disabled")
		return
	}

	// Verify password.
	if !u.PasswordHash.Valid || u.PasswordHash.String == "" {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := user.VerifyPassword(u.PasswordHash.String, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Get user permissions.
	perms, err := q.GetUserPermissions(ctx, u.ID)
	if err != nil {
		h.logger.Error("get user permissions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	principal := Principal{
		UserID:   u.ID,
		Username: u.Username,
		Role:     perms.Role,
		Permissions: Permissions{
			CanDownload:     perms.CanDownload == 1,
			CanUpload:       perms.CanUpload == 1,
			CanEmailSend:    perms.CanEmailSend == 1,
			CanEditMetadata: perms.CanEditMetadata == 1,
			OPDSAccess:      perms.OpdsAccess == 1,
		},
	}

	// Populate library IDs for non-admin users.
	// Admins have access to all libraries, so we leave LibraryIDs nil.
	if perms.Role != "ADMIN" {
		libraryIDs, err := q.ListUserLibraryIDs(ctx, u.ID)
		if err != nil {
			h.logger.Error("list user library ids on login", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		principal.LibraryIDs = libraryIDs
	}

	// Issue access token.
	accessToken, err := IssueAccessToken(principal, h.secret)
	if err != nil {
		h.logger.Error("issue access token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Issue refresh token.
	plainRefresh, hashRefresh, err := IssueRefreshToken()
	if err != nil {
		h.logger.Error("issue refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Store refresh token hash in database.
	err = q.CreateRefreshToken(ctx, user.CreateRefreshTokenParams{
		UserID:    u.ID,
		TokenHash: hashRefresh,
		ExpiresAt: time.Now().Add(RefreshTokenExpiry()).UTC().Format(time.RFC3339),
	})
	if err != nil {
		h.logger.Error("store refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := loginResponse{
		AccessToken:  accessToken,
		RefreshToken: plainRefresh,
		User: userInfo{
			ID:       u.ID,
			Username: u.Username,
			Email:    u.Email.String,
			Name:     u.Name.String,
			Role:     perms.Role,
		},
	}

	h.logger.Info("user logged in", "user_id", u.ID, "username", u.Username)
	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:   &u.ID,
			Username: u.Username,
			Action:   audit.ActionUserLogin,
			IPAddress: ip,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleRefresh handles POST /api/auth/refresh.
func (h *Handler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh token is required")
		return
	}

	ctx := r.Context()
	q := user.New(h.db)

	// Look up the refresh token by hash.
	tokenHash := HashToken(req.RefreshToken)
	rt, err := q.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		h.logger.Error("get refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Check expiry.
	expiresAt, err := time.Parse(time.RFC3339, rt.ExpiresAt)
	if err != nil {
		h.logger.Error("parse refresh token expiry", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if time.Now().After(expiresAt) {
		// Revoke the expired token.
		_ = q.RevokeRefreshToken(ctx, tokenHash)
		writeError(w, http.StatusUnauthorized, "refresh token expired")
		return
	}

	// Revoke the old refresh token (rotation).
	if err := q.RevokeRefreshToken(ctx, tokenHash); err != nil {
		h.logger.Error("revoke old refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Look up the user.
	u, err := q.GetUserByID(ctx, rt.UserID)
	if err != nil {
		h.logger.Error("get user by id for refresh", "error", err)
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	if u.Enabled == 0 {
		writeError(w, http.StatusUnauthorized, "account disabled")
		return
	}

	// Get permissions.
	perms, err := q.GetUserPermissions(ctx, u.ID)
	if err != nil {
		h.logger.Error("get user permissions for refresh", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	principal := Principal{
		UserID:   u.ID,
		Username: u.Username,
		Role:     perms.Role,
		Permissions: Permissions{
			CanDownload:     perms.CanDownload == 1,
			CanUpload:       perms.CanUpload == 1,
			CanEmailSend:    perms.CanEmailSend == 1,
			CanEditMetadata: perms.CanEditMetadata == 1,
			OPDSAccess:      perms.OpdsAccess == 1,
		},
	}

	// Populate library IDs for non-admin users on refresh.
	if perms.Role != "ADMIN" {
		libraryIDs, err := q.ListUserLibraryIDs(ctx, u.ID)
		if err != nil {
			h.logger.Error("list user library ids on refresh", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		principal.LibraryIDs = libraryIDs
	}

	// Issue new access token.
	accessToken, err := IssueAccessToken(principal, h.secret)
	if err != nil {
		h.logger.Error("issue access token on refresh", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Issue new refresh token.
	plainRefresh, hashRefresh, err := IssueRefreshToken()
	if err != nil {
		h.logger.Error("issue refresh token on refresh", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	err = q.CreateRefreshToken(ctx, user.CreateRefreshTokenParams{
		UserID:    u.ID,
		TokenHash: hashRefresh,
		ExpiresAt: time.Now().Add(RefreshTokenExpiry()).UTC().Format(time.RFC3339),
	})
	if err != nil {
		h.logger.Error("store new refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, refreshResponse{
		AccessToken:  accessToken,
		RefreshToken: plainRefresh,
	})
}

// HandleLogout handles POST /api/auth/logout.
// This endpoint does not require an access token. The refresh token itself is
// the secret — requiring an access token would prevent logout when the access
// token has expired, which is the most common time a user needs to log out.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh token is required")
		return
	}

	ctx := r.Context()
	q := user.New(h.db)

	tokenHash := HashToken(req.RefreshToken)

	// Look up the user before revoking so we can audit the logout.
	var userID int64
	var username string
	if h.auditSvc != nil {
		rt, err := q.GetRefreshToken(ctx, tokenHash)
		if err == nil {
			userID = rt.UserID
			if u, err := q.GetUserByID(ctx, rt.UserID); err == nil {
				username = u.Username
			}
		}
	}

	if err := q.RevokeRefreshToken(ctx, tokenHash); err != nil {
		h.logger.Error("revoke refresh token on logout", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.auditSvc != nil && userID != 0 {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:   &userID,
			Username: username,
			Action:   audit.ActionUserLogout,
			IPAddress: ip,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleMe handles GET /api/auth/me.
func (h *Handler) HandleMe(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFromContext(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx := r.Context()
	q := user.New(h.db)

	u, err := q.GetUserByID(ctx, p.UserID)
	if err != nil {
		h.logger.Error("get user for /me", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, userInfo{
		ID:       u.ID,
		Username: u.Username,
		Email:    u.Email.String,
		Name:     u.Name.String,
		Role:     p.Role,
	})
}

// HandleChangePassword handles PATCH /api/auth/me/password.
func (h *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFromContext(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req changePasswordRequest
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
	q := user.New(h.db)

	// Get current user to verify old password.
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

	if err := user.VerifyPassword(u.PasswordHash.String, req.CurrentPassword); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Hash new password.
	newHash, err := user.HashPassword(req.NewPassword)
	if err != nil {
		h.logger.Error("hash new password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Update password.
	if err := q.UpdateUserPassword(ctx, user.UpdateUserPasswordParams{
		PasswordHash: sql.NullString{String: newHash, Valid: true},
		ID:           p.UserID,
	}); err != nil {
		h.logger.Error("update password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("user changed password", "user_id", p.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
