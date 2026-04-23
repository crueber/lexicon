package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig holds OIDC provider configuration.
type OIDCConfig struct {
	Enabled      bool
	ProviderName string
	ClientID     string
	ClientSecret string
	IssuerURI    string
	Scopes       string
}

// OIDCService manages OIDC authentication.
type OIDCService struct {
	db           *sql.DB
	verifier     *oidc.IDTokenVerifier
	oauth2Config *oauth2.Config
	cfg          OIDCConfig
}

// NewOIDCService creates an OIDC service. Returns nil if OIDC is not enabled.
func NewOIDCService(db *sql.DB, cfg OIDCConfig) (*OIDCService, error) {
	if !cfg.Enabled || cfg.IssuerURI == "" {
		return nil, nil
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURI)
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if cfg.Scopes != "" {
		for _, s := range strings.Split(cfg.Scopes, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				scopes = append(scopes, s)
			}
		}
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
		RedirectURL:  "/api/auth/oidc/callback",
	}

	return &OIDCService{
		db:           db,
		verifier:     verifier,
		oauth2Config: oauth2Config,
		cfg:          cfg,
	}, nil
}

// GenerateState creates a random state parameter and stores the session.
// It returns (state, nonce, error).
func (s *OIDCService) GenerateState(redirectURL string) (string, string, error) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", "", fmt.Errorf("generate nonce: %w", err)
	}
	nonce := base64.URLEncoding.EncodeToString(nonceBytes)

	_, err := s.db.Exec(
		"INSERT INTO oidc_session (state, nonce, redirect_url, created_at) VALUES (?, ?, ?, datetime('now'))",
		state, nonce, redirectURL)
	if err != nil {
		return "", "", fmt.Errorf("store oidc session: %w", err)
	}

	return state, nonce, nil
}

// OIDCCallbackResult holds the result of a successful OIDC callback.
type OIDCCallbackResult struct {
	Email      string
	Name       string
	Subject    string
	Groups     []string
	RedirectURL string
}

// HandleCallback exchanges the code for a token, validates the ID token, and
// returns the extracted claims. The caller is responsible for user creation
// and JWT issuance.
func (s *OIDCService) HandleCallback(ctx context.Context, state, code string) (*OIDCCallbackResult, error) {
	var nonce string
	var redirectURL sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT nonce, redirect_url FROM oidc_session WHERE state = ?", state,
	).Scan(&nonce, &redirectURL)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid state parameter")
		}
		return nil, fmt.Errorf("lookup oidc session: %w", err)
	}

	// Exchange authorization code for tokens.
	oauth2Token, err := s.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}

	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify id token: %w", err)
	}

	if idToken.Nonce != nonce {
		return nil, fmt.Errorf("nonce mismatch")
	}

	var claims struct {
		Email  string   `json:"email"`
		Name   string   `json:"name"`
		Sub    string   `json:"sub"`
		Groups []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	// Clean up session.
	_, _ = s.db.ExecContext(ctx, "DELETE FROM oidc_session WHERE state = ?", state)

	result := &OIDCCallbackResult{
		Email:       claims.Email,
		Name:        claims.Name,
		Subject:     claims.Sub,
		Groups:      claims.Groups,
		RedirectURL: redirectURL.String,
	}
	return result, nil
}

// AuthCodeURL returns the OIDC authorization URL for the given state and nonce.
func (s *OIDCService) AuthCodeURL(state, nonce string) string {
	return s.oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce))
}

// CleanupOldSessions removes OIDC sessions older than the given duration.
func (s *OIDCService) CleanupOldSessions(ctx context.Context, maxAge time.Duration) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM oidc_session WHERE created_at < datetime('now', ?)",
		fmt.Sprintf("-%d seconds", int(maxAge.Seconds())))
	return err
}
