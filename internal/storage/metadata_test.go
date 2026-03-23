package storage_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/crueber/lexicon/internal/storage"
)

// makeTestEPUBWithMetadata creates a minimal EPUB with rich OPF metadata.
func makeTestEPUBWithMetadata(t *testing.T, dir string) string {
	t.Helper()
	epubPath := filepath.Join(dir, "test_meta.epub")
	f, err := os.Create(epubPath)
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

	// OEBPS/content.opf with rich metadata
	opf, err := w.Create("OEBPS/content.opf")
	if err != nil {
		t.Fatalf("create content.opf: %v", err)
	}
	opfXML := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>The Great Test Book</dc:title>
    <dc:creator opf:role="aut">Jane Author</dc:creator>
    <dc:creator opf:role="aut">John Coauthor</dc:creator>
    <dc:description>A wonderful test book about testing.</dc:description>
    <dc:publisher>Test Publisher</dc:publisher>
    <dc:date>2023-06-15</dc:date>
    <dc:language>en</dc:language>
    <dc:identifier opf:scheme="ISBN">978-3-16-148410-0</dc:identifier>
    <dc:subject>Fiction</dc:subject>
    <dc:subject>Testing</dc:subject>
    <meta name="calibre:series" content="Test Series"/>
    <meta name="calibre:series_index" content="3"/>
  </metadata>
  <manifest>
    <item id="content" href="content.html" media-type="application/xhtml+xml"/>
  </manifest>
</package>`
	if _, err := opf.Write([]byte(opfXML)); err != nil {
		t.Fatalf("write content.opf: %v", err)
	}

	return epubPath
}

// makeTestCBZWithComicInfo creates a minimal CBZ with ComicInfo.xml.
func makeTestCBZWithComicInfo(t *testing.T, dir string) string {
	t.Helper()
	cbzPath := filepath.Join(dir, "test_meta.cbz")
	f, err := os.Create(cbzPath)
	if err != nil {
		t.Fatalf("create cbz: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// ComicInfo.xml
	ci, err := w.Create("ComicInfo.xml")
	if err != nil {
		t.Fatalf("create ComicInfo.xml: %v", err)
	}
	comicInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<ComicInfo>
  <Title>Issue #5: The Reckoning</Title>
  <Series>Amazing Comics</Series>
  <Number>5</Number>
  <Volume>2</Volume>
  <Year>2022</Year>
  <Month>3</Month>
  <Writer>Comic Writer</Writer>
  <Publisher>Comics Inc</Publisher>
  <Genre>Superhero</Genre>
  <Characters>Hero,Villain,Sidekick</Characters>
  <Teams>Team Alpha</Teams>
  <Locations>New York,Gotham</Locations>
  <StoryArc>The Big Arc</StoryArc>
  <AgeRating>Teen</AgeRating>
  <BlackAndWhite>No</BlackAndWhite>
  <Manga>No</Manga>
  <LanguageISO>en</LanguageISO>
</ComicInfo>`
	if _, err := ci.Write([]byte(comicInfoXML)); err != nil {
		t.Fatalf("write ComicInfo.xml: %v", err)
	}

	// Add a dummy image so it's a valid CBZ.
	img, err := w.Create("page001.jpg")
	if err != nil {
		t.Fatalf("create page001.jpg: %v", err)
	}
	if _, err := img.Write([]byte("fake image data")); err != nil {
		t.Fatalf("write page001.jpg: %v", err)
	}

	return cbzPath
}

func TestExtractMetadata_EPUB(t *testing.T) {
	dir := t.TempDir()
	epubPath := makeTestEPUBWithMetadata(t, dir)

	meta, err := storage.ExtractMetadata(epubPath, "EPUB")
	if err != nil {
		t.Fatalf("ExtractMetadata EPUB: %v", err)
	}
	if meta == nil {
		t.Fatal("ExtractMetadata EPUB: expected metadata, got nil")
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"title", meta.Title, "The Great Test Book"},
		{"description", meta.Description, "A wonderful test book about testing."},
		{"publisher", meta.Publisher, "Test Publisher"},
		{"publish_date", meta.PublishDate, "2023-06-15"},
		{"language", meta.Language, "en"},
		{"isbn13", meta.ISBN13, "9783161484100"},
		{"series", meta.Series, "Test Series"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q; want %q", tt.got, tt.want)
			}
		})
	}

	t.Run("authors", func(t *testing.T) {
		if len(meta.Authors) != 2 {
			t.Fatalf("authors count = %d; want 2", len(meta.Authors))
		}
		if meta.Authors[0] != "Jane Author" {
			t.Errorf("authors[0] = %q; want %q", meta.Authors[0], "Jane Author")
		}
		if meta.Authors[1] != "John Coauthor" {
			t.Errorf("authors[1] = %q; want %q", meta.Authors[1], "John Coauthor")
		}
	})

	t.Run("categories", func(t *testing.T) {
		if len(meta.Categories) < 2 {
			t.Fatalf("categories count = %d; want >= 2", len(meta.Categories))
		}
	})

	t.Run("series_index", func(t *testing.T) {
		if meta.SeriesIndex != 3.0 {
			t.Errorf("series_index = %v; want 3.0", meta.SeriesIndex)
		}
	})
}

func TestExtractMetadata_EPUB_NoMetadata(t *testing.T) {
	dir := t.TempDir()
	// Create a minimal EPUB with no metadata fields.
	epubPath := filepath.Join(dir, "empty_meta.epub")
	func() {
		f, err := os.Create(epubPath)
		if err != nil {
			t.Fatalf("create epub: %v", err)
		}
		defer f.Close()

		w := zip.NewWriter(f)
		defer w.Close()

		mf, _ := w.Create("mimetype")
		_, _ = mf.Write([]byte("application/epub+zip"))

		cf, _ := w.Create("META-INF/container.xml")
		_, _ = cf.Write([]byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

		opf, _ := w.Create("content.opf")
		_, _ = opf.Write([]byte(`<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
  </metadata>
  <manifest/>
</package>`))
	}()

	meta, err := storage.ExtractMetadata(epubPath, "EPUB")
	if err != nil {
		t.Fatalf("ExtractMetadata EPUB empty: %v", err)
	}
	// Empty metadata should return nil.
	if meta != nil {
		t.Errorf("expected nil for empty metadata, got %+v", meta)
	}
}

func TestExtractMetadata_CBZ(t *testing.T) {
	dir := t.TempDir()
	cbzPath := makeTestCBZWithComicInfo(t, dir)

	meta, err := storage.ExtractMetadata(cbzPath, "CBZ")
	if err != nil {
		t.Fatalf("ExtractMetadata CBZ: %v", err)
	}
	if meta == nil {
		t.Fatal("ExtractMetadata CBZ: expected metadata, got nil")
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"title", meta.Title, "Issue #5: The Reckoning"},
		{"series", meta.Series, "Amazing Comics"},
		{"publisher", meta.Publisher, "Comics Inc"},
		{"language", meta.Language, "en"},
		{"publish_date", meta.PublishDate, "2022-03"},
		{"comic_genre", meta.ComicGenre, "Superhero"},
		{"comic_age_rating", meta.ComicAgeRating, "Teen"},
		{"comic_story_arc", meta.ComicStoryArc, "The Big Arc"},
		{"comic_manga", meta.ComicManga, "No"},
		{"comic_writer", meta.ComicWriter, "Comic Writer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q; want %q", tt.got, tt.want)
			}
		})
	}

	t.Run("authors", func(t *testing.T) {
		if len(meta.Authors) != 1 || meta.Authors[0] != "Comic Writer" {
			t.Errorf("authors = %v; want [Comic Writer]", meta.Authors)
		}
	})

	t.Run("series_index", func(t *testing.T) {
		if meta.SeriesIndex != 5.0 {
			t.Errorf("series_index = %v; want 5.0", meta.SeriesIndex)
		}
	})

	t.Run("comic_volume", func(t *testing.T) {
		if meta.ComicVolume == nil || *meta.ComicVolume != 2 {
			t.Errorf("comic_volume = %v; want 2", meta.ComicVolume)
		}
	})

	t.Run("comic_year", func(t *testing.T) {
		if meta.ComicYear == nil || *meta.ComicYear != 2022 {
			t.Errorf("comic_year = %v; want 2022", meta.ComicYear)
		}
	})

	t.Run("comic_characters", func(t *testing.T) {
		if len(meta.ComicCharacters) != 3 {
			t.Errorf("comic_characters count = %d; want 3", len(meta.ComicCharacters))
		}
	})

	t.Run("comic_locations", func(t *testing.T) {
		if len(meta.ComicLocations) != 2 {
			t.Errorf("comic_locations count = %d; want 2", len(meta.ComicLocations))
		}
	})

	t.Run("categories", func(t *testing.T) {
		if len(meta.Categories) != 1 || meta.Categories[0] != "Superhero" {
			t.Errorf("categories = %v; want [Superhero]", meta.Categories)
		}
	})

	t.Run("black_and_white", func(t *testing.T) {
		if meta.ComicBlackAndWhite {
			t.Error("comic_black_and_white = true; want false (value is 'No')")
		}
	})
}

func TestExtractMetadata_CBZ_NoComicInfo(t *testing.T) {
	dir := t.TempDir()
	cbzPath := filepath.Join(dir, "no_comicinfo.cbz")
	f, err := os.Create(cbzPath)
	if err != nil {
		t.Fatalf("create cbz: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	img, _ := w.Create("page001.jpg")
	_, _ = img.Write([]byte("fake image"))
	w.Close()

	meta, err := storage.ExtractMetadata(cbzPath, "CBZ")
	if err != nil {
		t.Fatalf("ExtractMetadata CBZ no comicinfo: %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil for CBZ without ComicInfo.xml, got %+v", meta)
	}
}

func TestExtractMetadata_UnsupportedFormats(t *testing.T) {
	formats := []string{"CBR", "CB7", "MOBI", "AZW3", "FB2", "OPUS"}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			meta, err := storage.ExtractMetadata("/nonexistent/file", format)
			if err != nil {
				t.Errorf("ExtractMetadata %s: unexpected error: %v", format, err)
			}
			if meta != nil {
				t.Errorf("ExtractMetadata %s: expected nil, got %+v", format, meta)
			}
		})
	}
}

func TestExtractMetadata_EPUB_ISBN10(t *testing.T) {
	dir := t.TempDir()
	epubPath := filepath.Join(dir, "isbn10.epub")
	func() {
		f, err := os.Create(epubPath)
		if err != nil {
			t.Fatalf("create epub: %v", err)
		}
		defer f.Close()

		w := zip.NewWriter(f)
		defer w.Close()

		mf, _ := w.Create("mimetype")
		_, _ = mf.Write([]byte("application/epub+zip"))

		cf, _ := w.Create("META-INF/container.xml")
		_, _ = cf.Write([]byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

		opf, _ := w.Create("content.opf")
		_, _ = opf.Write([]byte(`<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>ISBN10 Test</dc:title>
    <dc:identifier opf:scheme="ISBN">0-306-40615-2</dc:identifier>
  </metadata>
  <manifest/>
</package>`))
	}()

	meta, err := storage.ExtractMetadata(epubPath, "EPUB")
	if err != nil {
		t.Fatalf("ExtractMetadata EPUB ISBN10: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if meta.ISBN10 != "0306406152" {
		t.Errorf("isbn10 = %q; want %q", meta.ISBN10, "0306406152")
	}
	if meta.ISBN13 != "" {
		t.Errorf("isbn13 = %q; want empty", meta.ISBN13)
	}
}
