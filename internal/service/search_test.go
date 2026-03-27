package service

import (
	"strings"
	"testing"
)

func TestBuildSearchClause_Empty(t *testing.T) {
	result := BuildSearchClause("", "", []string{"name"}, nil, 1)
	if result.SQL != "" {
		t.Errorf("expected empty SQL, got %q", result.SQL)
	}
	if result.ArgN != 1 {
		t.Errorf("expected argN=1, got %d", result.ArgN)
	}
}

func TestBuildSearchClause_Contains(t *testing.T) {
	result := BuildSearchClause("starbucks", "contains", []string{"t.name", "t.merchant_name"}, map[string]bool{"t.merchant_name": true}, 1)
	if !strings.Contains(result.SQL, "ILIKE") {
		t.Errorf("expected ILIKE in SQL, got %q", result.SQL)
	}
	if len(result.Args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(result.Args))
	}
	if result.Args[0] != "starbucks" {
		t.Errorf("expected arg 'starbucks', got %v", result.Args[0])
	}
	if result.ArgN != 2 {
		t.Errorf("expected argN=2, got %d", result.ArgN)
	}
}

func TestBuildSearchClause_ContainsDefault(t *testing.T) {
	// Empty mode string should default to contains
	result := BuildSearchClause("test", "", []string{"name"}, nil, 3)
	if !strings.Contains(result.SQL, "ILIKE") {
		t.Errorf("expected ILIKE in SQL, got %q", result.SQL)
	}
	if result.ArgN != 4 {
		t.Errorf("expected argN=4, got %d", result.ArgN)
	}
}

func TestBuildSearchClause_Words_SingleWord(t *testing.T) {
	// Single word should behave like contains
	result := BuildSearchClause("starbucks", "words", []string{"name"}, nil, 1)
	if !strings.Contains(result.SQL, "ILIKE") {
		t.Errorf("expected ILIKE in SQL, got %q", result.SQL)
	}
	if len(result.Args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(result.Args))
	}
}

func TestBuildSearchClause_Words_MultiWord(t *testing.T) {
	result := BuildSearchClause("century link", "words", []string{"t.name", "t.merchant_name"}, nil, 1)
	// Should have 2 args (one per word), ANDed together
	if len(result.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(result.Args))
	}
	if result.Args[0] != "century" {
		t.Errorf("expected first arg 'century', got %v", result.Args[0])
	}
	if result.Args[1] != "link" {
		t.Errorf("expected second arg 'link', got %v", result.Args[1])
	}
	if result.ArgN != 3 {
		t.Errorf("expected argN=3, got %d", result.ArgN)
	}
	// SQL should contain AND for word joining
	if !strings.Contains(result.SQL, " AND ") {
		t.Errorf("expected AND in SQL for multi-word, got %q", result.SQL)
	}
}

func TestBuildSearchClause_Fuzzy(t *testing.T) {
	result := BuildSearchClause("starbuks", "fuzzy", []string{"t.name", "t.merchant_name"}, map[string]bool{"t.merchant_name": true}, 1)
	if !strings.Contains(result.SQL, "similarity") {
		t.Errorf("expected similarity() in SQL, got %q", result.SQL)
	}
	// Nullable column should have IS NOT NULL guard
	if !strings.Contains(result.SQL, "IS NOT NULL") {
		t.Errorf("expected IS NOT NULL guard for nullable column, got %q", result.SQL)
	}
	if len(result.Args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(result.Args))
	}
}

func TestBuildSearchClause_CommaOR(t *testing.T) {
	result := BuildSearchClause("starbucks,amazon", "contains", []string{"name"}, nil, 1)
	// Should have 2 args (one per term), ORed
	if len(result.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(result.Args))
	}
	if result.Args[0] != "starbucks" {
		t.Errorf("expected first arg 'starbucks', got %v", result.Args[0])
	}
	if result.Args[1] != "amazon" {
		t.Errorf("expected second arg 'amazon', got %v", result.Args[1])
	}
	// SQL should contain OR for term joining
	if !strings.Contains(result.SQL, " OR ") {
		t.Errorf("expected OR in SQL for comma-separated, got %q", result.SQL)
	}
	if result.ArgN != 3 {
		t.Errorf("expected argN=3, got %d", result.ArgN)
	}
}

func TestBuildSearchClause_CommaWithSpaces(t *testing.T) {
	result := BuildSearchClause(" starbucks , amazon , ", "contains", []string{"name"}, nil, 1)
	// Should trim spaces and skip empty
	if len(result.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(result.Args))
	}
}

func TestBuildSearchClause_CommaWithWords(t *testing.T) {
	// Comma-separated + words mode: each comma term has words ANDed, terms ORed
	result := BuildSearchClause("century link,verizon wireless", "words", []string{"name"}, nil, 1)
	if len(result.Args) != 4 {
		t.Fatalf("expected 4 args (2 words per term), got %d", len(result.Args))
	}
	if result.ArgN != 5 {
		t.Errorf("expected argN=5, got %d", result.ArgN)
	}
}

func TestBuildSearchClause_MultipleColumns(t *testing.T) {
	result := BuildSearchClause("test", "contains", []string{"t.name", "t.merchant_name"}, nil, 1)
	// Should OR the columns
	if strings.Count(result.SQL, "ILIKE") != 2 {
		t.Errorf("expected 2 ILIKE clauses (one per column), got %q", result.SQL)
	}
	// But only 1 arg (same param used for both columns)
	if len(result.Args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(result.Args))
	}
}

func TestBuildExcludeSearchClause_Basic(t *testing.T) {
	result := BuildExcludeSearchClause("starbucks", []string{"t.name", "t.merchant_name"}, map[string]bool{"t.merchant_name": true}, 1)
	if !strings.Contains(result.SQL, "NOT ILIKE") {
		t.Errorf("expected NOT ILIKE in SQL, got %q", result.SQL)
	}
	// Nullable column should have IS NULL OR guard
	if !strings.Contains(result.SQL, "IS NULL OR") {
		t.Errorf("expected IS NULL OR guard for nullable column, got %q", result.SQL)
	}
	if len(result.Args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(result.Args))
	}
}

func TestBuildExcludeSearchClause_CommaOR(t *testing.T) {
	result := BuildExcludeSearchClause("starbucks,amazon", []string{"name"}, nil, 1)
	// Should have 2 args, each term excluded separately
	if len(result.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(result.Args))
	}
	// Each term should generate a NOT ILIKE
	if strings.Count(result.SQL, "NOT ILIKE") != 2 {
		t.Errorf("expected 2 NOT ILIKE clauses, got %q", result.SQL)
	}
}

func TestBuildExcludeSearchClause_Empty(t *testing.T) {
	result := BuildExcludeSearchClause("", []string{"name"}, nil, 1)
	if result.SQL != "" {
		t.Errorf("expected empty SQL, got %q", result.SQL)
	}
}

func TestValidateSearchMode(t *testing.T) {
	for _, mode := range []string{"contains", "words", "fuzzy"} {
		if !ValidateSearchMode(mode) {
			t.Errorf("expected %q to be valid", mode)
		}
	}
	for _, mode := range []string{"", "regex", "exact", "CONTAINS"} {
		if ValidateSearchMode(mode) {
			t.Errorf("expected %q to be invalid", mode)
		}
	}
}

func TestSplitTerms(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"starbucks", 1},
		{"starbucks,amazon", 2},
		{"starbucks, amazon, target", 3},
		{" , , ", 0},
		{"", 0},
		{"one,,two", 2},
	}
	for _, tt := range tests {
		result := splitTerms(tt.input)
		if len(result) != tt.expected {
			t.Errorf("splitTerms(%q): expected %d terms, got %d (%v)", tt.input, tt.expected, len(result), result)
		}
	}
}
