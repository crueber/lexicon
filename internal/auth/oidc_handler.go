package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/user"
	"github.com/go-chi/chi/v5"
)

// OIDCHandler handles OIDC authentication HTTP endpoints.
type OIDCHandler struct {
	db       *sql.DB
	secret   string
	logger   *slog.Logger
	service  *OIDCService
	auditSvc *audit.Service
}

// NewOIDCHandler creates a new OIDC handler. The service may be nil if OIDC
// is not configured.
func NewOIDCHandler(db *sql.DB, secret string, logger *slog.Logger, service *OIDCService) *OIDCHandler {
	return &OIDCHandler{
		db:      db,
		secret:  secret,
		logger:  logger,
		service: service,
	}
}

// WithAuditService sets the audit service for logging auth events.
func (h *OIDCHandler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// providerInfo is the public representation of an OIDC provider.
type providerInfo struct {
	Name string `json:"name"`
}

// HandleProviders handles GET /api/auth/oidc/providers.
func (h *OIDCHandler) HandleProviders(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || !h.service.cfg.Enabled {
		writeJSON(w, http.StatusOK, []providerInfo{})
		return
	}

	writeJSON(w, http.StatusOK, []providerInfo{
		{Name: h.service.cfg.ProviderName},
	})
}

// HandleAuthorize handles GET /api/auth/oidc/{provider}/authorize.
func (h *OIDCHandler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || !h.service.cfg.Enabled {
		writeError(w, http.StatusNotFound, "oidc not configured")
		return
	}

	providerName := chi.URLParam(r, "provider")
	if providerName == "" || providerName != h.service.cfg.ProviderName {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	redirectURL := r.URL.Query().Get("redirect")
	// Only allow same-origin relative paths to prevent open redirect attacks.
	if redirectURL != "" && !isValidRedirectPath(redirectURL) {
		writeError(w, http.StatusBadRequest, "invalid redirect")
		return
	}

	state, nonce, err := h.service.GenerateState(redirectURL)
	if err != nil {
		h.logger.Error("generate oidc state", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	authURL := h.service.AuthCodeURL(state, nonce)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles GET /api/auth/oidc/callback.
func (h *OIDCHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || !h.service.cfg.Enabled {
		writeError(w, http.StatusNotFound, "oidc not configured")
		return
	}

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeError(w, http.StatusBadRequest, "missing state or code")
		return
	}

	ctx := r.Context()
	result, err := h.service.HandleCallback(ctx, state, code)
	if err != nil {
		h.logger.Error("oidc callback", "error", err)
		writeError(w, http.StatusUnauthorized, "oidc authentication failed")
		return
	}

	// Find or create user.
	u, created, err := h.findOrCreateUser(ctx, result)
	if err != nil {
		h.logger.Error("oidc find or create user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if u.Enabled == 0 {
		writeError(w, http.StatusUnauthorized, "account disabled")
		return
	}

	// Build principal and issue tokens.
	principal, err := h.buildPrincipal(ctx, u)
	if err != nil {
		h.logger.Error("oidc build principal", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	accessToken, err := IssueAccessToken(*principal, h.secret)
	if err != nil {
		h.logger.Error("issue access token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	plainRefresh, hashRefresh, err := IssueRefreshToken()
	if err != nil {
		h.logger.Error("issue refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	q := user.New(h.db)
	if err := q.CreateRefreshToken(ctx, user.CreateRefreshTokenParams{
		UserID:    u.ID,
		TokenHash: hashRefresh,
		ExpiresAt: time.Now().Add(RefreshTokenExpiry()).UTC().Format(time.RFC3339),
	}); err != nil {
		h.logger.Error("store refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("oidc user logged in", "user_id", u.ID, "username", u.Username)
	if h.auditSvc != nil {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		action := audit.ActionUserLogin
		if created {
			action = audit.ActionOIDCUserCreated
		}
		h.auditSvc.Log(ctx, audit.LogParams{
			UserID:    &u.ID,
			Username:  u.Username,
			Action:    action,
			IPAddress: ip,
		})
	}

	// Redirect to frontend with tokens. Always use /login to prevent open
	// redirects — the redirect path was validated in HandleAuthorize but we
	// never redirect to an arbitrary host.
	redirectURL := "/login?token=" + accessToken + "&refresh=" + plainRefresh
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// isValidRedirectPath reports whether redirect is a safe same-origin path.
func isValidRedirectPath(redirect string) bool {
	// Must start with / and not contain a host or protocol.
	if !strings.HasPrefix(redirect, "/") {
		return false
	}
	// Reject protocol-relative URLs and anything that looks like a full URL.
	if strings.HasPrefix(redirect, "//") {
		return false
	}
	return true
}

// findOrCreateUser looks up a user by email or creates one from OIDC claims.
// It returns the user and a boolean indicating whether the user was newly created.
func (h *OIDCHandler) findOrCreateUser(ctx context.Context, result *OIDCCallbackResult) (user.User, bool, error) {
	q := user.New(h.db)

	// Try to find by email.
	if result.Email != "" {
		u, err := q.GetUserByEmail(ctx, sql.NullString{String: result.Email, Valid: true})
		if err == nil {
			return u, false, nil
		}
		if err != sql.ErrNoRows {
			return user.User{}, false, fmt.Errorf("get user by email: %w", err)
		}
	}

	// Generate a username from the email or subject.
	username := result.Email
	if username == "" {
		username = "oidc_" + result.Subject
	}
	// Ensure uniqueness by appending a suffix if needed.
	baseUsername := username
	for i := 1; ; i++ {
		_, err := q.GetUserByUsername(ctx, username)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return user.User{}, false, fmt.Errorf("check username: %w", err)
		}
		username = fmt.Sprintf("%s_%d", baseUsername, i)
	}

	// Create user with a random password (not used for OIDC login).
	newUser, err := user.CreateUserWithDefaults(ctx, h.db, user.CreateUserServiceParams{
		Username: username,
		Password: generateRandomPassword(),
		Name:     result.Name,
		Email:    result.Email,
	})
	if err != nil {
		return user.User{}, false, fmt.Errorf("create user: %w", err)
	}

	// Apply group-based permissions if groups are present.
	if len(result.Groups) > 0 {
		if err := h.applyGroupPermissions(ctx, newUser.ID, result.Groups); err != nil {
			h.logger.Warn("failed to apply oidc group permissions", "error", err)
		}
	}

	return newUser, true, nil
}

// buildPrincipal builds a Principal from a user record.
func (h *OIDCHandler) buildPrincipal(ctx context.Context, u user.User) (*Principal, error) {
	q := user.New(h.db)

	perms, err := q.GetUserPermissions(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("get user permissions: %w", err)
	}

	principal := &Principal{
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

	if perms.Role != "ADMIN" {
		libraryIDs, err := q.ListUserLibraryIDs(ctx, u.ID)
		if err != nil {
			return nil, fmt.Errorf("list user library ids: %w", err)
		}
		principal.LibraryIDs = libraryIDs
	}

	return principal, nil
}

// applyGroupPermissions updates user permissions based on OIDC group mappings.
func (h *OIDCHandler) applyGroupPermissions(ctx context.Context, userID int64, groups []string) error {
	for _, g := range groups {
		var permissionBit string
		err := h.db.QueryRowContext(ctx,
			"SELECT permission_bit FROM oidc_group_mapping WHERE group_name = ?", g,
		).Scan(&permissionBit)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return fmt.Errorf("lookup group mapping: %w", err)
		}

		// Simple group-to-role mapping: if permission_bit is "ADMIN", promote.
		if permissionBit == "ADMIN" {
			q := user.New(h.db)
			if err := q.UpsertUserPermissions(ctx, user.UpsertUserPermissionsParams{
				UserID: userID,
				Role:   "ADMIN",
			}); err != nil {
				return fmt.Errorf("upsert admin permissions: %w", err)
			}
		}
	}
	return nil
}

// generateRandomPassword creates a random 32-character password.
func generateRandomPassword() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based password on failure.
		return fmt.Sprintf("oidc_%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}
