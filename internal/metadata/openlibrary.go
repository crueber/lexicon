package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// OpenLibraryProvider implements Provider for the Open Library API.
// No API key is required.
type OpenLibraryProvider struct {
	httpClient *http.Client
	logger     *slog.Logger
	mu         sync.Mutex
	lastCall   time.Time
}

// NewOpenLibraryProvider creates a new OpenLibraryProvider.
func NewOpenLibraryProvider(logger *slog.Logger) *OpenLibraryProvider {
	return &OpenLibraryProvider{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Name returns the provider name.
func (p *OpenLibraryProvider) Name() string {
	return "open_library"
}

// Search searches Open Library for books matching the query.
func (p *OpenLibraryProvider) Search(ctx context.Context, query Query) ([]Result, error) {
	// ISBN lookup takes priority.
	if query.ISBN != "" {
		return p.searchByISBN(ctx, query.ISBN)
	}

	if query.Title == "" && query.Author == "" {
		return nil, nil
	}

	params := url.Values{}
	if query.Title != "" {
		params.Set("title", query.Title)
	}
	if query.Author != "" {
		params.Set("author", query.Author)
	}
	params.Set("limit", "10")

	apiURL := "https://openlibrary.org/search.json?" + params.Encode()

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("open library search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open library search: unexpected status %d", resp.StatusCode)
	}

	var result openLibrarySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("open library search: decode response: %w", err)
	}

	results := make([]Result, 0, len(result.Docs))
	for _, doc := range result.Docs {
		results = append(results, p.mapDocToResult(doc))
	}

	return results, nil
}

// FetchByID fetches a single book by its Open Library work key (e.g. "OL12345W").
func (p *OpenLibraryProvider) FetchByID(ctx context.Context, providerID string) (*Result, error) {
	apiURL := fmt.Sprintf("https://openlibrary.org/works/%s.json", url.PathEscape(providerID))

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("open library fetch by id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("open library fetch by id: work %q not found", providerID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open library fetch by id: unexpected status %d", resp.StatusCode)
	}

	var work openLibraryWork
	if err := json.NewDecoder(resp.Body).Decode(&work); err != nil {
		return nil, fmt.Errorf("open library fetch by id: decode response: %w", err)
	}

	result := p.mapWorkToResult(providerID, work)
	return &result, nil
}

// searchByISBN looks up a book by ISBN using the Open Library ISBN endpoint.
func (p *OpenLibraryProvider) searchByISBN(ctx context.Context, isbn string) ([]Result, error) {
	apiURL := fmt.Sprintf("https://openlibrary.org/isbn/%s.json", url.PathEscape(isbn))

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("open library isbn lookup: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open library isbn lookup: unexpected status %d", resp.StatusCode)
	}

	var edition openLibraryEdition
	if err := json.NewDecoder(resp.Body).Decode(&edition); err != nil {
		return nil, fmt.Errorf("open library isbn lookup: decode response: %w", err)
	}

	result := p.mapEditionToResult(edition)
	return []Result{result}, nil
}

// rateLimit enforces a minimum 1-second gap between requests.
// It is safe for concurrent use.
func (p *OpenLibraryProvider) rateLimit() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.lastCall.IsZero() {
		elapsed := time.Since(p.lastCall)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}
	p.lastCall = time.Now()
}

// doRequest performs an HTTP GET request.
func (p *OpenLibraryProvider) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	return p.httpClient.Do(req)
}

// mapDocToResult converts an Open Library search document to a Result.
func (p *OpenLibraryProvider) mapDocToResult(doc openLibraryDoc) Result {
	result := Result{
		Provider:    p.Name(),
		Title:       doc.Title,
		Authors:     doc.AuthorNames,
		PublishDate: p.firstYear(doc.PublishYear),
		Language:    p.firstString(doc.Language),
	}

	// Extract work key as provider ID (strip leading "/works/").
	if doc.Key != "" {
		result.ProviderID = stripPrefix(doc.Key, "/works/")
	}

	// Cover URL from cover ID.
	if doc.CoverI != 0 {
		result.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.CoverI)
	}

	// ISBNs.
	if len(doc.ISBN) > 0 {
		for _, isbn := range doc.ISBN {
			switch len(isbn) {
			case 10:
				if result.ISBN10 == "" {
					result.ISBN10 = isbn
				}
			case 13:
				if result.ISBN13 == "" {
					result.ISBN13 = isbn
				}
			}
		}
	}

	// Publisher.
	if len(doc.Publisher) > 0 {
		result.Publisher = doc.Publisher[0]
	}

	// Subject as categories.
	result.Categories = doc.Subject

	return result
}

// mapEditionToResult converts an Open Library edition (ISBN lookup) to a Result.
func (p *OpenLibraryProvider) mapEditionToResult(edition openLibraryEdition) Result {
	result := Result{
		Provider:    p.Name(),
		Title:       edition.Title,
		PublishDate: edition.PublishDate,
	}

	if len(edition.Publishers) > 0 {
		result.Publisher = edition.Publishers[0]
	}

	// Extract work key as provider ID.
	if edition.Works != nil && len(edition.Works) > 0 {
		result.ProviderID = stripPrefix(edition.Works[0].Key, "/works/")
	}

	// Cover URL.
	if len(edition.Covers) > 0 && edition.Covers[0] > 0 {
		result.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", edition.Covers[0])
	}

	// ISBNs.
	for _, isbn := range edition.ISBN10 {
		if result.ISBN10 == "" {
			result.ISBN10 = isbn
		}
	}
	for _, isbn := range edition.ISBN13 {
		if result.ISBN13 == "" {
			result.ISBN13 = isbn
		}
	}

	// Page count.
	result.PageCount = edition.NumberOfPages

	return result
}

// mapWorkToResult converts an Open Library work to a Result.
func (p *OpenLibraryProvider) mapWorkToResult(workID string, work openLibraryWork) Result {
	result := Result{
		ProviderID:  workID,
		Provider:    p.Name(),
		Title:       work.Title,
		Description: work.descriptionText(),
	}

	// Subjects as categories.
	result.Categories = work.Subjects

	return result
}

// firstString returns the first element of a string slice, or empty string.
func (p *OpenLibraryProvider) firstString(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}

// firstYear returns the first element of an int slice as a string, or empty string.
func (p *OpenLibraryProvider) firstYear(years []int) string {
	if len(years) == 0 {
		return ""
	}
	return strconv.Itoa(years[0])
}

// stripPrefix removes a leading prefix from s, returning the remainder.
func stripPrefix(s, prefix string) string {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

// openLibrarySearchResponse is the top-level response from the Open Library search API.
type openLibrarySearchResponse struct {
	NumFound int              `json:"numFound"`
	Docs     []openLibraryDoc `json:"docs"`
}

// openLibraryDoc represents a single document in the Open Library search results.
type openLibraryDoc struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	AuthorNames []string `json:"author_name"`
	PublishYear []int    `json:"publish_year"` // API returns integers
	Language    []string `json:"language"`
	ISBN        []string `json:"isbn"`
	Publisher   []string `json:"publisher"`
	Subject     []string `json:"subject"`
	CoverI      int64    `json:"cover_i"`
}

// openLibraryEdition represents an Open Library edition (from ISBN lookup).
type openLibraryEdition struct {
	Title         string               `json:"title"`
	Publishers    []string             `json:"publishers"`
	PublishDate   string               `json:"publish_date"`
	NumberOfPages int                  `json:"number_of_pages"`
	ISBN10        []string             `json:"isbn_10"`
	ISBN13        []string             `json:"isbn_13"`
	Covers        []int64              `json:"covers"`
	Works         []openLibraryWorkRef `json:"works"`
}

// openLibraryWorkRef is a reference to a work from an edition.
type openLibraryWorkRef struct {
	Key string `json:"key"`
}

// openLibraryWork represents an Open Library work (from works API).
type openLibraryWork struct {
	Title       string   `json:"title"`
	Description any      `json:"description"` // can be string or {"type": ..., "value": ...}
	Subjects    []string `json:"subjects"`
}

// descriptionText extracts the description text from the polymorphic description field.
func (w *openLibraryWork) descriptionText() string {
	if w.Description == nil {
		return ""
	}
	switch v := w.Description.(type) {
	case string:
		return v
	case map[string]any:
		if val, ok := v["value"].(string); ok {
			return val
		}
	}
	return ""
}
