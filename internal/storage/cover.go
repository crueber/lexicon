package storage

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif" // register GIF decoder
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dhowden/tag"
	"github.com/disintegration/imaging"
	"github.com/mholt/archives"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	_ "golang.org/x/image/webp" // register WEBP decoder
)

const (
	// maxMegapixels is the maximum allowed decoded image size (50 megapixels).
	maxMegapixels = 50_000_000

	// coverMaxWidth is the maximum width for the full-size cover image.
	coverMaxWidth = 800

	// audioCoverMaxSize is the maximum dimension for audiobook square covers.
	audioCoverMaxSize = 600

	// thumbnailWidth is the thumbnail width.
	thumbnailWidth = 200

	// thumbnailHeight is the thumbnail height.
	thumbnailHeight = 300

	// jpegQuality is the JPEG encoding quality for saved covers.
	jpegQuality = 85
)

// imageExtensions is the set of image file extensions used in comic archives.
var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

// ExtractCover extracts the cover image from a book file.
// Returns the raw image bytes and the detected MIME type, or nil if no cover found.
func ExtractCover(filePath, format string) ([]byte, string, error) {
	switch format {
	case "EPUB":
		return extractEPUBCover(filePath)
	case "PDF":
		return extractPDFCover(filePath)
	case "CBZ":
		return extractCBZCover(filePath)
	case "CBR":
		return extractArchiveCover(filePath, archives.Rar{})
	case "CB7":
		return extractArchiveCover(filePath, archives.SevenZip{})
	case "M4B", "M4A", "MP3":
		return extractAudioCover(filePath)
	default:
		// MOBI, AZW3, FB2, OPUS — not supported in this phase.
		return nil, "", nil
	}
}

// ProcessCover decodes raw image bytes, validates them, resizes to standard sizes,
// and saves them to the covers directory.
// Returns the relative cover path (e.g., "covers/books/42/cover.jpg") or error.
func ProcessCover(rawBytes []byte, bookID int64, dataDir string, isAudio bool) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(rawBytes))
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	pixels := bounds.Dx() * bounds.Dy()
	if pixels > maxMegapixels {
		return "", fmt.Errorf("image too large: %d megapixels exceeds limit of %d", pixels/1_000_000, maxMegapixels/1_000_000)
	}

	coverDir := filepath.Join(dataDir, "covers", "books", fmt.Sprintf("%d", bookID))
	if err := os.MkdirAll(coverDir, 0o750); err != nil {
		return "", fmt.Errorf("create cover directory: %w", err)
	}

	// Save full-size cover.
	var fullSize image.Image
	if isAudio {
		// Square crop for audiobooks.
		fullSize = imaging.Fill(img, audioCoverMaxSize, audioCoverMaxSize, imaging.Center, imaging.Lanczos)
	} else {
		// Resize to max width, maintaining aspect ratio.
		fullSize = imaging.Resize(img, coverMaxWidth, 0, imaging.Lanczos)
	}

	coverPath := filepath.Join(coverDir, "cover.jpg")
	if err := saveJPEG(fullSize, coverPath); err != nil {
		return "", fmt.Errorf("save cover: %w", err)
	}

	// Save thumbnail.
	thumbnail := imaging.Fill(img, thumbnailWidth, thumbnailHeight, imaging.Center, imaging.Lanczos)
	thumbnailPath := filepath.Join(coverDir, "thumbnail.jpg")
	if err := saveJPEG(thumbnail, thumbnailPath); err != nil {
		return "", fmt.Errorf("save thumbnail: %w", err)
	}

	return fmt.Sprintf("covers/books/%d/cover.jpg", bookID), nil
}

// saveJPEG encodes an image as JPEG and writes it to the given path.
func saveJPEG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return fmt.Errorf("encode jpeg: %w", err)
	}
	return nil
}

// extractEPUBCover extracts the cover image from an EPUB file.
func extractEPUBCover(filePath string) ([]byte, string, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open epub: %w", err)
	}
	defer r.Close()

	// Find the OPF file.
	opfPath, err := findOPFPath(r)
	if err == nil && opfPath != "" {
		// Try to extract cover from OPF manifest.
		if data, mime, err := extractCoverFromOPF(r, opfPath); err == nil && data != nil {
			return data, mime, nil
		}
	}

	// Fallback: look for common cover filenames.
	for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "COVER.JPG", "COVER.JPEG", "COVER.PNG"} {
		for _, f := range r.File {
			if strings.EqualFold(filepath.Base(f.Name), name) {
				data, err := readZipFile(f)
				if err != nil {
					continue
				}
				return data, mimeForExt(filepath.Ext(f.Name)), nil
			}
		}
	}

	return nil, "", nil
}

// opfContainer is used to parse the META-INF/container.xml file.
type opfContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

// findOPFPath finds the path to the OPF file in an EPUB zip.
func findOPFPath(r *zip.ReadCloser) (string, error) {
	// Try META-INF/container.xml first.
	for _, f := range r.File {
		if strings.EqualFold(f.Name, "meta-inf/container.xml") {
			data, err := readZipFile(f)
			if err != nil {
				break
			}
			var container opfContainer
			if err := xml.Unmarshal(data, &container); err != nil {
				break
			}
			if len(container.Rootfiles) > 0 {
				return container.Rootfiles[0].FullPath, nil
			}
		}
	}

	// Fallback: find any .opf file.
	for _, f := range r.File {
		if strings.ToLower(filepath.Ext(f.Name)) == ".opf" {
			return f.Name, nil
		}
	}

	return "", nil
}

// opfPackage is used to parse the OPF manifest.
type opfPackage struct {
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Metadata struct {
		Metas []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
}

// extractCoverFromOPF parses the OPF file and extracts the cover image.
func extractCoverFromOPF(r *zip.ReadCloser, opfPath string) ([]byte, string, error) {
	var opfFile *zip.File
	for _, f := range r.File {
		if f.Name == opfPath {
			opfFile = f
			break
		}
	}
	if opfFile == nil {
		return nil, "", nil
	}

	data, err := readZipFile(opfFile)
	if err != nil {
		return nil, "", fmt.Errorf("read opf: %w", err)
	}

	var pkg opfPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, "", fmt.Errorf("parse opf: %w", err)
	}

	opfDir := filepath.Dir(opfPath)

	// Find cover image ID from metadata.
	coverID := ""
	for _, meta := range pkg.Metadata.Metas {
		if strings.EqualFold(meta.Name, "cover") {
			coverID = meta.Content
			break
		}
	}

	// Search manifest for cover image.
	for _, item := range pkg.Manifest.Items {
		isCover := strings.Contains(item.Properties, "cover-image") ||
			strings.EqualFold(item.ID, "cover") ||
			strings.EqualFold(item.ID, "cover-image") ||
			(coverID != "" && item.ID == coverID)

		if !isCover {
			continue
		}

		if !strings.HasPrefix(item.MediaType, "image/") {
			continue
		}

		// Resolve the href relative to the OPF directory.
		imgPath := item.Href
		if opfDir != "." && opfDir != "" {
			imgPath = opfDir + "/" + item.Href
		}

		for _, f := range r.File {
			if f.Name == imgPath || strings.EqualFold(f.Name, imgPath) {
				imgData, err := readZipFile(f)
				if err != nil {
					return nil, "", fmt.Errorf("read cover image: %w", err)
				}
				return imgData, item.MediaType, nil
			}
		}
	}

	return nil, "", nil
}

// extractPDFCover extracts the first image from the first page of a PDF.
func extractPDFCover(filePath string) ([]byte, string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	var firstImage []byte
	var firstMIME string

	err = pdfapi.ExtractImages(f, []string{"1"}, func(img pdfmodel.Image, _ bool, _ int) error {
		if firstImage != nil {
			return nil // already have one
		}
		data, readErr := io.ReadAll(img)
		if readErr != nil {
			return readErr
		}
		firstImage = data
		firstMIME = mimeForFileType(img.FileType)
		return nil
	}, nil)

	if err != nil {
		// PDF may not have extractable images — not an error.
		slog.Debug("pdf cover extraction failed", "path", filePath, "error", err)
		return nil, "", nil
	}

	return firstImage, firstMIME, nil
}

// extractCBZCover extracts the first image from a CBZ (ZIP) comic archive.
func extractCBZCover(filePath string) ([]byte, string, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open cbz: %w", err)
	}
	defer r.Close()

	// Collect image files and sort alphabetically.
	var imageFiles []*zip.File
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExtensions[ext] {
			imageFiles = append(imageFiles, f)
		}
	}

	if len(imageFiles) == 0 {
		return nil, "", nil
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	data, err := readZipFile(imageFiles[0])
	if err != nil {
		return nil, "", fmt.Errorf("read first image: %w", err)
	}

	return data, mimeForExt(filepath.Ext(imageFiles[0].Name)), nil
}

// extractArchiveCover extracts the first image (alphabetically) from a CBR or CB7 archive.
// It reads all image entries to determine the sort order, then returns the first one.
func extractArchiveCover(filePath string, extractor archives.Extractor) ([]byte, string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	type imageEntry struct {
		name string
		data []byte
		mime string
	}

	var images []imageEntry

	ctx := context.Background()
	err = extractor.Extract(ctx, f, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !imageExtensions[ext] {
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

		images = append(images, imageEntry{
			name: info.Name(),
			data: data,
			mime: mimeForExt(ext),
		})
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("extract archive: %w", err)
	}

	if len(images) == 0 {
		return nil, "", nil
	}

	sort.Slice(images, func(i, j int) bool {
		return images[i].name < images[j].name
	})

	return images[0].data, images[0].mime, nil
}

// extractAudioCover extracts embedded artwork from an audio file.
func extractAudioCover(filePath string) ([]byte, string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open audio file: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		// Not all audio files have tags — not an error.
		return nil, "", nil
	}

	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, "", nil
	}

	return pic.Data, pic.MIMEType, nil
}

// readZipFile reads the contents of a zip.File into a byte slice.
func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// mimeForExt returns the MIME type for a given file extension.
func mimeForExt(ext string) string {
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

// mimeForFileType returns the MIME type for a pdfcpu FileType string.
// pdfcpu uses short type strings like "jpg", "png", "tif", "jpx".
func mimeForFileType(fileType string) string {
	switch strings.ToLower(fileType) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "tif", "tiff":
		return "image/tiff"
	case "jpx":
		return "image/jpx"
	default:
		return "image/jpeg"
	}
}
