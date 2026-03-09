package service

import (
	"context"
	"fmt"
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
		return nil, fmt.Errorf("invalid group_by: %s. Must be one of: category, month, week, day, category_month", params.GroupBy)
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
	switch params.GroupBy {
	case "category":
		selectCols = "t.category_primary AS category, t.iso_currency_code"
		groupCols = "t.category_primary, t.iso_currency_code"
		orderCols = "SUM(t.amount) DESC"
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
		selectCols = "t.category_primary AS category, to_char(date_trunc('month', t.date), 'YYYY-MM') AS period, t.iso_currency_code"
		groupCols = "t.category_primary, date_trunc('month', t.date), t.iso_currency_code"
		orderCols = "date_trunc('month', t.date) DESC, SUM(t.amount) DESC"
	}

	query := fmt.Sprintf(`SELECT %s, SUM(t.amount) AS total_amount, COUNT(*) AS transaction_count
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
WHERE t.deleted_at IS NULL`, selectCols)

	args := []any{}
	argN := 1

	if !params.IncludePending {
		query += " AND t.pending = false"
	}

	query += fmt.Sprintf(" AND t.date >= $%d", argN)
	args = append(args, *params.StartDate)
	argN++

	query += fmt.Sprintf(" AND t.date < $%d", argN)
	args = append(args, *params.EndDate)
	argN++

	if params.AccountID != nil {
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, *params.AccountID)
		argN++
	}

	if params.UserID != nil {
		query += fmt.Sprintf(" AND bc.user_id = $%d", argN)
		args = append(args, *params.UserID)
		argN++
	}

	if params.Category != nil {
		query += fmt.Sprintf(" AND t.category_primary = $%d", argN)
		args = append(args, *params.Category)
		argN++
	}

	query += fmt.Sprintf(" GROUP BY %s ORDER BY %s", groupCols, orderCols)

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
			err = rows.Scan(&row.Category, &row.IsoCurrencyCode, &row.TotalAmount, &row.TransactionCount)
		case "month", "week", "day":
			err = rows.Scan(&row.Period, &row.IsoCurrencyCode, &row.TotalAmount, &row.TransactionCount)
		case "category_month":
			err = rows.Scan(&row.Category, &row.Period, &row.IsoCurrencyCode, &row.TotalAmount, &row.TransactionCount)
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
