package server

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/crueber/lexicon/migrations"
)

// RunMigrations applies all pending database migrations. It uses embedded
// migration files and logs the before/after version.
func RunMigrations(db *sql.DB, logger *slog.Logger) error {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	version, dirty, _ := m.Version()
	logger.Info("migration status before",
		"version", version,
		"dirty", dirty,
	)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}

	version, dirty, _ = m.Version()
	logger.Info("migration status after",
		"version", version,
		"dirty", dirty,
	)

	return nil
}
