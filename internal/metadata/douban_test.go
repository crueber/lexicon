package metadata

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestDoubanProvider_Name(t *testing.T) {
	p := NewDoubanProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if got := p.Name(); got != "douban" {
		t.Errorf("Name() = %q; want %q", got, "douban")
	}
}

func TestDoubanProvider_Search_EmptyQuery(t *testing.T) {
	p := NewDoubanProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	results, err := p.Search(context.Background(), Query{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if results != nil {
		t.Errorf("Search() = %v; want nil for empty query", results)
	}
}

func TestDoubanProvider_InterfaceCompliance(t *testing.T) {
	var _ Provider = (*DoubanProvider)(nil)
}

func TestDoubanProvider_ParseSearchItem(t *testing.T) {
	p := NewDoubanProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	item := `<div class="item-root"><a href="https://book.douban.com/subject/12345/"><img src="https://img.example.com/cover.jpg" alt="Test Book"></a><div class="meta abstract">Test Author<br/>Publisher<br/>2020-01</div></div><!-- /item-root -->`

	result := p.parseSearchItem(item)
	if result.ProviderID != "12345" {
		t.Errorf("ProviderID = %q; want %q", result.ProviderID, "12345")
	}
	if result.Title != "Test Book" {
		t.Errorf("Title = %q; want %q", result.Title, "Test Book")
	}
	if len(result.Authors) != 1 || result.Authors[0] != "Test Author" {
		t.Errorf("Authors = %v; want [Test Author]", result.Authors)
	}
	if result.CoverURL != "https://img.example.com/cover.jpg" {
		t.Errorf("CoverURL = %q; want %q", result.CoverURL, "https://img.example.com/cover.jpg")
	}
	if result.Language != "zh" {
		t.Errorf("Language = %q; want %q", result.Language, "zh")
	}
}

func TestDoubanProvider_ParseDetailPage(t *testing.T) {
	p := NewDoubanProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	html := `<html><head></head><body>` +
		`<span property="v:itemreviewed">Test Book</span>` +
		`<span class="pl">作者</span><a href="/search/author">Test Author</a></span>` +
		`<div class="intro"><p>A great book.</p></div>` +
		`<img src="https://img.example.com/cover.jpg" rel="v:image">` +
		`<div id="info"><span class="pl">出版社:</span>Test Publisher<br/><span class="pl">出版年:</span>2020-01<br/></div>` +
		`</body></html>`

	result := p.parseDetailPage("12345", html)
	if result.ProviderID != "12345" {
		t.Errorf("ProviderID = %q; want %q", result.ProviderID, "12345")
	}
	if result.Title != "Test Book" {
		t.Errorf("Title = %q; want %q", result.Title, "Test Book")
	}
	if len(result.Authors) != 1 || result.Authors[0] != "Test Author" {
		t.Errorf("Authors = %v; want [Test Author]", result.Authors)
	}
	if result.Description != "A great book." {
		t.Errorf("Description = %q; want %q", result.Description, "A great book.")
	}
	if result.CoverURL != "https://img.example.com/cover.jpg" {
		t.Errorf("CoverURL = %q; want %q", result.CoverURL, "https://img.example.com/cover.jpg")
	}
	if result.Publisher != "Test Publisher" {
		t.Errorf("Publisher = %q; want %q", result.Publisher, "Test Publisher")
	}
	if result.PublishDate != "2020-01" {
		t.Errorf("PublishDate = %q; want %q", result.PublishDate, "2020-01")
	}
	if result.Language != "zh" {
		t.Errorf("Language = %q; want %q", result.Language, "zh")
	}
}

func TestDoubanProvider_ParseDetailPage_MissingFields(t *testing.T) {
	p := NewDoubanProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	html := `<html><head></head><body></body></html>`

	result := p.parseDetailPage("99999", html)
	if result.ProviderID != "99999" {
		t.Errorf("ProviderID = %q; want %q", result.ProviderID, "99999")
	}
	if result.Title != "" {
		t.Errorf("Title = %q; want empty", result.Title)
	}
	if len(result.Authors) != 0 {
		t.Errorf("Authors = %v; want empty", result.Authors)
	}
}
