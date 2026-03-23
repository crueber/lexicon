package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	bookpkg "github.com/crueber/lexicon/internal/book"
	"github.com/crueber/lexicon/internal/storage"
)

// supportedFormats maps file extensions to their format strings.
var supportedFormats = map[string]string{
	".epub": "EPUB",
	".pdf":  "PDF",
	".cbz":  "CBZ",
	".cbr":  "CBR",
	".cb7":  "CB7",
	".mobi": "MOBI",
	".azw3": "AZW3",
	".fb2":  "FB2",
	".m4b":  "M4B",
	".m4a":  "M4A",
	".mp3":  "MP3",
	".opus": "OPUS",
}

// audioFormats is the set of extensions that are audio formats.
var audioFormats = map[string]bool{
	".m4b":  true,
	".m4a":  true,
	".mp3":  true,
	".opus": true,
}

// comicFormats is the set of extensions that are comic formats.
var comicFormats = map[string]bool{
	".cbz": true,
	".cbr": true,
	".cb7": true,
}

// bookTypeForExtension returns the book type for a given file extension.
func bookTypeForExtension(ext string) string {
	if comicFormats[ext] {
		return "COMIC"
	}
	if audioFormats[ext] {
		return "AUDIOBOOK"
	}
	return "EBOOK"
}

// bookTypeForFolder determines the book type for a folder based on the
// extensions of files it contains.
// - If ALL files are audio → AUDIOBOOK
// - If any file is a comic → COMIC
// - Otherwise → EBOOK
func bookTypeForFolder(exts []string) string {
	allAudio := true
	hasComic := false

	for _, ext := range exts {
		if !audioFormats[ext] {
			allAudio = false
		}
		if comicFormats[ext] {
			hasComic = true
		}
	}

	if allAudio && len(exts) > 0 {
		return "AUDIOBOOK"
	}
	if hasComic {
		return "COMIC"
	}
	return "EBOOK"
}

// ScanResult holds the outcome of a library scan.
type ScanResult struct {
	BooksAdded   int
	FilesAdded   int
	FilesUpdated int
	Errors       []error
}

// Scanner scans library directories and creates/updates book records.
type Scanner struct {
	db      *sql.DB
	dataDir string
	logger  *slog.Logger
}

// NewScanner creates a new Scanner.
func NewScanner(db *sql.DB, dataDir string, logger *slog.Logger) *Scanner {
	return &Scanner{
		db:      db,
		dataDir: dataDir,
		logger:  logger,
	}
}

// ScanLibrary scans all paths in a library and creates/updates book and
// book_file records. It respects context cancellation.
func (s *Scanner) ScanLibrary(ctx context.Context, lib Library, paths []LibraryPath) (*ScanResult, error) {
	result := &ScanResult{}

	for _, lp := range paths {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		s.logger.Info("scanning library path",
			"library_id", lib.ID,
			"path", lp.Path,
			"mode", lib.OrganizationMode,
		)

		var err error
		switch lib.OrganizationMode {
		case "BOOK_PER_FILE":
			err = s.scanBookPerFile(ctx, lib, lp.Path, result)
		case "BOOK_PER_FOLDER":
			err = s.scanBookPerFolder(ctx, lib, lp.Path, result)
		default:
			err = fmt.Errorf("unknown organization mode: %s", lib.OrganizationMode)
		}

		if err != nil {
			// A path-level error is collected but we continue with other paths.
			result.Errors = append(result.Errors, fmt.Errorf("scan path %q: %w", lp.Path, err))
		}
	}

	s.logger.Info("library scan complete",
		"library_id", lib.ID,
		"books_added", result.BooksAdded,
		"files_added", result.FilesAdded,
		"files_updated", result.FilesUpdated,
		"errors", len(result.Errors),
	)

	return result, nil
}

// scanBookPerFile walks a directory and creates one book per supported file.
func (s *Scanner) scanBookPerFile(ctx context.Context, lib Library, root string, result *ScanResult) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("walk %q: %w", path, err))
			return nil // continue walking
		}

		// Check for cancellation on each entry.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		format, ok := supportedFormats[ext]
		if !ok {
			return nil
		}

		s.logger.Debug("processing file", "path", path, "format", format)

		if err := s.processFileBookPerFile(ctx, lib, path, format, result); err != nil {
			s.logger.Warn("error processing file", "path", path, "error", err)
			result.Errors = append(result.Errors, fmt.Errorf("process file %q: %w", path, err))
		}

		return nil
	})
}

// processFileBookPerFile handles a single file in BOOK_PER_FILE mode.
func (s *Scanner) processFileBookPerFile(ctx context.Context, lib Library, path, format string, result *ScanResult) error {
	// Compute fingerprint.
	fp, err := storage.Fingerprint(path)
	if err != nil {
		return fmt.Errorf("fingerprint: %w", err)
	}

	bq := bookpkg.New(s.db)

	// Check if a book_file exists by path.
	existing, err := bq.GetBookFileByPath(ctx, path)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get book file by path: %w", err)
	}

	if err == nil {
		// File exists by path — check if fingerprint changed.
		if !existing.Fingerprint.Valid || existing.Fingerprint.String != fp {
			info, statErr := statFile(path)
			if statErr != nil {
				return fmt.Errorf("stat file: %w", statErr)
			}
			if updateErr := bq.UpdateBookFileFingerprint(ctx, bookpkg.UpdateBookFileFingerprintParams{
				Fingerprint: sql.NullString{String: fp, Valid: true},
				FileSize:    sql.NullInt64{Int64: info, Valid: true},
				ID:          existing.ID,
			}); updateErr != nil {
				return fmt.Errorf("update fingerprint: %w", updateErr)
			}
			result.FilesUpdated++
			s.logger.Debug("updated fingerprint", "path", path)
		}
		return nil
	}

	// File not found by path — check by fingerprint (file may have moved).
	byFP, err := bq.GetBookFileByFingerprint(ctx, sql.NullString{String: fp, Valid: true})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get book file by fingerprint: %w", err)
	}

	if err == nil {
		// File was moved — update the path.
		if updateErr := bq.UpdateBookFilePath(ctx, bookpkg.UpdateBookFilePathParams{
			FilePath: path,
			ID:       byFP.ID,
		}); updateErr != nil {
			return fmt.Errorf("update file path: %w", updateErr)
		}
		result.FilesUpdated++
		s.logger.Debug("detected file move", "old_path", byFP.FilePath, "new_path", path)
		return nil
	}

	// New file — create book + book_file + book_metadata in a transaction.
	bookType := bookTypeForExtension(strings.ToLower(filepath.Ext(path)))
	if err := s.createBookWithFile(ctx, lib.ID, path, format, fp, bookType, sql.NullInt64{}, result); err != nil {
		return err
	}

	return nil
}

// scanBookPerFolder walks a directory and groups files by their parent folder.
func (s *Scanner) scanBookPerFolder(ctx context.Context, lib Library, root string, result *ScanResult) error {
	// Collect all supported files grouped by directory.
	dirFiles := make(map[string][]string) // dir → []filePath

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.Errors = append(result.Errors, fmt.Errorf("walk %q: %w", path, walkErr))
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := supportedFormats[ext]; !ok {
			return nil
		}

		dir := filepath.Dir(path)
		dirFiles[dir] = append(dirFiles[dir], path)
		return nil
	})
	if err != nil {
		return err
	}

	// Process each directory.
	for dir, files := range dirFiles {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sort.Strings(files) // deterministic ordering for track numbers

		if err := s.processFolderBookPerFolder(ctx, lib, dir, files, result); err != nil {
			s.logger.Warn("error processing folder", "dir", dir, "error", err)
			result.Errors = append(result.Errors, fmt.Errorf("process folder %q: %w", dir, err))
		}
	}

	return nil
}

// processFolderBookPerFolder handles a single directory in BOOK_PER_FOLDER mode.
func (s *Scanner) processFolderBookPerFolder(ctx context.Context, lib Library, dir string, files []string, result *ScanResult) error {
	bq := bookpkg.New(s.db)

	// Determine book type from the files in this folder.
	exts := make([]string, len(files))
	for i, f := range files {
		exts[i] = strings.ToLower(filepath.Ext(f))
	}
	bookType := bookTypeForFolder(exts)

	// Check if a book already exists for this folder in this library.
	existingBook, err := bq.GetBookByFolderPath(ctx, bookpkg.GetBookByFolderPathParams{
		LibraryID:  lib.ID,
		FolderPath: sql.NullString{String: dir, Valid: true},
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get book by folder path: %w", err)
	}

	var bookID int64
	if errors.Is(err, sql.ErrNoRows) {
		// New folder — create a book record.
		tx, txErr := s.db.BeginTx(ctx, nil)
		if txErr != nil {
			return fmt.Errorf("begin transaction: %w", txErr)
		}
		defer tx.Rollback() //nolint:errcheck

		tq := bookpkg.New(tx)
		newBook, createErr := tq.CreateBook(ctx, bookpkg.CreateBookParams{
			LibraryID:  lib.ID,
			FolderPath: sql.NullString{String: dir, Valid: true},
			BookType:   bookType,
		})
		if createErr != nil {
			return fmt.Errorf("create book: %w", createErr)
		}

		if metaErr := tq.UpsertBookMetadata(ctx, bookpkg.UpsertBookMetadataParams{
			BookID: newBook.ID,
		}); metaErr != nil {
			return fmt.Errorf("create book metadata: %w", metaErr)
		}

		if commitErr := tx.Commit(); commitErr != nil {
			return fmt.Errorf("commit transaction: %w", commitErr)
		}

		bookID = newBook.ID
		result.BooksAdded++
		s.logger.Debug("created book for folder", "dir", dir, "book_id", bookID)

		// Extract cover from the first file in the folder (non-fatal).
		if len(files) > 0 {
			firstExt := strings.ToLower(filepath.Ext(files[0]))
			firstFormat := supportedFormats[firstExt]
			s.extractAndSaveCover(ctx, bookID, files[0], firstFormat, bookType)
		}
	} else {
		bookID = existingBook.ID
	}

	// Process each file in the folder.
	for i, filePath := range files {
		ext := strings.ToLower(filepath.Ext(filePath))
		format := supportedFormats[ext]

		fp, fpErr := storage.Fingerprint(filePath)
		if fpErr != nil {
			s.logger.Warn("fingerprint error", "path", filePath, "error", fpErr)
			result.Errors = append(result.Errors, fmt.Errorf("fingerprint %q: %w", filePath, fpErr))
			continue
		}

		// Check if this file already exists.
		existingFile, fileErr := bq.GetBookFileByPath(ctx, filePath)
		if fileErr != nil && !errors.Is(fileErr, sql.ErrNoRows) {
			result.Errors = append(result.Errors, fmt.Errorf("get book file by path %q: %w", filePath, fileErr))
			continue
		}

		if fileErr == nil {
			// File exists — check fingerprint.
			if !existingFile.Fingerprint.Valid || existingFile.Fingerprint.String != fp {
				info, statErr := statFile(filePath)
				if statErr != nil {
					result.Errors = append(result.Errors, fmt.Errorf("stat file %q: %w", filePath, statErr))
					continue
				}
				if updateErr := bq.UpdateBookFileFingerprint(ctx, bookpkg.UpdateBookFileFingerprintParams{
					Fingerprint: sql.NullString{String: fp, Valid: true},
					FileSize:    sql.NullInt64{Int64: info, Valid: true},
					ID:          existingFile.ID,
				}); updateErr != nil {
					result.Errors = append(result.Errors, fmt.Errorf("update fingerprint %q: %w", filePath, updateErr))
					continue
				}
				result.FilesUpdated++
			}
			continue
		}

		// Check by fingerprint (file may have moved).
		byFP, fpLookupErr := bq.GetBookFileByFingerprint(ctx, sql.NullString{String: fp, Valid: true})
		if fpLookupErr != nil && !errors.Is(fpLookupErr, sql.ErrNoRows) {
			result.Errors = append(result.Errors, fmt.Errorf("get book file by fingerprint %q: %w", filePath, fpLookupErr))
			continue
		}

		if fpLookupErr == nil {
			// File was moved — update path.
			if updateErr := bq.UpdateBookFilePath(ctx, bookpkg.UpdateBookFilePathParams{
				FilePath: filePath,
				ID:       byFP.ID,
			}); updateErr != nil {
				result.Errors = append(result.Errors, fmt.Errorf("update file path %q: %w", filePath, updateErr))
				continue
			}
			result.FilesUpdated++
			continue
		}

		// New file — add to the existing book.
		trackNum := sql.NullInt64{}
		if bookType == "AUDIOBOOK" {
			trackNum = sql.NullInt64{Int64: int64(i + 1), Valid: true}
		}

		info, statErr := statFile(filePath)
		if statErr != nil {
			result.Errors = append(result.Errors, fmt.Errorf("stat file %q: %w", filePath, statErr))
			continue
		}

		if _, createErr := bq.CreateBookFile(ctx, bookpkg.CreateBookFileParams{
			BookID:      bookID,
			FilePath:    filePath,
			Format:      format,
			FileSize:    sql.NullInt64{Int64: info, Valid: true},
			Fingerprint: sql.NullString{String: fp, Valid: true},
			TrackNumber: trackNum,
		}); createErr != nil {
			result.Errors = append(result.Errors, fmt.Errorf("create book file %q: %w", filePath, createErr))
			continue
		}

		result.FilesAdded++
		s.logger.Debug("added file to folder book", "path", filePath, "book_id", bookID)
	}

	return nil
}

// createBookWithFile creates a book, book_file, and book_metadata in a single
// transaction. Used by BOOK_PER_FILE mode.
func (s *Scanner) createBookWithFile(ctx context.Context, libraryID int64, path, format, fp, bookType string, trackNum sql.NullInt64, result *ScanResult) error {
	info, err := statFile(path)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	tq := bookpkg.New(tx)

	book, err := tq.CreateBook(ctx, bookpkg.CreateBookParams{
		LibraryID: libraryID,
		BookType:  bookType,
	})
	if err != nil {
		return fmt.Errorf("create book: %w", err)
	}

	if _, err := tq.CreateBookFile(ctx, bookpkg.CreateBookFileParams{
		BookID:      book.ID,
		FilePath:    path,
		Format:      format,
		FileSize:    sql.NullInt64{Int64: info, Valid: true},
		Fingerprint: sql.NullString{String: fp, Valid: true},
		TrackNumber: trackNum,
	}); err != nil {
		return fmt.Errorf("create book file: %w", err)
	}

	if err := tq.UpsertBookMetadata(ctx, bookpkg.UpsertBookMetadataParams{
		BookID: book.ID,
	}); err != nil {
		return fmt.Errorf("create book metadata: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	result.BooksAdded++
	result.FilesAdded++
	s.logger.Debug("created book", "path", path, "book_id", book.ID, "type", bookType)

	// Extract and save cover (non-fatal).
	s.extractAndSaveCover(ctx, book.ID, path, format, bookType)

	return nil
}

// extractAndSaveCover attempts to extract a cover from the given file and save
// it to the data directory. Failures are logged at Debug level and do not
// propagate — cover extraction is best-effort.
func (s *Scanner) extractAndSaveCover(ctx context.Context, bookID int64, filePath, format, bookType string) {
	if s.dataDir == "" {
		return
	}

	rawBytes, _, err := storage.ExtractCover(filePath, format)
	if err != nil {
		s.logger.Debug("cover extraction failed", "path", filePath, "format", format, "error", err)
		return
	}
	if rawBytes == nil {
		return
	}

	isAudio := bookType == "AUDIOBOOK"
	coverPath, err := storage.ProcessCover(rawBytes, bookID, s.dataDir, isAudio)
	if err != nil {
		s.logger.Debug("cover processing failed", "path", filePath, "book_id", bookID, "error", err)
		return
	}

	bq := bookpkg.New(s.db)
	if err := bq.UpdateBookCover(ctx, bookpkg.UpdateBookCoverParams{
		CoverPath: sql.NullString{String: coverPath, Valid: true},
		BookID:    bookID,
	}); err != nil {
		s.logger.Debug("update book cover failed", "book_id", bookID, "error", err)
	}
}

// statFile returns the file size in bytes.
func statFile(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
