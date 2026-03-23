package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const _hardcoverGraphQLURL = "https://api.hardcover.app/v1/graphql"

// hardcoverSearchQuery is the GraphQL query used to search for books.
const _hardcoverSearchQuery = `
query SearchBooks($query: String!) {
  search(query: $query, query_type: "Book", per_page: 10) {
    results {
      ... on Book {
        id
        title
        contributions { author { name } }
        description
        release_date
        pages
        language { language }
        image { url }
        isbn_13
        series_books { series { name } position }
      }
    }
  }
}`

// HardcoverProvider implements Provider for the Hardcover GraphQL API.
type HardcoverProvider struct {
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
	mu         sync.Mutex
	lastCall   time.Time
}

// NewHardcoverProvider creates a new HardcoverProvider.
// If apiKey is empty, Search and FetchByID return empty results.
func NewHardcoverProvider(apiKey string, logger *slog.Logger) *HardcoverProvider {
	return &HardcoverProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Name returns the provider name.
func (p *HardcoverProvider) Name() string {
	return "hardcover"
}

// Search searches Hardcover for books matching the query.
// Returns empty results if no API key is configured.
func (p *HardcoverProvider) Search(ctx context.Context, query Query) ([]Result, error) {
	if p.apiKey == "" {
		p.logger.Debug("hardcover provider: no API key configured, skipping search")
		return nil, nil
	}

	q := buildHardcoverQuery(query)
	if q == "" {
		return nil, nil
	}

	body, err := json.Marshal(map[string]any{
		"query":     _hardcoverSearchQuery,
		"variables": map[string]string{"query": q},
	})
	if err != nil {
		return nil, fmt.Errorf("hardcover search: marshal request: %w", err)
	}

	p.rateLimit()

	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("hardcover search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hardcover search: unexpected status %d", resp.StatusCode)
	}

	var result hardcoverSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hardcover search: decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("hardcover search: graphql error: %s", result.Errors[0].Message)
	}

	books := result.Data.Search.Results
	results := make([]Result, 0, len(books))
	for _, book := range books {
		results = append(results, p.mapToResult(book))
	}

	return results, nil
}

// FetchByID fetches a single book by its Hardcover ID.
// Returns nil if no API key is configured.
func (p *HardcoverProvider) FetchByID(ctx context.Context, providerID string) (*Result, error) {
	if p.apiKey == "" {
		p.logger.Debug("hardcover provider: no API key configured, skipping fetch")
		return nil, nil
	}

	const fetchQuery = `
query FetchBook($id: Int!) {
  books(where: {id: {_eq: $id}}) {
    id
    title
    contributions { author { name } }
    description
    release_date
    pages
    language { language }
    image { url }
    isbn_13
    series_books { series { name } position }
  }
}`

	body, err := json.Marshal(map[string]any{
		"query":     fetchQuery,
		"variables": map[string]string{"id": providerID},
	})
	if err != nil {
		return nil, fmt.Errorf("hardcover fetch by id: marshal request: %w", err)
	}

	p.rateLimit()

	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("hardcover fetch by id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hardcover fetch by id: unexpected status %d", resp.StatusCode)
	}

	var result hardcoverFetchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hardcover fetch by id: decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("hardcover fetch by id: graphql error: %s", result.Errors[0].Message)
	}

	if len(result.Data.Books) == 0 {
		return nil, nil
	}

	r := p.mapToResult(result.Data.Books[0])
	return &r, nil
}

// rateLimit enforces a minimum 1-second gap between requests.
// It is safe for concurrent use.
func (p *HardcoverProvider) rateLimit() {
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

// doRequest performs a GraphQL POST request to the Hardcover API.
func (p *HardcoverProvider) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, _hardcoverGraphQLURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	return p.httpClient.Do(req)
}

// mapToResult converts a Hardcover book to a Result.
func (p *HardcoverProvider) mapToResult(book hardcoverBook) Result {
	result := Result{
		ProviderID:  fmt.Sprintf("%d", book.ID),
		Provider:    p.Name(),
		Title:       book.Title,
		Description: book.Description,
		PublishDate: book.ReleaseDate,
		PageCount:   book.Pages,
		ISBN13:      book.ISBN13,
		HardcoverID: fmt.Sprintf("%d", book.ID),
	}

	// Extract authors from contributions.
	for _, c := range book.Contributions {
		if c.Author.Name != "" {
			result.Authors = append(result.Authors, c.Author.Name)
		}
	}

	// Language.
	if book.Language != nil {
		result.Language = book.Language.Language
	}

	// Cover image.
	if book.Image != nil {
		result.CoverURL = book.Image.URL
	}

	// Series info (use first series entry).
	if len(book.SeriesBooks) > 0 {
		sb := book.SeriesBooks[0]
		result.Series = sb.Series.Name
		result.SeriesIndex = sb.Position
	}

	return result
}

// buildHardcoverQuery constructs a search query string from a Query.
func buildHardcoverQuery(query Query) string {
	if query.ISBN != "" {
		return query.ISBN
	}
	if query.Title != "" && query.Author != "" {
		return query.Title + " " + query.Author
	}
	if query.Title != "" {
		return query.Title
	}
	return query.Author
}

// hardcoverSearchResponse is the top-level GraphQL response for a search.
type hardcoverSearchResponse struct {
	Data struct {
		Search struct {
			Results []hardcoverBook `json:"results"`
		} `json:"search"`
	} `json:"data"`
	Errors []hardcoverError `json:"errors"`
}

// hardcoverFetchResponse is the top-level GraphQL response for a fetch by ID.
type hardcoverFetchResponse struct {
	Data struct {
		Books []hardcoverBook `json:"books"`
	} `json:"data"`
	Errors []hardcoverError `json:"errors"`
}

// hardcoverError represents a GraphQL error.
type hardcoverError struct {
	Message string `json:"message"`
}

// hardcoverBook represents a book in the Hardcover API.
type hardcoverBook struct {
	ID            int                     `json:"id"`
	Title         string                  `json:"title"`
	Contributions []hardcoverContribution `json:"contributions"`
	Description   string                  `json:"description"`
	ReleaseDate   string                  `json:"release_date"`
	Pages         int                     `json:"pages"`
	Language      *hardcoverLanguage      `json:"language"`
	Image         *hardcoverImage         `json:"image"`
	ISBN13        string                  `json:"isbn_13"`
	SeriesBooks   []hardcoverSeriesBook   `json:"series_books"`
}

// hardcoverContribution links a book to an author.
type hardcoverContribution struct {
	Author hardcoverAuthor `json:"author"`
}

// hardcoverAuthor represents an author in the Hardcover API.
type hardcoverAuthor struct {
	Name string `json:"name"`
}

// hardcoverLanguage represents a language in the Hardcover API.
type hardcoverLanguage struct {
	Language string `json:"language"`
}

// hardcoverImage represents a cover image in the Hardcover API.
type hardcoverImage struct {
	URL string `json:"url"`
}

// hardcoverSeriesBook links a book to a series with a position.
type hardcoverSeriesBook struct {
	Series   hardcoverSeries `json:"series"`
	Position float64         `json:"position"`
}

// hardcoverSeries represents a series in the Hardcover API.
type hardcoverSeries struct {
	Name string `json:"name"`
}
