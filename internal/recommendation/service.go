package recommendation

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"

	"github.com/crueber/lexicon/internal/task"
)

// SimilarBook is a book recommendation with similarity score.
type SimilarBook struct {
	ID         int64    `json:"id"`
	LibraryID  int64    `json:"libraryId"`
	BookType   string   `json:"bookType"`
	Title      *string  `json:"title"`
	Authors    []string `json:"authors"`
	CoverPath  *string  `json:"coverPath"`
	AddedDate  *string  `json:"addedDate"`
	Similarity float32  `json:"similarity"`
}

// Service provides book recommendation functionality.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewService creates a new recommendation Service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// RebuildAllVectors recomputes feature vectors for all books.
func (s *Service) RebuildAllVectors(ctx context.Context, reporter task.Reporter) error {
	q := New(s.db)

	books, err := q.ListAllBooksForVectorRebuild(ctx)
	if err != nil {
		return fmt.Errorf("list books for rebuild: %w", err)
	}

	total := len(books)
	for i, b := range books {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		reporter.Progress(i, total, fmt.Sprintf("Processing book %d/%d", i+1, total))

		authorNames, err := q.ListBookAuthors(ctx, b.ID)
		if err != nil {
			s.logger.Warn("list book authors for vector rebuild", "book_id", b.ID, "error", err)
			authorNames = nil
		}

		seriesNames, err := q.ListBookSeries(ctx, b.ID)
		if err != nil {
			s.logger.Warn("list book series for vector rebuild", "book_id", b.ID, "error", err)
			seriesNames = nil
		}

		categoryNames, err := q.ListBookCategories(ctx, b.ID)
		if err != nil {
			s.logger.Warn("list book categories for vector rebuild", "book_id", b.ID, "error", err)
			categoryNames = nil
		}

		tagNames, err := q.ListBookTags(ctx, b.ID)
		if err != nil {
			s.logger.Warn("list book tags for vector rebuild", "book_id", b.ID, "error", err)
			tagNames = nil
		}

		// Skip books with no meaningful features.
		if len(authorNames) == 0 && len(seriesNames) == 0 &&
			len(categoryNames) == 0 && len(tagNames) == 0 &&
			!b.Language.Valid && !b.Publisher.Valid {
			continue
		}

		language := ""
		if b.Language.Valid {
			language = b.Language.String
		}
		publisher := ""
		if b.Publisher.Valid {
			publisher = b.Publisher.String
		}

		vector := HashBookFeatures(authorNames, seriesNames, categoryNames, tagNames, language, publisher)
		if err := q.UpsertBookVector(ctx, UpsertBookVectorParams{
			BookID: b.ID,
			Vector: vectorToBytes(vector),
		}); err != nil {
			s.logger.Error("upsert book vector", "book_id", b.ID, "error", err)
			continue
		}
	}

	reporter.Progress(total, total, "Vector rebuild complete")
	return nil
}

// FindSimilarBooks finds books similar to the given book ID.
// A per-author cap of 3 is applied to the primary author (first author).
// If libraryIDs is non-empty, only books from those libraries are returned.
func (s *Service) FindSimilarBooks(ctx context.Context, bookID int64, libraryIDs []int64, limit int) ([]SimilarBook, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}

	q := New(s.db)

	// Fetch source vector.
	sourceBlob, err := q.GetBookVector(ctx, bookID)
	if err != nil {
		if err == sql.ErrNoRows {
			return make([]SimilarBook, 0), nil
		}
		return nil, fmt.Errorf("get source vector: %w", err)
	}
	sourceVec := vectorFromBytes(sourceBlob)

	// Fetch all other vectors.
	rows, err := q.ListAllBookVectors(ctx)
	if err != nil {
		return nil, fmt.Errorf("list book vectors: %w", err)
	}

	// Build library filter set.
	libraryFilter := make(map[int64]struct{})
	for _, lid := range libraryIDs {
		libraryFilter[lid] = struct{}{}
	}

	// Batch-fetch all authors for candidate books.
	authorRows, err := q.ListAuthorsForBooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list authors for books: %w", err)
	}
	authorMap := make(map[int64][]string)
	for _, ar := range authorRows {
		authorMap[ar.BookID] = append(authorMap[ar.BookID], ar.Name)
	}

	type scored struct {
		book       SimilarBook
		similarity float32
	}
	var scoredBooks []scored

	for _, row := range rows {
		if row.BookID == bookID {
			continue
		}
		// Apply library filter.
		if len(libraryFilter) > 0 {
			if _, ok := libraryFilter[row.LibraryID]; !ok {
				continue
			}
		}
		vec := vectorFromBytes(row.Vector)
		sim := CosineSimilarity(sourceVec, vec)

		sb := SimilarBook{
			ID:         row.BookID,
			LibraryID:  row.LibraryID,
			BookType:   row.BookType,
			Authors:    authorMap[row.BookID],
			Similarity: sim,
		}
		if row.Title.Valid {
			sb.Title = &row.Title.String
		}
		if row.CoverPath.Valid {
			sb.CoverPath = &row.CoverPath.String
		}
		if row.AddedDate.Valid {
			sb.AddedDate = &row.AddedDate.String
		}

		scoredBooks = append(scoredBooks, scored{book: sb, similarity: sim})
	}

	// Sort by similarity descending.
	sort.Slice(scoredBooks, func(i, j int) bool {
		return scoredBooks[i].similarity > scoredBooks[j].similarity
	})

	// Apply per-author cap.
	authorCounts := make(map[string]int)
	result := make([]SimilarBook, 0)
	for _, sb := range scoredBooks {
		primaryAuthor := ""
		if len(sb.book.Authors) > 0 {
			primaryAuthor = sb.book.Authors[0]
		}

		if primaryAuthor != "" {
			if authorCounts[primaryAuthor] >= 3 {
				continue
			}
			authorCounts[primaryAuthor]++
		}

		result = append(result, sb.book)
		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

// NewRebuildFunc returns a TaskFunc that rebuilds all book vectors.
func NewRebuildFunc(svc *Service) task.TaskFunc {
	return func(ctx context.Context, payload string, reporter task.Reporter) error {
		return svc.RebuildAllVectors(ctx, reporter)
	}
}
