package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Benchmarks for BuildSearchClause and BuildExcludeSearchClause.
//
// These cover the hot path used by almost every MCP read and REST read
// that filters transactions, across three search modes and a range of
// column/term widths.
//
// Run with:
//   go test -run=^$ -bench=BenchmarkBuildSearch -benchmem ./internal/service/ -count=5

// Small: 1 column, 1 term, contains mode.
func BenchmarkBuildSearchClause_SmallContains(b *testing.B) {
	cols := []string{"t.name"}
	var nullable map[string]bool
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildSearchClause("starbucks", "contains", cols, nullable, 1)
	}
}

// Medium: 2 columns, 3 comma-separated terms, words mode.
func BenchmarkBuildSearchClause_MediumWords(b *testing.B) {
	cols := []string{"t.name", "t.merchant_name"}
	nullable := map[string]bool{"t.merchant_name": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildSearchClause("century link,verizon wireless,amazon prime", "words", cols, nullable, 1)
	}
}

// Large: 4 columns, 5 terms, fuzzy mode.
func BenchmarkBuildSearchClause_LargeFuzzy(b *testing.B) {
	cols := []string{"t.name", "t.merchant_name", "t.description", "t.note"}
	nullable := map[string]bool{"t.merchant_name": true, "t.description": true, "t.note": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildSearchClause("starbucks,amazon,target,walmart,costco", "fuzzy", cols, nullable, 1)
	}
}

// Large combined: a fuzzy search clause plus an exclude clause, mirroring a
// realistic "hunt for unknown charges while excluding known merchants" query.
func BenchmarkBuildSearchClause_LargeFuzzyWithExclude(b *testing.B) {
	cols := []string{"t.name", "t.merchant_name", "t.description", "t.note"}
	nullable := map[string]bool{"t.merchant_name": true, "t.description": true, "t.note": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		search := BuildSearchClause("starbucks,amazon,target,walmart,costco", "fuzzy", cols, nullable, 1)
		_ = BuildExcludeSearchClause("netflix,spotify,hulu,disney,apple", cols, nullable, search.ArgN)
	}
}

// Exclude alone at medium width, for isolated attribution of the exclude path.
func BenchmarkBuildExcludeSearchClause_Medium(b *testing.B) {
	cols := []string{"t.name", "t.merchant_name"}
	nullable := map[string]bool{"t.merchant_name": true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildExcludeSearchClause("starbucks,amazon,target", cols, nullable, 1)
	}
}

// --- Equivalence checks ---
//
// These tests pin the exact SQL produced by BuildSearchClause and
// BuildExcludeSearchClause against the previous (Sprintf/Join) implementation
// so the perf refactor is proven to be byte-identical. They are regular Go
// tests (not benchmarks) and run under `go test ./internal/service/...`.

// referenceSearchResult is a copy of the pre-refactor implementation, kept
// here purely as an oracle. Any change to BuildSearchClause that does not
// preserve the exact produced SQL will fail TestBuildSearchClauseEquivalence.
func referenceSearchResult(search string, mode string, columns []string, nullableColumns map[string]bool, argN int) SearchClauseResult {
	if search == "" {
		return SearchClauseResult{ArgN: argN}
	}
	if mode == "" {
		mode = SearchModeContains
	}

	terms := splitTerms(search)
	if len(terms) == 0 {
		return SearchClauseResult{ArgN: argN}
	}

	var termClauses []string
	var args []any

	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}

		var clause string
		switch mode {
		case SearchModeFuzzy:
			clause, args, argN = refFuzzy(term, columns, nullableColumns, args, argN)
		case SearchModeWords:
			clause, args, argN = refWords(term, columns, nullableColumns, args, argN)
		default:
			clause, args, argN = refContains(term, columns, args, argN)
		}
		if clause != "" {
			termClauses = append(termClauses, clause)
		}
	}

	if len(termClauses) == 0 {
		return SearchClauseResult{ArgN: argN}
	}

	sql := " AND (" + strings.Join(termClauses, " OR ") + ")"
	return SearchClauseResult{SQL: sql, Args: args, ArgN: argN}
}

func referenceExcludeResult(search string, columns []string, nullableColumns map[string]bool, argN int) SearchClauseResult {
	if search == "" {
		return SearchClauseResult{ArgN: argN}
	}

	terms := splitTerms(search)
	if len(terms) == 0 {
		return SearchClauseResult{ArgN: argN}
	}

	var termClauses []string
	var args []any

	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}

		var colClauses []string
		for _, col := range columns {
			if nullableColumns[col] {
				colClauses = append(colClauses, fmt.Sprintf("(%s IS NULL OR %s NOT ILIKE '%%' || $%d || '%%')", col, col, argN))
			} else {
				colClauses = append(colClauses, fmt.Sprintf("%s NOT ILIKE '%%' || $%d || '%%'", col, argN))
			}
		}
		args = append(args, term)
		argN++
		termClauses = append(termClauses, "("+strings.Join(colClauses, " AND ")+")")
	}

	if len(termClauses) == 0 {
		return SearchClauseResult{ArgN: argN}
	}

	var parts []string
	for _, tc := range termClauses {
		parts = append(parts, " AND "+tc)
	}
	return SearchClauseResult{SQL: strings.Join(parts, ""), Args: args, ArgN: argN}
}

func refContains(term string, columns []string, args []any, argN int) (string, []any, int) {
	var colClauses []string
	for _, col := range columns {
		colClauses = append(colClauses, fmt.Sprintf("%s ILIKE '%%' || $%d || '%%'", col, argN))
	}
	args = append(args, term)
	argN++
	return "(" + strings.Join(colClauses, " OR ") + ")", args, argN
}

func refWords(term string, columns []string, nullableColumns map[string]bool, args []any, argN int) (string, []any, int) {
	words := strings.Fields(term)
	if len(words) == 0 {
		return "", args, argN
	}
	if len(words) == 1 {
		return refContains(words[0], columns, args, argN)
	}
	var wordClauses []string
	for _, word := range words {
		var colClauses []string
		for _, col := range columns {
			colClauses = append(colClauses, fmt.Sprintf("%s ILIKE '%%' || $%d || '%%'", col, argN))
		}
		args = append(args, word)
		argN++
		wordClauses = append(wordClauses, "("+strings.Join(colClauses, " OR ")+")")
	}
	_ = nullableColumns
	return "(" + strings.Join(wordClauses, " AND ") + ")", args, argN
}

func refFuzzy(term string, columns []string, nullableColumns map[string]bool, args []any, argN int) (string, []any, int) {
	var colClauses []string
	for _, col := range columns {
		if nullableColumns[col] {
			colClauses = append(colClauses, fmt.Sprintf("(%s IS NOT NULL AND similarity(%s, $%d) > 0.15)", col, col, argN))
		} else {
			colClauses = append(colClauses, fmt.Sprintf("similarity(%s, $%d) > 0.15", col, argN))
		}
	}
	args = append(args, term)
	argN++
	return "(" + strings.Join(colClauses, " OR ") + ")", args, argN
}

func TestBuildSearchClauseEquivalence(t *testing.T) {
	txCols := []string{"t.name", "t.merchant_name"}
	txNullable := map[string]bool{"t.merchant_name": true}
	wideCols := []string{"t.name", "t.merchant_name", "t.description", "t.note"}
	wideNullable := map[string]bool{"t.merchant_name": true, "t.description": true, "t.note": true}
	ruleCols := []string{"tr.name"}

	type tc struct {
		name     string
		search   string
		mode     string
		cols     []string
		nullable map[string]bool
		argN     int
	}
	cases := []tc{
		{"empty", "", "contains", txCols, txNullable, 1},
		{"simple-contains", "starbucks", "contains", txCols, txNullable, 1},
		{"default-mode", "starbucks", "", txCols, txNullable, 5},
		{"words-single", "starbucks", "words", txCols, txNullable, 1},
		{"words-multi", "century link", "words", txCols, txNullable, 1},
		{"words-comma", "century link,verizon wireless", "words", txCols, txNullable, 1},
		{"fuzzy-simple", "starbuks", "fuzzy", txCols, txNullable, 1},
		{"fuzzy-wide", "starbucks,amazon,target,walmart,costco", "fuzzy", wideCols, wideNullable, 7},
		{"contains-comma", "starbucks,amazon", "contains", txCols, txNullable, 1},
		{"contains-comma-spaces", " starbucks , amazon , ", "contains", txCols, txNullable, 1},
		{"contains-single-col", "foo", "contains", ruleCols, nil, 3},
		{"contains-no-nullable", "foo,bar,baz", "contains", ruleCols, nil, 1},
		{"all-whitespace-term", " , , ", "contains", txCols, txNullable, 1},
		{"mixed-whitespace", "foo,  ,bar", "contains", txCols, txNullable, 1},
		{"words-with-extra-spaces", "  century   link  ", "words", txCols, txNullable, 9},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BuildSearchClause(c.search, c.mode, c.cols, c.nullable, c.argN)
			want := referenceSearchResult(c.search, c.mode, c.cols, c.nullable, c.argN)
			if got.SQL != want.SQL {
				t.Errorf("SQL mismatch:\n  got:  %q\n  want: %q", got.SQL, want.SQL)
			}
			if !reflect.DeepEqual(got.Args, want.Args) {
				t.Errorf("Args mismatch:\n  got:  %v\n  want: %v", got.Args, want.Args)
			}
			if got.ArgN != want.ArgN {
				t.Errorf("ArgN mismatch: got %d, want %d", got.ArgN, want.ArgN)
			}
		})
	}
}

func TestBuildExcludeSearchClauseEquivalence(t *testing.T) {
	txCols := []string{"t.name", "t.merchant_name"}
	txNullable := map[string]bool{"t.merchant_name": true}
	ruleCols := []string{"tr.name"}

	type tc struct {
		name     string
		search   string
		cols     []string
		nullable map[string]bool
		argN     int
	}
	cases := []tc{
		{"empty", "", txCols, txNullable, 1},
		{"simple", "starbucks", txCols, txNullable, 1},
		{"comma", "starbucks,amazon", txCols, txNullable, 1},
		{"comma-with-spaces", " starbucks , amazon , ", txCols, txNullable, 1},
		{"no-nullable", "foo", ruleCols, nil, 1},
		{"multi-terms-nonnullable", "foo,bar,baz", ruleCols, nil, 3},
		{"mixed-whitespace", "foo,  ,bar", txCols, txNullable, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BuildExcludeSearchClause(c.search, c.cols, c.nullable, c.argN)
			want := referenceExcludeResult(c.search, c.cols, c.nullable, c.argN)
			if got.SQL != want.SQL {
				t.Errorf("SQL mismatch:\n  got:  %q\n  want: %q", got.SQL, want.SQL)
			}
			if !reflect.DeepEqual(got.Args, want.Args) {
				t.Errorf("Args mismatch:\n  got:  %v\n  want: %v", got.Args, want.Args)
			}
			if got.ArgN != want.ArgN {
				t.Errorf("ArgN mismatch: got %d, want %d", got.ArgN, want.ArgN)
			}
		})
	}
}
