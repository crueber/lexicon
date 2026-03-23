package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// GoogleBooksProvider implements Provider for the Google Books API.
type GoogleBooksProvider struct {
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
	mu         sync.Mutex
	lastCall   time.Time
}

// NewGoogleBooksProvider creates a new GoogleBooksProvider.
// apiKey is optional; if empty, requests still work but are rate-limited more aggressively.
func NewGoogleBooksProvider(apiKey string, logger *slog.Logger) *GoogleBooksProvider {
	return &GoogleBooksProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Name returns the provider name.
func (p *GoogleBooksProvider) Name() string {
	return "google_books"
}

// Search searches Google Books for books matching the query.
func (p *GoogleBooksProvider) Search(ctx context.Context, query Query) ([]Result, error) {
	q := p.buildQuery(query)
	if q == "" {
		return nil, nil
	}

	apiURL := p.buildSearchURL(q)

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("google books search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google books search: unexpected status %d", resp.StatusCode)
	}

	var result googleBooksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("google books search: decode response: %w", err)
	}

	results := make([]Result, 0, len(result.Items))
	for _, item := range result.Items {
		results = append(results, p.mapToResult(item))
	}

	return results, nil
}

// FetchByID fetches a single book by its Google Books volume ID.
func (p *GoogleBooksProvider) FetchByID(ctx context.Context, providerID string) (*Result, error) {
	apiURL := fmt.Sprintf("https://www.googleapis.com/books/v1/volumes/%s", url.PathEscape(providerID))
	if p.apiKey != "" {
		apiURL += "?key=" + url.QueryEscape(p.apiKey)
	}

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("google books fetch by id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("google books fetch by id: volume %q not found", providerID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google books fetch by id: unexpected status %d", resp.StatusCode)
	}

	var item googleBooksVolume
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("google books fetch by id: decode response: %w", err)
	}

	result := p.mapToResult(item)
	return &result, nil
}

// buildQuery constructs the Google Books query string from a Query.
func (p *GoogleBooksProvider) buildQuery(query Query) string {
	if query.ISBN != "" {
		return "isbn:" + query.ISBN
	}

	var parts []string
	if query.Title != "" {
		parts = append(parts, "intitle:"+query.Title)
	}
	if query.Author != "" {
		parts = append(parts, "inauthor:"+query.Author)
	}

	return strings.Join(parts, "+")
}

// buildSearchURL constructs the full search URL.
func (p *GoogleBooksProvider) buildSearchURL(q string) string {
	params := url.Values{}
	params.Set("q", q)
	params.Set("maxResults", "10")
	if p.apiKey != "" {
		params.Set("key", p.apiKey)
	}
	return "https://www.googleapis.com/books/v1/volumes?" + params.Encode()
}

// rateLimit enforces a minimum 1-second gap between requests.
// It is safe for concurrent use.
func (p *GoogleBooksProvider) rateLimit() {
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
func (p *GoogleBooksProvider) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	return p.httpClient.Do(req)
}

// mapToResult converts a Google Books volume to a Result.
func (p *GoogleBooksProvider) mapToResult(item googleBooksVolume) Result {
	info := item.VolumeInfo

	result := Result{
		ProviderID:    item.ID,
		Provider:      p.Name(),
		Title:         info.Title,
		Subtitle:      info.Subtitle,
		Authors:       info.Authors,
		Description:   info.Description,
		Publisher:     info.Publisher,
		PublishDate:   info.PublishedDate,
		PageCount:     info.PageCount,
		Language:      info.Language,
		Categories:    info.Categories,
		GoogleBooksID: item.ID,
	}

	// Extract ISBNs from industry identifiers.
	for _, id := range info.IndustryIdentifiers {
		switch id.Type {
		case "ISBN_10":
			result.ISBN10 = id.Identifier
		case "ISBN_13":
			result.ISBN13 = id.Identifier
		}
	}

	// Extract cover image URL (prefer large, fall back to thumbnail).
	if info.ImageLinks.Large != "" {
		result.CoverURL = info.ImageLinks.Large
	} else if info.ImageLinks.Medium != "" {
		result.CoverURL = info.ImageLinks.Medium
	} else if info.ImageLinks.Thumbnail != "" {
		result.CoverURL = info.ImageLinks.Thumbnail
	} else if info.ImageLinks.SmallThumbnail != "" {
		result.CoverURL = info.ImageLinks.SmallThumbnail
	}

	// Use HTTPS for cover URLs.
	if strings.HasPrefix(result.CoverURL, "http://") {
		result.CoverURL = "https://" + result.CoverURL[7:]
	}

	return result
}

// googleBooksResponse is the top-level response from the Google Books volumes search API.
type googleBooksResponse struct {
	TotalItems int                 `json:"totalItems"`
	Items      []googleBooksVolume `json:"items"`
}

// googleBooksVolume represents a single volume in the Google Books API.
type googleBooksVolume struct {
	ID         string                `json:"id"`
	VolumeInfo googleBooksVolumeInfo `json:"volumeInfo"`
}

// googleBooksVolumeInfo holds the metadata for a volume.
type googleBooksVolumeInfo struct {
	Title               string                  `json:"title"`
	Subtitle            string                  `json:"subtitle"`
	Authors             []string                `json:"authors"`
	Publisher           string                  `json:"publisher"`
	PublishedDate       string                  `json:"publishedDate"`
	Description         string                  `json:"description"`
	IndustryIdentifiers []googleBooksIdentifier `json:"industryIdentifiers"`
	PageCount           int                     `json:"pageCount"`
	Categories          []string                `json:"categories"`
	Language            string                  `json:"language"`
	ImageLinks          googleBooksImageLinks   `json:"imageLinks"`
}

// googleBooksIdentifier holds an industry identifier (ISBN, etc.).
type googleBooksIdentifier struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

// googleBooksImageLinks holds image URLs for a volume.
type googleBooksImageLinks struct {
	SmallThumbnail string `json:"smallThumbnail"`
	Thumbnail      string `json:"thumbnail"`
	Small          string `json:"small"`
	Medium         string `json:"medium"`
	Large          string `json:"large"`
	ExtraLarge     string `json:"extraLarge"`
}
