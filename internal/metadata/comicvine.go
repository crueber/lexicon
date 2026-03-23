package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ComicVineProvider implements Provider for the ComicVine REST API.
// It only returns results when the query BookType is "COMIC".
type ComicVineProvider struct {
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
	mu         sync.Mutex
	lastCall   time.Time
}

// NewComicVineProvider creates a new ComicVineProvider.
// If apiKey is empty, Search and FetchByID return empty results.
func NewComicVineProvider(apiKey string, logger *slog.Logger) *ComicVineProvider {
	return &ComicVineProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Name returns the provider name.
func (p *ComicVineProvider) Name() string {
	return "comic_vine"
}

// Search searches ComicVine for comic issues matching the query.
// Returns empty results if no API key is configured or if BookType is not "COMIC".
func (p *ComicVineProvider) Search(ctx context.Context, query Query) ([]Result, error) {
	if p.apiKey == "" {
		p.logger.Debug("comic vine provider: no API key configured, skipping search")
		return nil, nil
	}

	if query.BookType != "COMIC" {
		return nil, nil
	}

	if query.Title == "" {
		return nil, nil
	}

	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("format", "json")
	params.Set("query", query.Title)
	params.Set("resources", "issue")
	params.Set("limit", "10")

	apiURL := "https://comicvine.gamespot.com/api/search/?" + params.Encode()

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("comic vine search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("comic vine search: unexpected status %d", resp.StatusCode)
	}

	var result comicVineSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("comic vine search: decode response: %w", err)
	}

	if result.StatusCode != 1 {
		return nil, fmt.Errorf("comic vine search: API error %d: %s", result.StatusCode, result.Error)
	}

	results := make([]Result, 0, len(result.Results))
	for _, issue := range result.Results {
		results = append(results, p.mapToResult(issue))
	}

	return results, nil
}

// FetchByID fetches a single comic issue by its ComicVine issue ID.
// Returns nil if no API key is configured.
func (p *ComicVineProvider) FetchByID(ctx context.Context, providerID string) (*Result, error) {
	if p.apiKey == "" {
		p.logger.Debug("comic vine provider: no API key configured, skipping fetch")
		return nil, nil
	}

	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("format", "json")

	apiURL := fmt.Sprintf("https://comicvine.gamespot.com/api/issue/4000-%s/?%s",
		url.PathEscape(providerID), params.Encode())

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("comic vine fetch by id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("comic vine fetch by id: issue %q not found", providerID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("comic vine fetch by id: unexpected status %d", resp.StatusCode)
	}

	var result comicVineFetchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("comic vine fetch by id: decode response: %w", err)
	}

	if result.StatusCode != 1 {
		return nil, fmt.Errorf("comic vine fetch by id: API error %d: %s", result.StatusCode, result.Error)
	}

	r := p.mapToResult(result.Results)
	return &r, nil
}

// rateLimit enforces a minimum 1-second gap between requests.
// It is safe for concurrent use.
func (p *ComicVineProvider) rateLimit() {
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
func (p *ComicVineProvider) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	// ComicVine requires a User-Agent header.
	req.Header.Set("User-Agent", "Lexicon/1.0")

	return p.httpClient.Do(req)
}

// mapToResult converts a ComicVine issue to a Result.
func (p *ComicVineProvider) mapToResult(issue comicVineIssue) Result {
	result := Result{
		ProviderID:  fmt.Sprintf("%d", issue.ID),
		Provider:    p.Name(),
		Title:       issue.Name,
		Description: issue.Description,
		PublishDate: issue.CoverDate,
		ComicVineID: fmt.Sprintf("%d", issue.ID),
	}

	// Volume name maps to series.
	if issue.Volume.Name != "" {
		result.Series = issue.Volume.Name
	}

	// Issue number maps to series index.
	if issue.IssueNumber != "" {
		var idx float64
		if _, err := fmt.Sscanf(issue.IssueNumber, "%f", &idx); err == nil {
			result.SeriesIndex = idx
		}
	}

	// Cover image.
	if issue.Image.MediumURL != "" {
		result.CoverURL = issue.Image.MediumURL
	} else if issue.Image.SmallURL != "" {
		result.CoverURL = issue.Image.SmallURL
	}

	// Use the volume name as the title if the issue has no name.
	if result.Title == "" && issue.Volume.Name != "" {
		result.Title = issue.Volume.Name
		if issue.IssueNumber != "" {
			result.Title += " #" + issue.IssueNumber
		}
	}

	return result
}

// comicVineSearchResponse is the top-level response from the ComicVine search API.
type comicVineSearchResponse struct {
	Error      string           `json:"error"`
	StatusCode int              `json:"status_code"`
	Results    []comicVineIssue `json:"results"`
}

// comicVineFetchResponse is the top-level response from the ComicVine issue fetch API.
type comicVineFetchResponse struct {
	Error      string         `json:"error"`
	StatusCode int            `json:"status_code"`
	Results    comicVineIssue `json:"results"`
}

// comicVineIssue represents a single comic issue in the ComicVine API.
type comicVineIssue struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	CoverDate   string          `json:"cover_date"`
	IssueNumber string          `json:"issue_number"`
	Volume      comicVineVolume `json:"volume"`
	Image       comicVineImage  `json:"image"`
}

// comicVineVolume represents a comic volume (series) in the ComicVine API.
type comicVineVolume struct {
	Name string `json:"name"`
}

// comicVineImage holds image URLs for a ComicVine issue.
type comicVineImage struct {
	SmallURL  string `json:"small_url"`
	MediumURL string `json:"medium_url"`
	LargeURL  string `json:"large_url"`
}
