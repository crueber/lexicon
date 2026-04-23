package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/crueber/lexicon/internal/book"
)

var filenameSanitizer = regexp.MustCompile(`[<>:"/\\|?*]`)

// NewFileOrganizationFunc returns a TaskFunc that renames book files according to a pattern.
func NewFileOrganizationFunc(db *sql.DB, logger *slog.Logger) TaskFunc {
	return func(ctx context.Context, payload string, reporter Reporter) error {
		q := book.New(db)

		// Fetch all books with files, metadata, and authors.
		rows, err := q.ListBooksWithFilesAndMetadata(ctx)
		if err != nil {
			return fmt.Errorf("list books: %w", err)
		}

		reporter.Progress(0, len(rows), "starting file organization")

		for i, b := range rows {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Skip if no file.
			if !b.FilePath.Valid || b.FilePath.String == "" {
				continue
			}

			// Skip if library path is missing.
			if b.LibraryPath == "" {
				logger.Warn("missing library path for book", "book_id", b.BookID)
				continue
			}

			// Get authors as comma-separated string.
			authorStr := ""
			if b.AuthorNames != "" {
				authors := strings.Split(b.AuthorNames, "|")
				for j := range authors {
					authors[j] = strings.TrimSpace(authors[j])
				}
				authorStr = strings.Join(authors, ", ")
			}
			if authorStr == "" {
				authorStr = "Unknown"
			}

			// Determine file extension.
			ext := filepath.Ext(b.FilePath.String)

			// Build new path from pattern.
			pattern := payload
			if pattern == "" {
				pattern = "{author}/{title}{ext}"
			}

			title := ""
			if b.Title.Valid {
				title = b.Title.String
			}
			series := ""
			if b.SeriesName.Valid {
				series = b.SeriesName.String
			}
			var seriesIndex float64
			if b.SeriesNumber.Valid {
				seriesIndex = b.SeriesNumber.Float64
			}

			relPath := applyPattern(pattern, title, authorStr, series, seriesIndex, ext)
			if relPath == "" || relPath == b.FilePath.String {
				continue
			}

			newPath := filepath.Join(b.LibraryPath, relPath)
			if newPath == b.FilePath.String {
				continue
			}

			// Prevent overwriting existing files.
			if _, err := os.Stat(newPath); err == nil {
				logger.Warn("organization target already exists, skipping", "path", newPath)
				continue
			}

			// Create target directory.
			newDir := filepath.Dir(newPath)
			if err := os.MkdirAll(newDir, 0755); err != nil {
				logger.Warn("create directory for organization", "dir", newDir, "error", err)
				continue
			}

			// Move file.
			if err := os.Rename(b.FilePath.String, newPath); err != nil {
				logger.Warn("rename file", "from", b.FilePath.String, "to", newPath, "error", err)
				continue
			}

			// Update database record.
			if err := q.UpdateBookFilePath(ctx, book.UpdateBookFilePathParams{
				FilePath: newPath,
				ID:       b.FileID.Int64,
			}); err != nil {
				logger.Warn("update file path in db", "file_id", b.FileID.Int64, "error", err)
				// Try to move back.
				_ = os.Rename(newPath, b.FilePath.String)
				continue
			}

			reporter.Progress(i+1, len(rows), fmt.Sprintf("organized %s", title))
		}

		return nil
	}
}

// applyPattern substitutes tokens in a pattern string.
// Supported tokens: {title}, {author}, {series}, {series_index}, {ext}
func applyPattern(pattern, title, author, series string, seriesIndex float64, ext string) string {
	// Sanitize filename components.
	sanitize := func(s string) string {
		s = filenameSanitizer.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
		return s
	}

	result := pattern
	result = strings.ReplaceAll(result, "{title}", sanitize(title))
	result = strings.ReplaceAll(result, "{author}", sanitize(author))
	result = strings.ReplaceAll(result, "{series}", sanitize(series))

	seriesIndexStr := ""
	if seriesIndex > 0 {
		seriesIndexStr = fmt.Sprintf("%.1f", seriesIndex)
		seriesIndexStr = strings.TrimRight(seriesIndexStr, ".0")
	}
	result = strings.ReplaceAll(result, "{series_index}", seriesIndexStr)
	result = strings.ReplaceAll(result, "{ext}", ext)

	return result
}
