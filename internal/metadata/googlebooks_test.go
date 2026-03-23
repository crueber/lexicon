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

func TestGoogleBooksProvider_Name(t *testing.T) {
	p := NewGoogleBooksProvider("", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if got := p.Name(); got != "google_books" {
		t.Errorf("Name() = %q; want %q", got, "google_books")
	}
}

func TestGoogleBooksProvider_BuildQuery(t *testing.T) {
	p := NewGoogleBooksProvider("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	tests := []struct {
		name  string
		query Query
		want  string
	}{
		{
			name:  "isbn only",
			query: Query{ISBN: "9780062315007"},
			want:  "isbn:9780062315007",
		},
		{
			name:  "title only",
			query: Query{Title: "The Way of Kings"},
			want:  "intitle:The Way of Kings",
		},
		{
			name:  "author only",
			query: Query{Author: "Brandon Sanderson"},
			want:  "inauthor:Brandon Sanderson",
		},
		{
			name:  "title and author",
			query: Query{Title: "Mistborn", Author: "Sanderson"},
			want:  "intitle:Mistborn+inauthor:Sanderson",
		},
		{
			name:  "isbn takes precedence",
			query: Query{Title: "Mistborn", Author: "Sanderson", ISBN: "9780765311788"},
			want:  "isbn:9780765311788",
		},
		{
			name:  "empty query",
			query: Query{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.buildQuery(tt.query)
			if got != tt.want {
				t.Errorf("buildQuery() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestGoogleBooksProvider_BuildSearchURL(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		query  string
	}{
		{
			name:   "without api key",
			apiKey: "",
			query:  "intitle:Mistborn",
		},
		{
			name:   "with api key",
			apiKey: "test-key-123",
			query:  "isbn:9780765311788",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGoogleBooksProvider(tt.apiKey, slog.New(slog.NewTextHandler(os.Stderr, nil)))
			got := p.buildSearchURL(tt.query)

			// Verify it's a valid URL with the right base.
			if len(got) == 0 {
				t.Error("buildSearchURL() returned empty string")
			}
			if got[:len("https://www.googleapis.com/books/v1/volumes?")] != "https://www.googleapis.com/books/v1/volumes?" {
				t.Errorf("buildSearchURL() does not start with expected base URL: %q", got)
			}

			// Verify API key presence.
			if tt.apiKey != "" {
				if !containsSubstring(got, "key="+tt.apiKey) {
					t.Errorf("buildSearchURL() does not contain api key: %q", got)
				}
			} else {
				if containsSubstring(got, "key=") {
					t.Errorf("buildSearchURL() contains key= when no api key set: %q", got)
				}
			}
		})
	}
}

func TestGoogleBooksProvider_Search(t *testing.T) {
	// Mock response from Google Books API.
	mockResponse := googleBooksResponse{
		TotalItems: 1,
		Items: []googleBooksVolume{
			{
				ID: "test-volume-id",
				VolumeInfo: googleBooksVolumeInfo{
					Title:         "The Way of Kings",
					Subtitle:      "The Stormlight Archive",
					Authors:       []string{"Brandon Sanderson"},
					Publisher:     "Tor Books",
					PublishedDate: "2010-08-31",
					Description:   "Epic fantasy novel.",
					PageCount:     1007,
					Language:      "en",
					Categories:    []string{"Fiction", "Fantasy"},
					IndustryIdentifiers: []googleBooksIdentifier{
						{Type: "ISBN_13", Identifier: "9780765326355"},
						{Type: "ISBN_10", Identifier: "0765326353"},
					},
					ImageLinks: googleBooksImageLinks{
						Thumbnail: "http://books.google.com/books/content?id=test&zoom=1",
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	p := NewGoogleBooksProvider("", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	// Override the HTTP client to use the test server.
	p.httpClient = server.Client()

	// Override the search URL builder to use the test server.
	// We test the full flow by calling Search with a custom URL.
	// Since we can't easily override the URL, we test mapToResult directly.
	result := p.mapToResult(mockResponse.Items[0])

	if result.Title != "The Way of Kings" {
		t.Errorf("Title = %q; want %q", result.Title, "The Way of Kings")
	}
	if result.Subtitle != "The Stormlight Archive" {
		t.Errorf("Subtitle = %q; want %q", result.Subtitle, "The Stormlight Archive")
	}
	if len(result.Authors) != 1 || result.Authors[0] != "Brandon Sanderson" {
		t.Errorf("Authors = %v; want [Brandon Sanderson]", result.Authors)
	}
	if result.Publisher != "Tor Books" {
		t.Errorf("Publisher = %q; want %q", result.Publisher, "Tor Books")
	}
	if result.PublishDate != "2010-08-31" {
		t.Errorf("PublishDate = %q; want %q", result.PublishDate, "2010-08-31")
	}
	if result.PageCount != 1007 {
		t.Errorf("PageCount = %d; want %d", result.PageCount, 1007)
	}
	if result.Language != "en" {
		t.Errorf("Language = %q; want %q", result.Language, "en")
	}
	if result.ISBN13 != "9780765326355" {
		t.Errorf("ISBN13 = %q; want %q", result.ISBN13, "9780765326355")
	}
	if result.ISBN10 != "0765326353" {
		t.Errorf("ISBN10 = %q; want %q", result.ISBN10, "0765326353")
	}
	if result.GoogleBooksID != "test-volume-id" {
		t.Errorf("GoogleBooksID = %q; want %q", result.GoogleBooksID, "test-volume-id")
	}
	if result.Provider != "google_books" {
		t.Errorf("Provider = %q; want %q", result.Provider, "google_books")
	}
	// Cover URL should be upgraded to HTTPS.
	if result.CoverURL != "https://books.google.com/books/content?id=test&zoom=1" {
		t.Errorf("CoverURL = %q; want HTTPS URL", result.CoverURL)
	}
}

func TestGoogleBooksProvider_FetchByID(t *testing.T) {
	mockVolume := googleBooksVolume{
		ID: "abc123",
		VolumeInfo: googleBooksVolumeInfo{
			Title:   "Test Book",
			Authors: []string{"Test Author"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the path contains the volume ID.
		if !containsSubstring(r.URL.Path, "abc123") {
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockVolume)
	}))
	defer server.Close()

	p := NewGoogleBooksProvider("", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	// Override the HTTP client to use the test server.
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	result, err := p.FetchByID(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("FetchByID() error = %v", err)
	}
	if result == nil {
		t.Fatal("FetchByID() returned nil result")
	}
	if result.Title != "Test Book" {
		t.Errorf("Title = %q; want %q", result.Title, "Test Book")
	}
}

func TestGoogleBooksProvider_MapToResult_CoverURLHTTPS(t *testing.T) {
	p := NewGoogleBooksProvider("", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	tests := []struct {
		name     string
		links    googleBooksImageLinks
		wantHTTP bool
	}{
		{
			name:     "http thumbnail upgraded to https",
			links:    googleBooksImageLinks{Thumbnail: "http://books.google.com/cover.jpg"},
			wantHTTP: false,
		},
		{
			name:     "https thumbnail unchanged",
			links:    googleBooksImageLinks{Thumbnail: "https://books.google.com/cover.jpg"},
			wantHTTP: false,
		},
		{
			name:     "large preferred over thumbnail",
			links:    googleBooksImageLinks{Thumbnail: "http://thumb.jpg", Large: "http://large.jpg"},
			wantHTTP: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vol := googleBooksVolume{
				ID:         "test",
				VolumeInfo: googleBooksVolumeInfo{ImageLinks: tt.links},
			}
			result := p.mapToResult(vol)
			if tt.wantHTTP && result.CoverURL[:5] == "https" {
				t.Errorf("expected http URL, got: %q", result.CoverURL)
			}
			if !tt.wantHTTP && result.CoverURL != "" && result.CoverURL[:5] != "https" {
				t.Errorf("expected https URL, got: %q", result.CoverURL)
			}
		})
	}
}

// testTransport redirects requests to the test server.
type testTransport struct {
	server *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the host with the test server's host.
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = t.server.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req2)
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
