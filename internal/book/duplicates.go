package book

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

// DuplicatePreset defines how aggressively to detect duplicates.
type DuplicatePreset string

const (
	PresetStrict    DuplicatePreset = "STRICT"
	PresetModerate  DuplicatePreset = "MODERATE"
	PresetLoose     DuplicatePreset = "LOOSE"
	PresetTitleOnly DuplicatePreset = "TITLE_ONLY"
)

// DuplicateGroup represents a set of potentially duplicate books.
type DuplicateGroup struct {
	Key     string  `json:"key"`
	BookIDs []int64 `json:"bookIds"`
}

// FindDuplicates finds duplicate books using the given preset.
func FindDuplicates(ctx context.Context, db *sql.DB, preset DuplicatePreset) ([]DuplicateGroup, error) {
	q := New(db)

	// Fetch all books with metadata and authors.
	books, err := q.ListBooksWithMetadataAndAuthors(ctx)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}

	groups := make(map[string][]int64)

	for _, b := range books {
		key := computeDuplicateKey(b, preset)
		if key != "" {
			groups[key] = append(groups[key], b.ID)
		}
	}

	// Fetch dismissed pairs.
	dismissedRows, err := q.ListDismissedDuplicates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list dismissed duplicates: %w", err)
	}

	dismissed := make(map[int64]map[int64]bool)
	for _, d := range dismissedRows {
		if dismissed[d.BookIDA] == nil {
			dismissed[d.BookIDA] = make(map[int64]bool)
		}
		if dismissed[d.BookIDB] == nil {
			dismissed[d.BookIDB] = make(map[int64]bool)
		}
		dismissed[d.BookIDA][d.BookIDB] = true
		dismissed[d.BookIDB][d.BookIDA] = true
	}

	var result []DuplicateGroup
	for key, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		// Only include the group if at least one pair is not dismissed.
		hasUndismissed := false
		for i := 0; i < len(ids) && !hasUndismissed; i++ {
			for j := i + 1; j < len(ids); j++ {
				if !dismissed[ids[i]][ids[j]] {
					hasUndismissed = true
					break
				}
			}
		}
		if hasUndismissed {
			result = append(result, DuplicateGroup{Key: key, BookIDs: ids})
		}
	}

	return result, nil
}

// computeDuplicateKey generates a comparison key for a book based on the preset.
func computeDuplicateKey(b ListBooksWithMetadataAndAuthorsRow, preset DuplicatePreset) string {
	// Extract author names from pipe-delimited string.
	var authors []string
	for _, a := range strings.Split(b.AuthorNames, "|") {
		authors = append(authors, normalizeText(a))
	}
	authorKey := strings.Join(authors, "|")

	title := ""
	if b.Title.Valid {
		title = normalizeText(b.Title.String)
	}

	switch preset {
	case PresetStrict:
		// title (exact normalized) + author + ISBN
		isbn := ""
		if b.Isbn13.Valid {
			isbn = b.Isbn13.String
		}
		if isbn == "" && b.Isbn10.Valid {
			isbn = b.Isbn10.String
		}
		return fmt.Sprintf("strict:%s|%s|%s", title, authorKey, isbn)
	case PresetModerate:
		return fmt.Sprintf("moderate:%s|%s", title, authorKey)
	case PresetLoose:
		// Remove subtitle (everything after first colon or dash)
		title = strings.SplitN(title, ":", 2)[0]
		title = strings.SplitN(title, "-", 2)[0]
		return fmt.Sprintf("loose:%s", title)
	case PresetTitleOnly:
		return fmt.Sprintf("title:%s", title)
	default:
		return fmt.Sprintf("moderate:%s|%s", title, authorKey)
	}
}

// normalizeText lowercases, removes punctuation, collapses whitespace,
// and ignores "The"/"A"/"An" prefixes.
func normalizeText(s string) string {
	s = strings.ToLower(s)
	// Remove punctuation.
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	s = b.String()
	// Collapse whitespace.
	fields := strings.Fields(s)
	// Ignore leading "The"/"A"/"An".
	if len(fields) > 0 {
		switch fields[0] {
		case "the", "a", "an":
			fields = fields[1:]
		}
	}
	return strings.Join(fields, " ")
}
