package service

import (
	"context"
	"fmt"
	"time"
)

// OverviewStats answers three questions about the dataset, in order: how big
// is it (Scope), is it current (Freshness), and what's open / needs attention
// (Backlog). The household roster is included as a small directory so the
// agent has the user list in hand without an extra resource read. Detail
// beyond this — per-account balances, per-connection sync timestamps, the
// category taxonomy — is available via the corresponding live resources.
type OverviewStats struct {
	Users     []OverviewUser    `json:"users"`
	Scope     OverviewScope     `json:"scope"`
	Freshness OverviewFreshness `json:"freshness"`
	Backlog   OverviewBacklog   `json:"backlog"`
}

// OverviewUser is the minimal household-roster row.
type OverviewUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OverviewScope: how big is the dataset.
type OverviewScope struct {
	TransactionCount          int64  `json:"transaction_count"`
	AccountCount              int    `json:"account_count"`
	CategoryCount             int    `json:"category_count"`
	EarliestTransactionDate   string `json:"earliest_transaction_date,omitempty"`
	LatestTransactionDate     string `json:"latest_transaction_date,omitempty"`
}

// OverviewFreshness: is the data current.
//
// `last_sync_at` is the most recent successful sync across all active
// connections. `transactions_added_last_*` count rows whose `created_at`
// falls in the window — i.e., when Breadbox first ingested them, not when
// the underlying transaction occurred. That distinction matters: a fresh
// connection can backfill years of history in one go, but the freshness of
// the *dataset* tracks ingest time.
type OverviewFreshness struct {
	LastSyncAt              *string `json:"last_sync_at,omitempty"`
	ErroredConnectionCount  int     `json:"errored_connection_count"`
	PendingTransactionCount int64   `json:"pending_transaction_count"`
	TransactionsAddedLast24h int64  `json:"transactions_added_last_24h"`
	TransactionsAddedLast7d  int64  `json:"transactions_added_last_7d"`
}

// OverviewBacklog: what's open / needs human attention.
type OverviewBacklog struct {
	NeedsReviewCount         int64 `json:"needs_review_count"`
	UnmappedTransactionCount int64 `json:"unmapped_transaction_count"`
}

// dependentExclusionWhere is the standard predicate that excludes
// dependent-linked transactions from counts so a shared credit card doesn't
// double up. Inlined into every count query so the numbers consistently
// reflect the user-facing definition of "transactions in this dataset".
const dependentExclusionWhere = `
	(a.is_dependent_linked = FALSE OR NOT EXISTS (
		SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id
	))
`

// GetOverviewStats returns the lightweight dataset overview served via
// `breadbox://overview`. Detail consumers (per-connection sync, per-account
// balances, full user/tag/category lists) read the dedicated resources.
func (s *Service) GetOverviewStats(ctx context.Context) (*OverviewStats, error) {
	stats := &OverviewStats{}

	// Scope.
	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(MIN(date)::text, ''), COALESCE(MAX(date)::text, '')
		   FROM transactions t
		   JOIN accounts a ON t.account_id = a.id
		  WHERE t.deleted_at IS NULL AND `+dependentExclusionWhere).
		Scan(&stats.Scope.TransactionCount, &stats.Scope.EarliestTransactionDate, &stats.Scope.LatestTransactionDate); err != nil {
		return nil, fmt.Errorf("count transactions: %w", err)
	}

	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&stats.Scope.AccountCount); err != nil {
		return nil, fmt.Errorf("count accounts: %w", err)
	}

	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM categories`).Scan(&stats.Scope.CategoryCount); err != nil {
		return nil, fmt.Errorf("count categories: %w", err)
	}

	// Household roster.
	stats.Users = []OverviewUser{}
	userRows, err := s.Pool.Query(ctx, `SELECT short_id, name FROM users ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer userRows.Close()
	for userRows.Next() {
		var u OverviewUser
		if err := userRows.Scan(&u.ID, &u.Name); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		stats.Users = append(stats.Users, u)
	}
	if err := userRows.Err(); err != nil {
		return nil, fmt.Errorf("user rows: %w", err)
	}

	// Freshness.
	var lastSync *time.Time
	if err := s.Pool.QueryRow(ctx,
		`SELECT MAX(last_synced_at) FROM bank_connections WHERE status != 'disconnected'`).
		Scan(&lastSync); err != nil {
		return nil, fmt.Errorf("max last_synced_at: %w", err)
	}
	if lastSync != nil {
		formatted := lastSync.UTC().Format(time.RFC3339)
		stats.Freshness.LastSyncAt = &formatted
	}

	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM bank_connections WHERE status IN ('error', 'pending_reauth')`).
		Scan(&stats.Freshness.ErroredConnectionCount); err != nil {
		return nil, fmt.Errorf("count errored connections: %w", err)
	}

	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions t
		   JOIN accounts a ON t.account_id = a.id
		  WHERE t.deleted_at IS NULL AND t.pending = true AND `+dependentExclusionWhere).
		Scan(&stats.Freshness.PendingTransactionCount); err != nil {
		return nil, fmt.Errorf("count pending: %w", err)
	}

	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions t
		   JOIN accounts a ON t.account_id = a.id
		  WHERE t.deleted_at IS NULL
		    AND t.created_at >= NOW() - INTERVAL '24 hours'
		    AND `+dependentExclusionWhere).
		Scan(&stats.Freshness.TransactionsAddedLast24h); err != nil {
		return nil, fmt.Errorf("count txns added 24h: %w", err)
	}

	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions t
		   JOIN accounts a ON t.account_id = a.id
		  WHERE t.deleted_at IS NULL
		    AND t.created_at >= NOW() - INTERVAL '7 days'
		    AND `+dependentExclusionWhere).
		Scan(&stats.Freshness.TransactionsAddedLast7d); err != nil {
		return nil, fmt.Errorf("count txns added 7d: %w", err)
	}

	// Backlog.
	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions t
		   JOIN accounts a ON t.account_id = a.id
		   JOIN transaction_tags tt ON tt.transaction_id = t.id
		   JOIN tags tg ON tg.id = tt.tag_id
		  WHERE t.deleted_at IS NULL
		    AND tg.slug = 'needs-review'
		    AND `+dependentExclusionWhere).
		Scan(&stats.Backlog.NeedsReviewCount); err != nil {
		return nil, fmt.Errorf("count needs-review: %w", err)
	}

	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions t
		   JOIN accounts a ON t.account_id = a.id
		  WHERE t.deleted_at IS NULL AND t.category_id IS NULL AND `+dependentExclusionWhere).
		Scan(&stats.Backlog.UnmappedTransactionCount); err != nil {
		return nil, fmt.Errorf("count unmapped: %w", err)
	}

	return stats, nil
}
