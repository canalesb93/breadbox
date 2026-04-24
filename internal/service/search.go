package service

import (
	"strconv"
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

	// Pre-allocate a single builder for the whole clause and an args slice
	// sized for the common case. In "words" mode one term may emit multiple
	// args (one per word) — the slice will grow if needed but the upfront
	// capacity covers the usual non-words path exactly.
	var sb strings.Builder
	sb.Grow(estimateClauseSize(mode, len(columns), len(terms), columns))
	args := make([]any, 0, len(terms))

	// We build the clause in a scratch segment so that if no term produces
	// output we can return early without needing to trim the builder.
	// Structure: " AND ( <term1> OR <term2> ... )"
	// The outer " AND (" and trailing ")" are only written if ≥1 term wrote
	// content.
	wrote := false
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}

		// In words mode we need to know up-front if the term produces any
		// words — otherwise it emits no SQL and we must not write the
		// separator before it.
		if mode == SearchModeWords && len(strings.Fields(term)) == 0 {
			continue
		}

		if !wrote {
			sb.WriteString(" AND (")
			wrote = true
		} else {
			sb.WriteString(" OR ")
		}

		switch mode {
		case SearchModeFuzzy:
			args, argN = writeFuzzyClause(&sb, term, columns, nullableColumns, args, argN)
		case SearchModeWords:
			args, argN = writeWordsClause(&sb, term, columns, nullableColumns, args, argN)
		default: // contains
			args, argN = writeContainsClause(&sb, term, columns, args, argN)
		}
	}

	if !wrote {
		return SearchClauseResult{ArgN: argN}
	}

	sb.WriteByte(')')
	return SearchClauseResult{SQL: sb.String(), Args: args, ArgN: argN}
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

	var sb strings.Builder
	sb.Grow(estimateExcludeSize(len(columns), len(terms), columns, nullableColumns))
	args := make([]any, 0, len(terms))

	wrote := false
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}

		// Each term contributes a " AND (col1clause AND col2clause ...)"
		// fragment.
		sb.WriteString(" AND (")

		argNStr := strconv.Itoa(argN)
		for i, col := range columns {
			if i > 0 {
				sb.WriteString(" AND ")
			}
			if nullableColumns[col] {
				// (col IS NULL OR col NOT ILIKE '%' || $N || '%')
				sb.WriteByte('(')
				sb.WriteString(col)
				sb.WriteString(" IS NULL OR ")
				sb.WriteString(col)
				sb.WriteString(" NOT ILIKE '%' || $")
				sb.WriteString(argNStr)
				sb.WriteString(" || '%')")
			} else {
				// col NOT ILIKE '%' || $N || '%'
				sb.WriteString(col)
				sb.WriteString(" NOT ILIKE '%' || $")
				sb.WriteString(argNStr)
				sb.WriteString(" || '%'")
			}
		}

		sb.WriteByte(')')

		args = append(args, term)
		argN++
		wrote = true
	}

	if !wrote {
		return SearchClauseResult{ArgN: argN}
	}

	return SearchClauseResult{SQL: sb.String(), Args: args, ArgN: argN}
}

// splitTerms splits a search string on commas for multi-value OR.
// Single terms (no commas) return a slice with one element.
func splitTerms(search string) []string {
	// Fast path: no comma — single-term slice (or empty).
	if !strings.Contains(search, ",") {
		trimmed := strings.TrimSpace(search)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}

	// Pre-size to the exact comma count + 1 to avoid regrowing.
	n := strings.Count(search, ",") + 1
	result := make([]string, 0, n)
	start := 0
	for i := 0; i < len(search); i++ {
		if search[i] == ',' {
			p := strings.TrimSpace(search[start:i])
			if p != "" {
				result = append(result, p)
			}
			start = i + 1
		}
	}
	p := strings.TrimSpace(search[start:])
	if p != "" {
		result = append(result, p)
	}
	return result
}

// writeContainsClause appends an ILIKE substring clause for a single term
// across columns directly to sb. Returns the updated args slice and argN.
func writeContainsClause(sb *strings.Builder, term string, columns []string, args []any, argN int) ([]any, int) {
	argNStr := strconv.Itoa(argN)
	sb.WriteByte('(')
	for i, col := range columns {
		if i > 0 {
			sb.WriteString(" OR ")
		}
		sb.WriteString(col)
		sb.WriteString(" ILIKE '%' || $")
		sb.WriteString(argNStr)
		sb.WriteString(" || '%'")
	}
	sb.WriteByte(')')
	args = append(args, term)
	argN++
	return args, argN
}

// writeWordsClause splits the term on spaces and requires all words to match.
// Each word must appear in at least one of the searched columns. Caller must
// ensure the term produces at least one word.
func writeWordsClause(sb *strings.Builder, term string, columns []string, _ map[string]bool, args []any, argN int) ([]any, int) {
	words := strings.Fields(term)
	if len(words) == 0 {
		// Defensive: caller should have filtered this out.
		return args, argN
	}
	// Single word: identical to contains.
	if len(words) == 1 {
		return writeContainsClause(sb, words[0], columns, args, argN)
	}

	// Multi-word: ((col OR col) AND (col OR col) ...)
	sb.WriteByte('(')
	for wi, word := range words {
		if wi > 0 {
			sb.WriteString(" AND ")
		}
		argNStr := strconv.Itoa(argN)
		sb.WriteByte('(')
		for ci, col := range columns {
			if ci > 0 {
				sb.WriteString(" OR ")
			}
			sb.WriteString(col)
			sb.WriteString(" ILIKE '%' || $")
			sb.WriteString(argNStr)
			sb.WriteString(" || '%'")
		}
		sb.WriteByte(')')
		args = append(args, word)
		argN++
	}
	sb.WriteByte(')')
	return args, argN
}

// writeFuzzyClause uses pg_trgm similarity() for typo-tolerant matching.
// Threshold is 0.15 (lower than default 0.3) to catch more partial matches.
func writeFuzzyClause(sb *strings.Builder, term string, columns []string, nullableColumns map[string]bool, args []any, argN int) ([]any, int) {
	argNStr := strconv.Itoa(argN)
	sb.WriteByte('(')
	for i, col := range columns {
		if i > 0 {
			sb.WriteString(" OR ")
		}
		if nullableColumns[col] {
			// (col IS NOT NULL AND similarity(col, $N) > 0.15)
			sb.WriteByte('(')
			sb.WriteString(col)
			sb.WriteString(" IS NOT NULL AND similarity(")
			sb.WriteString(col)
			sb.WriteString(", $")
			sb.WriteString(argNStr)
			sb.WriteString(") > 0.15)")
		} else {
			// similarity(col, $N) > 0.15
			sb.WriteString("similarity(")
			sb.WriteString(col)
			sb.WriteString(", $")
			sb.WriteString(argNStr)
			sb.WriteString(") > 0.15")
		}
	}
	sb.WriteByte(')')
	args = append(args, term)
	argN++
	return args, argN
}

// estimateClauseSize approximates the output buffer size for
// BuildSearchClause so the Builder can Grow once rather than reallocating.
// Overshoot is free; undershoot triggers a realloc.
func estimateClauseSize(mode string, numCols, numTerms int, columns []string) int {
	colLenSum := 0
	for _, c := range columns {
		colLenSum += len(c)
	}
	// Per-column fixed overhead (ILIKE '%' || $NN || '%' + separators).
	perColumn := 24
	if mode == SearchModeFuzzy {
		// Fuzzy clauses are longer (similarity() and optional IS NOT NULL).
		perColumn = 64
	}
	// Words mode may AND multiple word sub-clauses per term; allow 4x slack.
	multiplier := 1
	if mode == SearchModeWords {
		multiplier = 4
	}
	// " AND (" + trailing ")" + per-term ( " OR " + "(... )" )
	return 8 + numTerms*multiplier*(colLenSum+numCols*perColumn+8)
}

// estimateExcludeSize approximates the output buffer size for
// BuildExcludeSearchClause.
func estimateExcludeSize(numCols, numTerms int, columns []string, nullableColumns map[string]bool) int {
	colLenSum := 0
	nullableCount := 0
	for _, c := range columns {
		colLenSum += len(c)
		if nullableColumns[c] {
			nullableCount++
		}
	}
	// Per-column: non-nullable ~32, nullable ~60 (IS NULL OR col NOT ILIKE...).
	perColumnNonNull := 32
	perColumnNullable := 64
	perTerm := 7 + // " AND ("
		colLenSum +
		(numCols-nullableCount)*perColumnNonNull +
		nullableCount*perColumnNullable +
		1 // ")"
	return 8 + numTerms*perTerm
}

// TransactionSearchColumns are the standard columns searched for transactions.
var TransactionSearchColumns = []string{"t.provider_name", "t.provider_merchant_name"}

// TransactionNullableColumns marks which transaction search columns are nullable.
var TransactionNullableColumns = map[string]bool{"t.provider_merchant_name": true}

// validSearchFields lists recognized search_field values.
var validSearchFields = map[string]bool{
	"all":      true,
	"name":     true,
	"merchant": true,
}

// ValidateSearchField returns true if the given field is valid.
func ValidateSearchField(field string) bool {
	return validSearchFields[field]
}

// resolveSearchField returns the columns and nullable map for a given search field.
func resolveSearchField(field *string) ([]string, map[string]bool) {
	if field == nil || *field == "" || *field == "all" {
		return TransactionSearchColumns, TransactionNullableColumns
	}
	switch *field {
	case "name":
		return []string{"t.provider_name"}, map[string]bool{}
	case "merchant":
		return []string{"t.provider_merchant_name"}, map[string]bool{"t.provider_merchant_name": true}
	default:
		return TransactionSearchColumns, TransactionNullableColumns
	}
}

// RuleSearchColumns are the columns searched for transaction rules.
var RuleSearchColumns = []string{"tr.name"}

// RuleNullableColumns — no nullable columns for rule search.
var RuleNullableColumns = map[string]bool{}
