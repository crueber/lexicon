package metadata

import "context"

// Provider is the interface all metadata providers must implement.
type Provider interface {
	Name() string
	Search(ctx context.Context, query Query) ([]Result, error)
	FetchByID(ctx context.Context, providerID string) (*Result, error)
}

// Query holds search parameters.
type Query struct {
	Title     string
	Author    string
	ISBN      string
	BookType  string // EBOOK, AUDIOBOOK, COMIC
	LibraryID int64  // optional: filter providers by library metadata sources
}

// Result holds metadata from a provider.
type Result struct {
	ProviderID  string
	Provider    string
	Title       string
	Subtitle    string
	Authors     []string
	Description string
	Publisher   string
	PublishDate string
	PageCount   int
	Language    string
	ISBN10      string
	ISBN13      string
	CoverURL    string
	Series      string
	SeriesIndex float64
	Categories  []string
	Tags        []string
	// Provider-specific IDs
	GoogleBooksID string
	AmazonID      string
	GoodreadsID   string
	HardcoverID   string
	AudibleID     string
	ComicVineID   string
}

// Proposal is a saved metadata result awaiting acceptance or rejection.
type Proposal struct {
	ID         int64
	BookID     int64
	Provider   string
	ProviderID string
	Status     string
	Data       Result
	CreatedAt  string
}


