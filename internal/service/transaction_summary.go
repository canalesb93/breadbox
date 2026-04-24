package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TransactionSummaryParams holds parameters for aggregated transaction queries.
type TransactionSummaryParams struct {
	StartDate      *time.Time
	EndDate        *time.Time
	GroupBy        string // "category", "month", "week", "day", "category_month"
	AccountID      *string
	UserID         *string
	Category       *string
	IncludePending bool
	SpendingOnly   bool // Only include positive amounts (debits/spending)
}

// TransactionSummaryResult is the response for a transaction summary query.
type TransactionSummaryResult struct {
	Summary []TransactionSummaryRow    `json:"summary"`
	Totals  TransactionSummaryTotals   `json:"totals"`
	Filters TransactionSummaryFilters  `json:"filters"`
}

// TransactionSummaryRow is a single aggregated row.
type TransactionSummaryRow struct {
	Category         *string `json:"category,omitempty"`
	CategoryIcon     *string `json:"category_icon,omitempty"`
	CategoryColor    *string `json:"category_color,omitempty"`
	Period           *string `json:"period,omitempty"`
	TotalAmount      float64 `json:"total_amount"`
	TransactionCount int64   `json:"transaction_count"`
	IsoCurrencyCode  string  `json:"iso_currency_code"`
}

// TransactionSummaryTotals holds grand totals across all rows.
type TransactionSummaryTotals struct {
	TotalAmount      *float64 `json:"total_amount,omitempty"`
	TransactionCount int64    `json:"transaction_count"`
	Note             string   `json:"note,omitempty"`
}

// TransactionSummaryFilters echoes the applied filters back.
type TransactionSummaryFilters struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	GroupBy   string `json:"group_by"`
}

var validGroupBy = map[string]bool{
	"category":       true,
	"month":          true,
	"week":           true,
	"day":            true,
	"category_month": true,
}

// GetTransactionSummary returns aggregated transaction totals.
func (s *Service) GetTransactionSummary(ctx context.Context, params TransactionSummaryParams) (*TransactionSummaryResult, error) {
	if !validGroupBy[params.GroupBy] {
		return nil, fmt.Errorf("%w: invalid group_by: %s. Must be one of: category, month, week, day, category_month", ErrInvalidParameter, params.GroupBy)
	}

	// Default date range: 30 days ago to tomorrow (exclusive end).
	now := time.Now()
	if params.StartDate == nil {
		t := now.AddDate(0, 0, -30)
		params.StartDate = &t
	}
	if params.EndDate == nil {
		t := now.AddDate(0, 0, 1)
		params.EndDate = &t
	}

	// Build SELECT clause based on group_by.
	var selectCols, groupCols, orderCols string
	joinCategories := false
	switch params.GroupBy {
	case "category":
		selectCols = "COALESCE(cat.display_name, INITCAP(REPLACE(t.provider_category_primary, '_', ' ')), 'Uncategorized') AS category, cat.icon AS category_icon, cat.color AS category_color, t.iso_currency_code"
		groupCols = "COALESCE(cat.display_name, INITCAP(REPLACE(t.provider_category_primary, '_', ' ')), 'Uncategorized'), cat.icon, cat.color, t.iso_currency_code"
		orderCols = "SUM(t.amount) DESC"
		joinCategories = true
	case "month":
		selectCols = "to_char(date_trunc('month', t.date), 'YYYY-MM') AS period, t.iso_currency_code"
		groupCols = "date_trunc('month', t.date), t.iso_currency_code"
		orderCols = "date_trunc('month', t.date) DESC"
	case "week":
		selectCols = "to_char(date_trunc('week', t.date), 'YYYY-MM-DD') AS period, t.iso_currency_code"
		groupCols = "date_trunc('week', t.date), t.iso_currency_code"
		orderCols = "date_trunc('week', t.date) DESC"
	case "day":
		selectCols = "t.date::text AS period, t.iso_currency_code"
		groupCols = "t.date, t.iso_currency_code"
		orderCols = "t.date DESC"
	case "category_month":
		selectCols = "COALESCE(cat.display_name, INITCAP(REPLACE(t.provider_category_primary, '_', ' ')), 'Uncategorized') AS category, cat.icon AS category_icon, cat.color AS category_color, to_char(date_trunc('month', t.date), 'YYYY-MM') AS period, t.iso_currency_code"
		groupCols = "COALESCE(cat.display_name, INITCAP(REPLACE(t.provider_category_primary, '_', ' ')), 'Uncategorized'), cat.icon, cat.color, date_trunc('month', t.date), t.iso_currency_code"
		orderCols = "date_trunc('month', t.date) DESC, SUM(t.amount) DESC"
		joinCategories = true
	}

	var buf strings.Builder
	buf.Grow(512)
	args := make([]any, 0, 8)
	argN := 1

	buf.WriteString("SELECT ")
	buf.WriteString(selectCols)
	buf.WriteString(", SUM(t.amount) AS total_amount, COUNT(*) AS transaction_count\nFROM transactions t\nJOIN accounts a ON t.account_id = a.id\nLEFT JOIN bank_connections bc ON a.connection_id = bc.id")
	if joinCategories {
		buf.WriteString("\nLEFT JOIN categories cat ON t.category_id = cat.id")
	}
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
		aid, err := s.resolveAccountID(ctx, *params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid account_id", ErrInvalidParameter)
		}
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, aid)
		argN++
	}

	if params.UserID != nil {
		uid, err := s.resolveUserID(ctx, *params.UserID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid user_id", ErrInvalidParameter)
		}
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, uid)
		argN++
	}

	if params.Category != nil {
		buf.WriteString(" AND t.provider_category_primary = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.Category)
		argN++ //nolint:ineffassign // kept for consistency with other filters
	}

	buf.WriteString(" GROUP BY ")
	buf.WriteString(groupCols)
	buf.WriteString(" ORDER BY ")
	buf.WriteString(orderCols)
	buf.WriteString(" LIMIT 1000")

	query := buf.String()

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("transaction summary query: %w", err)
	}
	defer rows.Close()

	var summary []TransactionSummaryRow
	currencies := map[string]bool{}
	var grandTotal float64
	var grandCount int64

	for rows.Next() {
		var row TransactionSummaryRow
		switch params.GroupBy {
		case "category":
			err = rows.Scan(&row.Category, &row.CategoryIcon, &row.CategoryColor, &row.IsoCurrencyCode, &row.TotalAmount, &row.TransactionCount)
		case "month", "week", "day":
			err = rows.Scan(&row.Period, &row.IsoCurrencyCode, &row.TotalAmount, &row.TransactionCount)
		case "category_month":
			err = rows.Scan(&row.Category, &row.CategoryIcon, &row.CategoryColor, &row.Period, &row.IsoCurrencyCode, &row.TotalAmount, &row.TransactionCount)
		}
		if err != nil {
			return nil, fmt.Errorf("scan summary row: %w", err)
		}
		summary = append(summary, row)
		currencies[row.IsoCurrencyCode] = true
		grandTotal += row.TotalAmount
		grandCount += row.TransactionCount
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("summary rows: %w", err)
	}

	totals := TransactionSummaryTotals{
		TransactionCount: grandCount,
	}
	if len(currencies) <= 1 {
		totals.TotalAmount = &grandTotal
	} else {
		totals.Note = "Multiple currencies — see per-row totals."
	}

	result := &TransactionSummaryResult{
		Summary: summary,
		Totals:  totals,
		Filters: TransactionSummaryFilters{
			StartDate: params.StartDate.Format("2006-01-02"),
			EndDate:   params.EndDate.Format("2006-01-02"),
			GroupBy:   params.GroupBy,
		},
	}

	if result.Summary == nil {
		result.Summary = []TransactionSummaryRow{}
	}

	return result, nil
}

// GetMerchantSummary returns aggregated merchant-level statistics.
func (s *Service) GetMerchantSummary(ctx context.Context, params MerchantSummaryParams) (*MerchantSummaryResult, error) {
	// Default date range: 90 days ago to tomorrow (exclusive end).
	now := time.Now()
	if params.StartDate == nil {
		t := now.AddDate(0, 0, -90)
		params.StartDate = &t
	}
	if params.EndDate == nil {
		t := now.AddDate(0, 0, 1)
		params.EndDate = &t
	}

	if params.MinCount < 1 {
		params.MinCount = 1
	}

	var buf strings.Builder
	buf.Grow(512)
	args := make([]any, 0, 12)
	argN := 1

	buf.WriteString(`SELECT COALESCE(t.provider_merchant_name, t.provider_name) AS merchant,
		COUNT(*) AS transaction_count,
		SUM(t.amount) AS total_amount,
		AVG(t.amount) AS avg_amount,
		MIN(t.date)::text AS first_date,
		MAX(t.date)::text AS last_date,
		t.iso_currency_code
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id`)

	joinCategories := false
	if params.CategorySlug != nil {
		joinCategories = true
	}
	if joinCategories {
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
		aid, err := s.resolveAccountID(ctx, *params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid account_id", ErrInvalidParameter)
		}
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, aid)
		argN++
	}

	if params.UserID != nil {
		uid, err := s.resolveUserID(ctx, *params.UserID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid user_id", ErrInvalidParameter)
		}
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, uid)
		argN++
	}

	if params.CategorySlug != nil {
		catRow, err := s.Queries.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			// Unknown slug — no results
			return &MerchantSummaryResult{Merchants: []MerchantSummaryRow{}, Totals: MerchantSummaryTotals{}, Filters: MerchantSummaryFilters{MinCount: params.MinCount}}, nil
		}
		n := strconv.Itoa(argN)
		if !catRow.ParentID.Valid {
			buf.WriteString(" AND (c.id = $")
			buf.WriteString(n)
			buf.WriteString(" OR c.parent_id = $")
			buf.WriteString(n)
			buf.WriteByte(')')
			args = append(args, catRow.ID)
			argN++
		} else {
			buf.WriteString(" AND t.category_id = $")
			buf.WriteString(n)
			args = append(args, catRow.ID)
			argN++
		}
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

	buf.WriteString(" GROUP BY COALESCE(t.provider_merchant_name, t.provider_name), t.iso_currency_code")
	buf.WriteString(" HAVING COUNT(*) >= $")
	buf.WriteString(strconv.Itoa(argN))
	args = append(args, params.MinCount)
	argN++ //nolint:ineffassign // kept for consistency with other filters

	buf.WriteString(" ORDER BY COUNT(*) DESC LIMIT 500")

	query := buf.String()

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("merchant summary query: %w", err)
	}
	defer rows.Close()

	var merchants []MerchantSummaryRow
	currencies := map[string]bool{}
	var grandTotal float64
	var grandCount int64

	for rows.Next() {
		var row MerchantSummaryRow
		err = rows.Scan(&row.Merchant, &row.TransactionCount, &row.TotalAmount, &row.AvgAmount, &row.FirstDate, &row.LastDate, &row.IsoCurrencyCode)
		if err != nil {
			return nil, fmt.Errorf("scan merchant summary row: %w", err)
		}
		merchants = append(merchants, row)
		currencies[row.IsoCurrencyCode] = true
		grandTotal += row.TotalAmount
		grandCount += row.TransactionCount
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("merchant summary rows: %w", err)
	}

	totals := MerchantSummaryTotals{
		MerchantCount:    int64(len(merchants)),
		TransactionCount: grandCount,
	}
	if len(currencies) <= 1 {
		totals.TotalAmount = &grandTotal
	} else {
		totals.Note = "Multiple currencies — see per-row totals."
	}

	result := &MerchantSummaryResult{
		Merchants: merchants,
		Totals:    totals,
		Filters: MerchantSummaryFilters{
			StartDate: params.StartDate.Format("2006-01-02"),
			EndDate:   params.EndDate.Format("2006-01-02"),
			MinCount:  params.MinCount,
		},
	}

	if result.Merchants == nil {
		result.Merchants = []MerchantSummaryRow{}
	}

	return result, nil
}
