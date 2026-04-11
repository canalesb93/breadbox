package admin

import (
	"context"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// GetConfigBool reads a boolean config key from app_config.
// Returns false if the key doesn't exist, has a null value, or can't be parsed.
func GetConfigBool(ctx context.Context, queries *db.Queries, key string) bool {
	cfg, err := queries.GetAppConfig(ctx, key)
	if err != nil {
		return false
	}
	return cfg.Value.Valid && cfg.Value.String == "true"
}

// GetConfigString reads a string config key from app_config.
// Returns empty string if the key doesn't exist or has a null value.
func GetConfigString(ctx context.Context, queries *db.Queries, key string) string {
	cfg, err := queries.GetAppConfig(ctx, key)
	if err != nil || !cfg.Value.Valid {
		return ""
	}
	return cfg.Value.String
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
