package service

import (
	"context"
	"fmt"
)

// OverviewStats contains high-level statistics about the Breadbox dataset.
type OverviewStats struct {
	UserCount        int    `json:"user_count"`
	ConnectionCount  int    `json:"connection_count"`
	AccountCount     int    `json:"account_count"`
	TransactionCount int64  `json:"transaction_count"`
	EarliestDate     string `json:"earliest_transaction_date,omitempty"`
	LatestDate       string `json:"latest_transaction_date,omitempty"`
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
		"SELECT COUNT(*), COALESCE(MIN(date)::text, ''), COALESCE(MAX(date)::text, '') FROM transactions WHERE deleted_at IS NULL").
		Scan(&stats.TransactionCount, &stats.EarliestDate, &stats.LatestDate)
	if err != nil {
		return nil, fmt.Errorf("count transactions: %w", err)
	}

	return stats, nil
}
