package metadata

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// LubimyCzytacProvider implements Provider for LubimyCzytac (Polish books).
// It scrapes HTML from lubimyczytac.pl.
type LubimyCzytacProvider struct {
	httpClient *http.Client
	logger     *slog.Logger
	mu         sync.Mutex
	lastCall   time.Time
}

// NewLubimyCzytacProvider creates a new LubimyCzytacProvider.
func NewLubimyCzytacProvider(logger *slog.Logger) *LubimyCzytacProvider {
	return &LubimyCzytacProvider{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Name returns the provider name.
func (p *LubimyCzytacProvider) Name() string {
	return "lubimyczytac"
}

// Search searches LubimyCzytac for books matching the query.
func (p *LubimyCzytacProvider) Search(ctx context.Context, query Query) ([]Result, error) {
	q := query.Title
	if q == "" {
		q = query.Author
	}
	if q == "" {
		return nil, nil
	}

	apiURL := "https://lubimyczytac.pl/szukaj/ksiazki?phrase=" + url.QueryEscape(q)

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("lubimyczytac search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lubimyczytac search: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lubimyczytac search: read body: %w", err)
	}

	html := string(body)
	results := p.parseSearchResults(html)
	return results, nil
}

// FetchByID fetches a single book by its LubimyCzytac book ID.
func (p *LubimyCzytacProvider) FetchByID(ctx context.Context, providerID string) (*Result, error) {
	apiURL := fmt.Sprintf("https://lubimyczytac.pl/ksiazka/%s", url.PathEscape(providerID))

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("lubimyczytac fetch by id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("lubimyczytac fetch by id: book %q not found", providerID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lubimyczytac fetch by id: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lubimyczytac fetch by id: read body: %w", err)
	}

	result := p.parseDetailPage(providerID, string(body))
	return &result, nil
}

// rateLimit enforces a minimum 2-second gap between requests.
func (p *LubimyCzytacProvider) rateLimit() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.lastCall.IsZero() {
		elapsed := time.Since(p.lastCall)
		if elapsed < 2*time.Second {
			time.Sleep(2*time.Second - elapsed)
		}
	}
	p.lastCall = time.Now()
}

// doRequest performs an HTTP GET request.
func (p *LubimyCzytacProvider) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Lexicon/1.0")

	return p.httpClient.Do(req)
}

// parseSearchResults extracts book results from the LubimyCzytac search HTML.
func (p *LubimyCzytacProvider) parseSearchResults(html string) []Result {
	var results []Result

	// Each result contains a form with action="/ksiazka/{id}/{slug}"
	// Split by the form action pattern to isolate items.
	items := extractAllBetween(html, `<form action="/ksiazka/`, `</form>`)
	for _, item := range items {
		result := p.parseSearchItem(item)
		if result.ProviderID != "" && result.Title != "" {
			results = append(results, result)
		}
	}

	return results
}

// parseSearchItem extracts a single Result from a search item HTML fragment.
func (p *LubimyCzytacProvider) parseSearchItem(item string) Result {
	var result Result
	result.Provider = p.Name()
	result.Language = "pl"

	// Extract ID from the form action: /ksiazka/5210871/tosia-i-tymek-ida-do-zoo
	actionEnd := strings.Index(item, `"`)
	if actionEnd == -1 {
		actionEnd = strings.Index(item, ` `)
	}
	if actionEnd != -1 {
		action := item[:actionEnd]
		parts := strings.Split(action, "/")
		if len(parts) > 0 {
			result.ProviderID = parts[0]
		}
	}

	// Extract title.
	titleBlock := extractBetween(item, `authorAllBooks__singleTextTitle float-left"`, `</a>`)
	if titleBlock != "" {
		if idx := strings.LastIndex(titleBlock, `">`); idx != -1 {
			result.Title = strings.TrimSpace(htmlUnescape(titleBlock[idx+2:]))
		}
	}

	// Extract authors.
	authorBlock := extractBetween(item, `authorAllBooks__singleTextAuthor`, `</div>`)
	if authorBlock != "" {
		authorParts := strings.Split(authorBlock, "</a>")
		for _, part := range authorParts {
			if idx := strings.LastIndex(part, ">"); idx != -1 {
				author := cleanText(htmlUnescape(part[idx+1:]))
				if author != "" {
					result.Authors = append(result.Authors, author)
				}
			}
		}
	}

	// Extract cover URL from img tag.
	imgBlock := extractBetween(item, `<img`, `/>`)
	if imgBlock == "" {
		imgBlock = extractBetween(item, `<img`, `>`)
	}
	if imgBlock != "" {
		src := extractBetween(imgBlock, `src="`, `"`)
		if src == "" {
			src = extractBetween(imgBlock, `data-src="`, `"`)
		}
		if src != "" {
			result.CoverURL = src
		}
	}

	return result
}

// parseDetailPage extracts a Result from the LubimyCzytac detail page HTML.
func (p *LubimyCzytacProvider) parseDetailPage(providerID, html string) Result {
	var result Result
	result.ProviderID = providerID
	result.Provider = p.Name()
	result.Language = "pl"

	// Title: <h1 class="book__title"> Title </h1>
	result.Title = htmlUnescape(cleanText(extractBetween(html, `<h1 class="book__title">`, `</h1>`)))

	// Author: <span class="author pb-2"><a class="link-name" href="...">Author</a>
	authorBlock := extractBetween(html, `<span class="author pb-2">`, `</span>`)
	if authorBlock == "" {
		authorBlock = extractBetween(html, `<span class="author">`, `</span>`)
	}
	if authorBlock != "" {
		authorParts := strings.Split(authorBlock, "</a>")
		for _, part := range authorParts {
			if idx := strings.LastIndex(part, ">"); idx != -1 {
				author := cleanText(htmlUnescape(part[idx+1:]))
				if author != "" {
					result.Authors = append(result.Authors, author)
				}
			}
		}
	}

	// Description: <div class="book__description text-collapse ..." id="book-description"> Description </div>
	descBlock := extractBetween(html, `id="book-description">`, `</div>`)
	if descBlock != "" {
		result.Description = strings.TrimSpace(htmlUnescape(strings.ReplaceAll(descBlock, `<br />`, "\n")))
	}

	// Cover: find the large cover image.
	// <img src="https://s.lubimyczytac.pl/upload/books/.../...-352x500.jpg" ... alt="Okładka książki ...">
	coverBlock := extractBetween(html, `<img`, `alt="Okładka książki`)
	if coverBlock != "" {
		src := extractBetween(coverBlock, `src="`, `"`)
		if src != "" {
			result.CoverURL = src
		}
	}
	if result.CoverURL == "" {
		// Fallback: any image with the cover alt text.
		coverBlock = extractBetween(html, `<img`, `alt="Okładka`)
		if coverBlock != "" {
			src := extractBetween(coverBlock, `src="`, `"`)
			if src != "" {
				result.CoverURL = src
			}
		}
	}

	return result
}
