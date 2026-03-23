package storage

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/dhowden/tag"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
)

// BookMetadata holds metadata extracted from a book file.
type BookMetadata struct {
	Title       string
	Authors     []string
	Description string
	Publisher   string
	PublishDate string
	Language    string
	ISBN10      string
	ISBN13      string
	Series      string
	SeriesIndex float64
	Categories  []string
	Tags        []string
	// Comic-specific
	ComicVolume        *int
	ComicNumber        *float64
	ComicYear          *int
	ComicWriter        string
	ComicPublisher     string
	ComicGenre         string
	ComicAgeRating     string
	ComicBlackAndWhite bool
	ComicManga         string
	ComicCharacters    []string
	ComicTeams         []string
	ComicLocations     []string
	ComicStoryArc      string
	// Audio-specific
	TrackTitle  string
	TrackNumber int
	AlbumTitle  string // = book title for audiobooks
	Duration    int    // seconds
}

// ExtractMetadata extracts embedded metadata from a book file.
// Returns nil if no metadata can be extracted (not an error).
func ExtractMetadata(filePath, format string) (*BookMetadata, error) {
	switch format {
	case "EPUB":
		return extractEPUBMetadata(filePath)
	case "PDF":
		return extractPDFMetadata(filePath)
	case "CBZ":
		return extractCBZMetadata(filePath)
	case "M4B", "M4A", "MP3":
		return extractAudioMetadata(filePath)
	default:
		return nil, nil
	}
}

// --- EPUB ---

// opfMetadataPackage is used to parse the full OPF metadata section.
type opfMetadataPackage struct {
	Metadata opfMetadata `xml:"metadata"`
}

type opfMetadata struct {
	Titles       []string        `xml:"title"`
	Creators     []opfCreator    `xml:"creator"`
	Descriptions []string        `xml:"description"`
	Publishers   []string        `xml:"publisher"`
	Dates        []string        `xml:"date"`
	Languages    []string        `xml:"language"`
	Identifiers  []opfIdentifier `xml:"identifier"`
	Subjects     []string        `xml:"subject"`
	Metas        []opfMeta       `xml:"meta"`
}

type opfCreator struct {
	Value string `xml:",chardata"`
	Role  string `xml:"role,attr"`
}

type opfIdentifier struct {
	Value  string `xml:",chardata"`
	Scheme string `xml:"scheme,attr"`
	ID     string `xml:"id,attr"`
}

type opfMeta struct {
	Name    string `xml:"name,attr"`
	Content string `xml:"content,attr"`
	// EPUB3 style
	Property string `xml:",chardata"`
	Prop     string `xml:"property,attr"`
}

func extractEPUBMetadata(filePath string) (*BookMetadata, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("open epub: %w", err)
	}
	defer r.Close()

	opfPath, err := findOPFPath(r)
	if err != nil || opfPath == "" {
		return nil, nil //nolint:nilerr // no OPF = no metadata, not an error
	}

	var opfFile *zip.File
	for _, f := range r.File {
		if f.Name == opfPath {
			opfFile = f
			break
		}
	}
	if opfFile == nil {
		return nil, nil
	}

	data, err := readZipFile(opfFile)
	if err != nil {
		return nil, fmt.Errorf("read opf: %w", err)
	}

	var pkg opfMetadataPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse opf: %w", err)
	}

	meta := &BookMetadata{}
	m := pkg.Metadata

	// Title
	if len(m.Titles) > 0 {
		meta.Title = strings.TrimSpace(m.Titles[0])
	}

	// Authors (dc:creator)
	for _, c := range m.Creators {
		name := strings.TrimSpace(c.Value)
		if name != "" {
			meta.Authors = append(meta.Authors, name)
		}
	}

	// Description
	if len(m.Descriptions) > 0 {
		meta.Description = strings.TrimSpace(m.Descriptions[0])
	}

	// Publisher
	if len(m.Publishers) > 0 {
		meta.Publisher = strings.TrimSpace(m.Publishers[0])
	}

	// Date
	if len(m.Dates) > 0 {
		meta.PublishDate = strings.TrimSpace(m.Dates[0])
	}

	// Language
	if len(m.Languages) > 0 {
		meta.Language = strings.TrimSpace(m.Languages[0])
	}

	// Identifiers (ISBN)
	for _, id := range m.Identifiers {
		val := strings.TrimSpace(id.Value)
		scheme := strings.ToLower(id.Scheme)
		idLower := strings.ToLower(id.ID)
		if strings.Contains(scheme, "isbn") || strings.Contains(idLower, "isbn") {
			digits := extractDigits(val)
			switch len(digits) {
			case 10:
				meta.ISBN10 = digits
			case 13:
				meta.ISBN13 = digits
			}
		}
	}

	// Subjects → Categories
	for _, s := range m.Subjects {
		s = strings.TrimSpace(s)
		if s != "" {
			meta.Categories = append(meta.Categories, s)
		}
	}

	// Meta tags (Calibre extensions)
	for _, mt := range m.Metas {
		name := strings.ToLower(mt.Name)
		content := strings.TrimSpace(mt.Content)
		switch name {
		case "calibre:series":
			meta.Series = content
		case "calibre:series_index":
			if v, err := strconv.ParseFloat(content, 64); err == nil {
				meta.SeriesIndex = v
			}
		case "calibre:subject":
			if content != "" {
				meta.Categories = append(meta.Categories, content)
			}
		}
	}

	if isEmptyMetadata(meta) {
		return nil, nil
	}
	return meta, nil
}

// --- PDF ---

func extractPDFMetadata(filePath string) (*BookMetadata, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	info, err := pdfapi.PDFInfo(f, filePath, nil, false, nil)
	if err != nil {
		// PDF may be malformed or encrypted — not a fatal error.
		return nil, nil //nolint:nilerr
	}

	meta := &BookMetadata{}

	if info.Title != "" {
		meta.Title = strings.TrimSpace(info.Title)
	}
	if info.Author != "" {
		author := strings.TrimSpace(info.Author)
		if author != "" {
			meta.Authors = []string{author}
		}
	}
	if info.Subject != "" {
		meta.Description = strings.TrimSpace(info.Subject)
	}
	if len(info.Keywords) > 0 {
		for _, kw := range info.Keywords {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				meta.Tags = append(meta.Tags, kw)
			}
		}
	}
	if info.CreationDate != "" {
		meta.PublishDate = strings.TrimSpace(info.CreationDate)
	}

	if isEmptyMetadata(meta) {
		return nil, nil
	}
	return meta, nil
}

// --- CBZ ---

// comicInfoXML represents the ComicInfo.xml schema used in CBZ files.
type comicInfoXML struct {
	Title         string `xml:"Title"`
	Series        string `xml:"Series"`
	Number        string `xml:"Number"`
	Volume        *int   `xml:"Volume"`
	Year          *int   `xml:"Year"`
	Month         *int   `xml:"Month"`
	Writer        string `xml:"Writer"`
	Publisher     string `xml:"Publisher"`
	Genre         string `xml:"Genre"`
	Characters    string `xml:"Characters"`
	Teams         string `xml:"Teams"`
	Locations     string `xml:"Locations"`
	StoryArc      string `xml:"StoryArc"`
	AgeRating     string `xml:"AgeRating"`
	BlackAndWhite string `xml:"BlackAndWhite"`
	Manga         string `xml:"Manga"`
	LanguageISO   string `xml:"LanguageISO"`
}

func extractCBZMetadata(filePath string) (*BookMetadata, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("open cbz: %w", err)
	}
	defer r.Close()

	var comicInfoFile *zip.File
	for _, f := range r.File {
		if strings.EqualFold(filepath.Base(f.Name), "comicinfo.xml") {
			comicInfoFile = f
			break
		}
	}
	if comicInfoFile == nil {
		return nil, nil
	}

	data, err := readZipFile(comicInfoFile)
	if err != nil {
		return nil, fmt.Errorf("read comicinfo.xml: %w", err)
	}

	var ci comicInfoXML
	if err := xml.Unmarshal(data, &ci); err != nil {
		return nil, fmt.Errorf("parse comicinfo.xml: %w", err)
	}

	meta := &BookMetadata{}

	if ci.Title != "" {
		meta.Title = strings.TrimSpace(ci.Title)
	}
	if ci.Series != "" {
		meta.Series = strings.TrimSpace(ci.Series)
	}
	if ci.Number != "" {
		if v, err := strconv.ParseFloat(strings.TrimSpace(ci.Number), 64); err == nil {
			meta.SeriesIndex = v
			meta.ComicNumber = &v
		}
	}
	if ci.Volume != nil {
		meta.ComicVolume = ci.Volume
	}
	if ci.Year != nil {
		meta.ComicYear = ci.Year
		if ci.Month != nil {
			meta.PublishDate = fmt.Sprintf("%04d-%02d", *ci.Year, *ci.Month)
		} else {
			meta.PublishDate = fmt.Sprintf("%04d", *ci.Year)
		}
	}
	if ci.Writer != "" {
		writer := strings.TrimSpace(ci.Writer)
		if writer != "" {
			meta.Authors = []string{writer}
			meta.ComicWriter = writer
		}
	}
	if ci.Publisher != "" {
		meta.Publisher = strings.TrimSpace(ci.Publisher)
		meta.ComicPublisher = meta.Publisher
	}
	if ci.Genre != "" {
		genre := strings.TrimSpace(ci.Genre)
		if genre != "" {
			meta.Categories = []string{genre}
			meta.ComicGenre = genre
		}
	}
	if ci.Characters != "" {
		meta.ComicCharacters = splitCommaList(ci.Characters)
	}
	if ci.Teams != "" {
		meta.ComicTeams = splitCommaList(ci.Teams)
	}
	if ci.Locations != "" {
		meta.ComicLocations = splitCommaList(ci.Locations)
	}
	if ci.StoryArc != "" {
		meta.ComicStoryArc = strings.TrimSpace(ci.StoryArc)
	}
	if ci.AgeRating != "" {
		meta.ComicAgeRating = strings.TrimSpace(ci.AgeRating)
	}
	if strings.EqualFold(strings.TrimSpace(ci.BlackAndWhite), "yes") {
		meta.ComicBlackAndWhite = true
	}
	if ci.Manga != "" {
		meta.ComicManga = strings.TrimSpace(ci.Manga)
	}
	if ci.LanguageISO != "" {
		meta.Language = strings.TrimSpace(ci.LanguageISO)
	}

	if isEmptyMetadata(meta) {
		return nil, nil
	}
	return meta, nil
}

// --- Audio ---

func extractAudioMetadata(filePath string) (*BookMetadata, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open audio file: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		// Not all audio files have tags — not an error.
		return nil, nil //nolint:nilerr
	}

	meta := &BookMetadata{}

	if m.Title() != "" {
		meta.TrackTitle = strings.TrimSpace(m.Title())
	}
	if m.Album() != "" {
		meta.AlbumTitle = strings.TrimSpace(m.Album())
		meta.Title = meta.AlbumTitle
	}

	// Prefer Artist; fall back to AlbumArtist.
	artist := strings.TrimSpace(m.Artist())
	if artist == "" {
		artist = strings.TrimSpace(m.AlbumArtist())
	}
	if artist != "" {
		meta.Authors = []string{artist}
	}

	if m.Genre() != "" {
		genre := strings.TrimSpace(m.Genre())
		if genre != "" {
			meta.Categories = []string{genre}
		}
	}

	if m.Year() != 0 {
		meta.PublishDate = strconv.Itoa(m.Year())
	}

	trackNum, _ := m.Track()
	if trackNum > 0 {
		meta.TrackNumber = trackNum
	}

	if isEmptyMetadata(meta) {
		return nil, nil
	}
	return meta, nil
}

// --- Helpers ---

// extractDigits strips all non-digit characters from s and returns the result.
func extractDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitCommaList splits a comma-separated string and trims whitespace from each element.
func splitCommaList(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isEmptyMetadata returns true if the metadata has no meaningful content.
func isEmptyMetadata(m *BookMetadata) bool {
	return m.Title == "" &&
		len(m.Authors) == 0 &&
		m.Description == "" &&
		m.Publisher == "" &&
		m.PublishDate == "" &&
		m.Language == "" &&
		m.ISBN10 == "" &&
		m.ISBN13 == "" &&
		m.Series == "" &&
		len(m.Categories) == 0 &&
		len(m.Tags) == 0 &&
		m.AlbumTitle == "" &&
		m.TrackTitle == ""
}
