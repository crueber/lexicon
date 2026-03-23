package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key-at-least-32-chars!"

func TestIssueAndValidateAccessToken(t *testing.T) {
	p := Principal{
		UserID:   42,
		Username: "testuser",
		Role:     "ADMIN",
		Permissions: Permissions{
			CanDownload:     true,
			CanUpload:       false,
			CanEmailSend:    true,
			CanEditMetadata: false,
			OPDSAccess:      true,
		},
		LibraryIDs: []int64{1, 2, 3},
	}

	tokenString, err := IssueAccessToken(p, testSecret)
	if err != nil {
		t.Fatalf("IssueAccessToken() error: %v", err)
	}

	if tokenString == "" {
		t.Fatal("IssueAccessToken() returned empty token")
	}

	got, err := ValidateAccessToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error: %v", err)
	}

	if got.UserID != p.UserID {
		t.Errorf("UserID = %d; want %d", got.UserID, p.UserID)
	}
	if got.Username != p.Username {
		t.Errorf("Username = %q; want %q", got.Username, p.Username)
	}
	if got.Role != p.Role {
		t.Errorf("Role = %q; want %q", got.Role, p.Role)
	}
	if got.Permissions.CanDownload != p.Permissions.CanDownload {
		t.Errorf("CanDownload = %v; want %v", got.Permissions.CanDownload, p.Permissions.CanDownload)
	}
	if got.Permissions.CanUpload != p.Permissions.CanUpload {
		t.Errorf("CanUpload = %v; want %v", got.Permissions.CanUpload, p.Permissions.CanUpload)
	}
	if got.Permissions.CanEmailSend != p.Permissions.CanEmailSend {
		t.Errorf("CanEmailSend = %v; want %v", got.Permissions.CanEmailSend, p.Permissions.CanEmailSend)
	}
	if got.Permissions.CanEditMetadata != p.Permissions.CanEditMetadata {
		t.Errorf("CanEditMetadata = %v; want %v", got.Permissions.CanEditMetadata, p.Permissions.CanEditMetadata)
	}
	if got.Permissions.OPDSAccess != p.Permissions.OPDSAccess {
		t.Errorf("OPDSAccess = %v; want %v", got.Permissions.OPDSAccess, p.Permissions.OPDSAccess)
	}
}

func TestValidateAccessTokenWrongSecret(t *testing.T) {
	p := Principal{
		UserID:   1,
		Username: "user",
		Role:     "USER",
	}

	tokenString, err := IssueAccessToken(p, testSecret)
	if err != nil {
		t.Fatalf("IssueAccessToken() error: %v", err)
	}

	_, err = ValidateAccessToken(tokenString, "wrong-secret-key-at-least-32-chars!")
	if err == nil {
		t.Error("ValidateAccessToken() with wrong secret should return error")
	}
}

func TestValidateAccessTokenExpired(t *testing.T) {
	// Create a token that's already expired by manually constructing claims.
	now := time.Now()
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "1",
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
			Issuer:    "lexicon",
		},
		Username: "expired",
		Role:     "USER",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	tokenString, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}

	_, err = ValidateAccessToken(tokenString, testSecret)
	if err == nil {
		t.Error("ValidateAccessToken() with expired token should return error")
	}
}

func TestValidateAccessTokenInvalid(t *testing.T) {
	_, err := ValidateAccessToken("not-a-valid-jwt", testSecret)
	if err == nil {
		t.Error("ValidateAccessToken() with invalid token should return error")
	}
}

func TestValidateAccessTokenEmptyString(t *testing.T) {
	_, err := ValidateAccessToken("", testSecret)
	if err == nil {
		t.Error("ValidateAccessToken() with empty string should return error")
	}
}

func TestIssueRefreshToken(t *testing.T) {
	plain1, hash1, err := IssueRefreshToken()
	if err != nil {
		t.Fatalf("IssueRefreshToken() error: %v", err)
	}

	if plain1 == "" {
		t.Error("IssueRefreshToken() returned empty plaintext")
	}
	if hash1 == "" {
		t.Error("IssueRefreshToken() returned empty hash")
	}

	// Verify hash matches.
	if got := HashToken(plain1); got != hash1 {
		t.Errorf("HashToken(plaintext) = %q; want %q", got, hash1)
	}

	// Verify uniqueness.
	plain2, hash2, err := IssueRefreshToken()
	if err != nil {
		t.Fatalf("second IssueRefreshToken() error: %v", err)
	}

	if plain1 == plain2 {
		t.Error("two IssueRefreshToken() calls returned same plaintext")
	}
	if hash1 == hash2 {
		t.Error("two IssueRefreshToken() calls returned same hash")
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	token := "test-token-value"
	h1 := HashToken(token)
	h2 := HashToken(token)
	if h1 != h2 {
		t.Errorf("HashToken() not deterministic: %q != %q", h1, h2)
	}
}
