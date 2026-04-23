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

// DoubanProvider implements Provider for Douban (Chinese books).
// It scrapes HTML from book.douban.com.
type DoubanProvider struct {
	httpClient *http.Client
	logger     *slog.Logger
	mu         sync.Mutex
	lastCall   time.Time
}

// NewDoubanProvider creates a new DoubanProvider.
func NewDoubanProvider(logger *slog.Logger) *DoubanProvider {
	return &DoubanProvider{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Name returns the provider name.
func (p *DoubanProvider) Name() string {
	return "douban"
}

// Search searches Douban for books matching the query.
func (p *DoubanProvider) Search(ctx context.Context, query Query) ([]Result, error) {
	q := query.Title
	if q == "" {
		q = query.Author
	}
	if q == "" {
		return nil, nil
	}

	apiURL := "https://book.douban.com/subject_search?search_text=" + url.QueryEscape(q)

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("douban search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("douban search: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("douban search: read body: %w", err)
	}

	html := string(body)
	results := p.parseSearchResults(html)
	return results, nil
}

// FetchByID fetches a single book by its Douban subject ID.
func (p *DoubanProvider) FetchByID(ctx context.Context, providerID string) (*Result, error) {
	apiURL := fmt.Sprintf("https://book.douban.com/subject/%s/", url.PathEscape(providerID))

	p.rateLimit()

	resp, err := p.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("douban fetch by id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("douban fetch by id: book %q not found", providerID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("douban fetch by id: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("douban fetch by id: read body: %w", err)
	}

	result := p.parseDetailPage(providerID, string(body))
	return &result, nil
}

// rateLimit enforces a minimum 2-second gap between requests.
func (p *DoubanProvider) rateLimit() {
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
func (p *DoubanProvider) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Lexicon/1.0")

	return p.httpClient.Do(req)
}

// parseSearchResults extracts book results from the Douban search HTML.
func (p *DoubanProvider) parseSearchResults(html string) []Result {
	var results []Result

	// Each result item is in a div with class "item-root".
	items := extractAllBetween(html, `<div class="item-root">`, `</div><!-- /item-root -->`)
	for _, item := range items {
		result := p.parseSearchItem(item)
		if result.ProviderID != "" && result.Title != "" {
			results = append(results, result)
		}
	}

	return results
}

// parseSearchItem extracts a single Result from a search item HTML fragment.
func (p *DoubanProvider) parseSearchItem(item string) Result {
	var result Result
	result.Provider = p.Name()
	result.Language = "zh"

	// Extract detail link and ID: <a href="https://book.douban.com/subject/12345/">
	linkStart := `<a href="https://book.douban.com/subject/`
	link := extractBetween(item, linkStart, `/"`)
	if link == "" {
		// Try without https://
		link = extractBetween(item, `<a href="/subject/`, `/"`)
	}
	if link != "" {
		result.ProviderID = link
	}

	// Extract title from alt attribute of img tag.
	title := extractBetween(item, `<img src="`, `">`)
	if title != "" {
		// The alt text is between the last quote and the closing >
		altIdx := strings.LastIndex(title, `" alt="`)
		if altIdx != -1 {
			result.Title = htmlUnescape(title[altIdx+7:])
		}
	}
	if result.Title == "" {
		// Fallback: try to extract from title attribute or nearby text.
		result.Title = htmlUnescape(extractBetween(item, `title="`, `"`))
	}

	// Extract cover URL.
	if title != "" {
		coverEnd := strings.Index(title, `"`)
		if coverEnd != -1 {
			result.CoverURL = title[:coverEnd]
		}
	}

	// Extract author from the meta paragraph.
	authorBlock := extractBetween(item, `<div class="meta abstract">`, `</div>`)
	if authorBlock != "" {
		// The author is usually the first line.
		lines := strings.Split(authorBlock, "<br/>")
		if len(lines) > 0 {
			author := cleanText(htmlUnescape(strings.ReplaceAll(lines[0], "\n", "")))
			if author != "" {
				result.Authors = []string{author}
			}
		}
	}

	return result
}

// parseDetailPage extracts a Result from the Douban detail page HTML.
func (p *DoubanProvider) parseDetailPage(providerID, html string) Result {
	var result Result
	result.ProviderID = providerID
	result.Provider = p.Name()
	result.Language = "zh"

	// Title: <span property="v:itemreviewed">{title}</span>
	result.Title = htmlUnescape(extractBetween(html, `<span property="v:itemreviewed">`, `</span>`))

	// Author: look for author links.
	authorBlock := extractBetween(html, `<span class="pl">作者</span>`, `</span>`)
	if authorBlock == "" {
		authorBlock = extractBetween(html, `<span class="pl"> 作者</span>`, `</span>`)
	}
	if authorBlock != "" {
		author := htmlUnescape(extractBetween(authorBlock, `">`, `</a>`))
		if author == "" {
			author = htmlUnescape(extractBetween(authorBlock, `>`, ``))
		}
		if author != "" {
			author = strings.TrimSuffix(author, "</a>")
			result.Authors = []string{cleanText(author)}
		}
	}

	// Description: <div class="intro">{description}</div>
	desc := extractBetween(html, `<div class="intro">`, `</div>`)
	if desc != "" {
		desc = htmlUnescape(strings.ReplaceAll(desc, "<p>", "\n"))
		desc = strings.ReplaceAll(desc, "</p>", "")
		result.Description = strings.TrimSpace(desc)
	}

	// Cover: <img src="{cover_url}" rel="v:image">
	coverBlock := extractBetween(html, `<img src="`, `" rel="v:image">`)
	if coverBlock == "" {
		coverBlock = extractBetween(html, `<img src="`, `" rel="v:image"/>`)
	}
	if coverBlock == "" {
		coverBlock = extractBetween(html, `<img src="`, `" rel="v:image" />`)
	}
	if coverBlock != "" {
		result.CoverURL = coverBlock
	}

	// Publisher and date from info section.
	infoBlock := extractBetween(html, `<div id="info"`, `</div>`)
	if infoBlock != "" {
		// Extract publisher.
		pubBlock := extractBetween(infoBlock, `<span class="pl">出版社:</span>`, `<br/>`)
		if pubBlock == "" {
			pubBlock = extractBetween(infoBlock, `<span class="pl">出版社</span>`, `<br/>`)
		}
		if pubBlock != "" {
			result.Publisher = cleanText(htmlUnescape(extractBetween(pubBlock, `">`, `</a>`)))
			if result.Publisher == "" {
				result.Publisher = cleanText(htmlUnescape(pubBlock))
			}
		}

		// Extract publish date.
		dateBlock := extractBetween(infoBlock, `<span class="pl">出版年:</span>`, `<br/>`)
		if dateBlock == "" {
			dateBlock = extractBetween(infoBlock, `<span class="pl">出版年</span>`, `<br/>`)
		}
		if dateBlock != "" {
			result.PublishDate = cleanText(htmlUnescape(extractBetween(dateBlock, `">`, `</a>`)))
			if result.PublishDate == "" {
				result.PublishDate = cleanText(htmlUnescape(dateBlock))
			}
		}
	}

	return result
}

// extractBetween extracts the text between two substrings.
func extractBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i == -1 {
		return ""
	}
	i += len(start)
	if end == "" {
		return s[i:]
	}
	j := strings.Index(s[i:], end)
	if j == -1 {
		return ""
	}
	return s[i : i+j]
}

// extractAllBetween extracts all occurrences of text between start and end.
func extractAllBetween(s, start, end string) []string {
	var results []string
	for {
		i := strings.Index(s, start)
		if i == -1 {
			break
		}
		i += len(start)
		j := strings.Index(s[i:], end)
		if j == -1 {
			break
		}
		results = append(results, s[i:i+j])
		s = s[i+j:]
	}
	return results
}

// htmlUnescape unescapes common HTML entities.
func htmlUnescape(s string) string {
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&#34;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}

// cleanText trims whitespace and removes extra newlines.
func cleanText(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
