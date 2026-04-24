package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// OverviewStats contains high-level statistics about the Breadbox dataset.
type OverviewStats struct {
	UserCount               int                    `json:"user_count"`
	ConnectionCount         int                    `json:"connection_count"`
	AccountCount            int                    `json:"account_count"`
	TransactionCount        int64                  `json:"transaction_count"`
	PendingTransactionCount int64                  `json:"pending_transaction_count"`
	CategoryCount           int                    `json:"category_count"`
	UnmappedCount           int                    `json:"unmapped_transaction_count"`
	EarliestDate            string                 `json:"earliest_transaction_date,omitempty"`
	LatestDate              string                 `json:"latest_transaction_date,omitempty"`
	Users                   []OverviewUser         `json:"users"`
	AccountsByType          map[string]int         `json:"accounts_by_type"`
	Connections             []OverviewConnection   `json:"connections"`
	SpendingSummary30d      *OverviewSpending      `json:"spending_summary_30d,omitempty"`
	Permissions             *OverviewPermissions   `json:"permissions,omitempty"`
}

// OverviewPermissions describes what the current API key/session can do.
type OverviewPermissions struct {
	Role                    string `json:"role"`
	CanReadAllTransactions  bool   `json:"can_read_all_transactions"`
	CanEditTransactions     bool   `json:"can_edit_transactions"`
	CanManageConnections    bool   `json:"can_manage_connections"`
	CanManageSettings       bool   `json:"can_manage_settings"`
}

// OverviewUser is a minimal user representation for the overview.
type OverviewUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OverviewConnection is a connection summary for the overview.
type OverviewConnection struct {
	ID              string  `json:"id"`
	Provider        string  `json:"provider"`
	InstitutionName *string `json:"institution_name"`
	Status          string  `json:"status"`
	LastSyncedAt    *string `json:"last_synced_at"`
	AccountCount    int     `json:"account_count"`
}

// OverviewSpending is a 30-day spending summary for the overview.
type OverviewSpending struct {
	TotalAmount      float64                `json:"total_amount"`
	TransactionCount int64                  `json:"transaction_count"`
	IsoCurrencyCode  string                 `json:"iso_currency_code"`
	TopCategories    []OverviewCategorySpend `json:"top_categories"`
}

// OverviewCategorySpend is a category spending row for the overview.
type OverviewCategorySpend struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
	Count    int64   `json:"count"`
}

// GetOverviewStats returns aggregate counts and the transaction date range.
func (s *Service) GetOverviewStats(ctx context.Context) (*OverviewStats, error) {
	stats := &OverviewStats{}

	err := s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&stats.UserCount)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	err = s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM bank_connections WHERE status != 'disconnected'").Scan(&stats.ConnectionCount)
	if err != nil {
		return nil, fmt.Errorf("count connections: %w", err)
	}

	err = s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM accounts").Scan(&stats.AccountCount)
	if err != nil {
		return nil, fmt.Errorf("count accounts: %w", err)
	}

	err = s.Pool.QueryRow(ctx,
		"SELECT COUNT(*), COALESCE(MIN(date)::text, ''), COALESCE(MAX(date)::text, '') FROM transactions t JOIN accounts a ON t.account_id = a.id WHERE t.deleted_at IS NULL AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))").
		Scan(&stats.TransactionCount, &stats.EarliestDate, &stats.LatestDate)
	if err != nil {
		return nil, fmt.Errorf("count transactions: %w", err)
	}

	err = s.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions t JOIN accounts a ON t.account_id = a.id WHERE t.deleted_at IS NULL AND t.pending = true AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))").
		Scan(&stats.PendingTransactionCount)
	if err != nil {
		return nil, fmt.Errorf("count pending: %w", err)
	}

	err = s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM categories").Scan(&stats.CategoryCount)
	if err != nil {
		return nil, fmt.Errorf("count categories: %w", err)
	}

	err = s.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions t JOIN accounts a ON t.account_id = a.id WHERE t.category_id IS NULL AND t.deleted_at IS NULL AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))").
		Scan(&stats.UnmappedCount)
	if err != nil {
		return nil, fmt.Errorf("count unmapped: %w", err)
	}

	// Users list
	userRows, err := s.Pool.Query(ctx, "SELECT id, name FROM users ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer userRows.Close()
	stats.Users = []OverviewUser{}
	for userRows.Next() {
		var u OverviewUser
		var id pgtype.UUID
		if err := userRows.Scan(&id, &u.Name); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.ID = formatUUID(id)
		stats.Users = append(stats.Users, u)
	}
	if err := userRows.Err(); err != nil {
		return nil, fmt.Errorf("user rows: %w", err)
	}

	// Accounts by type
	stats.AccountsByType = map[string]int{}
	typeRows, err := s.Pool.Query(ctx, "SELECT type, COUNT(*) FROM accounts GROUP BY type ORDER BY COUNT(*) DESC")
	if err != nil {
		return nil, fmt.Errorf("accounts by type: %w", err)
	}
	defer typeRows.Close()
	for typeRows.Next() {
		var t string
		var c int
		if err := typeRows.Scan(&t, &c); err != nil {
			return nil, fmt.Errorf("scan account type: %w", err)
		}
		stats.AccountsByType[t] = c
	}
	if err := typeRows.Err(); err != nil {
		return nil, fmt.Errorf("account type rows: %w", err)
	}

	// Connections with account counts
	stats.Connections = []OverviewConnection{}
	connRows, err := s.Pool.Query(ctx, `
		SELECT bc.id, bc.provider, bc.institution_name, bc.status, bc.last_synced_at,
			(SELECT COUNT(*) FROM accounts WHERE connection_id = bc.id)
		FROM bank_connections bc
		WHERE bc.status != 'disconnected'
		ORDER BY bc.institution_name`)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	defer connRows.Close()
	for connRows.Next() {
		var c OverviewConnection
		var id pgtype.UUID
		var instName *string
		var lastSynced *time.Time
		if err := connRows.Scan(&id, &c.Provider, &instName, &c.Status, &lastSynced, &c.AccountCount); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		c.ID = formatUUID(id)
		c.InstitutionName = instName
		if lastSynced != nil {
			s := lastSynced.UTC().Format(time.RFC3339)
			c.LastSyncedAt = &s
		}
		stats.Connections = append(stats.Connections, c)
	}
	if err := connRows.Err(); err != nil {
		return nil, fmt.Errorf("connection rows: %w", err)
	}

	// 30-day spending summary
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	var spendTotal float64
	var spendCount int64
	var spendCurrency *string
	var currencyCount int
	err = s.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(t.amount), 0), COUNT(*), COUNT(DISTINCT t.iso_currency_code)
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		WHERE t.deleted_at IS NULL AND t.pending = false AND t.date >= $1 AND t.amount > 0 AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))`,
		thirtyDaysAgo).Scan(&spendTotal, &spendCount, &currencyCount)
	if err != nil {
		return nil, fmt.Errorf("spending summary: %w", err)
	}

	if spendCount > 0 && currencyCount == 1 {
		err = s.Pool.QueryRow(ctx, `
			SELECT t.iso_currency_code FROM transactions t
			JOIN accounts a ON t.account_id = a.id
			WHERE t.deleted_at IS NULL AND t.pending = false AND t.date >= $1 AND t.amount > 0 AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id)) LIMIT 1`,
			thirtyDaysAgo).Scan(&spendCurrency)
		if err != nil {
			return nil, fmt.Errorf("spending currency: %w", err)
		}

		spending := &OverviewSpending{
			TotalAmount:      spendTotal,
			TransactionCount: spendCount,
			IsoCurrencyCode:  *spendCurrency,
			TopCategories:    []OverviewCategorySpend{},
		}

		catRows, err := s.Pool.Query(ctx, `
			SELECT COALESCE(t.provider_category_primary, 'UNCATEGORIZED'), SUM(t.amount), COUNT(*)
			FROM transactions t
			JOIN accounts a ON t.account_id = a.id
			WHERE t.deleted_at IS NULL AND t.pending = false AND t.date >= $1 AND t.amount > 0 AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))
			GROUP BY t.provider_category_primary
			ORDER BY SUM(t.amount) DESC
			LIMIT 5`, thirtyDaysAgo)
		if err != nil {
			return nil, fmt.Errorf("top categories: %w", err)
		}
		defer catRows.Close()
		for catRows.Next() {
			var cs OverviewCategorySpend
			if err := catRows.Scan(&cs.Category, &cs.Amount, &cs.Count); err != nil {
				return nil, fmt.Errorf("scan category spend: %w", err)
			}
			spending.TopCategories = append(spending.TopCategories, cs)
		}
		if err := catRows.Err(); err != nil {
			return nil, fmt.Errorf("category spend rows: %w", err)
		}

		stats.SpendingSummary30d = spending
	}

	return stats, nil
}
