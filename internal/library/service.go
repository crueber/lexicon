package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/crueber/lexicon/internal/auth"
)

// ErrNotFound is returned when a requested library does not exist.
var ErrNotFound = errors.New("library not found")

// ErrAccessDenied is returned when a user does not have access to a library.
var ErrAccessDenied = errors.New("access denied")

// Service provides business logic for library management.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewService creates a new library Service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// CreateParams holds the parameters for creating a new library.
type CreateParams struct {
	Name              string
	Icon              *string
	IconColor         *string
	OrganizationMode  string
	FileNamingPattern *string
	Paths             []string
}

// UpdateParams holds the parameters for updating a library.
type UpdateParams struct {
	Name              string
	Icon              *string
	IconColor         *string
	OrganizationMode  string
	FileNamingPattern *string
}

// ListForUser returns libraries the user has access to.
// Admins see all libraries. Regular users see only their permitted libraries.
func (s *Service) ListForUser(ctx context.Context, principal *auth.Principal) ([]Library, error) {
	q := New(s.db)

	if principal.IsAdmin() {
		libs, err := q.ListLibraries(ctx)
		if err != nil {
			return nil, fmt.Errorf("list libraries: %w", err)
		}
		return libs, nil
	}

	// Regular user: filter to permitted library IDs.
	all, err := q.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	permitted := make(map[int64]struct{}, len(principal.LibraryIDs))
	for _, id := range principal.LibraryIDs {
		permitted[id] = struct{}{}
	}

	result := make([]Library, 0, len(principal.LibraryIDs))
	for _, lib := range all {
		if _, ok := permitted[lib.ID]; ok {
			result = append(result, lib)
		}
	}
	return result, nil
}

// GetByID returns a library if the user has access to it.
func (s *Service) GetByID(ctx context.Context, id int64, principal *auth.Principal) (*Library, error) {
	q := New(s.db)

	lib, err := q.GetLibraryByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get library by id: %w", err)
	}

	if !principal.IsAdmin() {
		permitted := false
		for _, lid := range principal.LibraryIDs {
			if lid == id {
				permitted = true
				break
			}
		}
		if !permitted {
			return nil, ErrAccessDenied
		}
	}

	return &lib, nil
}

// Create creates a new library and optionally adds initial paths.
// Admin-only enforcement is done at the handler level.
func (s *Service) Create(ctx context.Context, params CreateParams) (*Library, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	q := New(tx)

	lib, err := q.CreateLibrary(ctx, CreateLibraryParams{
		Name:              params.Name,
		Icon:              nullString(params.Icon),
		IconColor:         nullString(params.IconColor),
		OrganizationMode:  params.OrganizationMode,
		FileNamingPattern: nullString(params.FileNamingPattern),
	})
	if err != nil {
		return nil, fmt.Errorf("create library: %w", err)
	}

	for _, path := range params.Paths {
		if _, err := q.CreateLibraryPath(ctx, CreateLibraryPathParams{
			LibraryID: lib.ID,
			Path:      path,
		}); err != nil {
			return nil, fmt.Errorf("add library path %q: %w", path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &lib, nil
}

// Update updates a library's metadata.
// Admin-only enforcement is done at the handler level.
func (s *Service) Update(ctx context.Context, id int64, params UpdateParams) error {
	q := New(s.db)

	// Verify the library exists first.
	if _, err := q.GetLibraryByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get library: %w", err)
	}

	if err := q.UpdateLibrary(ctx, UpdateLibraryParams{
		ID:                id,
		Name:              params.Name,
		Icon:              nullString(params.Icon),
		IconColor:         nullString(params.IconColor),
		OrganizationMode:  params.OrganizationMode,
		FileNamingPattern: nullString(params.FileNamingPattern),
	}); err != nil {
		return fmt.Errorf("update library: %w", err)
	}

	return nil
}

// Delete deletes a library by ID.
// Admin-only enforcement is done at the handler level.
func (s *Service) Delete(ctx context.Context, id int64) error {
	q := New(s.db)

	// Verify the library exists first.
	if _, err := q.GetLibraryByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get library: %w", err)
	}

	if err := q.DeleteLibrary(ctx, id); err != nil {
		return fmt.Errorf("delete library: %w", err)
	}

	return nil
}

// AddPath adds a filesystem path to a library.
func (s *Service) AddPath(ctx context.Context, libraryID int64, path string) (*LibraryPath, error) {
	q := New(s.db)

	// Verify the library exists.
	if _, err := q.GetLibraryByID(ctx, libraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get library: %w", err)
	}

	lp, err := q.CreateLibraryPath(ctx, CreateLibraryPathParams{
		LibraryID: libraryID,
		Path:      path,
	})
	if err != nil {
		return nil, fmt.Errorf("add library path: %w", err)
	}

	return &lp, nil
}

// RemovePath removes a filesystem path from a library.
// It verifies the path belongs to the given library before deleting.
func (s *Service) RemovePath(ctx context.Context, libraryID int64, pathID int64) error {
	q := New(s.db)

	lp, err := q.GetLibraryPathByID(ctx, pathID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get library path: %w", err)
	}

	if lp.LibraryID != libraryID {
		return ErrNotFound
	}

	if err := q.DeleteLibraryPath(ctx, pathID); err != nil {
		return fmt.Errorf("remove library path: %w", err)
	}

	return nil
}

// ListPaths returns all paths for a library.
func (s *Service) ListPaths(ctx context.Context, libraryID int64) ([]LibraryPath, error) {
	q := New(s.db)

	// Verify the library exists.
	if _, err := q.GetLibraryByID(ctx, libraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get library: %w", err)
	}

	paths, err := q.ListLibraryPaths(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list library paths: %w", err)
	}

	return paths, nil
}

// ListMetadataSources returns configured metadata sources for a library.
func (s *Service) ListMetadataSources(ctx context.Context, libraryID int64) ([]GetLibraryMetadataSourcesRow, error) {
	q := New(s.db)
	rows, err := q.GetLibraryMetadataSources(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list library metadata sources: %w", err)
	}
	return rows, nil
}

// SetMetadataSources replaces all metadata sources for a library.
func (s *Service) SetMetadataSources(ctx context.Context, libraryID int64, sources []MetadataSourceResponse) error {
	q := New(s.db)

	existing, err := q.GetLibraryMetadataSources(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("get existing metadata sources: %w", err)
	}
	for _, row := range existing {
		if err := q.DeleteLibraryMetadataSource(ctx, DeleteLibraryMetadataSourceParams{
			LibraryID: libraryID,
			Provider:  row.Provider,
		}); err != nil {
			return fmt.Errorf("delete metadata source %q: %w", row.Provider, err)
		}
	}

	for _, src := range sources {
		if src.Provider == "" {
			continue
		}
		if err := q.SetLibraryMetadataSource(ctx, SetLibraryMetadataSourceParams{
			LibraryID:     libraryID,
			Provider:      src.Provider,
			FieldPriority: src.FieldPriority,
		}); err != nil {
			return fmt.Errorf("set metadata source %q: %w", src.Provider, err)
		}
	}

	return nil
}

// nullString converts a *string to sql.NullString.
func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
