package storage_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/crueber/lexicon/internal/storage"
)

// writeTestFile creates a temporary file with the given content and returns its path.
func writeTestFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test file %q: %v", path, err)
	}
	return path
}

func TestFingerprint_SmallFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello, world")
	path := writeTestFile(t, dir, "small.txt", content)

	fp, err := storage.Fingerprint(path)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}

	if fp == "" {
		t.Error("fingerprint is empty")
	}

	// Should be a 32-char hex string (MD5).
	if len(fp) != 32 {
		t.Errorf("fingerprint length = %d; want 32", len(fp))
	}
}

func TestFingerprint_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "empty.txt", []byte{})

	fp, err := storage.Fingerprint(path)
	if err != nil {
		t.Fatalf("Fingerprint empty file: %v", err)
	}

	if fp == "" {
		t.Error("fingerprint of empty file is empty")
	}

	// MD5 of empty input is well-known.
	const md5Empty = "d41d8cd98f00b204e9800998ecf8427e"
	if fp != md5Empty {
		t.Errorf("fingerprint of empty file = %q; want %q", fp, md5Empty)
	}
}

func TestFingerprint_Deterministic(t *testing.T) {
	dir := t.TempDir()
	content := bytes.Repeat([]byte("abcdefgh"), 1000)
	path := writeTestFile(t, dir, "deterministic.bin", content)

	fp1, err := storage.Fingerprint(path)
	if err != nil {
		t.Fatalf("first Fingerprint: %v", err)
	}

	fp2, err := storage.Fingerprint(path)
	if err != nil {
		t.Fatalf("second Fingerprint: %v", err)
	}

	if fp1 != fp2 {
		t.Errorf("fingerprints differ: %q vs %q", fp1, fp2)
	}
}

func TestFingerprint_DifferentFiles(t *testing.T) {
	dir := t.TempDir()

	path1 := writeTestFile(t, dir, "file1.txt", []byte("content one"))
	path2 := writeTestFile(t, dir, "file2.txt", []byte("content two"))

	fp1, err := storage.Fingerprint(path1)
	if err != nil {
		t.Fatalf("Fingerprint file1: %v", err)
	}

	fp2, err := storage.Fingerprint(path2)
	if err != nil {
		t.Fatalf("Fingerprint file2: %v", err)
	}

	if fp1 == fp2 {
		t.Error("different files produced the same fingerprint")
	}
}

func TestFingerprint_LargeFile(t *testing.T) {
	// Create a file larger than 128KB to exercise the partial-read path.
	dir := t.TempDir()

	// 200KB file with distinct start and end content.
	const size = 200 * 1024
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 256)
	}
	path := writeTestFile(t, dir, "large.bin", content)

	fp, err := storage.Fingerprint(path)
	if err != nil {
		t.Fatalf("Fingerprint large file: %v", err)
	}

	if fp == "" {
		t.Error("fingerprint of large file is empty")
	}

	if len(fp) != 32 {
		t.Errorf("fingerprint length = %d; want 32", len(fp))
	}

	// Verify determinism on large file.
	fp2, err := storage.Fingerprint(path)
	if err != nil {
		t.Fatalf("second Fingerprint large file: %v", err)
	}

	if fp != fp2 {
		t.Errorf("large file fingerprints differ: %q vs %q", fp, fp2)
	}
}

func TestFingerprint_LargeFilePartialRead(t *testing.T) {
	// Verify that two large files with the same first+last 64KB but different
	// middle content produce the same fingerprint (by design — partial hash).
	dir := t.TempDir()

	const chunkSize = 64 * 1024
	const totalSize = 300 * 1024

	// Both files have identical first and last 64KB.
	start := bytes.Repeat([]byte{0xAA}, chunkSize)
	end := bytes.Repeat([]byte{0xBB}, chunkSize)

	// Middle content differs.
	middle1 := bytes.Repeat([]byte{0x11}, totalSize-2*chunkSize)
	middle2 := bytes.Repeat([]byte{0x22}, totalSize-2*chunkSize)

	content1 := append(append(start, middle1...), end...)
	content2 := append(append(start, middle2...), end...)

	path1 := writeTestFile(t, dir, "large1.bin", content1)
	path2 := writeTestFile(t, dir, "large2.bin", content2)

	fp1, err := storage.Fingerprint(path1)
	if err != nil {
		t.Fatalf("Fingerprint large1: %v", err)
	}

	fp2, err := storage.Fingerprint(path2)
	if err != nil {
		t.Fatalf("Fingerprint large2: %v", err)
	}

	// By design, partial fingerprinting means these should match.
	if fp1 != fp2 {
		t.Errorf("expected same fingerprint for files with identical start+end; got %q vs %q", fp1, fp2)
	}
}

func TestFingerprint_FileNotFound(t *testing.T) {
	_, err := storage.Fingerprint("/nonexistent/path/to/file.txt")
	if err == nil {
		t.Error("expected error for nonexistent file; got nil")
	}
}
