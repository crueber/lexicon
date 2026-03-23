package library

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crueber/lexicon/internal/ws"
)

func TestIsUnderPath(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		root string
		want bool
	}{
		{
			name: "exact match",
			dir:  "/books",
			root: "/books",
			want: true,
		},
		{
			name: "subdirectory",
			dir:  "/books/fiction",
			root: "/books",
			want: true,
		},
		{
			name: "deep subdirectory",
			dir:  "/books/fiction/scifi",
			root: "/books",
			want: true,
		},
		{
			name: "sibling directory",
			dir:  "/music",
			root: "/books",
			want: false,
		},
		{
			name: "parent directory",
			dir:  "/",
			root: "/books",
			want: false,
		},
		{
			name: "prefix but not subdir",
			dir:  "/bookshelves",
			root: "/books",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUnderPath(tt.dir, tt.root)
			if got != tt.want {
				t.Errorf("isUnderPath(%q, %q) = %v; want %v", tt.dir, tt.root, got, tt.want)
			}
		})
	}
}

func TestWatcher_AddRemovePath(t *testing.T) {
	// Create a temp directory to watch.
	dir := t.TempDir()

	hub := ws.NewHub(slog.Default())
	w, err := NewWatcher(nil, nil, hub, slog.Default())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	// Add the path.
	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}

	w.mu.Lock()
	_, ok := w.watched[dir]
	w.mu.Unlock()
	if !ok {
		t.Error("path should be in watched set after AddPath")
	}

	// Adding the same path again should be a no-op.
	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath (duplicate): %v", err)
	}

	// Remove the path.
	w.RemovePath(dir)

	w.mu.Lock()
	_, ok = w.watched[dir]
	w.mu.Unlock()
	if ok {
		t.Error("path should not be in watched set after RemovePath")
	}
}

// TestWatcher_FileChangeTriggersEvent tests that creating a file in a watched
// directory triggers a filesystem event. We use a very short debounce to keep
// the test fast.
func TestWatcher_FileChangeTriggersEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem event test in short mode")
	}

	dir := t.TempDir()

	hub := ws.NewHub(slog.Default())

	// Use a very short debounce for the test.
	shortDebounce := 100 * time.Millisecond

	w, err := NewWatcher(nil, nil, hub, slog.Default(), WithDebounce(shortDebounce))
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}

	// Track whether the event loop received an event.
	eventReceived := make(chan string, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run a simplified event loop that just captures the directory.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-w.fsWatcher.Events:
				if !ok {
					return
				}
				if event.Op != 0 {
					select {
					case eventReceived <- filepath.Dir(event.Name):
					default:
					}
				}
			case <-w.fsWatcher.Errors:
			}
		}
	}()

	// Create a file in the watched directory.
	testFile := filepath.Join(dir, "test.epub")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Wait for the event.
	select {
	case gotDir := <-eventReceived:
		if !isUnderPath(gotDir, dir) {
			t.Errorf("event dir %q is not under watched dir %q", gotDir, dir)
		}
	case <-ctx.Done():
		t.Error("timed out waiting for filesystem event")
	}
}
