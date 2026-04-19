package admin

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// splitCSV splits a comma-separated string into trimmed non-empty entries.
// Used by URL params that accept multi-value lists (e.g. ?tags=a,b,c).
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// BalanceTotals holds aggregated asset/liability/net-worth values.
type BalanceTotals struct {
	TotalAssets      float64
	TotalLiabilities float64
	NetWorth         float64
}

// IsLiabilityAccount returns true for account types that represent liabilities (credit, loan).
func IsLiabilityAccount(accountType string) bool {
	return accountType == "credit" || accountType == "loan"
}

// ConnectionStaleness computes whether a connection is stale based on the
// global sync interval and any per-connection override.
// A connection is stale when it hasn't synced within 2x its effective interval
// (minimum 1 hour for overrides, 24 hours for the global default).
func ConnectionStaleness(
	globalSyncIntervalMinutes int,
	overrideMinutes pgtype.Int4,
	lastSyncedAt pgtype.Timestamptz,
	now time.Time,
) bool {
	globalSyncInterval := time.Duration(globalSyncIntervalMinutes) * time.Minute
	threshold := globalSyncInterval * 2
	if threshold < 24*time.Hour {
		threshold = 24 * time.Hour
	}

	if overrideMinutes.Valid {
		connInterval := time.Duration(overrideMinutes.Int32) * time.Minute
		threshold = connInterval * 2
		if threshold < time.Hour {
			threshold = time.Hour
		}
	}

	if lastSyncedAt.Valid {
		return now.Sub(lastSyncedAt.Time) > threshold
	}
	// Never synced = stale.
	return true
}
