package metadata

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHardcoverProvider_Name(t *testing.T) {
	p := NewHardcoverProvider("", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if got := p.Name(); got != "hardcover" {
		t.Errorf("Name() = %q; want %q", got, "hardcover")
	}
}

func TestHardcoverProvider_Search_NoAPIKey(t *testing.T) {
	p := NewHardcoverProvider("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	results, err := p.Search(context.Background(), Query{Title: "Mistborn"})
	if err != nil {
		t.Fatalf("Search() error = %v; want nil when no API key", err)
	}
	if results != nil {
		t.Errorf("Search() = %v; want nil when no API key", results)
	}
}

func TestHardcoverProvider_FetchByID_NoAPIKey(t *testing.T) {
	p := NewHardcoverProvider("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	result, err := p.FetchByID(context.Background(), "12345")
	if err != nil {
		t.Fatalf("FetchByID() error = %v; want nil when no API key", err)
	}
	if result != nil {
		t.Errorf("FetchByID() = %v; want nil when no API key", result)
	}
}

func TestHardcoverProvider_Search_ParsesResponse(t *testing.T) {
	mockResponse := hardcoverSearchResponse{}
	mockResponse.Data.Search.Results = []hardcoverBook{
		{
			ID:          42,
			Title:       "The Final Empire",
			Description: "A fantasy novel.",
			ReleaseDate: "2006-07-17",
			Pages:       541,
			ISBN13:      "9780765311788",
			Contributions: []hardcoverContribution{
				{Author: hardcoverAuthor{Name: "Brandon Sanderson"}},
			},
			Language: &hardcoverLanguage{Language: "English"},
			Image:    &hardcoverImage{URL: "https://example.com/cover.jpg"},
			SeriesBooks: []hardcoverSeriesBook{
				{
					Series:   hardcoverSeries{Name: "Mistborn"},
					Position: 1.0,
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a POST with JSON body.
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "expected application/json", http.StatusBadRequest)
			return
		}
		// Verify Authorization header is present.
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}

		// Verify the request body contains a query.
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if _, ok := body["query"]; !ok {
			http.Error(w, "missing query field", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	p := NewHardcoverProvider("test-api-key", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	// Override the HTTP client to redirect to the test server.
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	results, err := p.Search(context.Background(), Query{Title: "Mistborn"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results; want 1", len(results))
	}

	r := results[0]
	if r.Title != "The Final Empire" {
		t.Errorf("Title = %q; want %q", r.Title, "The Final Empire")
	}
	if r.ProviderID != "42" {
		t.Errorf("ProviderID = %q; want %q", r.ProviderID, "42")
	}
	if r.HardcoverID != "42" {
		t.Errorf("HardcoverID = %q; want %q", r.HardcoverID, "42")
	}
	if len(r.Authors) != 1 || r.Authors[0] != "Brandon Sanderson" {
		t.Errorf("Authors = %v; want [Brandon Sanderson]", r.Authors)
	}
	if r.Description != "A fantasy novel." {
		t.Errorf("Description = %q; want %q", r.Description, "A fantasy novel.")
	}
	if r.PageCount != 541 {
		t.Errorf("PageCount = %d; want %d", r.PageCount, 541)
	}
	if r.ISBN13 != "9780765311788" {
		t.Errorf("ISBN13 = %q; want %q", r.ISBN13, "9780765311788")
	}
	if r.Language != "English" {
		t.Errorf("Language = %q; want %q", r.Language, "English")
	}
	if r.CoverURL != "https://example.com/cover.jpg" {
		t.Errorf("CoverURL = %q; want %q", r.CoverURL, "https://example.com/cover.jpg")
	}
	if r.Series != "Mistborn" {
		t.Errorf("Series = %q; want %q", r.Series, "Mistborn")
	}
	if r.SeriesIndex != 1.0 {
		t.Errorf("SeriesIndex = %f; want %f", r.SeriesIndex, 1.0)
	}
	if r.Provider != "hardcover" {
		t.Errorf("Provider = %q; want %q", r.Provider, "hardcover")
	}
}

func TestHardcoverProvider_Search_GraphQLError(t *testing.T) {
	mockResponse := hardcoverSearchResponse{
		Errors: []hardcoverError{{Message: "unauthorized"}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	p := NewHardcoverProvider("test-api-key", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	_, err := p.Search(context.Background(), Query{Title: "Mistborn"})
	if err == nil {
		t.Error("Search() error = nil; want error for GraphQL error response")
	}
}

func TestBuildHardcoverQuery(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		want  string
	}{
		{
			name:  "isbn only",
			query: Query{ISBN: "9780765311788"},
			want:  "9780765311788",
		},
		{
			name:  "title and author",
			query: Query{Title: "Mistborn", Author: "Sanderson"},
			want:  "Mistborn Sanderson",
		},
		{
			name:  "title only",
			query: Query{Title: "Mistborn"},
			want:  "Mistborn",
		},
		{
			name:  "author only",
			query: Query{Author: "Sanderson"},
			want:  "Sanderson",
		},
		{
			name:  "empty query",
			query: Query{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildHardcoverQuery(tt.query)
			if got != tt.want {
				t.Errorf("buildHardcoverQuery() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestHardcoverProvider_MapToResult_NoLanguage(t *testing.T) {
	p := NewHardcoverProvider("key", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	book := hardcoverBook{
		ID:    1,
		Title: "Test Book",
		// Language is nil
	}

	result := p.mapToResult(book)
	if result.Language != "" {
		t.Errorf("Language = %q; want empty string when language is nil", result.Language)
	}
}

func TestHardcoverProvider_MapToResult_NoImage(t *testing.T) {
	p := NewHardcoverProvider("key", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	book := hardcoverBook{
		ID:    1,
		Title: "Test Book",
		// Image is nil
	}

	result := p.mapToResult(book)
	if result.CoverURL != "" {
		t.Errorf("CoverURL = %q; want empty string when image is nil", result.CoverURL)
	}
}
