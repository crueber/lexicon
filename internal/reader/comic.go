package reader

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mholt/archives"
)

// imageExts is the set of image file extensions recognised in comic archives.
var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

// ComicPageInfo holds info about a single page in a comic archive.
type ComicPageInfo struct {
	Index    int    `json:"index"`
	Filename string `json:"filename"`
}

// ListComicPages returns the ordered list of image pages in a comic archive.
// Supports CBZ (ZIP), CBR (RAR), CB7 (7-zip) formats.
func ListComicPages(filePath, format string) ([]ComicPageInfo, error) {
	switch format {
	case "CBZ":
		return listCBZPages(filePath)
	case "CBR":
		return listArchivePages(filePath, archives.Rar{})
	case "CB7":
		return listArchivePages(filePath, archives.SevenZip{})
	default:
		return nil, fmt.Errorf("unsupported comic format: %s", format)
	}
}

// GetComicPage extracts and returns a single page image from a comic archive.
// Returns the image bytes and MIME type.
func GetComicPage(filePath, format string, pageIndex int) ([]byte, string, error) {
	switch format {
	case "CBZ":
		return getCBZPage(filePath, pageIndex)
	case "CBR":
		return getArchivePage(filePath, pageIndex, archives.Rar{})
	case "CB7":
		return getArchivePage(filePath, pageIndex, archives.SevenZip{})
	default:
		return nil, "", fmt.Errorf("unsupported comic format: %s", format)
	}
}

// listCBZPages lists image pages in a CBZ (ZIP) archive, sorted alphabetically.
func listCBZPages(filePath string) ([]ComicPageInfo, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("open cbz: %w", err)
	}
	defer r.Close()

	var names []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExts[ext] {
			names = append(names, f.Name)
		}
	}

	sort.Strings(names)

	pages := make([]ComicPageInfo, len(names))
	for i, name := range names {
		pages[i] = ComicPageInfo{
			Index:    i,
			Filename: filepath.Base(name),
		}
	}
	return pages, nil
}

// getCBZPage extracts a single page from a CBZ (ZIP) archive by index.
func getCBZPage(filePath string, pageIndex int) ([]byte, string, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open cbz: %w", err)
	}
	defer r.Close()

	var imageFiles []*zip.File
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExts[ext] {
			imageFiles = append(imageFiles, f)
		}
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	if pageIndex < 0 || pageIndex >= len(imageFiles) {
		return nil, "", fmt.Errorf("page index %d out of range (0-%d)", pageIndex, len(imageFiles)-1)
	}

	f := imageFiles[pageIndex]
	data, err := readZipEntry(f)
	if err != nil {
		return nil, "", fmt.Errorf("read page %d: %w", pageIndex, err)
	}

	return data, mimeForComicExt(filepath.Ext(f.Name)), nil
}

// listArchivePages lists image pages in a CBR or CB7 archive, sorted alphabetically.
func listArchivePages(filePath string, extractor archives.Extractor) ([]ComicPageInfo, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	var names []string

	ctx := context.Background()
	err = extractor.Extract(ctx, f, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if imageExts[ext] {
			names = append(names, info.Name())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list archive: %w", err)
	}

	sort.Strings(names)

	pages := make([]ComicPageInfo, len(names))
	for i, name := range names {
		pages[i] = ComicPageInfo{
			Index:    i,
			Filename: filepath.Base(name),
		}
	}
	return pages, nil
}

// getArchivePage extracts a single page from a CBR or CB7 archive by index.
func getArchivePage(filePath string, pageIndex int, extractor archives.Extractor) ([]byte, string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	// Collect all image names first to determine sort order.
	var names []string
	ctx := context.Background()
	err = extractor.Extract(ctx, f, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if imageExts[ext] {
			names = append(names, info.Name())
		}
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("list archive: %w", err)
	}

	sort.Strings(names)

	if pageIndex < 0 || pageIndex >= len(names) {
		return nil, "", fmt.Errorf("page index %d out of range (0-%d)", pageIndex, len(names)-1)
	}

	targetName := names[pageIndex]

	// Reopen the file to extract the target entry.
	f2, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open archive for extraction: %w", err)
	}
	defer f2.Close()

	var pageData []byte
	var pageMIME string

	err = extractor.Extract(ctx, f2, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() || info.Name() != targetName {
			return nil
		}
		rc, openErr := info.Open()
		if openErr != nil {
			return openErr
		}
		defer rc.Close()

		data, readErr := io.ReadAll(rc)
		if readErr != nil {
			return readErr
		}
		pageData = data
		pageMIME = mimeForComicExt(filepath.Ext(info.Name()))
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("extract page %d: %w", pageIndex, err)
	}

	if pageData == nil {
		return nil, "", fmt.Errorf("page %d not found in archive", pageIndex)
	}

	return pageData, pageMIME, nil
}

// readZipEntry reads the contents of a zip.File into a byte slice.
func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// mimeForComicExt returns the MIME type for a given image file extension.
func mimeForComicExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
