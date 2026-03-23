package storage

import (
	"crypto/md5" //nolint:gosec // MD5 is used for file deduplication, not security
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

const (
	// fingerprintChunkSize is the number of bytes read from the start and end
	// of a file when computing a partial fingerprint.
	fingerprintChunkSize = 64 * 1024 // 64KB
)

// Fingerprint computes a partial MD5 hash of a file for duplicate detection.
// It reads the first 64KB and last 64KB of the file, then hashes those bytes.
// For files smaller than 128KB, the entire file is hashed.
// Returns a hex-encoded MD5 string.
func Fingerprint(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	size := info.Size()

	h := md5.New() //nolint:gosec // MD5 is used for file deduplication, not security

	if size <= 2*fingerprintChunkSize {
		// File is small enough to hash entirely.
		if _, err := io.Copy(h, f); err != nil {
			return "", fmt.Errorf("hash file: %w", err)
		}
	} else {
		// Read first 64KB.
		buf := make([]byte, fingerprintChunkSize)
		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF {
			return "", fmt.Errorf("read file start: %w", err)
		}
		if _, err := h.Write(buf[:n]); err != nil {
			return "", fmt.Errorf("hash file start: %w", err)
		}

		// Seek to last 64KB.
		offset := size - fingerprintChunkSize
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek to file end: %w", err)
		}

		n, err = io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF {
			return "", fmt.Errorf("read file end: %w", err)
		}
		if _, err := h.Write(buf[:n]); err != nil {
			return "", fmt.Errorf("hash file end: %w", err)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
