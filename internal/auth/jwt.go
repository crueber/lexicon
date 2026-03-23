package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenExpiry  = 15 * time.Minute
	refreshTokenExpiry = 30 * 24 * time.Hour
)

// RefreshTokenExpiry returns the duration after which refresh tokens expire.
func RefreshTokenExpiry() time.Duration {
	return refreshTokenExpiry
}

// claims is the custom JWT claims structure for access tokens.
type claims struct {
	jwt.RegisteredClaims
	Username    string      `json:"username"`
	Role        string      `json:"role"`
	Permissions permissions `json:"permissions"`
	LibraryIDs  []int64     `json:"library_ids"`
}

// permissions mirrors Permissions for JWT serialization.
type permissions struct {
	CanDownload     bool `json:"can_download"`
	CanUpload       bool `json:"can_upload"`
	CanEmailSend    bool `json:"can_email_send"`
	CanEditMetadata bool `json:"can_edit_metadata"`
	OPDSAccess      bool `json:"opds_access"`
}

// IssueAccessToken creates a signed HS256 JWT for the given principal.
// The token expires after 15 minutes.
func IssueAccessToken(p Principal, secret string) (string, error) {
	now := time.Now()
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", p.UserID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenExpiry)),
			Issuer:    "lexicon",
		},
		Username: p.Username,
		Role:     p.Role,
		Permissions: permissions{
			CanDownload:     p.Permissions.CanDownload,
			CanUpload:       p.Permissions.CanUpload,
			CanEmailSend:    p.Permissions.CanEmailSend,
			CanEditMetadata: p.Permissions.CanEditMetadata,
			OPDSAccess:      p.Permissions.OPDSAccess,
		},
		LibraryIDs: p.LibraryIDs,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// IssueRefreshToken generates a cryptographically random refresh token.
// It returns (plaintext, sha256Hash, error). The plaintext is sent to the
// client; the hash is stored in the database.
func IssueRefreshToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	plaintext := hex.EncodeToString(b)
	hash := HashToken(plaintext)
	return plaintext, hash, nil
}

// HashToken returns the SHA-256 hex digest of a token string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ValidateAccessToken parses and validates a JWT access token, returning
// the embedded Principal on success.
func ValidateAccessToken(tokenString, secret string) (*Principal, error) {
	token, err := jwt.ParseWithClaims(tokenString, &claims{}, func(_ *jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, fmt.Errorf("token expired: %w", err)
		}
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	c, ok := token.Claims.(*claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	var userID int64
	if _, err := fmt.Sscanf(c.Subject, "%d", &userID); err != nil {
		return nil, fmt.Errorf("parse user id from subject: %w", err)
	}

	p := &Principal{
		UserID:   userID,
		Username: c.Username,
		Role:     c.Role,
		Permissions: Permissions{
			CanDownload:     c.Permissions.CanDownload,
			CanUpload:       c.Permissions.CanUpload,
			CanEmailSend:    c.Permissions.CanEmailSend,
			CanEditMetadata: c.Permissions.CanEditMetadata,
			OPDSAccess:      c.Permissions.OPDSAccess,
		},
		LibraryIDs: c.LibraryIDs,
	}

	return p, nil
}
