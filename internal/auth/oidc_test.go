package auth

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// applyOIDCMigration adds the oidc_session table to a test database.
func applyOIDCMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "migrations", "021_oidc.up.sql"))
	if err != nil {
		t.Fatalf("read oidc migration: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply oidc migration: %v", err)
	}
}

func TestOIDCService_GenerateState(t *testing.T) {
	db := openTestDB(t)
	applyOIDCMigration(t, db)
	svc := &OIDCService{db: db}

	state, nonce, err := svc.GenerateState("/dashboard")
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}

	if state == "" {
		t.Error("GenerateState() returned empty state")
	}
	if nonce == "" {
		t.Error("GenerateState() returned empty nonce")
	}

	// Verify the session was stored.
	var storedNonce string
	var redirectURL sql.NullString
	err = db.QueryRowContext(context.Background(),
		"SELECT nonce, redirect_url FROM oidc_session WHERE state = ?", state,
	).Scan(&storedNonce, &redirectURL)
	if err != nil {
		t.Fatalf("lookup stored session: %v", err)
	}

	if storedNonce != nonce {
		t.Errorf("stored nonce = %q; want %q", storedNonce, nonce)
	}
	if redirectURL.String != "/dashboard" {
		t.Errorf("stored redirect_url = %q; want %q", redirectURL.String, "/dashboard")
	}
}

func TestOIDCService_GenerateState_Unique(t *testing.T) {
	db := openTestDB(t)
	applyOIDCMigration(t, db)
	svc := &OIDCService{db: db}

	state1, _, err := svc.GenerateState("")
	if err != nil {
		t.Fatalf("first GenerateState() error: %v", err)
	}

	state2, _, err := svc.GenerateState("")
	if err != nil {
		t.Fatalf("second GenerateState() error: %v", err)
	}

	if state1 == state2 {
		t.Error("two GenerateState() calls returned same state")
	}
}

func TestOIDCService_HandleCallback_InvalidState(t *testing.T) {
	db := openTestDB(t)
	applyOIDCMigration(t, db)
	svc := &OIDCService{db: db}

	_, err := svc.HandleCallback(context.Background(), "invalid-state", "code")
	if err == nil {
		t.Fatal("HandleCallback() with invalid state should return error")
	}

	if err.Error() != "invalid state parameter" {
		t.Errorf("error = %q; want %q", err.Error(), "invalid state parameter")
	}
}
