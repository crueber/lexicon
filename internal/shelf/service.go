package shelf

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
)

// ErrNotFound is returned when a shelf does not exist.
var ErrNotFound = errors.New("shelf not found")

// ErrForbidden is returned when a user tries to access a shelf they don't own.
var ErrForbidden = errors.New("access denied")

// CreateParams holds the parameters for creating a shelf.
type CreateParams struct {
	Name        string
	Description string
	Icon        string
	IconColor   string
	IsPublic    bool
}

// UpdateParams holds the parameters for updating a shelf.
type UpdateParams struct {
	Name        string
	Description string
	Icon        string
	IconColor   string
	IsPublic    bool
}

// ShelfWithCount is a shelf with its book count.
type ShelfWithCount struct {
	Shelf
	BookCount int64
}

// Service handles shelf business logic.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewService creates a new shelf Service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// ListForUser returns all shelves owned by the user, with book counts.
func (s *Service) ListForUser(ctx context.Context, userID int64) ([]ShelfWithCount, error) {
	q := New(s.db)

	shelves, err := q.ListShelvesForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list shelves for user: %w", err)
	}

	result := make([]ShelfWithCount, 0, len(shelves))
	for _, sh := range shelves {
		count, err := q.CountBooksInShelf(ctx, sh.ID)
		if err != nil {
			s.logger.Warn("count books in shelf", "shelf_id", sh.ID, "error", err)
			count = 0
		}
		result = append(result, ShelfWithCount{Shelf: sh, BookCount: count})
	}

	return result, nil
}

// GetByID returns a shelf, checking ownership (or public access).
func (s *Service) GetByID(ctx context.Context, id, userID int64) (*Shelf, error) {
	q := New(s.db)

	sh, err := q.GetShelfByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get shelf by id: %w", err)
	}

	// Allow access if the user owns the shelf or it is public.
	if sh.UserID != userID && sh.IsPublic == 0 {
		return nil, ErrForbidden
	}

	return &sh, nil
}

// Create creates a new shelf for the user.
func (s *Service) Create(ctx context.Context, userID int64, params CreateParams) (*Shelf, error) {
	q := New(s.db)

	isPublic := int64(0)
	if params.IsPublic {
		isPublic = 1
	}

	sh, err := q.CreateShelf(ctx, CreateShelfParams{
		UserID: userID,
		Name:   params.Name,
		Description: sql.NullString{
			String: params.Description,
			Valid:  params.Description != "",
		},
		Icon: sql.NullString{
			String: params.Icon,
			Valid:  params.Icon != "",
		},
		IconColor: sql.NullString{
			String: params.IconColor,
			Valid:  params.IconColor != "",
		},
		IsPublic: isPublic,
	})
	if err != nil {
		return nil, fmt.Errorf("create shelf: %w", err)
	}

	return &sh, nil
}

// Update updates a shelf (owner only).
func (s *Service) Update(ctx context.Context, id, userID int64, params UpdateParams) error {
	q := New(s.db)

	// Verify ownership.
	sh, err := q.GetShelfByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get shelf for update: %w", err)
	}
	if sh.UserID != userID {
		return ErrForbidden
	}

	isPublic := int64(0)
	if params.IsPublic {
		isPublic = 1
	}

	if err := q.UpdateShelf(ctx, UpdateShelfParams{
		Name: params.Name,
		Description: sql.NullString{
			String: params.Description,
			Valid:  params.Description != "",
		},
		Icon: sql.NullString{
			String: params.Icon,
			Valid:  params.Icon != "",
		},
		IconColor: sql.NullString{
			String: params.IconColor,
			Valid:  params.IconColor != "",
		},
		IsPublic: isPublic,
		ID:       id,
		UserID:   userID,
	}); err != nil {
		return fmt.Errorf("update shelf: %w", err)
	}

	return nil
}

// Delete deletes a shelf (owner only).
func (s *Service) Delete(ctx context.Context, id, userID int64) error {
	q := New(s.db)

	// Verify ownership.
	sh, err := q.GetShelfByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get shelf for delete: %w", err)
	}
	if sh.UserID != userID {
		return ErrForbidden
	}

	if err := q.DeleteShelf(ctx, DeleteShelfParams{ID: id, UserID: userID}); err != nil {
		return fmt.Errorf("delete shelf: %w", err)
	}

	return nil
}

// AddBook adds a book to a shelf (owner only).
func (s *Service) AddBook(ctx context.Context, shelfID, bookID, userID int64) error {
	q := New(s.db)

	// Verify ownership.
	sh, err := q.GetShelfByID(ctx, shelfID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get shelf for add book: %w", err)
	}
	if sh.UserID != userID {
		return ErrForbidden
	}

	if err := q.AddBookToShelf(ctx, AddBookToShelfParams{
		ShelfID:   shelfID,
		BookID:    bookID,
		ShelfID_2: shelfID,
	}); err != nil {
		return fmt.Errorf("add book to shelf: %w", err)
	}

	return nil
}

// RemoveBook removes a book from a shelf (owner only).
func (s *Service) RemoveBook(ctx context.Context, shelfID, bookID, userID int64) error {
	q := New(s.db)

	// Verify ownership.
	sh, err := q.GetShelfByID(ctx, shelfID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get shelf for remove book: %w", err)
	}
	if sh.UserID != userID {
		return ErrForbidden
	}

	if err := q.RemoveBookFromShelf(ctx, RemoveBookFromShelfParams{
		ShelfID: shelfID,
		BookID:  bookID,
	}); err != nil {
		return fmt.Errorf("remove book from shelf: %w", err)
	}

	return nil
}

// ListBooks returns books in a shelf with basic metadata.
func (s *Service) ListBooks(ctx context.Context, shelfID, userID int64) ([]ListBooksInShelfRow, error) {
	q := New(s.db)

	// Verify access (owner or public).
	sh, err := q.GetShelfByID(ctx, shelfID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get shelf for list books: %w", err)
	}
	if sh.UserID != userID && sh.IsPublic == 0 {
		return nil, ErrForbidden
	}

	books, err := q.ListBooksInShelf(ctx, shelfID)
	if err != nil {
		return nil, fmt.Errorf("list books in shelf: %w", err)
	}

	return books, nil
}

// ListShelvesContainingBook returns shelves (owned by user) that contain a book.
func (s *Service) ListShelvesContainingBook(ctx context.Context, bookID, userID int64) ([]Shelf, error) {
	q := New(s.db)

	shelves, err := q.ListShelvesContainingBook(ctx, ListShelvesContainingBookParams{
		BookID: bookID,
		UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("list shelves containing book: %w", err)
	}

	return shelves, nil
}
