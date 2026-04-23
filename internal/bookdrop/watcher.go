package bookdrop

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/crueber/lexicon/internal/storage"
	"github.com/crueber/lexicon/internal/ws"
)

// stabilityDelay is the duration to wait for file stability.
const stabilityDelay = 5 * time.Second

// supportedFormats is the set of book file extensions we accept.
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

// fileWatch tracks the state of a file being watched for stability.
type fileWatch struct {
	path  string
	size  int64
	phase int // 0 = initial wait, 1 = size recorded, waiting to confirm
	timer *time.Timer
}

// Watcher watches a drop directory for new book files.
type Watcher struct {
	db        *sql.DB
	dropPath  string
	logger    *slog.Logger
	hub       *ws.Hub
	fsWatcher *fsnotify.Watcher
	mu        sync.Mutex
	watches   map[string]*fileWatch
}

// NewWatcher creates a new BookDrop watcher.
func NewWatcher(dropPath string, db *sql.DB, logger *slog.Logger, hub *ws.Hub) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		db:        db,
		dropPath:  dropPath,
		logger:    logger,
		hub:       hub,
		fsWatcher: fsw,
		watches:   make(map[string]*fileWatch),
	}

	return w, nil
}

// Start begins watching the drop directory. It blocks until ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	if err := w.fsWatcher.Add(w.dropPath); err != nil {
		w.logger.Warn("failed to watch bookdrop path", "path", w.dropPath, "error", err)
		// Still run the event loop so we can handle context cancellation.
	} else {
		w.logger.Info("watching bookdrop path", "path", w.dropPath)
	}

	// Perform an initial scan of the drop directory to catch files that
	// were placed there before the watcher started.
	w.ScanNow(ctx)

	w.runEventLoop(ctx)
	return nil
}

// ScanNow performs an immediate scan of the drop directory.
// It is used by the background BOOKDROP_SCAN task.
func (w *Watcher) ScanNow(ctx context.Context) {
	w.scanExistingFiles(ctx)
}

// scanExistingFiles walks the drop directory and processes any book files
// that are not already recorded.
func (w *Watcher) scanExistingFiles(ctx context.Context) {
	entries, err := os.ReadDir(w.dropPath)
	if err != nil {
		w.logger.Warn("failed to read bookdrop directory", "path", w.dropPath, "error", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(w.dropPath, entry.Name())
		if !w.isBookFile(path) {
			continue
		}
		if w.alreadyRecorded(ctx, path) {
			continue
		}
		w.processStableFile(ctx, path)
	}
}

// alreadyRecorded checks if a file path already exists in the bookdrop_file table.
func (w *Watcher) alreadyRecorded(ctx context.Context, path string) bool {
	q := New(w.db)
	count, err := q.CountBookdropFilesByPath(ctx, path)
	if err != nil {
		return false
	}
	return count > 0
}

// Stop stops the watcher.
func (w *Watcher) Stop() error {
	return w.fsWatcher.Close()
}

// runEventLoop processes filesystem events with per-file stability detection.
func (w *Watcher) runEventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.cancelAllTimers()
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			if event.Op == fsnotify.Chmod {
				continue
			}

			if event.Op&fsnotify.Create == 0 && event.Op&fsnotify.Write == 0 {
				continue
			}

			path := event.Name
			if !w.isBookFile(path) {
				continue
			}

			w.handleFileEvent(path)

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn("bookdrop filesystem watcher error", "error", err)
		}
	}
}

// handleFileEvent handles a filesystem event for a single file.
func (w *Watcher) handleFileEvent(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	fw, ok := w.watches[path]
	if ok {
		fw.timer.Stop()
	} else {
		fw = &fileWatch{path: path}
		w.watches[path] = fw
	}

	fw.phase = 0
	fw.timer = time.AfterFunc(stabilityDelay, func() {
		w.checkStability(path)
	})
}

// checkStability checks if a file has remained stable.
func (w *Watcher) checkStability(path string) {
	w.mu.Lock()
	fw, ok := w.watches[path]
	if !ok {
		w.mu.Unlock()
		return
	}

	if fw.phase == 0 {
		info, err := os.Stat(path)
		if err != nil {
			delete(w.watches, path)
			w.mu.Unlock()
			return
		}
		fw.size = info.Size()
		fw.phase = 1
		fw.timer = time.AfterFunc(stabilityDelay, func() {
			w.checkStability(path)
		})
		w.mu.Unlock()
		return
	}

	// Phase 1: verify size unchanged.
	info, err := os.Stat(path)
	if err != nil {
		delete(w.watches, path)
		w.mu.Unlock()
		return
	}

	if info.Size() != fw.size {
		fw.size = info.Size()
		fw.timer = time.AfterFunc(stabilityDelay, func() {
			w.checkStability(path)
		})
		w.mu.Unlock()
		return
	}

	delete(w.watches, path)
	w.mu.Unlock()

	w.processStableFile(context.Background(), path)
}

// cancelAllTimers stops all pending stability timers.
func (w *Watcher) cancelAllTimers() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, fw := range w.watches {
		fw.timer.Stop()
	}
	w.watches = make(map[string]*fileWatch)
}

// isBookFile reports whether the path has a supported book extension.
func (w *Watcher) isBookFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := supportedFormats[ext]
	return ok
}

// processStableFile extracts metadata and cover, inserts a database record,
// and broadcasts a WebSocket event.
func (w *Watcher) processStableFile(ctx context.Context, path string) {
	// Skip if already recorded.
	if w.alreadyRecorded(ctx, path) {
		w.logger.Debug("bookdrop file already recorded, skipping", "path", path)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		w.logger.Warn("failed to stat bookdrop file", "path", path, "error", err)
		return
	}

	filename := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	format := supportedFormats[ext]

	// Extract metadata.
	var extractedTitle, extractedAuthors sql.NullString
	meta, err := storage.ExtractMetadata(path, format)
	if err != nil {
		w.logger.Debug("bookdrop metadata extraction failed", "path", path, "error", err)
	} else if meta != nil {
		if meta.Title != "" {
			extractedTitle = sql.NullString{String: meta.Title, Valid: true}
		}
		if len(meta.Authors) > 0 {
			extractedAuthors = sql.NullString{String: strings.Join(meta.Authors, ", "), Valid: true}
		}
	}

	// Extract cover.
	var extractedCoverPath sql.NullString
	rawBytes, _, err := storage.ExtractCover(path, format)
	if err != nil {
		w.logger.Debug("bookdrop cover extraction failed", "path", path, "error", err)
	} else if rawBytes != nil {
		// Save the extracted cover to a temporary location within the drop path.
		coverName := strings.TrimSuffix(filename, ext) + ".jpg"
		coverPath := filepath.Join(w.dropPath, ".covers", coverName)
		if err := os.MkdirAll(filepath.Dir(coverPath), 0o750); err != nil {
			w.logger.Debug("failed to create bookdrop cover directory", "path", coverPath, "error", err)
		} else {
			if err := os.WriteFile(coverPath, rawBytes, 0o640); err != nil {
				w.logger.Debug("failed to write bookdrop cover", "path", coverPath, "error", err)
			} else {
				extractedCoverPath = sql.NullString{String: coverPath, Valid: true}
			}
		}
	}

	q := New(w.db)
	record, err := q.CreateBookdropFile(ctx, CreateBookdropFileParams{
		OriginalFilename:   filename,
		FilePath:           path,
		FileSize:           info.Size(),
		Status:             "PENDING",
		ExtractedTitle:     extractedTitle,
		ExtractedAuthors:   extractedAuthors,
		ExtractedCoverPath: extractedCoverPath,
	})
	if err != nil {
		w.logger.Error("failed to create bookdrop file record", "path", path, "error", err)
		return
	}

	w.logger.Info("bookdrop file arrived", "path", path, "id", record.ID)

	w.hub.BroadcastToAll(ws.Message{
		Type: "BOOKDROP_FILE_ARRIVED",
		Payload: map[string]any{
			"id":               record.ID,
			"originalFilename": record.OriginalFilename,
			"fileSize":         record.FileSize,
			"extractedTitle":   record.ExtractedTitle,
			"extractedAuthors": record.ExtractedAuthors,
		},
	})
}
