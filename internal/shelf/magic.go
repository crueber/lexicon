package shelf

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// RuleGroup is a group of rules combined with AND or OR.
type RuleGroup struct {
	Operator string     `json:"operator"` // "AND" or "OR"
	Rules    []RuleItem `json:"rules"`
}

// RuleItem is either a leaf condition or a nested group.
type RuleItem struct {
	// Leaf condition fields (set when Type == "condition")
	Type     string `json:"type"`     // "condition" or "group"
	Field    string `json:"field"`    // e.g., "title", "author", "category"
	Operator string `json:"operator"` // "contains", "equals", etc.
	Value    string `json:"value"`

	// Nested group (set when Type == "group")
	Group *RuleGroup `json:"group,omitempty"`
}

// BookResult is a book returned by magic shelf evaluation.
type BookResult struct {
	ID        int64          `json:"id"`
	LibraryID int64          `json:"libraryId"`
	BookType  string         `json:"bookType"`
	AddedDate sql.NullString `json:"addedDate"`
	Title     sql.NullString `json:"title"`
	CoverPath sql.NullString `json:"coverPath"`
}

// allowedFields maps rule field names to their SQL column expressions.
// Using an allowlist prevents SQL injection via field names.
var allowedFields = map[string]string{
	"title":      "bm.title",
	"author":     "a.name",
	"category":   "c.name",
	"tag":        "t.name",
	"series":     "s.name",
	"language":   "bm.language",
	"book_type":  "b.book_type",
	"format":     "bf.format",
	"added_date": "b.added_date",
	"page_count": "bm.page_count",
	"publisher":  "bm.publisher",
}

// allowedOperators is the set of valid operator names.
// Using an allowlist prevents SQL injection via operator names.
var allowedOperators = map[string]bool{
	"contains":     true,
	"equals":       true,
	"starts_with":  true,
	"ends_with":    true,
	"greater_than": true,
	"less_than":    true,
	"is_empty":     true,
	"is_not_empty": true,
}

// allowedSortFields maps sort field names to their SQL column expressions.
var allowedSortFields = map[string]string{
	"title":      "bm.title",
	"added_date": "b.added_date",
	"author":     "a.name",
	"page_count": "bm.page_count",
}

// ErrInvalidField is returned when an unknown field name is used in a rule.
var ErrInvalidField = errors.New("invalid rule field")

// ErrInvalidOperator is returned when an unknown operator is used in a rule.
var ErrInvalidOperator = errors.New("invalid rule operator")

// ErrInvalidSortField is returned when an unknown sort field is specified.
var ErrInvalidSortField = errors.New("invalid sort field")

// ErrInvalidSortDir is returned when an invalid sort direction is specified.
var ErrInvalidSortDir = errors.New("invalid sort direction: must be ASC or DESC")

// BuildQuery builds a safe parameterized SQL WHERE clause from a RuleGroup.
// It returns the WHERE clause string (without the "WHERE" keyword) and the
// parameter values to bind. Returns an error if any field or operator is invalid.
func BuildQuery(group RuleGroup, libraryIDs []int64) (string, []any, error) {
	if len(libraryIDs) == 0 {
		return "1=0", nil, nil
	}

	// Build the library ID placeholders.
	libPlaceholders := make([]string, len(libraryIDs))
	libArgs := make([]any, len(libraryIDs))
	for i, id := range libraryIDs {
		libPlaceholders[i] = "?"
		libArgs[i] = id
	}
	libClause := fmt.Sprintf("b.library_id IN (%s)", strings.Join(libPlaceholders, ", "))

	// Build the rule group clause.
	ruleClause, ruleArgs, err := buildGroupClause(group)
	if err != nil {
		return "", nil, err
	}

	var where string
	if ruleClause == "" {
		where = libClause
	} else {
		where = fmt.Sprintf("%s AND (%s)", libClause, ruleClause)
	}

	args := append(libArgs, ruleArgs...)
	return where, args, nil
}

// buildGroupClause recursively builds a SQL clause for a RuleGroup.
func buildGroupClause(group RuleGroup) (string, []any, error) {
	if len(group.Rules) == 0 {
		return "", nil, nil
	}

	op := strings.ToUpper(group.Operator)
	if op != "AND" && op != "OR" {
		op = "AND" // default to AND
	}

	parts := make([]string, 0, len(group.Rules))
	var args []any

	for _, item := range group.Rules {
		switch item.Type {
		case "condition":
			clause, itemArgs, err := buildConditionClause(item)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, clause)
			args = append(args, itemArgs...)
		case "group":
			if item.Group == nil {
				continue
			}
			clause, itemArgs, err := buildGroupClause(*item.Group)
			if err != nil {
				return "", nil, err
			}
			if clause != "" {
				parts = append(parts, fmt.Sprintf("(%s)", clause))
				args = append(args, itemArgs...)
			}
		default:
			// Skip unknown item types.
		}
	}

	if len(parts) == 0 {
		return "", nil, nil
	}

	return strings.Join(parts, fmt.Sprintf(" %s ", op)), args, nil
}

// buildConditionClause builds a SQL clause for a single RuleItem condition.
func buildConditionClause(item RuleItem) (string, []any, error) {
	col, ok := allowedFields[item.Field]
	if !ok {
		return "", nil, fmt.Errorf("%w: %q", ErrInvalidField, item.Field)
	}

	if !allowedOperators[item.Operator] {
		return "", nil, fmt.Errorf("%w: %q", ErrInvalidOperator, item.Operator)
	}

	switch item.Operator {
	case "contains":
		return fmt.Sprintf("%s LIKE ?", col), []any{"%" + item.Value + "%"}, nil
	case "equals":
		return fmt.Sprintf("%s = ?", col), []any{item.Value}, nil
	case "starts_with":
		return fmt.Sprintf("%s LIKE ?", col), []any{item.Value + "%"}, nil
	case "ends_with":
		return fmt.Sprintf("%s LIKE ?", col), []any{"%" + item.Value}, nil
	case "greater_than":
		return fmt.Sprintf("%s > ?", col), []any{item.Value}, nil
	case "less_than":
		return fmt.Sprintf("%s < ?", col), []any{item.Value}, nil
	case "is_empty":
		return fmt.Sprintf("(%s IS NULL OR %s = '')", col, col), nil, nil
	case "is_not_empty":
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", col, col), nil, nil
	default:
		// Already validated above; this is unreachable.
		return "", nil, fmt.Errorf("%w: %q", ErrInvalidOperator, item.Operator)
	}
}

// buildMagicShelfQuery builds the full SQL query for evaluating a magic shelf.
// It returns the query string and the arguments to bind.
func buildMagicShelfQuery(shelf MagicShelf, libraryIDs []int64, countOnly bool) (string, []any, error) {
	var group RuleGroup
	if err := json.Unmarshal([]byte(shelf.Rules), &group); err != nil {
		return "", nil, fmt.Errorf("parse rules: %w", err)
	}

	where, args, err := BuildQuery(group, libraryIDs)
	if err != nil {
		return "", nil, fmt.Errorf("build query: %w", err)
	}

	if countOnly {
		query := fmt.Sprintf(`
SELECT COUNT(DISTINCT b.id)
FROM book b
LEFT JOIN book_metadata bm ON bm.book_id = b.id
LEFT JOIN book_author ba ON ba.book_id = b.id
LEFT JOIN author a ON a.id = ba.author_id
LEFT JOIN book_category bc ON bc.book_id = b.id
LEFT JOIN category c ON c.id = bc.category_id
LEFT JOIN book_tag bt ON bt.book_id = b.id
LEFT JOIN tag t ON t.id = bt.tag_id
LEFT JOIN book_series bs ON bs.book_id = b.id
LEFT JOIN series s ON s.id = bs.series_id
LEFT JOIN book_file bf ON bf.book_id = b.id
WHERE %s`, where)
		return query, args, nil
	}

	// Validate sort field and direction.
	sortCol, ok := allowedSortFields[shelf.SortField]
	if !ok {
		sortCol = "b.added_date" // safe default
	}

	sortDir := strings.ToUpper(shelf.SortDir)
	if sortDir != "ASC" && sortDir != "DESC" {
		sortDir = "DESC" // safe default
	}

	query := fmt.Sprintf(`
SELECT DISTINCT b.id, b.library_id, b.book_type, b.added_date,
       bm.title, bm.cover_path
FROM book b
LEFT JOIN book_metadata bm ON bm.book_id = b.id
LEFT JOIN book_author ba ON ba.book_id = b.id
LEFT JOIN author a ON a.id = ba.author_id
LEFT JOIN book_category bc ON bc.book_id = b.id
LEFT JOIN category c ON c.id = bc.category_id
LEFT JOIN book_tag bt ON bt.book_id = b.id
LEFT JOIN tag t ON t.id = bt.tag_id
LEFT JOIN book_series bs ON bs.book_id = b.id
LEFT JOIN series s ON s.id = bs.series_id
LEFT JOIN book_file bf ON bf.book_id = b.id
WHERE %s
ORDER BY %s %s`, where, sortCol, sortDir)

	if shelf.LimitCount.Valid && shelf.LimitCount.Int64 > 0 {
		query += fmt.Sprintf("\nLIMIT %d", shelf.LimitCount.Int64)
	}

	return query, args, nil
}

// EvaluateMagicShelf evaluates a magic shelf and returns the matching books.
func EvaluateMagicShelf(ctx context.Context, db *sql.DB, shelf MagicShelf, libraryIDs []int64) ([]BookResult, error) {
	query, args, err := buildMagicShelfQuery(shelf, libraryIDs, false)
	if err != nil {
		return nil, fmt.Errorf("build magic shelf query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute magic shelf query: %w", err)
	}
	defer rows.Close()

	var results []BookResult
	for rows.Next() {
		var r BookResult
		if err := rows.Scan(&r.ID, &r.LibraryID, &r.BookType, &r.AddedDate, &r.Title, &r.CoverPath); err != nil {
			return nil, fmt.Errorf("scan book result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return results, nil
}

// CountMagicShelf returns the count of books matching a magic shelf's rules.
func CountMagicShelf(ctx context.Context, db *sql.DB, shelf MagicShelf, libraryIDs []int64) (int64, error) {
	query, args, err := buildMagicShelfQuery(shelf, libraryIDs, true)
	if err != nil {
		return 0, fmt.Errorf("build magic shelf count query: %w", err)
	}

	var count int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("execute magic shelf count query: %w", err)
	}

	return count, nil
}
