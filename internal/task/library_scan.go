package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/crueber/lexicon/internal/auth"
	"github.com/crueber/lexicon/internal/library"
)

// libraryScanDeps holds the dependencies needed for the LIBRARY_SCAN task.
type libraryScanDeps struct {
	db      *sql.DB
	svc     *library.Service
	scanner *library.Scanner
	logger  *slog.Logger
}

// adminPrincipal is a synthetic admin principal used for background tasks.
var adminPrincipal = &auth.Principal{
	UserID:   0,
	Username: "system",
	Role:     "ADMIN",
}

// NewLibraryScanFunc returns a TaskFunc that scans one or all libraries.
// It is registered with the Runner under TypeLibraryScan.
func NewLibraryScanFunc(db *sql.DB, svc *library.Service, scanner *library.Scanner, logger *slog.Logger) TaskFunc {
	return newLibraryScanFunc(libraryScanDeps{
		db:      db,
		svc:     svc,
		scanner: scanner,
		logger:  logger,
	})
}

// newLibraryScanFunc returns a TaskFunc that scans one or all libraries.
func newLibraryScanFunc(deps libraryScanDeps) TaskFunc {
	return func(ctx context.Context, payload string, reporter Reporter) error {
		libraryID, err := ParseLibraryScanPayload(payload)
		if err != nil {
			return fmt.Errorf("parse payload: %w", err)
		}

		// Build the list of libraries to scan.
		var libs []library.Library
		if libraryID != nil {
			lib, err := deps.svc.GetByID(ctx, *libraryID, adminPrincipal)
			if err != nil {
				return fmt.Errorf("get library %d: %w", *libraryID, err)
			}
			libs = []library.Library{*lib}
		} else {
			all, err := deps.svc.ListForUser(ctx, adminPrincipal)
			if err != nil {
				return fmt.Errorf("list libraries: %w", err)
			}
			libs = all
		}

		total := len(libs)
		for i, lib := range libs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			reporter.Progress(i, total, fmt.Sprintf("Scanning library: %s", lib.Name))

			paths, err := deps.svc.ListPaths(ctx, lib.ID)
			if err != nil {
				deps.logger.Error("list paths for library scan task",
					"library_id", lib.ID,
					"error", err,
				)
				continue
			}

			result, err := deps.scanner.ScanLibrary(ctx, lib, paths)
			if err != nil {
				deps.logger.Error("scan library task",
					"library_id", lib.ID,
					"error", err,
				)
				continue
			}

			deps.logger.Info("library scan task complete",
				"library_id", lib.ID,
				"books_added", result.BooksAdded,
				"files_added", result.FilesAdded,
				"files_updated", result.FilesUpdated,
				"errors", len(result.Errors),
			)
		}

		reporter.Progress(total, total, "Scan complete")
		return nil
	}
}
