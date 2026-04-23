package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/crueber/lexicon/internal/book"
)

// NewDuplicateDetectionFunc returns a TaskFunc that runs duplicate detection.
func NewDuplicateDetectionFunc(db *sql.DB, logger *slog.Logger) TaskFunc {
	return func(ctx context.Context, payload string, reporter Reporter) error {
		reporter.Progress(0, 1, "finding duplicates")
		preset := book.DuplicatePreset(payload)
		if preset == "" {
			preset = book.PresetModerate
		}
		groups, err := book.FindDuplicates(ctx, db, preset)
		if err != nil {
			return fmt.Errorf("find duplicates: %w", err)
		}
		reporter.Progress(1, 1, fmt.Sprintf("found %d duplicate groups", len(groups)))
		return nil
	}
}
