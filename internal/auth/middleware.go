package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// contextKey is an unexported type for context value keys in this package,
// preventing collisions with keys defined in other packages.
type contextKey struct{}

// principalKey is the context key for the authenticated Principal.
var principalKey = contextKey{}

// PrincipalFromContext extracts the authenticated Principal from the request
// context. Returns nil if no Principal is present.
func PrincipalFromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

// RequireAuth returns middleware that validates the Bearer token in the
// Authorization header and injects the Principal into the request context.
func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			tokenString, ok := strings.CutPrefix(authHeader, "Bearer ")
			if !ok || tokenString == "" {
				writeError(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			principal, err := ValidateAccessToken(tokenString, secret)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), principalKey, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin returns middleware that checks the authenticated user has
// the ADMIN role. Must be used after RequireAuth.
func RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := PrincipalFromContext(r.Context())
			if p == nil {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if !p.IsAdmin() {
				writeError(w, http.StatusForbidden, "admin access required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission returns middleware that checks the authenticated user
// has the specified permission. Must be used after RequireAuth.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := PrincipalFromContext(r.Context())
			if p == nil {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			if !hasPermission(p, perm) {
				writeError(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// hasPermission checks whether the principal has the named permission.
// Admins have all permissions.
func hasPermission(p *Principal, perm string) bool {
	if p.IsAdmin() {
		return true
	}
	switch perm {
	case "download":
		return p.Permissions.CanDownload
	case "upload":
		return p.Permissions.CanUpload
	case "email_send":
		return p.Permissions.CanEmailSend
	case "edit_metadata":
		return p.Permissions.CanEditMetadata
	case "opds":
		return p.Permissions.OPDSAccess
	default:
		return false
	}
}

// errorResponse is the JSON structure for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: message})
}
