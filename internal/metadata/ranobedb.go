package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// RanobeDBProvider implements Provider for RanobeDB (light novels).
// It uses the REST API at ranobedb.org/api/v0/.
type RanobeDBProvider struct {
	httpClient *http.Client
	logger     *slog.Logger
	mu         sync.Mutex
	lastCall   time.Time
}

// NewRanobeDBProvider creates a new RanobeDBProvider.
func NewRanobeDBProvider(logger *slog.Logger) *RanobeDBProvider {
	return &RanobeDBProvider{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Name returns the provider name.
func (p *RanobeDBProvider) Name() string {
	return "ranobedb"
}

// Search searches RanobeDB for books matching the query.
func (p *RanobeDBProvider) Search(ctx context.Context, query Query) ([]Result, error) {
	q := query.Title
	if q == "" {
		q = query.Author
	}
	if q == "" {
		return nil, nil
	}

	apiURL := "https://ranobedb.org/api/v0/books?q=" + url.QueryEscape(q)

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("ranobedb search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ranobedb search: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ranobedb search: read body: %w", err)
	}

	var searchResp ranobeDBSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("ranobedb search: decode response: %w", err)
	}

	results := make([]Result, 0, len(searchResp.Books))
	for _, book := range searchResp.Books {
		results = append(results, p.mapBookToResult(book))
	}

	return results, nil
}

// FetchByID fetches a single book by its RanobeDB book ID.
func (p *RanobeDBProvider) FetchByID(ctx context.Context, providerID string) (*Result, error) {
	apiURL := fmt.Sprintf("https://ranobedb.org/api/v0/book/%s", url.PathEscape(providerID))

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("ranobedb fetch by id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("ranobedb fetch by id: book %q not found", providerID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ranobedb fetch by id: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ranobedb fetch by id: read body: %w", err)
	}

	var detailResp ranobeDBDetailResponse
	if err := json.Unmarshal(body, &detailResp); err != nil {
		return nil, fmt.Errorf("ranobedb fetch by id: decode response: %w", err)
	}

	result := p.mapDetailToResult(providerID, detailResp.Book)
	return &result, nil
}

// rateLimit enforces a minimum 1-second gap between requests.
func (p *RanobeDBProvider) rateLimit() {
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
func (p *RanobeDBProvider) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	return p.httpClient.Do(req)
}

// mapBookToResult converts a RanobeDB search book to a Result.
func (p *RanobeDBProvider) mapBookToResult(book ranobeDBSearchBook) Result {
	result := Result{
		ProviderID: strconv.Itoa(book.ID),
		Provider:   p.Name(),
		Language:   book.Lang,
	}

	// Prefer romaji title, fallback to title.
	if book.Romaji != nil && *book.Romaji != "" {
		result.Title = *book.Romaji
	} else if book.Title != "" {
		result.Title = book.Title
	}

	// Cover URL.
	if book.Image.Filename != "" {
		result.CoverURL = "https://images.ranobedb.org/" + book.Image.Filename
	}

	// Publish date from c_release_date (YYYYMMDD integer).
	if book.CReleaseDate != 0 {
		s := strconv.Itoa(book.CReleaseDate)
		if len(s) == 8 {
			result.PublishDate = s[:4] + "-" + s[4:6] + "-" + s[6:]
		}
	}

	return result
}

// mapDetailToResult converts a RanobeDB detail book to a Result.
func (p *RanobeDBProvider) mapDetailToResult(providerID string, book ranobeDBDetailBook) Result {
	result := Result{
		ProviderID: providerID,
		Provider:   p.Name(),
		Language:   book.Lang,
	}

	// Prefer romaji title, fallback to title.
	if book.Romaji != nil && *book.Romaji != "" {
		result.Title = *book.Romaji
	} else if book.Title != "" {
		result.Title = book.Title
	}

	// Description.
	if book.Description != "" {
		result.Description = book.Description
	} else if book.DescriptionJA != "" {
		result.Description = book.DescriptionJA
	}

	// Cover URL.
	if book.Image.Filename != "" {
		result.CoverURL = "https://images.ranobedb.org/" + book.Image.Filename
	}

	// Authors from editions staff.
	if len(book.Editions) > 0 {
		for _, staff := range book.Editions[0].Staff {
			if staff.RoleType == "author" && staff.Name != "" {
				result.Authors = append(result.Authors, staff.Name)
			}
		}
	}

	// Publisher from publishers list.
	if len(book.Publishers) > 0 {
		result.Publisher = book.Publishers[0].Name
	}

	// ISBN from releases.
	for _, release := range book.Releases {
		if release.ISBN13 != "" && result.ISBN13 == "" {
			result.ISBN13 = release.ISBN13
		}
		if release.Pages > 0 && result.PageCount == 0 {
			result.PageCount = release.Pages
		}
	}

	// Publish date.
	if book.CReleaseDate != 0 {
		s := strconv.Itoa(book.CReleaseDate)
		if len(s) == 8 {
			result.PublishDate = s[:4] + "-" + s[4:6] + "-" + s[6:]
		}
	}

	// Series.
	if book.Series.Title != "" {
		result.Series = book.Series.Title
	}

	// Tags.
	for _, tag := range book.Tags {
		if tag.Name != "" {
			result.Tags = append(result.Tags, tag.Name)
		}
	}

	return result
}

// ranobeDBSearchResponse is the top-level response from the RanobeDB search API.
type ranobeDBSearchResponse struct {
	Books       []ranobeDBSearchBook `json:"books"`
	Count       string               `json:"count"`
	CurrentPage int                  `json:"currentPage"`
	TotalPages  int                  `json:"totalPages"`
}

// ranobeDBSearchBook represents a book in the RanobeDB search results.
type ranobeDBSearchBook struct {
	ID           int                `json:"id"`
	ImageID      int                `json:"image_id"`
	Lang         string             `json:"lang"`
	Romaji       *string            `json:"romaji"`
	RomajiOrig   string             `json:"romaji_orig"`
	Title        string             `json:"title"`
	TitleOrig    string             `json:"title_orig"`
	CReleaseDate int                `json:"c_release_date"`
	Olang        string             `json:"olang"`
	Image        ranobeDBImage      `json:"image"`
	SimScore     float64            `json:"sim_score"`
}

// ranobeDBDetailResponse is the top-level response from the RanobeDB detail API.
type ranobeDBDetailResponse struct {
	Book ranobeDBDetailBook `json:"book"`
}

// ranobeDBDetailBook represents a book in the RanobeDB detail response.
type ranobeDBDetailBook struct {
	ID            int                   `json:"id"`
	ImageID       int                   `json:"image_id"`
	Lang          string                `json:"lang"`
	Romaji        *string               `json:"romaji"`
	RomajiOrig    string                `json:"romaji_orig"`
	Title         string                `json:"title"`
	TitleOrig     string                `json:"title_orig"`
	CReleaseDate  int                   `json:"c_release_date"`
	Description   string                `json:"description"`
	DescriptionJA string                `json:"description_ja"`
	Olang         string                `json:"olang"`
	Image         ranobeDBImage         `json:"image"`
	Editions      []ranobeDBEdition     `json:"editions"`
	Releases      []ranobeDBRelease     `json:"releases"`
	Publishers    []ranobeDBPublisher   `json:"publishers"`
	Series        ranobeDBSeries        `json:"series"`
	Tags          []ranobeDBTag         `json:"tags"`
}

// ranobeDBImage represents an image in the RanobeDB API.
type ranobeDBImage struct {
	ID       int    `json:"id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Spoiler  bool   `json:"spoiler"`
	NSFW     bool   `json:"nsfw"`
	Filename string `json:"filename"`
}

// ranobeDBEdition represents an edition in the RanobeDB API.
type ranobeDBEdition struct {
	BookID int                `json:"book_id"`
	EID    int                `json:"eid"`
	Title  string             `json:"title"`
	Lang   *string            `json:"lang"`
	Staff  []ranobeDBStaff    `json:"staff"`
}

// ranobeDBStaff represents a staff member in the RanobeDB API.
type ranobeDBStaff struct {
	RoleType     string `json:"role_type"`
	Name         string `json:"name"`
	Romaji       *string `json:"romaji"`
	StaffID      int    `json:"staff_id"`
	StaffAliasID int    `json:"staff_alias_id"`
	Note         string `json:"note"`
}

// ranobeDBRelease represents a release in the RanobeDB API.
type ranobeDBRelease struct {
	ID          int     `json:"id"`
	ReleaseDate int     `json:"release_date"`
	Pages       int     `json:"pages"`
	Format      string  `json:"format"`
	Lang        string  `json:"lang"`
	Title       string  `json:"title"`
	Romaji      string  `json:"romaji"`
	ISBN13      string  `json:"isbn13"`
}

// ranobeDBPublisher represents a publisher in the RanobeDB API.
type ranobeDBPublisher struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	Romaji         *string `json:"romaji"`
	PublisherType  string  `json:"publisher_type"`
	Lang           string  `json:"lang"`
}

// ranobeDBSeries represents a series in the RanobeDB API.
type ranobeDBSeries struct {
	Title string `json:"title"`
	ID    int    `json:"id"`
	Lang  string `json:"lang"`
}

// ranobeDBTag represents a tag in the RanobeDB API.
type ranobeDBTag struct {
	Name  string `json:"name"`
	TType string `json:"ttype"`
	ID    int    `json:"id"`
}
