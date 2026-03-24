package kobo

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pgaskin/kepubify/v4/kepub"
)

// ConvertToKEPUB converts an EPUB file to KEPUB format and caches the result.
// If the cached file exists and is newer than the source file, the cached path
// is returned directly without re-converting.
//
// The kepub package is pure Go and builds with CGO_ENABLED=0.
func ConvertToKEPUB(epubPath, cacheDir string, bookFileID int64) (string, error) {
	cachePath := filepath.Join(cacheDir, fmt.Sprintf("%d.kepub.epub", bookFileID))

	// Check if the cached file exists and is newer than the source.
	srcInfo, err := os.Stat(epubPath)
	if err != nil {
		return "", fmt.Errorf("stat epub: %w", err)
	}

	if cacheInfo, err := os.Stat(cachePath); err == nil {
		if cacheInfo.ModTime().After(srcInfo.ModTime()) {
			return cachePath, nil
		}
	}

	// Ensure the cache directory exists.
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	// Open the source EPUB as a zip.
	zr, err := zip.OpenReader(epubPath)
	if err != nil {
		return "", fmt.Errorf("open epub: %w", err)
	}
	defer zr.Close()

	// Create a temporary file in the same directory as the final output.
	// This ensures the rename is atomic on most filesystems.
	tmp, err := os.CreateTemp(cacheDir, ".kepubify.*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // clean up on failure

	// Convert EPUB → KEPUB.
	converter := kepub.NewConverter()
	if err := converter.Convert(context.Background(), tmp, &zr.Reader); err != nil {
		tmp.Close()
		return "", fmt.Errorf("convert to kepub: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	// Atomically rename the temp file to the final cache path.
	if err := os.Rename(tmpName, cachePath); err != nil {
		return "", fmt.Errorf("rename kepub cache: %w", err)
	}

	return cachePath, nil
}
