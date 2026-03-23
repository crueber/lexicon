package library

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/crueber/lexicon/internal/ws"
)

// defaultDebounce is the time to wait after the last filesystem event before
// triggering a scan. This prevents multiple scans for rapid file operations.
const defaultDebounce = 5 * time.Second

// Watcher watches library paths for filesystem changes and triggers scans.
type Watcher struct {
	db        *sql.DB
	scanner   *Scanner
	hub       *ws.Hub
	logger    *slog.Logger
	fsWatcher *fsnotify.Watcher
	mu        sync.Mutex
	watched   map[string]struct{} // paths currently being watched
	debounce  time.Duration       // configurable for tests
}

// WatcherOption configures a Watcher.
type WatcherOption func(*Watcher)

// WithDebounce sets the debounce duration for the watcher.
// This is primarily used in tests to reduce the debounce time.
func WithDebounce(d time.Duration) WatcherOption {
	return func(w *Watcher) {
		w.debounce = d
	}
}

// NewWatcher creates a new Watcher.
func NewWatcher(db *sql.DB, scanner *Scanner, hub *ws.Hub, logger *slog.Logger, opts ...WatcherOption) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		db:        db,
		scanner:   scanner,
		hub:       hub,
		logger:    logger,
		fsWatcher: fsw,
		watched:   make(map[string]struct{}),
		debounce:  defaultDebounce,
	}

	for _, opt := range opts {
		opt(w)
	}

	return w, nil
}

// Start begins watching all library paths from the database.
// It blocks until ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	// Load all library paths from the database and start watching them.
	if err := w.loadAndWatchAll(ctx); err != nil {
		w.logger.Warn("failed to load library paths for watching", "error", err)
		// Non-fatal: we still run the event loop.
	}

	w.runEventLoop(ctx)
	return nil
}

// loadAndWatchAll loads all library paths from the database and adds them to
// the watcher.
func (w *Watcher) loadAndWatchAll(ctx context.Context) error {
	q := New(w.db)

	libs, err := q.ListLibraries(ctx)
	if err != nil {
		return err
	}

	for _, lib := range libs {
		paths, err := q.ListLibraryPaths(ctx, lib.ID)
		if err != nil {
			w.logger.Warn("failed to list paths for library",
				"library_id", lib.ID,
				"error", err,
			)
			continue
		}

		for _, lp := range paths {
			if err := w.AddPath(lp.Path); err != nil {
				w.logger.Warn("failed to watch path",
					"path", lp.Path,
					"error", err,
				)
			}
		}
	}

	return nil
}

// AddPath adds a path to the watch list.
func (w *Watcher) AddPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.watched[path]; ok {
		return nil // already watching
	}

	if err := w.fsWatcher.Add(path); err != nil {
		return err
	}

	w.watched[path] = struct{}{}
	w.logger.Debug("watching path", "path", path)
	return nil
}

// RemovePath removes a path from the watch list.
func (w *Watcher) RemovePath(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.watched[path]; !ok {
		return
	}

	if err := w.fsWatcher.Remove(path); err != nil {
		w.logger.Warn("failed to remove watch", "path", path, "error", err)
	}

	delete(w.watched, path)
	w.logger.Debug("stopped watching path", "path", path)
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	return w.fsWatcher.Close()
}

// runEventLoop processes filesystem events with per-directory debouncing.
func (w *Watcher) runEventLoop(ctx context.Context) {
	// dirTimers maps a directory path to its pending debounce timer.
	dirTimers := make(map[string]*time.Timer)

	for {
		select {
		case <-ctx.Done():
			// Cancel all pending timers.
			for _, t := range dirTimers {
				t.Stop()
			}
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			// Ignore chmod events — they don't indicate new content.
			if event.Op == fsnotify.Chmod {
				continue
			}

			dir := filepath.Dir(event.Name)

			// Reset or create the debounce timer for this directory.
			if t, exists := dirTimers[dir]; exists {
				t.Stop()
			}

			// Capture dir for the closure.
			capturedDir := dir
			dirTimers[dir] = time.AfterFunc(w.debounce, func() {
				w.handleDirChange(ctx, capturedDir)
			})

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn("filesystem watcher error", "error", err)
		}
	}
}

// handleDirChange finds the library for the changed directory and triggers a scan.
func (w *Watcher) handleDirChange(ctx context.Context, dir string) {
	q := New(w.db)

	libs, err := q.ListLibraries(ctx)
	if err != nil {
		w.logger.Error("list libraries for scan", "error", err)
		return
	}

	for _, lib := range libs {
		paths, err := q.ListLibraryPaths(ctx, lib.ID)
		if err != nil {
			w.logger.Warn("list paths for library", "library_id", lib.ID, "error", err)
			continue
		}

		// Check if the changed directory is under any of this library's paths.
		for _, lp := range paths {
			if isUnderPath(dir, lp.Path) {
				w.logger.Info("filesystem change detected, scanning library",
					"library_id", lib.ID,
					"changed_dir", dir,
					"library_path", lp.Path,
				)

				result, err := w.scanner.ScanLibrary(ctx, lib, paths)
				if err != nil {
					w.logger.Error("scan library after filesystem change",
						"library_id", lib.ID,
						"error", err,
					)
					return
				}

				// Broadcast scan complete event to all users.
				w.hub.BroadcastToAll(ws.Message{
					Type: "LIBRARY_SCAN_COMPLETE",
					Payload: map[string]any{
						"libraryId":  lib.ID,
						"booksAdded": result.BooksAdded,
						"filesAdded": result.FilesAdded,
					},
				})

				// Broadcast individual BOOK_ADDED events if books were added.
				if result.BooksAdded > 0 {
					w.hub.BroadcastToAll(ws.Message{
						Type: "BOOK_ADDED",
						Payload: map[string]any{
							"libraryId":  lib.ID,
							"booksAdded": result.BooksAdded,
						},
					})
				}

				// Only scan once per library even if multiple paths match.
				return
			}
		}
	}
}

// isUnderPath reports whether dir is equal to or a subdirectory of root.
func isUnderPath(dir, root string) bool {
	// Normalize both paths.
	dir = filepath.Clean(dir)
	root = filepath.Clean(root)

	if dir == root {
		return true
	}

	// Check if dir starts with root + separator.
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}

	// If rel starts with "..", dir is not under root.
	if len(rel) >= 2 && rel[:2] == ".." {
		return false
	}

	return true
}
