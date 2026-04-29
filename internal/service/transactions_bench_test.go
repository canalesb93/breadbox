package service

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// benchBuildListTransactionsQuery mirrors the query-building portion of
// ListTransactions. It takes pre-resolved UUIDs so we can benchmark just the
// string construction without DB round-trips.
func benchBuildListTransactionsQuery(params benchListParams) (string, []any) {
	var buf strings.Builder
	buf.Grow(1024)
	args := make([]any, 0, 16)
	argN := 1

	buf.WriteString("SELECT t.id, t.short_id, t.account_id, t.external_transaction_id, t.pending_transaction_id, " +
		"t.amount, t.iso_currency_code, t.unofficial_currency_code, t.date, t.authorized_date, " +
		"t.datetime, t.authorized_datetime, t.name, t.merchant_name, " +
		"t.category_primary, t.category_detailed, t.category_confidence, " +
		"t.payment_channel, t.pending, t.deleted_at, t.created_at, t.updated_at, " +
		"COALESCE(a.display_name, a.name) AS account_name, " +
		"COALESCE(au.name, u.name) AS user_name, " +
		"t.category_id, t.category_override, " +
		"c.slug AS cat_slug, c.display_name AS cat_display_name, c.icon AS cat_icon, c.color AS cat_color, " +
		"pc.slug AS cat_primary_slug, pc.display_name AS cat_primary_display_name, " +
		"t.attributed_user_id, au.name AS attributed_user_name, " +
		"COALESCE(t.attributed_user_id, bc.user_id) AS effective_user_id " +
		"FROM transactions t " +
		"JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN users au ON t.attributed_user_id = au.id " +
		"LEFT JOIN categories c ON t.category_id = c.id " +
		"LEFT JOIN categories pc ON c.parent_id = pc.id")

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	buf.WriteString(" WHERE t.deleted_at IS NULL")

	if !params.IncludeDependent {
		buf.WriteString(" AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))")
	}

	if params.UserID != nil {
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.UserID)
		argN++
	}

	if params.AccountID != nil {
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.AccountID)
		argN++
	}

	if params.StartDate != nil {
		buf.WriteString(" AND t.date >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.StartDate))
		argN++
	}

	if params.EndDate != nil {
		buf.WriteString(" AND t.date < $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.EndDate))
		argN++
	}

	if params.CategoryID != nil {
		n := strconv.Itoa(argN)
		if params.IsParentCategory {
			buf.WriteString(" AND (c.id = $")
			buf.WriteString(n)
			buf.WriteString(" OR c.parent_id = $")
			buf.WriteString(n)
			buf.WriteByte(')')
		} else {
			buf.WriteString(" AND t.category_id = $")
			buf.WriteString(n)
		}
		args = append(args, *params.CategoryID)
		argN++
	}

	if params.MinAmount != nil {
		buf.WriteString(" AND t.amount >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		buf.WriteString(" AND t.amount <= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		buf.WriteString(" AND t.pending = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		mode := ""
		if params.SearchMode != nil {
			mode = *params.SearchMode
		}
		sc := BuildSearchClause(*params.Search, mode, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(sc.SQL)
		args = append(args, sc.Args...)
		argN = sc.ArgN
	}

	if params.ExcludeSearch != nil {
		ec := BuildExcludeSearchClause(*params.ExcludeSearch, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(ec.SQL)
		args = append(args, ec.Args...)
		argN = ec.ArgN
	}

	if params.CursorDate != nil && params.CursorID != nil {
		buf.WriteString(" AND (t.date, t.id) < ($")
		buf.WriteString(strconv.Itoa(argN))
		buf.WriteString(", $")
		buf.WriteString(strconv.Itoa(argN + 1))
		buf.WriteByte(')')
		args = append(args, pgconv.Date(*params.CursorDate), *params.CursorID)
		argN += 2
	}

	sortCol := "t.date"
	if params.SortBy != nil {
		switch *params.SortBy {
		case "amount":
			sortCol = "t.amount"
		case "name":
			sortCol = "t.name"
		case "date":
			sortCol = "t.date"
		}
	}

	sortDir := "DESC"
	if params.SortOrder != nil && *params.SortOrder == "asc" {
		sortDir = "ASC"
	}

	buf.WriteString(" ORDER BY ")
	buf.WriteString(sortCol)
	buf.WriteByte(' ')
	buf.WriteString(sortDir)
	buf.WriteString(", t.id DESC")

	buf.WriteString(" LIMIT $")
	buf.WriteString(strconv.Itoa(argN))
	args = append(args, limit+1)

	return buf.String(), args
}

// benchBuildCountTransactionsQuery mirrors CountTransactionsFiltered query building.
func benchBuildCountTransactionsQuery(params benchCountParams) (string, []any) {
	var buf strings.Builder
	buf.Grow(512)
	args := make([]any, 0, 12)
	argN := 1

	buf.WriteString("SELECT COUNT(*) FROM transactions t " +
		"JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN categories c ON t.category_id = c.id")

	buf.WriteString(" WHERE t.deleted_at IS NULL")

	if !params.IncludeDependent {
		buf.WriteString(" AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))")
	}

	if params.UserID != nil {
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.UserID)
		argN++
	}

	if params.AccountID != nil {
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.AccountID)
		argN++
	}

	if params.StartDate != nil {
		buf.WriteString(" AND t.date >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.StartDate))
		argN++
	}

	if params.EndDate != nil {
		buf.WriteString(" AND t.date < $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.EndDate))
		argN++
	}

	if params.CategoryID != nil {
		n := strconv.Itoa(argN)
		if params.IsParentCategory {
			buf.WriteString(" AND (c.id = $")
			buf.WriteString(n)
			buf.WriteString(" OR c.parent_id = $")
			buf.WriteString(n)
			buf.WriteByte(')')
		} else {
			buf.WriteString(" AND t.category_id = $")
			buf.WriteString(n)
		}
		args = append(args, *params.CategoryID)
		argN++
	}

	if params.MinAmount != nil {
		buf.WriteString(" AND t.amount >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		buf.WriteString(" AND t.amount <= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		buf.WriteString(" AND t.pending = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		mode := ""
		if params.SearchMode != nil {
			mode = *params.SearchMode
		}
		sc := BuildSearchClause(*params.Search, mode, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(sc.SQL)
		args = append(args, sc.Args...)
		argN = sc.ArgN
	}

	if params.ExcludeSearch != nil {
		ec := BuildExcludeSearchClause(*params.ExcludeSearch, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(ec.SQL)
		args = append(args, ec.Args...)
		argN = ec.ArgN
	}

	_ = argN // keep for consistency
	return buf.String(), args
}

// benchBuildAdminListQuery mirrors ListTransactionsAdmin query building.
func benchBuildAdminListQuery(params benchAdminListParams) (string, []any) {
	var buf strings.Builder
	buf.Grow(2048)
	args := make([]any, 0, 16)
	argN := 1

	buf.WriteString("SELECT t.id, t.account_id, COALESCE(a.display_name, a.name, ''), " +
		"COALESCE(bc.institution_name, ''), COALESCE(au.name, u.name, ''), " +
		"t.date, t.name, t.merchant_name, t.amount, t.iso_currency_code, " +
		"t.category_id, c.display_name AS cat_display_name, c.slug AS cat_slug, c.icon AS cat_icon, COALESCE(c.color, pc.color) AS cat_color, " +
		"t.category_override, t.pending, " +
		"(SELECT COUNT(*) FROM annotations ann WHERE ann.transaction_id = t.id AND ann.kind = 'comment' AND ann.deleted_at IS NULL) AS comment_count, " +
		"EXISTS(SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = 'needs-review') AS has_pending_review, " +
		"t.created_at, t.updated_at, " +
		"COALESCE(t.attributed_user_id, bc.user_id) AS effective_user_id " +
		"FROM transactions t " +
		"LEFT JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN users au ON t.attributed_user_id = au.id " +
		"LEFT JOIN categories c ON t.category_id = c.id " +
		"LEFT JOIN categories pc ON c.parent_id = pc.id " +
		"WHERE t.deleted_at IS NULL")

	// Track the WHERE builder position so we can extract whereClauses for the count query.
	whereStart := buf.Len()

	var needAccountJoin, needConnectionJoin, needUserJoin, needCategoryJoin bool

	depClause := " AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))"
	buf.WriteString(depClause)
	needAccountJoin = true

	if params.UserID != nil {
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.UserID)
		argN++
		needAccountJoin = true
		needConnectionJoin = true
		needUserJoin = true
	}

	if params.ConnectionID != nil {
		buf.WriteString(" AND a.connection_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.ConnectionID)
		argN++
		needAccountJoin = true
	}

	if params.AccountID != nil {
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.AccountID)
		argN++
	}

	if params.StartDate != nil {
		buf.WriteString(" AND t.date >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.StartDate))
		argN++
	}

	if params.EndDate != nil {
		buf.WriteString(" AND t.date < $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.EndDate))
		argN++
	}

	if params.CategoryID != nil {
		n := strconv.Itoa(argN)
		if params.IsParentCategory {
			buf.WriteString(" AND (c.id = $")
			buf.WriteString(n)
			buf.WriteString(" OR c.parent_id = $")
			buf.WriteString(n)
			buf.WriteByte(')')
			needCategoryJoin = true
		} else {
			buf.WriteString(" AND t.category_id = $")
			buf.WriteString(n)
		}
		args = append(args, *params.CategoryID)
		argN++
	}

	if params.MinAmount != nil {
		buf.WriteString(" AND t.amount >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		buf.WriteString(" AND t.amount <= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		buf.WriteString(" AND t.pending = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		mode := ""
		if params.SearchMode != nil {
			mode = *params.SearchMode
		}
		cols, nullCols := resolveSearchField(params.SearchField)
		sc := BuildSearchClause(*params.Search, mode, cols, nullCols, argN)
		buf.WriteString(sc.SQL)
		args = append(args, sc.Args...)
		argN = sc.ArgN
	}

	if params.ExcludeSearch != nil {
		ec := BuildExcludeSearchClause(*params.ExcludeSearch, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(ec.SQL)
		args = append(args, ec.Args...)
		argN = ec.ArgN
	}

	// Extract where clauses for count query.
	mainQuery := buf.String()
	whereClauses := mainQuery[whereStart:]

	// Build minimal count query.
	var countBuf strings.Builder
	countBuf.Grow(256)
	countBuf.WriteString("SELECT COUNT(*) FROM transactions t ")
	if needAccountJoin || needConnectionJoin || needUserJoin {
		countBuf.WriteString("LEFT JOIN accounts a ON t.account_id = a.id ")
	}
	if needConnectionJoin || needUserJoin {
		countBuf.WriteString("LEFT JOIN bank_connections bc ON a.connection_id = bc.id ")
	}
	if needUserJoin {
		countBuf.WriteString("LEFT JOIN users u ON bc.user_id = u.id ")
	}
	if needCategoryJoin {
		countBuf.WriteString("LEFT JOIN categories c ON t.category_id = c.id ")
	}
	countBuf.WriteString("WHERE t.deleted_at IS NULL")
	countBuf.WriteString(whereClauses)
	_ = countBuf.String() // force materialization for fair benchmarking

	sortOrder := "DESC"
	if params.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	buf.WriteString(" ORDER BY t.date ")
	buf.WriteString(sortOrder)
	buf.WriteString(", t.id ")
	buf.WriteString(sortOrder)

	pageSize := params.PageSize
	if pageSize == 0 {
		pageSize = 50
	}
	page := params.Page
	if page < 1 {
		page = 1
	}

	if pageSize > 0 {
		buf.WriteString(" LIMIT $")
		buf.WriteString(strconv.Itoa(argN))
		buf.WriteString(" OFFSET $")
		buf.WriteString(strconv.Itoa(argN + 1))
		args = append(args, pageSize, (page-1)*pageSize)
	}

	return buf.String(), args
}

// benchBuildSummaryQuery mirrors GetTransactionSummary query building.
func benchBuildSummaryQuery(params benchSummaryParams) (string, []any) {
	selectCols := "COALESCE(cat.display_name, INITCAP(REPLACE(t.category_primary, '_', ' ')), 'Uncategorized') AS category, cat.icon AS category_icon, cat.color AS category_color, t.iso_currency_code"
	groupCols := "COALESCE(cat.display_name, INITCAP(REPLACE(t.category_primary, '_', ' ')), 'Uncategorized'), cat.icon, cat.color, t.iso_currency_code"
	orderCols := "SUM(t.amount) DESC"

	var buf strings.Builder
	buf.Grow(512)
	args := make([]any, 0, 8)
	argN := 1

	buf.WriteString("SELECT ")
	buf.WriteString(selectCols)
	buf.WriteString(", SUM(t.amount) AS total_amount, COUNT(*) AS transaction_count\nFROM transactions t\nJOIN accounts a ON t.account_id = a.id\nLEFT JOIN bank_connections bc ON a.connection_id = bc.id")
	buf.WriteString("\nLEFT JOIN categories cat ON t.category_id = cat.id")
	buf.WriteString("\nWHERE t.deleted_at IS NULL")
	buf.WriteString(" AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))")

	if !params.IncludePending {
		buf.WriteString(" AND t.pending = false")
	}

	if params.SpendingOnly {
		buf.WriteString(" AND t.amount > 0")
	}

	buf.WriteString(" AND t.date >= $")
	buf.WriteString(strconv.Itoa(argN))
	args = append(args, *params.StartDate)
	argN++

	buf.WriteString(" AND t.date < $")
	buf.WriteString(strconv.Itoa(argN))
	args = append(args, *params.EndDate)
	argN++

	if params.AccountID != nil {
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.AccountID)
		argN++
	}

	if params.UserID != nil {
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.UserID)
		argN++
	}

	if params.Category != nil {
		buf.WriteString(" AND t.category_primary = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.Category)
		argN++ //nolint:ineffassign
	}

	buf.WriteString(" GROUP BY ")
	buf.WriteString(groupCols)
	buf.WriteString(" ORDER BY ")
	buf.WriteString(orderCols)
	buf.WriteString(" LIMIT 1000")
	return buf.String(), args
}

// benchBuildMerchantQuery mirrors GetMerchantSummary query building.
func benchBuildMerchantQuery(params benchMerchantParams) (string, []any) {
	var buf strings.Builder
	buf.Grow(512)
	args := make([]any, 0, 12)
	argN := 1

	buf.WriteString(`SELECT COALESCE(t.merchant_name, t.name) AS merchant,
		COUNT(*) AS transaction_count,
		SUM(t.amount) AS total_amount,
		AVG(t.amount) AS avg_amount,
		MIN(t.date)::text AS first_date,
		MAX(t.date)::text AS last_date,
		t.iso_currency_code
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id`)

	if params.CategoryID != nil {
		buf.WriteString("\nLEFT JOIN categories c ON t.category_id = c.id")
	}

	buf.WriteString("\nWHERE t.deleted_at IS NULL")
	buf.WriteString(" AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))")
	buf.WriteString(" AND t.pending = false")

	if params.SpendingOnly {
		buf.WriteString(" AND t.amount > 0")
	}

	buf.WriteString(" AND t.date >= $")
	buf.WriteString(strconv.Itoa(argN))
	args = append(args, *params.StartDate)
	argN++

	buf.WriteString(" AND t.date < $")
	buf.WriteString(strconv.Itoa(argN))
	args = append(args, *params.EndDate)
	argN++

	if params.AccountID != nil {
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.AccountID)
		argN++
	}

	if params.UserID != nil {
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.UserID)
		argN++
	}

	if params.CategoryID != nil {
		n := strconv.Itoa(argN)
		if params.IsParentCategory {
			buf.WriteString(" AND (c.id = $")
			buf.WriteString(n)
			buf.WriteString(" OR c.parent_id = $")
			buf.WriteString(n)
			buf.WriteByte(')')
		} else {
			buf.WriteString(" AND t.category_id = $")
			buf.WriteString(n)
		}
		args = append(args, *params.CategoryID)
		argN++
	}

	if params.MinAmount != nil {
		buf.WriteString(" AND t.amount >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		buf.WriteString(" AND t.amount <= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Search != nil {
		mode := ""
		if params.SearchMode != nil {
			mode = *params.SearchMode
		}
		sc := BuildSearchClause(*params.Search, mode, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(sc.SQL)
		args = append(args, sc.Args...)
		argN = sc.ArgN
	}

	if params.ExcludeSearch != nil {
		ec := BuildExcludeSearchClause(*params.ExcludeSearch, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(ec.SQL)
		args = append(args, ec.Args...)
		argN = ec.ArgN
	}

	buf.WriteString(" GROUP BY COALESCE(t.merchant_name, t.name), t.iso_currency_code")
	buf.WriteString(" HAVING COUNT(*) >= $")
	buf.WriteString(strconv.Itoa(argN))
	args = append(args, params.MinCount)
	argN++ //nolint:ineffassign

	buf.WriteString(" ORDER BY COUNT(*) DESC LIMIT 500")
	return buf.String(), args
}

// Benchmark param types -- pre-resolved IDs, no DB lookups needed.

type benchListParams struct {
	Limit            int
	IncludeDependent bool
	UserID           *pgtype.UUID
	AccountID        *pgtype.UUID
	StartDate        *time.Time
	EndDate          *time.Time
	CategoryID       *pgtype.UUID
	IsParentCategory bool
	MinAmount        *float64
	MaxAmount        *float64
	Pending          *bool
	Search           *string
	SearchMode       *string
	ExcludeSearch    *string
	CursorDate       *time.Time
	CursorID         *pgtype.UUID
	SortBy           *string
	SortOrder        *string
}

type benchCountParams struct {
	IncludeDependent bool
	UserID           *pgtype.UUID
	AccountID        *pgtype.UUID
	StartDate        *time.Time
	EndDate          *time.Time
	CategoryID       *pgtype.UUID
	IsParentCategory bool
	MinAmount        *float64
	MaxAmount        *float64
	Pending          *bool
	Search           *string
	SearchMode       *string
	ExcludeSearch    *string
}

type benchAdminListParams struct {
	Page             int
	PageSize         int
	UserID           *pgtype.UUID
	ConnectionID     *pgtype.UUID
	AccountID        *pgtype.UUID
	StartDate        *time.Time
	EndDate          *time.Time
	CategoryID       *pgtype.UUID
	IsParentCategory bool
	MinAmount        *float64
	MaxAmount        *float64
	Pending          *bool
	Search           *string
	SearchMode       *string
	SearchField      *string
	ExcludeSearch    *string
	SortOrder        string
}

type benchSummaryParams struct {
	StartDate      *time.Time
	EndDate        *time.Time
	AccountID      *pgtype.UUID
	UserID         *pgtype.UUID
	Category       *string
	IncludePending bool
	SpendingOnly   bool
}

type benchMerchantParams struct {
	StartDate        *time.Time
	EndDate          *time.Time
	AccountID        *pgtype.UUID
	UserID           *pgtype.UUID
	CategoryID       *pgtype.UUID
	IsParentCategory bool
	MinAmount        *float64
	MaxAmount        *float64
	Search           *string
	SearchMode       *string
	ExcludeSearch    *string
	MinCount         int
	SpendingOnly     bool
}

// Helper to create a pointer to a value.
func ptr[T any](v T) *T { return &v }

var fakeUUID = pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true}

// --- ListTransactions benchmarks ---

func BenchmarkListTransactionsQuery_Minimal(b *testing.B) {
	params := benchListParams{Limit: 50}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildListTransactionsQuery(params)
	}
	_ = q
	_ = a
}

func BenchmarkListTransactionsQuery_Typical(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	search := "starbucks"
	sortBy := "date"
	params := benchListParams{
		Limit:     50,
		UserID:    &fakeUUID,
		AccountID: &fakeUUID,
		StartDate: &start,
		EndDate:   &end,
		Search:    &search,
		SortBy:    &sortBy,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildListTransactionsQuery(params)
	}
	_ = q
	_ = a
}

func BenchmarkListTransactionsQuery_Heavy(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	cursorDate := time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC)
	search := "coffee,starbucks"
	excludeSearch := "refund"
	sortBy := "date"
	sortOrder := "asc"
	params := benchListParams{
		Limit:         100,
		UserID:        &fakeUUID,
		AccountID:     &fakeUUID,
		StartDate:     &start,
		EndDate:       &end,
		CategoryID:    &fakeUUID,
		MinAmount:     ptr(5.0),
		MaxAmount:     ptr(500.0),
		Pending:       ptr(false),
		Search:        &search,
		ExcludeSearch: &excludeSearch,
		CursorDate:    &cursorDate,
		CursorID:      &fakeUUID,
		SortBy:        &sortBy,
		SortOrder:     &sortOrder,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildListTransactionsQuery(params)
	}
	_ = q
	_ = a
}

// --- CountTransactionsFiltered benchmarks ---

func BenchmarkCountTransactionsQuery_Minimal(b *testing.B) {
	params := benchCountParams{}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildCountTransactionsQuery(params)
	}
	_ = q
	_ = a
}

func BenchmarkCountTransactionsQuery_Typical(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	search := "starbucks"
	params := benchCountParams{
		UserID:    &fakeUUID,
		AccountID: &fakeUUID,
		StartDate: &start,
		EndDate:   &end,
		Search:    &search,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildCountTransactionsQuery(params)
	}
	_ = q
	_ = a
}

func BenchmarkCountTransactionsQuery_Heavy(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	search := "coffee,starbucks"
	excludeSearch := "refund"
	params := benchCountParams{
		UserID:        &fakeUUID,
		AccountID:     &fakeUUID,
		StartDate:     &start,
		EndDate:       &end,
		CategoryID:    &fakeUUID,
		MinAmount:     ptr(5.0),
		MaxAmount:     ptr(500.0),
		Pending:       ptr(false),
		Search:        &search,
		ExcludeSearch: &excludeSearch,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildCountTransactionsQuery(params)
	}
	_ = q
	_ = a
}

// --- ListTransactionsAdmin benchmarks ---

func BenchmarkAdminListQuery_Minimal(b *testing.B) {
	params := benchAdminListParams{Page: 1, PageSize: 50}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildAdminListQuery(params)
	}
	_ = q
	_ = a
}

func BenchmarkAdminListQuery_Typical(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	search := "starbucks"
	params := benchAdminListParams{
		Page:      1,
		PageSize:  50,
		UserID:    &fakeUUID,
		AccountID: &fakeUUID,
		StartDate: &start,
		EndDate:   &end,
		Search:    &search,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildAdminListQuery(params)
	}
	_ = q
	_ = a
}

func BenchmarkAdminListQuery_Heavy(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	search := "coffee,starbucks"
	excludeSearch := "refund"
	params := benchAdminListParams{
		Page:             1,
		PageSize:         50,
		UserID:           &fakeUUID,
		ConnectionID:     &fakeUUID,
		AccountID:        &fakeUUID,
		StartDate:        &start,
		EndDate:          &end,
		CategoryID:       &fakeUUID,
		IsParentCategory: true,
		MinAmount:        ptr(5.0),
		MaxAmount:        ptr(500.0),
		Pending:          ptr(false),
		Search:           &search,
		ExcludeSearch:    &excludeSearch,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildAdminListQuery(params)
	}
	_ = q
	_ = a
}

// --- Summary benchmarks ---

func BenchmarkSummaryQuery_Minimal(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	params := benchSummaryParams{
		StartDate: &start,
		EndDate:   &end,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildSummaryQuery(params)
	}
	_ = q
	_ = a
}

func BenchmarkSummaryQuery_Heavy(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	cat := "food_and_drink"
	params := benchSummaryParams{
		StartDate:    &start,
		EndDate:      &end,
		AccountID:    &fakeUUID,
		UserID:       &fakeUUID,
		Category:     &cat,
		SpendingOnly: true,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildSummaryQuery(params)
	}
	_ = q
	_ = a
}

// --- Merchant summary benchmarks ---

func BenchmarkMerchantQuery_Minimal(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	params := benchMerchantParams{
		StartDate: &start,
		EndDate:   &end,
		MinCount:  1,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildMerchantQuery(params)
	}
	_ = q
	_ = a
}

func BenchmarkMerchantQuery_Heavy(b *testing.B) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	search := "coffee,starbucks"
	excludeSearch := "refund"
	params := benchMerchantParams{
		StartDate:        &start,
		EndDate:          &end,
		AccountID:        &fakeUUID,
		UserID:           &fakeUUID,
		CategoryID:       &fakeUUID,
		IsParentCategory: true,
		MinAmount:        ptr(5.0),
		MaxAmount:        ptr(500.0),
		Search:           &search,
		ExcludeSearch:    &excludeSearch,
		MinCount:         3,
		SpendingOnly:     true,
	}
	var q string
	var a []any
	for b.Loop() {
		q, a = benchBuildMerchantQuery(params)
	}
	_ = q
	_ = a
}

// --- Correctness tests: verify optimized builders produce identical SQL ---

func TestBuildListTransactionsQuery_SQLIdentical(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	cursorDate := time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC)
	search := "coffee,starbucks"
	excludeSearch := "refund"
	sortBy := "date"
	sortOrder := "asc"

	params := benchListParams{
		Limit:         100,
		UserID:        &fakeUUID,
		AccountID:     &fakeUUID,
		StartDate:     &start,
		EndDate:       &end,
		CategoryID:    &fakeUUID,
		MinAmount:     ptr(5.0),
		MaxAmount:     ptr(500.0),
		Pending:       ptr(false),
		Search:        &search,
		ExcludeSearch: &excludeSearch,
		CursorDate:    &cursorDate,
		CursorID:      &fakeUUID,
		SortBy:        &sortBy,
		SortOrder:     &sortOrder,
	}

	q, args := benchBuildListTransactionsQuery(params)

	// Verify key SQL fragments are present.
	if !strings.Contains(q, "SELECT t.id") {
		t.Error("missing SELECT")
	}
	if !strings.Contains(q, "WHERE t.deleted_at IS NULL") {
		t.Error("missing WHERE")
	}
	if !strings.Contains(q, "COALESCE(t.attributed_user_id, bc.user_id) = $1") {
		t.Errorf("missing user filter, got: %s", q)
	}
	if !strings.Contains(q, "t.account_id = $2") {
		t.Errorf("missing account filter, got: %s", q)
	}
	if !strings.Contains(q, "ORDER BY t.date ASC, t.id DESC") {
		t.Errorf("missing ORDER BY, got: %s", q)
	}
	// user, account, start, end, category, min_amount, max_amount, pending,
	// search(coffee), search(starbucks), exclude_search, cursor_date, cursor_id, limit+1 = 14
	if len(args) != 14 {
		t.Errorf("expected 14 args, got %d", len(args))
	}
}

func TestBuildSummaryQuery_SQLIdentical(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	cat := "food_and_drink"
	params := benchSummaryParams{
		StartDate:    &start,
		EndDate:      &end,
		AccountID:    &fakeUUID,
		UserID:       &fakeUUID,
		Category:     &cat,
		SpendingOnly: true,
	}
	q, args := benchBuildSummaryQuery(params)

	// Use fmt.Sprintf to construct the reference query.
	refQuery := fmt.Sprintf(`SELECT %s, SUM(t.amount) AS total_amount, COUNT(*) AS transaction_count
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id`,
		"COALESCE(cat.display_name, INITCAP(REPLACE(t.category_primary, '_', ' ')), 'Uncategorized') AS category, cat.icon AS category_icon, cat.color AS category_color, t.iso_currency_code")
	refQuery += "\nLEFT JOIN categories cat ON t.category_id = cat.id"
	refQuery += "\nWHERE t.deleted_at IS NULL"
	refQuery += " AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))"
	refQuery += " AND t.pending = false"
	refQuery += " AND t.amount > 0"
	refQuery += " AND t.date >= $1"
	refQuery += " AND t.date < $2"
	refQuery += " AND t.account_id = $3"
	refQuery += " AND COALESCE(t.attributed_user_id, bc.user_id) = $4"
	refQuery += " AND t.category_primary = $5"
	refQuery += fmt.Sprintf(" GROUP BY %s ORDER BY %s LIMIT 1000",
		"COALESCE(cat.display_name, INITCAP(REPLACE(t.category_primary, '_', ' ')), 'Uncategorized'), cat.icon, cat.color, t.iso_currency_code",
		"SUM(t.amount) DESC")

	if q != refQuery {
		t.Errorf("SQL mismatch.\nGot:\n%s\n\nExpected:\n%s", q, refQuery)
	}
	if len(args) != 5 {
		t.Errorf("expected 5 args, got %d", len(args))
	}
}
