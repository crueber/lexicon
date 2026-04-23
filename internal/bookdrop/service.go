package bookdrop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/crueber/lexicon/internal/library"
)

// Service provides business logic for BookDrop file management.
type Service struct {
	db      *sql.DB
	scanner *library.Scanner
	logger  *slog.Logger
}

// NewService creates a new bookdrop Service.
func NewService(db *sql.DB, scanner *library.Scanner, logger *slog.Logger) *Service {
	return &Service{
		db:      db,
		scanner: scanner,
		logger:  logger,
	}
}

// ImportFile copies a bookdrop file into a library and creates a book record.
func (s *Service) ImportFile(ctx context.Context, id, targetLibraryID int64) (int64, error) {
	q := New(s.db)

	file, err := q.GetBookdropFileByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("bookdrop file not found")
		}
		return 0, fmt.Errorf("get bookdrop file: %w", err)
	}

	if file.Status != "PENDING" {
		return 0, fmt.Errorf("bookdrop file is not pending")
	}

	// Find library paths.
	lq := library.New(s.db)
	paths, err := lq.ListLibraryPaths(ctx, targetLibraryID)
	if err != nil {
		return 0, fmt.Errorf("list library paths: %w", err)
	}
	if len(paths) == 0 {
		return 0, fmt.Errorf("library has no paths")
	}

	libPath := paths[0].Path
	destPath := filepath.Join(libPath, filepath.Base(file.FilePath))

	// If a file already exists at the destination, append a number.
	destPath = uniqueFilePath(destPath)

	if err := copyFile(file.FilePath, destPath); err != nil {
		return 0, fmt.Errorf("copy file to library: %w", err)
	}

	// Run scanner logic on the copied file.
	bookID, err := s.scanner.ScanSingleFile(ctx, targetLibraryID, destPath)
	if err != nil {
		// Don't delete the copied file on scan error — admin can clean up manually.
		return 0, fmt.Errorf("scan imported file: %w", err)
	}

	// Update bookdrop record.
	if err := q.UpdateBookdropFileStatus(ctx, UpdateBookdropFileStatusParams{
		Status:         "IMPORTED",
		ImportedBookID: sql.NullInt64{Int64: bookID, Valid: true},
		ID:             id,
	}); err != nil {
		return 0, fmt.Errorf("update bookdrop status: %w", err)
	}

	s.logger.Info("bookdrop file imported",
		"bookdrop_id", id,
		"book_id", bookID,
		"library_id", targetLibraryID,
		"dest_path", destPath,
	)

	return bookID, nil
}

// RejectFile deletes a bookdrop file from disk and updates its status.
func (s *Service) RejectFile(ctx context.Context, id int64) error {
	q := New(s.db)

	file, err := q.GetBookdropFileByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("bookdrop file not found")
		}
		return fmt.Errorf("get bookdrop file: %w", err)
	}

	if file.Status != "PENDING" {
		return fmt.Errorf("bookdrop file is not pending")
	}

	// Delete the file from disk.
	if err := os.Remove(file.FilePath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("failed to delete bookdrop file", "path", file.FilePath, "error", err)
	}

	// Delete any extracted cover.
	if file.ExtractedCoverPath.Valid {
		if err := os.Remove(file.ExtractedCoverPath.String); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("failed to delete bookdrop cover", "path", file.ExtractedCoverPath.String, "error", err)
		}
	}

	if err := q.UpdateBookdropFileStatus(ctx, UpdateBookdropFileStatusParams{
		Status:         "REJECTED",
		ImportedBookID: sql.NullInt64{},
		ID:             id,
	}); err != nil {
		return fmt.Errorf("update bookdrop status: %w", err)
	}

	s.logger.Info("bookdrop file rejected", "bookdrop_id", id, "path", file.FilePath)
	return nil
}

// ListPending returns all PENDING bookdrop files.
func (s *Service) ListPending(ctx context.Context) ([]BookdropFile, error) {
	q := New(s.db)
	return q.ListPendingBookdropFiles(ctx)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}

// uniqueFilePath returns a path that does not already exist by appending a number.
func uniqueFilePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)

	for i := 1; i < 10000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}

	return path
}
