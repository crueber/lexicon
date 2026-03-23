package server

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// OpenDatabase opens a SQLite database at the given data directory path,
// creates the directory if it doesn't exist, and configures connection pragmas
// for WAL mode, busy timeout, foreign keys, and synchronous mode.
func OpenDatabase(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "lexicon.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Limit to a single connection to ensure pragmas (especially foreign_keys)
	// are consistently applied. database/sql may create new connections that
	// skip pragma setup. This serializes all queries but is acceptable for
	// our workload. TODO: revisit with a ConnInitHook or DSN pragma params
	// when concurrent read performance matters (Phase 07+).
	db.SetMaxOpenConns(1)

	if err := setPragmas(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}

	return db, nil
}

// setPragmas configures SQLite connection pragmas for optimal performance
// and correctness.
func setPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("%s: %w", pragma, err)
		}
	}

	return nil
}
