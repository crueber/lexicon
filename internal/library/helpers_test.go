package library_test

import (
	"log/slog"
	"os"
	"testing"
)

// newTestLogger creates a slog.Logger that writes to stderr at error level only.
// This keeps test output clean while still capturing errors.
func newTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
