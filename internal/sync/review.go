package sync

import (
	"context"
	"strconv"
	"strings"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// confidenceLevelToScore maps Plaid category_confidence enum strings to
// numeric scores for comparison against the confidence threshold.
func confidenceLevelToScore(level string) float64 {
	switch strings.ToUpper(level) {
	case "VERY_HIGH":
		return 0.95
	case "HIGH":
		return 0.80
	case "MEDIUM":
		return 0.50
	case "LOW":
		return 0.20
	case "UNKNOWN":
		return 0.0
	default:
		return -1 // unknown value, skip comparison
	}
}

// enqueueForReview adds a transaction to the review queue if it meets
// criteria (uncategorized, low confidence, or new).
func (e *Engine) enqueueForReview(ctx context.Context, txQueries *db.Queries, txnResult db.Transaction, isNew bool, confidenceThreshold float64, resolver *CategoryResolver) {
	// Skip if transaction has category_override = true
	if txnResult.CategoryOverride {
		return
	}

	reviewType := ""

	// Priority: uncategorized > low_confidence > new_transaction
	// Treat the "uncategorized" fallback category as no real category.
	isUncategorizedFallback := resolver != nil && txnResult.CategoryID.Valid &&
		txnResult.CategoryID.Bytes == resolver.UncategorizedID().Bytes
	hasCategory := (txnResult.CategoryPrimary.Valid || txnResult.CategoryID.Valid) && !isUncategorizedFallback
	if !hasCategory {
		reviewType = "uncategorized"
	} else if confidenceThreshold > 0 && txnResult.CategoryConfidence.Valid {
		conf := confidenceLevelToScore(txnResult.CategoryConfidence.String)
		if conf >= 0 && conf < confidenceThreshold {
			reviewType = "low_confidence"
		}
	}

	if reviewType == "" && isNew {
		reviewType = "new_transaction"
	}

	if reviewType == "" {
		return
	}

	params := db.EnqueueReviewParams{
		TransactionID: txnResult.ID,
		ReviewType:    reviewType,
	}
	if txnResult.CategoryID.Valid && !isUncategorizedFallback {
		params.SuggestedCategoryID = txnResult.CategoryID
	}
	if txnResult.CategoryConfidence.Valid {
		conf := confidenceLevelToScore(txnResult.CategoryConfidence.String)
		if conf >= 0 {
			params.ConfidenceScore = numericFromFloat(conf)
		}
	}

	// ON CONFLICT DO NOTHING handles the unique pending constraint
	_, _ = txQueries.EnqueueReview(ctx, params)
}

// numericFromFloat converts a float64 to pgtype.Numeric.
func numericFromFloat(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(f, 'f', -1, 64))
	return n
}
