package contentrestriction

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// RestrictionMode constants.
const (
	ModeExclude   = "EXCLUDE"
	ModeAllowOnly = "ALLOW_ONLY"
)

// RestrictionType constants.
const (
	TypeCategory      = "CATEGORY"
	TypeTag           = "TAG"
	TypeMood          = "MOOD"
	TypeAgeRating     = "AGE_RATING"
	TypeContentRating = "CONTENT_RATING"
)

// Service provides content restriction management.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewService creates a new content restriction Service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// Restriction is a content restriction for a user.
type Restriction struct {
	ID              int64  `json:"id"`
	UserID          int64  `json:"userId"`
	RestrictionType string `json:"restrictionType"`
	Value           string `json:"value"`
	Mode            string `json:"mode"`
}

// ListRestrictions returns all restrictions for a user.
func (s *Service) ListRestrictions(ctx context.Context, userID int64) ([]Restriction, error) {
	q := New(s.db)
	rows, err := q.ListRestrictionsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list restrictions: %w", err)
	}
	var result []Restriction
	for _, r := range rows {
		result = append(result, Restriction{
			ID:              r.ID,
			UserID:          r.UserID,
			RestrictionType: r.RestrictionType,
			Value:           r.Value,
			Mode:            r.Mode,
		})
	}
	return result, nil
}

// AddRestriction adds a restriction for a user.
func (s *Service) AddRestriction(ctx context.Context, userID int64, restrictionType, value, mode string) error {
	q := New(s.db)
	_, err := q.CreateRestriction(ctx, CreateRestrictionParams{
		UserID:          userID,
		RestrictionType: restrictionType,
		Value:           value,
		Mode:            mode,
	})
	if err != nil {
		return fmt.Errorf("create restriction: %w", err)
	}
	return nil
}

// RemoveRestriction removes a restriction by ID (verifying it belongs to the user).
func (s *Service) RemoveRestriction(ctx context.Context, userID, restrictionID int64) error {
	q := New(s.db)
	err := q.DeleteRestriction(ctx, DeleteRestrictionParams{
		ID:     restrictionID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("delete restriction: %w", err)
	}
	return nil
}

// FilterBookIDs applies content restrictions to a list of book IDs.
// It returns the filtered list of book IDs that the user is allowed to see.
// If the user has no restrictions, the original list is returned unchanged.
// Admin users bypass all restrictions (return original list).
func (s *Service) FilterBookIDs(ctx context.Context, userID int64, isAdmin bool, bookIDs []int64) ([]int64, error) {
	if isAdmin || len(bookIDs) == 0 {
		return bookIDs, nil
	}

	restrictions, err := s.ListRestrictions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list restrictions: %w", err)
	}
	if len(restrictions) == 0 {
		return bookIDs, nil
	}

	// Separate EXCLUDE and ALLOW_ONLY restrictions.
	var excludeRestrictions, allowOnlyRestrictions []Restriction
	for _, r := range restrictions {
		switch r.Mode {
		case ModeExclude:
			excludeRestrictions = append(excludeRestrictions, r)
		case ModeAllowOnly:
			allowOnlyRestrictions = append(allowOnlyRestrictions, r)
		}
	}

	// If there are ALLOW_ONLY restrictions, only allow books that match at least one.
	if len(allowOnlyRestrictions) > 0 {
		allowedIDs, err := s.getAllowedBookIDs(ctx, allowOnlyRestrictions, bookIDs)
		if err != nil {
			return nil, fmt.Errorf("get allowed books: %w", err)
		}
		bookIDs = allowedIDs
	}

	// Apply EXCLUDE restrictions.
	if len(excludeRestrictions) > 0 {
		excludedIDs, err := s.getExcludedBookIDs(ctx, excludeRestrictions, bookIDs)
		if err != nil {
			return nil, fmt.Errorf("get excluded books: %w", err)
		}
		// Build a set of excluded IDs.
		excludedSet := make(map[int64]struct{}, len(excludedIDs))
		for _, id := range excludedIDs {
			excludedSet[id] = struct{}{}
		}
		var filtered []int64
		for _, id := range bookIDs {
			if _, ok := excludedSet[id]; !ok {
				filtered = append(filtered, id)
			}
		}
		bookIDs = filtered
	}

	return bookIDs, nil
}

// getAllowedBookIDs returns all book IDs from the candidate list that match ANY ALLOW_ONLY restriction.
func (s *Service) getAllowedBookIDs(ctx context.Context, restrictions []Restriction, candidateIDs []int64) ([]int64, error) {
	q := New(s.db)
	candidateSet := make(map[int64]struct{}, len(candidateIDs))
	for _, id := range candidateIDs {
		candidateSet[id] = struct{}{}
	}

	allowedSet := make(map[int64]struct{})

	for _, r := range restrictions {
		var rows []int64
		var err error
		switch r.RestrictionType {
		case TypeCategory:
			rows, err = q.GetBookIDsByCategoryValue(ctx, r.Value)
		case TypeTag:
			rows, err = q.GetBookIDsByTagValue(ctx, r.Value)
		case TypeMood:
			rows, err = q.GetBookIDsByMoodValue(ctx, r.Value)
		case TypeAgeRating:
			rows, err = q.GetBookIDsByAgeRating(ctx, sql.NullString{String: r.Value, Valid: true})
		default:
			return nil, fmt.Errorf("unknown restriction type: %s", r.RestrictionType)
		}
		if err != nil {
			return nil, err
		}
		for _, id := range rows {
			if _, ok := candidateSet[id]; ok {
				allowedSet[id] = struct{}{}
			}
		}
	}

	var result []int64
	for id := range allowedSet {
		result = append(result, id)
	}
	return result, nil
}

// getExcludedBookIDs returns all book IDs from the candidate list that match ANY EXCLUDE restriction.
func (s *Service) getExcludedBookIDs(ctx context.Context, restrictions []Restriction, candidateIDs []int64) ([]int64, error) {
	q := New(s.db)
	candidateSet := make(map[int64]struct{}, len(candidateIDs))
	for _, id := range candidateIDs {
		candidateSet[id] = struct{}{}
	}

	excludedSet := make(map[int64]struct{})

	for _, r := range restrictions {
		var rows []int64
		var err error
		switch r.RestrictionType {
		case TypeCategory:
			rows, err = q.GetBookIDsByCategoryValue(ctx, r.Value)
		case TypeTag:
			rows, err = q.GetBookIDsByTagValue(ctx, r.Value)
		case TypeMood:
			rows, err = q.GetBookIDsByMoodValue(ctx, r.Value)
		case TypeAgeRating:
			rows, err = q.GetBookIDsByAgeRating(ctx, sql.NullString{String: r.Value, Valid: true})
		default:
			return nil, fmt.Errorf("unknown restriction type: %s", r.RestrictionType)
		}
		if err != nil {
			return nil, err
		}
		for _, id := range rows {
			if _, ok := candidateSet[id]; ok {
				excludedSet[id] = struct{}{}
			}
		}
	}

	var result []int64
	for id := range excludedSet {
		result = append(result, id)
	}
	return result, nil
}
