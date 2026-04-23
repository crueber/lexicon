package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/user"
)

// RemoteAuthConfig holds remote header auth configuration.
type RemoteAuthConfig struct {
	Enabled      bool
	UserHeader   string
	EmailHeader  string
	GroupsHeader string
	AutoCreate   bool
}

// RemoteAuthMiddleware creates middleware that authenticates via reverse-proxy headers.
func RemoteAuthMiddleware(db *sql.DB, cfg RemoteAuthConfig, auditSvc *audit.Service, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled || cfg.UserHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			username := r.Header.Get(cfg.UserHeader)
			if username == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Strip any existing principal to prevent spoofing.
			ctx := r.Context()
			if PrincipalFromContext(ctx) != nil {
				next.ServeHTTP(w, r)
				return
			}

			email := r.Header.Get(cfg.EmailHeader)
			groups := []string{}
			if cfg.GroupsHeader != "" {
				groups = strings.Split(r.Header.Get(cfg.GroupsHeader), ",")
				for i := range groups {
					groups[i] = strings.TrimSpace(groups[i])
				}
			}

			u, err := findOrCreateRemoteUser(r.Context(), db, username, email, cfg.AutoCreate)
			if err != nil {
				logger.Warn("remote auth user lookup failed", "error", err, "username", username)
				next.ServeHTTP(w, r)
				return
			}

			if u.Enabled == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Apply group permissions.
			if len(groups) > 0 {
				if err := applyRemoteGroupPermissions(r.Context(), db, u.ID, groups); err != nil {
					logger.Warn("remote auth group permissions failed", "error", err)
				}
			}

			principal, err := buildRemotePrincipal(r.Context(), db, u)
			if err != nil {
				logger.Warn("remote auth build principal failed", "error", err)
				next.ServeHTTP(w, r)
				return
			}

			ctx = context.WithValue(ctx, principalKey, principal)

			if auditSvc != nil {
				ip := r.RemoteAddr
				if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					ip = strings.Split(xff, ",")[0]
				}
				auditSvc.Log(ctx, audit.LogParams{
					UserID:   &u.ID,
					Username: u.Username,
					Action:   audit.ActionRemoteAuthLogin,
					IPAddress: ip,
				})
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// findOrCreateRemoteUser looks up a user by username or creates one.
func findOrCreateRemoteUser(ctx context.Context, db *sql.DB, username, email string, autoCreate bool) (user.User, error) {
	q := user.New(db)

	u, err := q.GetUserByUsername(ctx, username)
	if err == nil {
		return u, nil
	}
	if err != sql.ErrNoRows {
		return user.User{}, fmt.Errorf("get user by username: %w", err)
	}

	if !autoCreate {
		return user.User{}, fmt.Errorf("user not found and auto-create disabled")
	}

	// Try to find by email if username lookup failed.
	if email != "" {
		u, err := q.GetUserByEmail(ctx, sql.NullString{String: email, Valid: true})
		if err == nil {
			return u, nil
		}
		if err != sql.ErrNoRows {
			return user.User{}, fmt.Errorf("get user by email: %w", err)
		}
	}

	newUser, err := user.CreateUserWithDefaults(ctx, db, user.CreateUserServiceParams{
		Username: username,
		Password: generateRandomPassword(),
		Name:     username,
		Email:    email,
	})
	if err != nil {
		return user.User{}, fmt.Errorf("create remote user: %w", err)
	}

	return newUser, nil
}

// applyRemoteGroupPermissions updates user permissions based on group mappings.
func applyRemoteGroupPermissions(ctx context.Context, db *sql.DB, userID int64, groups []string) error {
	for _, g := range groups {
		if g == "" {
			continue
		}
		var permissionBit string
		err := db.QueryRowContext(ctx,
			"SELECT permission_bit FROM oidc_group_mapping WHERE group_name = ?", g,
		).Scan(&permissionBit)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return fmt.Errorf("lookup group mapping: %w", err)
		}

		if permissionBit == "ADMIN" {
			q := user.New(db)
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

// buildRemotePrincipal builds a Principal from a user record.
func buildRemotePrincipal(ctx context.Context, db *sql.DB, u user.User) (*Principal, error) {
	q := user.New(db)

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
