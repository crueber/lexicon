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

func TestRanobeDBProvider_Name(t *testing.T) {
	p := NewRanobeDBProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if got := p.Name(); got != "ranobedb" {
		t.Errorf("Name() = %q; want %q", got, "ranobedb")
	}
}

func TestRanobeDBProvider_Search_EmptyQuery(t *testing.T) {
	p := NewRanobeDBProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	results, err := p.Search(context.Background(), Query{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if results != nil {
		t.Errorf("Search() = %v; want nil for empty query", results)
	}
}

func TestRanobeDBProvider_InterfaceCompliance(t *testing.T) {
	var _ Provider = (*RanobeDBProvider)(nil)
}

func TestRanobeDBProvider_Search(t *testing.T) {
	romaji := "Baka to Test to Shoukanjuu"
	mockResponse := ranobeDBSearchResponse{
		Books: []ranobeDBSearchBook{
			{
				ID:           3367,
				ImageID:      3367,
				Lang:         "ja",
				Romaji:       &romaji,
				RomajiOrig:   "Baka to Test to Shoukanjuu",
				Title:        "バカとテストと召喚獣",
				TitleOrig:    "バカとテストと召喚獣",
				CReleaseDate: 20070129,
				Olang:        "ja",
				Image: ranobeDBImage{
					ID:       3367,
					Width:    240,
					Height:   339,
					Spoiler:  false,
					NSFW:     false,
					Filename: "gVDbusJoh5C1vNTd.jpg",
				},
				SimScore: 1,
			},
		},
		Count:       "1",
		CurrentPage: 1,
		TotalPages:  1,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "" {
			http.Error(w, "missing query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	p := NewRanobeDBProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	results, err := p.Search(context.Background(), Query{Title: "Baka to Test"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results; want 1", len(results))
	}

	r := results[0]
	if r.Title != "Baka to Test to Shoukanjuu" {
		t.Errorf("Title = %q; want %q", r.Title, "Baka to Test to Shoukanjuu")
	}
	if r.ProviderID != "3367" {
		t.Errorf("ProviderID = %q; want %q", r.ProviderID, "3367")
	}
	if r.Provider != "ranobedb" {
		t.Errorf("Provider = %q; want %q", r.Provider, "ranobedb")
	}
	if r.Language != "ja" {
		t.Errorf("Language = %q; want %q", r.Language, "ja")
	}
	wantCoverURL := "https://images.ranobedb.org/gVDbusJoh5C1vNTd.jpg"
	if r.CoverURL != wantCoverURL {
		t.Errorf("CoverURL = %q; want %q", r.CoverURL, wantCoverURL)
	}
	if r.PublishDate != "2007-01-29" {
		t.Errorf("PublishDate = %q; want %q", r.PublishDate, "2007-01-29")
	}
}

func TestRanobeDBProvider_FetchByID(t *testing.T) {
	romaji := "Baka to Test to Shoukanjuu"
	mockResponse := ranobeDBDetailResponse{
		Book: ranobeDBDetailBook{
			ID:            3367,
			ImageID:       3367,
			Lang:          "ja",
			Romaji:        &romaji,
			RomajiOrig:    "Baka to Test to Shoukanjuu",
			Title:         "バカとテストと召喚獣",
			TitleOrig:     "バカとテストと召喚獣",
			CReleaseDate:  20070129,
			Description:   "A comedy about school wars.",
			DescriptionJA: "第8回えんため大賞編集部特別賞受賞作",
			Olang:         "ja",
			Image: ranobeDBImage{
				ID:       3367,
				Width:    240,
				Height:   339,
				Filename: "gVDbusJoh5C1vNTd.jpg",
			},
			Editions: []ranobeDBEdition{
				{
					BookID: 3367,
					EID:    0,
					Title:  "Original edition",
					Staff: []ranobeDBStaff{
						{RoleType: "author", Name: "井上堅二", StaffID: 868},
						{RoleType: "artist", Name: "葉賀ユイ", StaffID: 869},
					},
				},
			},
			Releases: []ranobeDBRelease{
				{
					ID:          6638,
					ReleaseDate: 20070129,
					Pages:       283,
					Format:      "print",
					Lang:        "ja",
					Title:       "バカとテストと召喚獣",
					ISBN13:      "9784757733299",
				},
			},
			Publishers: []ranobeDBPublisher{
				{ID: 9, Name: "KADOKAWA", PublisherType: "publisher", Lang: "ja"},
				{ID: 32, Name: "ファミ通文庫", Romaji: strPtr("Famitsu Bunko"), PublisherType: "imprint", Lang: "ja"},
			},
			Series: ranobeDBSeries{
				Title: "Baka to Test to Shoukanjuu",
				ID:    805,
				Lang:  "ja",
			},
			Tags: []ranobeDBTag{
				{Name: "anime tie-in", TType: "tag", ID: 26},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !containsSubstring(r.URL.Path, "3367") {
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	p := NewRanobeDBProvider(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	result, err := p.FetchByID(context.Background(), "3367")
	if err != nil {
		t.Fatalf("FetchByID() error = %v", err)
	}
	if result == nil {
		t.Fatal("FetchByID() returned nil result")
	}
	if result.Title != "Baka to Test to Shoukanjuu" {
		t.Errorf("Title = %q; want %q", result.Title, "Baka to Test to Shoukanjuu")
	}
	if result.Description != "A comedy about school wars." {
		t.Errorf("Description = %q; want %q", result.Description, "A comedy about school wars.")
	}
	if result.ProviderID != "3367" {
		t.Errorf("ProviderID = %q; want %q", result.ProviderID, "3367")
	}
	if len(result.Authors) != 1 || result.Authors[0] != "井上堅二" {
		t.Errorf("Authors = %v; want [井上堅二]", result.Authors)
	}
	if result.Publisher != "KADOKAWA" {
		t.Errorf("Publisher = %q; want %q", result.Publisher, "KADOKAWA")
	}
	if result.ISBN13 != "9784757733299" {
		t.Errorf("ISBN13 = %q; want %q", result.ISBN13, "9784757733299")
	}
	if result.PageCount != 283 {
		t.Errorf("PageCount = %d; want %d", result.PageCount, 283)
	}
	if result.Series != "Baka to Test to Shoukanjuu" {
		t.Errorf("Series = %q; want %q", result.Series, "Baka to Test to Shoukanjuu")
	}
	if len(result.Tags) != 1 || result.Tags[0] != "anime tie-in" {
		t.Errorf("Tags = %v; want [anime tie-in]", result.Tags)
	}
}

func strPtr(s string) *string {
	return &s
}
