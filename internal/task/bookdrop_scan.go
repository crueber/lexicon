package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/crueber/lexicon/internal/bookdrop"
	"github.com/crueber/lexicon/internal/ws"
)

// NewBookdropScanFunc returns a TaskFunc that scans the bookdrop directory
// for any files not yet recorded and processes them.
func NewBookdropScanFunc(dropPath string, db *sql.DB, hub *ws.Hub, logger *slog.Logger) TaskFunc {
	return func(ctx context.Context, payload string, reporter Reporter) error {
		w, err := bookdrop.NewWatcher(dropPath, db, logger, hub)
		if err != nil {
			return fmt.Errorf("create bookdrop watcher: %w", err)
		}
		defer w.Stop() //nolint:errcheck

		w.ScanNow(ctx)
		return nil
	}
}
