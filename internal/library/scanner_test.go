package library_test

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/crueber/lexicon/internal/book"
	"github.com/crueber/lexicon/internal/library"
)

// createTestFile creates a file with the given content at the given path.
// It creates parent directories as needed.
func createTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}

// createLibraryInDB creates a library record directly in the database.
func createLibraryInDB(t *testing.T, db *sql.DB, name, mode string) library.Library {
	t.Helper()
	q := library.New(db)
	lib, err := q.CreateLibrary(context.Background(), library.CreateLibraryParams{
		Name:             name,
		OrganizationMode: mode,
	})
	if err != nil {
		t.Fatalf("create library %q: %v", name, err)
	}
	return lib
}

// addLibraryPath adds a path to a library and returns the LibraryPath.
func addLibraryPath(t *testing.T, db *sql.DB, libraryID int64, path string) library.LibraryPath {
	t.Helper()
	q := library.New(db)
	lp, err := q.CreateLibraryPath(context.Background(), library.CreateLibraryPathParams{
		LibraryID: libraryID,
		Path:      path,
	})
	if err != nil {
		t.Fatalf("add library path %q: %v", path, err)
	}
	return lp
}

// countBooks returns the number of books in the database for a library.
func countBooks(t *testing.T, db *sql.DB, libraryID int64) int {
	t.Helper()
	q := book.New(db)
	count, err := q.CountBooksByLibrary(context.Background(), libraryID)
	if err != nil {
		t.Fatalf("count books: %v", err)
	}
	return int(count)
}

// listBookFiles returns all book files for a given book.
func listBookFiles(t *testing.T, db *sql.DB, bookID int64) []book.BookFile {
	t.Helper()
	q := book.New(db)
	files, err := q.ListBookFiles(context.Background(), bookID)
	if err != nil {
		t.Fatalf("list book files: %v", err)
	}
	return files
}

// listBooksForLibrary returns all books for a library.
func listBooksForLibrary(t *testing.T, db *sql.DB, libraryID int64) []book.Book {
	t.Helper()
	q := book.New(db)
	books, err := q.ListBooksByLibrary(context.Background(), libraryID)
	if err != nil {
		t.Fatalf("list books: %v", err)
	}
	return books
}

// --- Tests ---

func TestScanner_BookPerFile_CreatesBookAndFile(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	// Create test files.
	epubPath := filepath.Join(dir, "book1.epub")
	pdfPath := filepath.Join(dir, "book2.pdf")
	createTestFile(t, epubPath, []byte("fake epub content"))
	createTestFile(t, pdfPath, []byte("fake pdf content"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 2 {
		t.Errorf("BooksAdded = %d; want 2", result.BooksAdded)
	}
	if result.FilesAdded != 2 {
		t.Errorf("FilesAdded = %d; want 2", result.FilesAdded)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v; want none", result.Errors)
	}

	if got := countBooks(t, db, lib.ID); got != 2 {
		t.Errorf("books in DB = %d; want 2", got)
	}
}

func TestScanner_BookPerFile_SkipsUnsupportedFiles(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	createTestFile(t, filepath.Join(dir, "book.epub"), []byte("epub"))
	createTestFile(t, filepath.Join(dir, "readme.txt"), []byte("text"))
	createTestFile(t, filepath.Join(dir, "image.jpg"), []byte("jpeg"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 1 {
		t.Errorf("BooksAdded = %d; want 1 (only epub)", result.BooksAdded)
	}
}

func TestScanner_BookPerFile_DetectsBookTypes(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	createTestFile(t, filepath.Join(dir, "ebook.epub"), []byte("epub"))
	createTestFile(t, filepath.Join(dir, "comic.cbz"), []byte("cbz"))
	createTestFile(t, filepath.Join(dir, "audio.m4b"), []byte("m4b"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 3 {
		t.Errorf("BooksAdded = %d; want 3", result.BooksAdded)
	}

	books := listBooksForLibrary(t, db, lib.ID)
	typeMap := make(map[string]int)
	for _, b := range books {
		typeMap[b.BookType]++
	}

	if typeMap["EBOOK"] != 1 {
		t.Errorf("EBOOK count = %d; want 1", typeMap["EBOOK"])
	}
	if typeMap["COMIC"] != 1 {
		t.Errorf("COMIC count = %d; want 1", typeMap["COMIC"])
	}
	if typeMap["AUDIOBOOK"] != 1 {
		t.Errorf("AUDIOBOOK count = %d; want 1", typeMap["AUDIOBOOK"])
	}
}

func TestScanner_BookPerFile_FingerprintMoveDetection(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	// Create a file and scan it.
	originalPath := filepath.Join(dir, "original.epub")
	createTestFile(t, originalPath, []byte("epub content for move test"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("first ScanLibrary: %v", err)
	}

	if result.BooksAdded != 1 {
		t.Fatalf("first scan: BooksAdded = %d; want 1", result.BooksAdded)
	}

	// Rename the file (simulate a move).
	movedPath := filepath.Join(dir, "moved.epub")
	if err := os.Rename(originalPath, movedPath); err != nil {
		t.Fatalf("rename file: %v", err)
	}

	// Scan again.
	result2, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("second ScanLibrary: %v", err)
	}

	// Should detect the move (file updated, not added).
	if result2.BooksAdded != 0 {
		t.Errorf("second scan: BooksAdded = %d; want 0 (file was moved, not new)", result2.BooksAdded)
	}
	if result2.FilesUpdated != 1 {
		t.Errorf("second scan: FilesUpdated = %d; want 1", result2.FilesUpdated)
	}

	// Total books should still be 1.
	if got := countBooks(t, db, lib.ID); got != 1 {
		t.Errorf("books in DB after move = %d; want 1", got)
	}

	// Verify the file path was updated.
	books := listBooksForLibrary(t, db, lib.ID)
	if len(books) != 1 {
		t.Fatalf("expected 1 book; got %d", len(books))
	}
	files := listBookFiles(t, db, books[0].ID)
	if len(files) != 1 {
		t.Fatalf("expected 1 file; got %d", len(files))
	}
	if files[0].FilePath != movedPath {
		t.Errorf("file path = %q; want %q", files[0].FilePath, movedPath)
	}
}

func TestScanner_BookPerFile_FingerprintChangeDetection(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	filePath := filepath.Join(dir, "book.epub")
	createTestFile(t, filePath, []byte("original content"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	if _, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp}); err != nil {
		t.Fatalf("first ScanLibrary: %v", err)
	}

	// Overwrite the file with different content.
	createTestFile(t, filePath, []byte("completely different content"))

	result2, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("second ScanLibrary: %v", err)
	}

	if result2.FilesUpdated != 1 {
		t.Errorf("FilesUpdated = %d; want 1", result2.FilesUpdated)
	}
	if result2.BooksAdded != 0 {
		t.Errorf("BooksAdded = %d; want 0 (same path, different content)", result2.BooksAdded)
	}
}

func TestScanner_BookPerFile_IdempotentRescan(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	createTestFile(t, filepath.Join(dir, "book.epub"), []byte("epub"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))

	// First scan.
	if _, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp}); err != nil {
		t.Fatalf("first ScanLibrary: %v", err)
	}

	// Second scan — should not add duplicates.
	result2, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("second ScanLibrary: %v", err)
	}

	if result2.BooksAdded != 0 {
		t.Errorf("second scan BooksAdded = %d; want 0", result2.BooksAdded)
	}
	if result2.FilesAdded != 0 {
		t.Errorf("second scan FilesAdded = %d; want 0", result2.FilesAdded)
	}

	if got := countBooks(t, db, lib.ID); got != 1 {
		t.Errorf("books in DB = %d; want 1", got)
	}
}

func TestScanner_BookPerFile_CreatesBookMetadata(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	createTestFile(t, filepath.Join(dir, "book.epub"), []byte("epub"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	if _, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp}); err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	books := listBooksForLibrary(t, db, lib.ID)
	if len(books) != 1 {
		t.Fatalf("expected 1 book; got %d", len(books))
	}

	// Verify book_metadata row was created.
	q := book.New(db)
	_, err := q.GetBookMetadata(context.Background(), books[0].ID)
	if err != nil {
		t.Errorf("GetBookMetadata: %v (expected metadata row to exist)", err)
	}
}

func TestScanner_BookPerFolder_GroupsFilesIntoOneBook(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	// Create multiple files in the same directory.
	createTestFile(t, filepath.Join(dir, "chapter1.epub"), []byte("chapter 1"))
	createTestFile(t, filepath.Join(dir, "chapter2.epub"), []byte("chapter 2"))
	createTestFile(t, filepath.Join(dir, "chapter3.epub"), []byte("chapter 3"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FOLDER")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	// All 3 files in the same folder → 1 book.
	if result.BooksAdded != 1 {
		t.Errorf("BooksAdded = %d; want 1", result.BooksAdded)
	}
	if result.FilesAdded != 3 {
		t.Errorf("FilesAdded = %d; want 3", result.FilesAdded)
	}

	if got := countBooks(t, db, lib.ID); got != 1 {
		t.Errorf("books in DB = %d; want 1", got)
	}

	books := listBooksForLibrary(t, db, lib.ID)
	files := listBookFiles(t, db, books[0].ID)
	if len(files) != 3 {
		t.Errorf("files for book = %d; want 3", len(files))
	}
}

func TestScanner_BookPerFolder_SeparateFoldersSeparateBooks(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	// Two subdirectories, each with one file.
	createTestFile(t, filepath.Join(dir, "bookA", "file.epub"), []byte("book a"))
	createTestFile(t, filepath.Join(dir, "bookB", "file.epub"), []byte("book b"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FOLDER")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 2 {
		t.Errorf("BooksAdded = %d; want 2", result.BooksAdded)
	}

	if got := countBooks(t, db, lib.ID); got != 2 {
		t.Errorf("books in DB = %d; want 2", got)
	}
}

func TestScanner_BookPerFolder_AudiobookDetection(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	// All audio files → AUDIOBOOK type.
	createTestFile(t, filepath.Join(dir, "track01.mp3"), []byte("mp3"))
	createTestFile(t, filepath.Join(dir, "track02.mp3"), []byte("mp3"))
	createTestFile(t, filepath.Join(dir, "track03.m4b"), []byte("m4b"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FOLDER")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 1 {
		t.Fatalf("BooksAdded = %d; want 1", result.BooksAdded)
	}

	books := listBooksForLibrary(t, db, lib.ID)
	if books[0].BookType != "AUDIOBOOK" {
		t.Errorf("BookType = %q; want AUDIOBOOK", books[0].BookType)
	}

	// Verify track numbers are assigned.
	files := listBookFiles(t, db, books[0].ID)
	for _, f := range files {
		if !f.TrackNumber.Valid {
			t.Errorf("file %q has no track number", f.FilePath)
		}
	}
}

func TestScanner_BookPerFolder_ComicDetection(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	createTestFile(t, filepath.Join(dir, "issue1.cbz"), []byte("cbz"))
	createTestFile(t, filepath.Join(dir, "issue2.cbr"), []byte("cbr"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FOLDER")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 1 {
		t.Fatalf("BooksAdded = %d; want 1", result.BooksAdded)
	}

	books := listBooksForLibrary(t, db, lib.ID)
	if books[0].BookType != "COMIC" {
		t.Errorf("BookType = %q; want COMIC", books[0].BookType)
	}
}

func TestScanner_BookPerFolder_AddsNewFilesToExistingBook(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	// First scan: one file.
	createTestFile(t, filepath.Join(dir, "track01.mp3"), []byte("track 1"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FOLDER")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	if _, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp}); err != nil {
		t.Fatalf("first ScanLibrary: %v", err)
	}

	if got := countBooks(t, db, lib.ID); got != 1 {
		t.Fatalf("after first scan: books = %d; want 1", got)
	}

	// Add a second file to the same directory.
	createTestFile(t, filepath.Join(dir, "track02.mp3"), []byte("track 2"))

	result2, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("second ScanLibrary: %v", err)
	}

	// No new books, but one new file.
	if result2.BooksAdded != 0 {
		t.Errorf("second scan BooksAdded = %d; want 0", result2.BooksAdded)
	}
	if result2.FilesAdded != 1 {
		t.Errorf("second scan FilesAdded = %d; want 1", result2.FilesAdded)
	}

	// Still 1 book.
	if got := countBooks(t, db, lib.ID); got != 1 {
		t.Errorf("after second scan: books = %d; want 1", got)
	}

	// But now 2 files.
	books := listBooksForLibrary(t, db, lib.ID)
	files := listBookFiles(t, db, books[0].ID)
	if len(files) != 2 {
		t.Errorf("files for book = %d; want 2", len(files))
	}
}

func TestScanner_ContextCancellation(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	// Create many files.
	for i := 0; i < 10; i++ {
		createTestFile(t, filepath.Join(dir, filepath.Join("sub", "book"+string(rune('a'+i))+".epub")), []byte("epub"))
	}

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	// Cancel context immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	_, err := scanner.ScanLibrary(ctx, lib, []library.LibraryPath{lp})
	// Should return context error or partial result without panicking.
	// The error may be nil if cancellation happened after the scan completed.
	_ = err // acceptable: either nil or context.Canceled
}

func TestScanner_EmptyDirectory(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 0 {
		t.Errorf("BooksAdded = %d; want 0", result.BooksAdded)
	}
}

func TestScanner_RecursiveDirectoryWalk(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()

	// Files in nested subdirectories.
	createTestFile(t, filepath.Join(dir, "level1", "book.epub"), []byte("epub"))
	createTestFile(t, filepath.Join(dir, "level1", "level2", "book.pdf"), []byte("pdf"))
	createTestFile(t, filepath.Join(dir, "level1", "level2", "level3", "book.mobi"), []byte("mobi"))

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, t.TempDir(), newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 3 {
		t.Errorf("BooksAdded = %d; want 3", result.BooksAdded)
	}
}

// makeSmallJPEG creates a small JPEG image and returns its bytes.
func makeSmallJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 150))
	for y := 0; y < 150; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// makeMinimalEPUBWithCover creates a minimal EPUB zip file with an embedded cover image.
func makeMinimalEPUBWithCover(t *testing.T, path string, coverBytes []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create epub: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// mimetype
	mf, err := w.Create("mimetype")
	if err != nil {
		t.Fatalf("create mimetype: %v", err)
	}
	if _, err := mf.Write([]byte("application/epub+zip")); err != nil {
		t.Fatalf("write mimetype: %v", err)
	}

	// META-INF/container.xml
	cf, err := w.Create("META-INF/container.xml")
	if err != nil {
		t.Fatalf("create container.xml: %v", err)
	}
	containerXML := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
	if _, err := cf.Write([]byte(containerXML)); err != nil {
		t.Fatalf("write container.xml: %v", err)
	}

	// OEBPS/content.opf
	opf, err := w.Create("OEBPS/content.opf")
	if err != nil {
		t.Fatalf("create content.opf: %v", err)
	}
	opfXML := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book</dc:title>
  </metadata>
  <manifest>
    <item id="cover-img" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>
  </manifest>
</package>`
	if _, err := opf.Write([]byte(opfXML)); err != nil {
		t.Fatalf("write content.opf: %v", err)
	}

	// OEBPS/images/cover.jpg
	imgFile, err := w.Create("OEBPS/images/cover.jpg")
	if err != nil {
		t.Fatalf("create cover image: %v", err)
	}
	if _, err := imgFile.Write(coverBytes); err != nil {
		t.Fatalf("write cover image: %v", err)
	}
}

// makeMinimalEPUBWithMetadata creates a minimal EPUB with both cover and metadata.
func makeMinimalEPUBWithMetadata(t *testing.T, path string, coverBytes []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create epub: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// mimetype
	mf, err := w.Create("mimetype")
	if err != nil {
		t.Fatalf("create mimetype: %v", err)
	}
	if _, err := mf.Write([]byte("application/epub+zip")); err != nil {
		t.Fatalf("write mimetype: %v", err)
	}

	// META-INF/container.xml
	cf, err := w.Create("META-INF/container.xml")
	if err != nil {
		t.Fatalf("create container.xml: %v", err)
	}
	containerXML := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
	if _, err := cf.Write([]byte(containerXML)); err != nil {
		t.Fatalf("write container.xml: %v", err)
	}

	// OEBPS/content.opf with metadata
	opf, err := w.Create("OEBPS/content.opf")
	if err != nil {
		t.Fatalf("create content.opf: %v", err)
	}
	opfXML := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Scanned Book Title</dc:title>
    <dc:creator opf:role="aut">Scanned Author</dc:creator>
    <dc:publisher>Scan Publisher</dc:publisher>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="cover-img" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>
  </manifest>
</package>`
	if _, err := opf.Write([]byte(opfXML)); err != nil {
		t.Fatalf("write content.opf: %v", err)
	}

	// OEBPS/images/cover.jpg
	imgFile, err := w.Create("OEBPS/images/cover.jpg")
	if err != nil {
		t.Fatalf("create cover image: %v", err)
	}
	if _, err := imgFile.Write(coverBytes); err != nil {
		t.Fatalf("write cover image: %v", err)
	}
}

func TestScanner_BookPerFile_ExtractsMetadataFromEPUB(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	dataDir := t.TempDir()

	epubPath := filepath.Join(dir, "book_with_meta.epub")
	coverBytes := makeSmallJPEG(t)
	makeMinimalEPUBWithMetadata(t, epubPath, coverBytes)

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, dataDir, newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 1 {
		t.Fatalf("BooksAdded = %d; want 1", result.BooksAdded)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	books := listBooksForLibrary(t, db, lib.ID)
	if len(books) != 1 {
		t.Fatalf("expected 1 book; got %d", len(books))
	}

	q := book.New(db)
	meta, err := q.GetBookMetadata(context.Background(), books[0].ID)
	if err != nil {
		t.Fatalf("GetBookMetadata: %v", err)
	}

	t.Run("title", func(t *testing.T) {
		if !meta.Title.Valid || meta.Title.String != "Scanned Book Title" {
			t.Errorf("title = %v; want %q", meta.Title, "Scanned Book Title")
		}
	})

	t.Run("publisher", func(t *testing.T) {
		if !meta.Publisher.Valid || meta.Publisher.String != "Scan Publisher" {
			t.Errorf("publisher = %v; want %q", meta.Publisher, "Scan Publisher")
		}
	})

	t.Run("language", func(t *testing.T) {
		if !meta.Language.Valid || meta.Language.String != "en" {
			t.Errorf("language = %v; want %q", meta.Language, "en")
		}
	})

	t.Run("authors", func(t *testing.T) {
		authors, err := q.ListBookAuthors(context.Background(), books[0].ID)
		if err != nil {
			t.Fatalf("ListBookAuthors: %v", err)
		}
		if len(authors) != 1 {
			t.Fatalf("authors count = %d; want 1", len(authors))
		}
		if authors[0].Name != "Scanned Author" {
			t.Errorf("author name = %q; want %q", authors[0].Name, "Scanned Author")
		}
	})
}

func TestScanner_BookPerFile_ExtractsCoverFromEPUB(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir()
	dataDir := t.TempDir()

	// Create a minimal EPUB with a cover image.
	epubPath := filepath.Join(dir, "book.epub")
	coverBytes := makeSmallJPEG(t)
	makeMinimalEPUBWithCover(t, epubPath, coverBytes)

	lib := createLibraryInDB(t, db, "Test Library", "BOOK_PER_FILE")
	lp := addLibraryPath(t, db, lib.ID, dir)

	scanner := library.NewScanner(db, dataDir, newTestLogger(t))
	result, err := scanner.ScanLibrary(context.Background(), lib, []library.LibraryPath{lp})
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}

	if result.BooksAdded != 1 {
		t.Fatalf("BooksAdded = %d; want 1", result.BooksAdded)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// Verify the cover was extracted and saved.
	books := listBooksForLibrary(t, db, lib.ID)
	if len(books) != 1 {
		t.Fatalf("expected 1 book; got %d", len(books))
	}

	q := book.New(db)
	meta, err := q.GetBookMetadata(context.Background(), books[0].ID)
	if err != nil {
		t.Fatalf("GetBookMetadata: %v", err)
	}

	if !meta.CoverPath.Valid || meta.CoverPath.String == "" {
		t.Error("expected cover_path to be set after scanning EPUB with cover")
	} else {
		// Verify the cover file actually exists on disk.
		coverFilePath := filepath.Join(dataDir, meta.CoverPath.String)
		if _, err := os.Stat(coverFilePath); err != nil {
			t.Errorf("cover file not found at %q: %v", coverFilePath, err)
		}
	}
}
