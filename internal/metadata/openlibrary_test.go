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

func TestOpenLibraryProvider_Name(t *testing.T) {
	p := NewOpenLibraryProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if got := p.Name(); got != "open_library" {
		t.Errorf("Name() = %q; want %q", got, "open_library")
	}
}

func TestOpenLibraryProvider_Search_TitleAuthor(t *testing.T) {
	mockResponse := openLibrarySearchResponse{
		NumFound: 1,
		Docs: []openLibraryDoc{
			{
				Key:         "/works/OL12345W",
				Title:       "The Way of Kings",
				AuthorNames: []string{"Brandon Sanderson"},
				PublishYear: []int{2010},
				Language:    []string{"eng"},
				ISBN:        []string{"9780765326355", "0765326353"},
				Publisher:   []string{"Tor Books"},
				Subject:     []string{"Fantasy", "Epic Fantasy"},
				CoverI:      12345,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameters are present.
		if r.URL.Query().Get("title") == "" {
			http.Error(w, "missing title", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	p := NewOpenLibraryProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	results, err := p.Search(context.Background(), Query{
		Title:  "The Way of Kings",
		Author: "Brandon Sanderson",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results; want 1", len(results))
	}

	r := results[0]
	if r.Title != "The Way of Kings" {
		t.Errorf("Title = %q; want %q", r.Title, "The Way of Kings")
	}
	if len(r.Authors) != 1 || r.Authors[0] != "Brandon Sanderson" {
		t.Errorf("Authors = %v; want [Brandon Sanderson]", r.Authors)
	}
	if r.ProviderID != "OL12345W" {
		t.Errorf("ProviderID = %q; want %q", r.ProviderID, "OL12345W")
	}
	if r.Provider != "open_library" {
		t.Errorf("Provider = %q; want %q", r.Provider, "open_library")
	}
	if r.ISBN13 != "9780765326355" {
		t.Errorf("ISBN13 = %q; want %q", r.ISBN13, "9780765326355")
	}
	if r.ISBN10 != "0765326353" {
		t.Errorf("ISBN10 = %q; want %q", r.ISBN10, "0765326353")
	}
	if r.Publisher != "Tor Books" {
		t.Errorf("Publisher = %q; want %q", r.Publisher, "Tor Books")
	}
	wantCoverURL := "https://covers.openlibrary.org/b/id/12345-L.jpg"
	if r.CoverURL != wantCoverURL {
		t.Errorf("CoverURL = %q; want %q", r.CoverURL, wantCoverURL)
	}
}

func TestOpenLibraryProvider_Search_EmptyQuery(t *testing.T) {
	p := NewOpenLibraryProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	results, err := p.Search(context.Background(), Query{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if results != nil {
		t.Errorf("Search() = %v; want nil for empty query", results)
	}
}

func TestOpenLibraryProvider_Search_ISBN(t *testing.T) {
	mockEdition := openLibraryEdition{
		Title:         "The Way of Kings",
		Publishers:    []string{"Tor Books"},
		PublishDate:   "August 31, 2010",
		NumberOfPages: 1007,
		ISBN10:        []string{"0765326353"},
		ISBN13:        []string{"9780765326355"},
		Covers:        []int64{12345},
		Works:         []openLibraryWorkRef{{Key: "/works/OL12345W"}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ISBN lookup hits /isbn/{isbn}.json
		if !containsSubstring(r.URL.Path, "9780765326355") {
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockEdition)
	}))
	defer server.Close()

	p := NewOpenLibraryProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	results, err := p.Search(context.Background(), Query{ISBN: "9780765326355"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results; want 1", len(results))
	}

	r := results[0]
	if r.Title != "The Way of Kings" {
		t.Errorf("Title = %q; want %q", r.Title, "The Way of Kings")
	}
	if r.PageCount != 1007 {
		t.Errorf("PageCount = %d; want %d", r.PageCount, 1007)
	}
	if r.ProviderID != "OL12345W" {
		t.Errorf("ProviderID = %q; want %q", r.ProviderID, "OL12345W")
	}
	wantCoverURL := "https://covers.openlibrary.org/b/id/12345-L.jpg"
	if r.CoverURL != wantCoverURL {
		t.Errorf("CoverURL = %q; want %q", r.CoverURL, wantCoverURL)
	}
}

func TestOpenLibraryProvider_Search_ISBNNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := NewOpenLibraryProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	results, err := p.Search(context.Background(), Query{ISBN: "0000000000"})
	if err != nil {
		t.Fatalf("Search() error = %v; want nil for not found", err)
	}
	if len(results) != 0 {
		t.Errorf("Search() returned %d results; want 0 for not found", len(results))
	}
}

func TestOpenLibraryProvider_FetchByID(t *testing.T) {
	mockWork := openLibraryWork{
		Title:       "The Way of Kings",
		Description: "An epic fantasy novel.",
		Subjects:    []string{"Fantasy", "Epic Fantasy"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !containsSubstring(r.URL.Path, "OL12345W") {
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockWork)
	}))
	defer server.Close()

	p := NewOpenLibraryProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	result, err := p.FetchByID(context.Background(), "OL12345W")
	if err != nil {
		t.Fatalf("FetchByID() error = %v", err)
	}
	if result == nil {
		t.Fatal("FetchByID() returned nil result")
	}
	if result.Title != "The Way of Kings" {
		t.Errorf("Title = %q; want %q", result.Title, "The Way of Kings")
	}
	if result.Description != "An epic fantasy novel." {
		t.Errorf("Description = %q; want %q", result.Description, "An epic fantasy novel.")
	}
	if result.ProviderID != "OL12345W" {
		t.Errorf("ProviderID = %q; want %q", result.ProviderID, "OL12345W")
	}
}

func TestOpenLibraryProvider_MapDocToResult_CoverURL(t *testing.T) {
	p := NewOpenLibraryProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	tests := []struct {
		name    string
		doc     openLibraryDoc
		wantURL string
	}{
		{
			name:    "with cover id",
			doc:     openLibraryDoc{CoverI: 99999},
			wantURL: "https://covers.openlibrary.org/b/id/99999-L.jpg",
		},
		{
			name:    "without cover id",
			doc:     openLibraryDoc{CoverI: 0},
			wantURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.mapDocToResult(tt.doc)
			if result.CoverURL != tt.wantURL {
				t.Errorf("CoverURL = %q; want %q", result.CoverURL, tt.wantURL)
			}
		})
	}
}

func TestOpenLibraryProvider_StripPrefix(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		prefix string
		want   string
	}{
		{
			name:   "strips /works/ prefix",
			s:      "/works/OL12345W",
			prefix: "/works/",
			want:   "OL12345W",
		},
		{
			name:   "no prefix to strip",
			s:      "OL12345W",
			prefix: "/works/",
			want:   "OL12345W",
		},
		{
			name:   "empty string",
			s:      "",
			prefix: "/works/",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripPrefix(tt.s, tt.prefix)
			if got != tt.want {
				t.Errorf("stripPrefix(%q, %q) = %q; want %q", tt.s, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestOpenLibraryProvider_WorkDescriptionText(t *testing.T) {
	tests := []struct {
		name string
		work openLibraryWork
		want string
	}{
		{
			name: "string description",
			work: openLibraryWork{Description: "A great book."},
			want: "A great book.",
		},
		{
			name: "object description",
			work: openLibraryWork{Description: map[string]any{"type": "/type/text", "value": "A great book."}},
			want: "A great book.",
		},
		{
			name: "nil description",
			work: openLibraryWork{Description: nil},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.work.descriptionText()
			if got != tt.want {
				t.Errorf("descriptionText() = %q; want %q", got, tt.want)
			}
		})
	}
}
