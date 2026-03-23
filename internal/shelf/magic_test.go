package shelf

import (
	"strings"
	"testing"
)

func TestBuildQuery_EmptyLibraries(t *testing.T) {
	group := RuleGroup{
		Operator: "AND",
		Rules: []RuleItem{
			{Type: "condition", Field: "title", Operator: "contains", Value: "test"},
		},
	}

	where, args, err := BuildQuery(group, nil)
	if err != nil {
		t.Fatalf("BuildQuery() error = %v; want nil", err)
	}
	if where != "1=0" {
		t.Errorf("BuildQuery() where = %q; want %q", where, "1=0")
	}
	if len(args) != 0 {
		t.Errorf("BuildQuery() args = %v; want empty", args)
	}
}

func TestBuildQuery_SingleCondition(t *testing.T) {
	tests := []struct {
		name       string
		field      string
		operator   string
		value      string
		wantLike   string
		wantArgVal any
		wantNoArgs bool
	}{
		{
			name:       "contains",
			field:      "title",
			operator:   "contains",
			value:      "Sanderson",
			wantLike:   "bm.title LIKE ?",
			wantArgVal: "%Sanderson%",
		},
		{
			name:       "equals",
			field:      "book_type",
			operator:   "equals",
			value:      "EBOOK",
			wantLike:   "b.book_type = ?",
			wantArgVal: "EBOOK",
		},
		{
			name:       "starts_with",
			field:      "author",
			operator:   "starts_with",
			value:      "Brandon",
			wantLike:   "a.name LIKE ?",
			wantArgVal: "Brandon%",
		},
		{
			name:       "ends_with",
			field:      "series",
			operator:   "ends_with",
			value:      "Cycle",
			wantLike:   "s.name LIKE ?",
			wantArgVal: "%Cycle",
		},
		{
			name:       "greater_than",
			field:      "page_count",
			operator:   "greater_than",
			value:      "300",
			wantLike:   "bm.page_count > ?",
			wantArgVal: "300",
		},
		{
			name:       "less_than",
			field:      "page_count",
			operator:   "less_than",
			value:      "100",
			wantLike:   "bm.page_count < ?",
			wantArgVal: "100",
		},
		{
			name:       "is_empty",
			field:      "language",
			operator:   "is_empty",
			value:      "",
			wantLike:   "(bm.language IS NULL OR bm.language = '')",
			wantNoArgs: true,
		},
		{
			name:       "is_not_empty",
			field:      "publisher",
			operator:   "is_not_empty",
			value:      "",
			wantLike:   "(bm.publisher IS NOT NULL AND bm.publisher != '')",
			wantNoArgs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := RuleGroup{
				Operator: "AND",
				Rules: []RuleItem{
					{Type: "condition", Field: tt.field, Operator: tt.operator, Value: tt.value},
				},
			}

			where, args, err := BuildQuery(group, []int64{1})
			if err != nil {
				t.Fatalf("BuildQuery() error = %v; want nil", err)
			}

			if !strings.Contains(where, tt.wantLike) {
				t.Errorf("BuildQuery() where = %q; want to contain %q", where, tt.wantLike)
			}

			if tt.wantNoArgs {
				// Only the library ID arg should be present.
				if len(args) != 1 {
					t.Errorf("BuildQuery() len(args) = %d; want 1 (only library ID)", len(args))
				}
			} else {
				if len(args) != 2 {
					t.Errorf("BuildQuery() len(args) = %d; want 2 (library ID + condition)", len(args))
				} else if args[1] != tt.wantArgVal {
					t.Errorf("BuildQuery() args[1] = %v; want %v", args[1], tt.wantArgVal)
				}
			}
		})
	}
}

func TestBuildQuery_ANDGroup(t *testing.T) {
	group := RuleGroup{
		Operator: "AND",
		Rules: []RuleItem{
			{Type: "condition", Field: "author", Operator: "contains", Value: "Sanderson"},
			{Type: "condition", Field: "category", Operator: "equals", Value: "Fantasy"},
		},
	}

	where, args, err := BuildQuery(group, []int64{1, 2})
	if err != nil {
		t.Fatalf("BuildQuery() error = %v; want nil", err)
	}

	if !strings.Contains(where, "AND") {
		t.Errorf("BuildQuery() where = %q; want AND operator", where)
	}
	if !strings.Contains(where, "a.name LIKE ?") {
		t.Errorf("BuildQuery() where = %q; want author condition", where)
	}
	if !strings.Contains(where, "c.name = ?") {
		t.Errorf("BuildQuery() where = %q; want category condition", where)
	}
	// 2 library IDs + 2 condition args = 4 total
	if len(args) != 4 {
		t.Errorf("BuildQuery() len(args) = %d; want 4", len(args))
	}
}

func TestBuildQuery_ORGroup(t *testing.T) {
	group := RuleGroup{
		Operator: "OR",
		Rules: []RuleItem{
			{Type: "condition", Field: "tag", Operator: "equals", Value: "sci-fi"},
			{Type: "condition", Field: "tag", Operator: "equals", Value: "fantasy"},
		},
	}

	where, args, err := BuildQuery(group, []int64{1})
	if err != nil {
		t.Fatalf("BuildQuery() error = %v; want nil", err)
	}

	if !strings.Contains(where, " OR ") {
		t.Errorf("BuildQuery() where = %q; want OR operator", where)
	}
	// 1 library ID + 2 condition args = 3 total
	if len(args) != 3 {
		t.Errorf("BuildQuery() len(args) = %d; want 3", len(args))
	}
}

func TestBuildQuery_NestedGroups(t *testing.T) {
	// (author contains "Sanderson") AND (tag = "fantasy" OR tag = "sci-fi")
	group := RuleGroup{
		Operator: "AND",
		Rules: []RuleItem{
			{Type: "condition", Field: "author", Operator: "contains", Value: "Sanderson"},
			{
				Type: "group",
				Group: &RuleGroup{
					Operator: "OR",
					Rules: []RuleItem{
						{Type: "condition", Field: "tag", Operator: "equals", Value: "fantasy"},
						{Type: "condition", Field: "tag", Operator: "equals", Value: "sci-fi"},
					},
				},
			},
		},
	}

	where, args, err := BuildQuery(group, []int64{1})
	if err != nil {
		t.Fatalf("BuildQuery() error = %v; want nil", err)
	}

	if !strings.Contains(where, "a.name LIKE ?") {
		t.Errorf("BuildQuery() where = %q; want author condition", where)
	}
	if !strings.Contains(where, " OR ") {
		t.Errorf("BuildQuery() where = %q; want nested OR group", where)
	}
	// 1 library ID + 1 author arg + 2 tag args = 4 total
	if len(args) != 4 {
		t.Errorf("BuildQuery() len(args) = %d; want 4", len(args))
	}
}

func TestBuildQuery_InvalidField(t *testing.T) {
	group := RuleGroup{
		Operator: "AND",
		Rules: []RuleItem{
			{Type: "condition", Field: "DROP TABLE books; --", Operator: "equals", Value: "x"},
		},
	}

	_, _, err := BuildQuery(group, []int64{1})
	if err == nil {
		t.Fatal("BuildQuery() error = nil; want ErrInvalidField")
	}
	if !strings.Contains(err.Error(), "invalid rule field") {
		t.Errorf("BuildQuery() error = %v; want to contain 'invalid rule field'", err)
	}
}

func TestBuildQuery_InvalidOperator(t *testing.T) {
	group := RuleGroup{
		Operator: "AND",
		Rules: []RuleItem{
			{Type: "condition", Field: "title", Operator: "'; DROP TABLE books; --", Value: "x"},
		},
	}

	_, _, err := BuildQuery(group, []int64{1})
	if err == nil {
		t.Fatal("BuildQuery() error = nil; want ErrInvalidOperator")
	}
	if !strings.Contains(err.Error(), "invalid rule operator") {
		t.Errorf("BuildQuery() error = %v; want to contain 'invalid rule operator'", err)
	}
}

func TestBuildQuery_SQLInjectionPrevention(t *testing.T) {
	// Even if the value contains SQL, it should be safely parameterized.
	group := RuleGroup{
		Operator: "AND",
		Rules: []RuleItem{
			{Type: "condition", Field: "title", Operator: "contains", Value: "'; DROP TABLE books; --"},
		},
	}

	where, args, err := BuildQuery(group, []int64{1})
	if err != nil {
		t.Fatalf("BuildQuery() error = %v; want nil", err)
	}

	// The WHERE clause should use a placeholder, not the raw value.
	if strings.Contains(where, "DROP TABLE") {
		t.Errorf("BuildQuery() where = %q; SQL injection found in WHERE clause", where)
	}

	// The value should be in args, not in the query string.
	found := false
	for _, arg := range args {
		if s, ok := arg.(string); ok && strings.Contains(s, "DROP TABLE") {
			found = true
		}
	}
	if !found {
		t.Errorf("BuildQuery() args = %v; expected injection string in args", args)
	}
}

func TestBuildQuery_AllSupportedFields(t *testing.T) {
	fields := []string{
		"title", "author", "category", "tag", "series",
		"language", "book_type", "format", "added_date", "page_count", "publisher",
	}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			group := RuleGroup{
				Operator: "AND",
				Rules: []RuleItem{
					{Type: "condition", Field: field, Operator: "equals", Value: "test"},
				},
			}

			_, _, err := BuildQuery(group, []int64{1})
			if err != nil {
				t.Errorf("BuildQuery() field=%q error = %v; want nil", field, err)
			}
		})
	}
}

func TestBuildQuery_AllSupportedOperators(t *testing.T) {
	operators := []string{
		"contains", "equals", "starts_with", "ends_with",
		"greater_than", "less_than", "is_empty", "is_not_empty",
	}

	for _, op := range operators {
		t.Run(op, func(t *testing.T) {
			group := RuleGroup{
				Operator: "AND",
				Rules: []RuleItem{
					{Type: "condition", Field: "title", Operator: op, Value: "test"},
				},
			}

			_, _, err := BuildQuery(group, []int64{1})
			if err != nil {
				t.Errorf("BuildQuery() operator=%q error = %v; want nil", op, err)
			}
		})
	}
}

func TestBuildQuery_EmptyRules(t *testing.T) {
	group := RuleGroup{
		Operator: "AND",
		Rules:    []RuleItem{},
	}

	where, args, err := BuildQuery(group, []int64{1})
	if err != nil {
		t.Fatalf("BuildQuery() error = %v; want nil", err)
	}

	// With no rules, only the library filter should be present.
	if !strings.Contains(where, "b.library_id IN") {
		t.Errorf("BuildQuery() where = %q; want library filter", where)
	}
	if len(args) != 1 {
		t.Errorf("BuildQuery() len(args) = %d; want 1 (library ID only)", len(args))
	}
}

func TestBuildQuery_MultipleLibraries(t *testing.T) {
	group := RuleGroup{
		Operator: "AND",
		Rules: []RuleItem{
			{Type: "condition", Field: "title", Operator: "contains", Value: "test"},
		},
	}

	where, args, err := BuildQuery(group, []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("BuildQuery() error = %v; want nil", err)
	}

	if !strings.Contains(where, "b.library_id IN (?, ?, ?)") {
		t.Errorf("BuildQuery() where = %q; want 3 library placeholders", where)
	}
	// 3 library IDs + 1 condition arg = 4 total
	if len(args) != 4 {
		t.Errorf("BuildQuery() len(args) = %d; want 4", len(args))
	}
}
