package service

import (
	"fmt"
	"strings"
)

// SearchMode controls how the search query is matched.
const (
	SearchModeContains = "contains" // default: single substring ILIKE
	SearchModeWords    = "words"    // split on spaces, AND all words
	SearchModeFuzzy    = "fuzzy"    // trigram similarity (pg_trgm)
)

// validSearchModes lists recognized search_mode values.
var validSearchModes = map[string]bool{
	SearchModeContains: true,
	SearchModeWords:    true,
	SearchModeFuzzy:    true,
}

// ValidateSearchMode returns true if the given mode is valid.
func ValidateSearchMode(mode string) bool {
	return validSearchModes[mode]
}

// SearchClauseResult holds the SQL fragment and args from building a search clause.
type SearchClauseResult struct {
	SQL  string
	Args []any
	ArgN int // updated argN after appending args
}

// BuildSearchClause generates a SQL WHERE clause fragment for searching text columns.
//
// It supports three modes:
//   - "contains" (default): ILIKE substring match, e.g. WHERE name ILIKE '%term%'
//   - "words": splits the search on spaces and ANDs each word, e.g. WHERE name ILIKE '%word1%' AND name ILIKE '%word2%'
//   - "fuzzy": uses pg_trgm similarity(), e.g. WHERE similarity(name, 'term') > 0.3
//
// Comma-separated values are ORed in all modes: "starbucks,amazon" matches either.
//
// columns specifies which DB columns to search (ORed together).
// nullable columns should be listed in nullableColumns so they get IS NOT NULL guards.
func BuildSearchClause(search string, mode string, columns []string, nullableColumns map[string]bool, argN int) SearchClauseResult {
	if search == "" {
		return SearchClauseResult{ArgN: argN}
	}
	if mode == "" {
		mode = SearchModeContains
	}

	// Split on commas for multi-value OR.
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
			clause, args, argN = buildFuzzyClause(term, columns, nullableColumns, args, argN)
		case SearchModeWords:
			clause, args, argN = buildWordsClause(term, columns, nullableColumns, args, argN)
		default: // contains
			clause, args, argN = buildContainsClause(term, columns, nullableColumns, args, argN)
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

// BuildExcludeSearchClause generates a SQL WHERE clause that excludes matching rows.
// Always uses "contains" mode — excluding fuzzy matches could be surprising.
// Comma-separated values are ORed (any match excludes the row).
func BuildExcludeSearchClause(search string, columns []string, nullableColumns map[string]bool, argN int) SearchClauseResult {
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

		// For exclude, each column must NOT match (AND logic per column).
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

	// Any term match should exclude: NOT (match1 OR match2)
	// Equivalent to: AND NOT match1 AND NOT match2
	var parts []string
	for _, tc := range termClauses {
		parts = append(parts, " AND "+tc)
	}
	return SearchClauseResult{SQL: strings.Join(parts, ""), Args: args, ArgN: argN}
}

// splitTerms splits a search string on commas for multi-value OR.
// Single terms (no commas) return a slice with one element.
func splitTerms(search string) []string {
	parts := strings.Split(search, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// buildContainsClause builds an ILIKE substring match for a single term across columns.
func buildContainsClause(term string, columns []string, nullableColumns map[string]bool, args []any, argN int) (string, []any, int) {
	var colClauses []string
	for _, col := range columns {
		colClauses = append(colClauses, fmt.Sprintf("%s ILIKE '%%' || $%d || '%%'", col, argN))
	}
	args = append(args, term)
	argN++
	return "(" + strings.Join(colClauses, " OR ") + ")", args, argN
}

// buildWordsClause splits the term on spaces and requires all words to match.
// Each word must appear in at least one of the searched columns.
func buildWordsClause(term string, columns []string, nullableColumns map[string]bool, args []any, argN int) (string, []any, int) {
	words := strings.Fields(term)
	if len(words) == 0 {
		return "", args, argN
	}
	// Single word: same as contains
	if len(words) == 1 {
		return buildContainsClause(words[0], columns, nullableColumns, args, argN)
	}

	// Each word must match at least one column
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
	return "(" + strings.Join(wordClauses, " AND ") + ")", args, argN
}

// buildFuzzyClause uses pg_trgm similarity() for typo-tolerant matching.
// Threshold is 0.15 (lower than default 0.3) to catch more partial matches.
func buildFuzzyClause(term string, columns []string, nullableColumns map[string]bool, args []any, argN int) (string, []any, int) {
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

// TransactionSearchColumns are the standard columns searched for transactions.
var TransactionSearchColumns = []string{"t.name", "t.merchant_name"}

// TransactionNullableColumns marks which transaction search columns are nullable.
var TransactionNullableColumns = map[string]bool{"t.merchant_name": true}

// RuleSearchColumns are the columns searched for transaction rules.
var RuleSearchColumns = []string{"tr.name"}

// RuleNullableColumns — no nullable columns for rule search.
var RuleNullableColumns = map[string]bool{}
