package metadata

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestLubimyCzytacProvider_Name(t *testing.T) {
	p := NewLubimyCzytacProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if got := p.Name(); got != "lubimyczytac" {
		t.Errorf("Name() = %q; want %q", got, "lubimyczytac")
	}
}

func TestLubimyCzytacProvider_Search_EmptyQuery(t *testing.T) {
	p := NewLubimyCzytacProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	results, err := p.Search(context.Background(), Query{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if results != nil {
		t.Errorf("Search() = %v; want nil for empty query", results)
	}
}

func TestLubimyCzytacProvider_InterfaceCompliance(t *testing.T) {
	var _ Provider = (*LubimyCzytacProvider)(nil)
}

func TestLubimyCzytacProvider_ParseSearchItem(t *testing.T) {
	p := NewLubimyCzytacProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	item := `5210871/tosia-i-tymek-ida-do-zoo" method="post" rel="nofollow">` +
		`<img loading="lazy" src="https://s.lubimyczytac.pl/upload/books/5210000/5210871/1308975-170x243.jpg" class="img-fluid" alt="Okładka książki Tosia i Tymek idą do zoo" /></form>` +
		`<div class="authorAllBooks__singleText relative"><div><a class="authorAllBooks__singleTextTitle float-left" href="/ksiazka/5210871/tosia-i-tymek-ida-do-zoo"> Tosia i Tymek idą do zoo </a></div>` +
		`<div class="authorAllBooks__singleTextAuthor authorAllBooks__singleTextAuthor--bottomMore"><a href="https://lubimyczytac.pl/autor/6069/jean-adamson">Jean Adamson</a>, <a href="https://lubimyczytac.pl/autor/6070/gareth-adamson">Gareth Adamson</a></div></div>`

	result := p.parseSearchItem(item)
	if result.ProviderID != "5210871" {
		t.Errorf("ProviderID = %q; want %q", result.ProviderID, "5210871")
	}
	if result.Title != "Tosia i Tymek idą do zoo" {
		t.Errorf("Title = %q; want %q", result.Title, "Tosia i Tymek idą do zoo")
	}
	if len(result.Authors) != 2 || result.Authors[0] != "Jean Adamson" || result.Authors[1] != "Gareth Adamson" {
		t.Errorf("Authors = %v; want [Jean Adamson Gareth Adamson]", result.Authors)
	}
	if result.CoverURL != "https://s.lubimyczytac.pl/upload/books/5210000/5210871/1308975-170x243.jpg" {
		t.Errorf("CoverURL = %q; want %q", result.CoverURL, "https://s.lubimyczytac.pl/upload/books/5210000/5210871/1308975-170x243.jpg")
	}
	if result.Language != "pl" {
		t.Errorf("Language = %q; want %q", result.Language, "pl")
	}
}

func TestLubimyCzytacProvider_ParseDetailPage(t *testing.T) {
	p := NewLubimyCzytacProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	html := `<html><body>` +
		`<h1 class="book__title"> Tosia i Tymek idą do zoo </h1>` +
		`<span class="author pb-2"><a class="link-name d-inline-grid" href="https://lubimyczytac.pl/autor/6069/jean-adamson">Jean Adamson</a>,&nbsp;<a class="link-name d-inline-grid" href="https://lubimyczytac.pl/autor/6070/gareth-adamson">Gareth Adamson</a></span>` +
		`<div class="book__description text-collapse pr-0 pl-0 pl-lg-3" id="book-description"> Tosia i Tymek to bliźnięta,<br />co przygód mają wiele. </div>` +
		`<img src="https://s.lubimyczytac.pl/upload/books/5210000/5210871/1308975-352x500.jpg" width="340" height="482" alt="Okładka książki Tosia i Tymek idą do zoo">` +
		`</body></html>`

	result := p.parseDetailPage("5210871", html)
	if result.ProviderID != "5210871" {
		t.Errorf("ProviderID = %q; want %q", result.ProviderID, "5210871")
	}
	if result.Title != "Tosia i Tymek idą do zoo" {
		t.Errorf("Title = %q; want %q", result.Title, "Tosia i Tymek idą do zoo")
	}
	if len(result.Authors) != 2 || result.Authors[0] != "Jean Adamson" || result.Authors[1] != "Gareth Adamson" {
		t.Errorf("Authors = %v; want [Jean Adamson Gareth Adamson]", result.Authors)
	}
	if result.Description != "Tosia i Tymek to bliźnięta,\nco przygód mają wiele." {
		t.Errorf("Description = %q; want %q", result.Description, "Tosia i Tymek to bliźnięta,\nco przygód mają wiele.")
	}
	if result.CoverURL != "https://s.lubimyczytac.pl/upload/books/5210000/5210871/1308975-352x500.jpg" {
		t.Errorf("CoverURL = %q; want %q", result.CoverURL, "https://s.lubimyczytac.pl/upload/books/5210000/5210871/1308975-352x500.jpg")
	}
	if result.Language != "pl" {
		t.Errorf("Language = %q; want %q", result.Language, "pl")
	}
}

func TestLubimyCzytacProvider_ParseDetailPage_MissingFields(t *testing.T) {
	p := NewLubimyCzytacProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	html := `<html><body></body></html>`

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
