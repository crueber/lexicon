package server

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDatabase(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase(%q) error: %v", dataDir, err)
	}
	defer db.Close()

	// Verify the database file was created.
	dbPath := filepath.Join(dataDir, "lexicon.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("database file not created at %s", dbPath)
	}
}

func TestOpenDatabaseCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "nested", "data")

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase(%q) error: %v", dataDir, err)
	}
	defer db.Close()

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Errorf("data directory not created at %s", dataDir)
	}
}

func TestDatabasePragmas(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "journal_mode", query: "PRAGMA journal_mode", want: "wal"},
		{name: "busy_timeout", query: "PRAGMA busy_timeout", want: "5000"},
		{name: "foreign_keys", query: "PRAGMA foreign_keys", want: "1"},
		{name: "synchronous", query: "PRAGMA synchronous", want: "1"}, // NORMAL = 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if err := db.QueryRow(tt.query).Scan(&got); err != nil {
				t.Fatalf("query %q error: %v", tt.query, err)
			}
			if got != tt.want {
				t.Errorf("%s = %q; want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestRunMigrations(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	logger := newLogger("error", "text")

	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("RunMigrations error: %v", err)
	}

	// Verify tables exist by querying them.
	tables := []string{"users", "user_permissions", "user_settings", "refresh_tokens", "app_settings"}
	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			query := "SELECT count(*) FROM " + table
			var count int
			if err := db.QueryRow(query).Scan(&count); err != nil {
				t.Errorf("table %q not accessible: %v", table, err)
			}
		})
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	logger := newLogger("error", "text")

	// Run migrations twice — the second run should be a no-op.
	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("first RunMigrations error: %v", err)
	}
	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("second RunMigrations error: %v", err)
	}
}

func TestMigrationsCreateCorrectSchema(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	logger := newLogger("error", "text")
	if err := RunMigrations(db, logger); err != nil {
		t.Fatalf("RunMigrations error: %v", err)
	}

	ctx := context.Background()

	// Insert a user and verify the schema works end-to-end.
	result, err := db.ExecContext(ctx,
		"INSERT INTO users (username, email, name) VALUES (?, ?, ?)",
		"testuser", "test@example.com", "Test User",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}

	// Verify user was created.
	var username string
	if err := db.QueryRowContext(ctx, "SELECT username FROM users WHERE id = ?", userID).Scan(&username); err != nil {
		t.Fatalf("select user: %v", err)
	}
	if username != "testuser" {
		t.Errorf("username = %q; want %q", username, "testuser")
	}

	// Verify foreign key constraint works — insert permissions for the user.
	_, err = db.ExecContext(ctx,
		"INSERT INTO user_permissions (user_id, role) VALUES (?, ?)",
		userID, "ADMIN",
	)
	if err != nil {
		t.Fatalf("insert permissions: %v", err)
	}

	// Verify foreign key constraint rejects invalid user_id.
	_, err = db.ExecContext(ctx,
		"INSERT INTO user_permissions (user_id, role) VALUES (?, ?)",
		99999, "USER",
	)
	if err == nil {
		t.Error("expected foreign key violation for invalid user_id; got nil")
	}

	// Verify cascade delete — deleting user should delete permissions.
	_, err = db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var permCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM user_permissions WHERE user_id = ?", userID).Scan(&permCount); err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permCount != 0 {
		t.Errorf("permissions count after cascade delete = %d; want 0", permCount)
	}
}

func TestDatabasePing(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Errorf("db.Ping() error: %v", err)
	}
}

// openTestDB is a helper that opens a database in a temp directory and runs
// migrations. It returns the database and a cleanup function.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dataDir := t.TempDir()
	db, err := OpenDatabase(dataDir)
	if err != nil {
		t.Fatalf("OpenDatabase error: %v", err)
	}

	logger := newLogger("error", "text")
	if err := RunMigrations(db, logger); err != nil {
		db.Close()
		t.Fatalf("RunMigrations error: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}
