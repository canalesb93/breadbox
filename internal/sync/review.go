package sync

import (
	"context"
	"strconv"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// enqueueForReview adds a transaction to the review queue if it meets
// criteria (uncategorized, low confidence, or new).
func (e *Engine) enqueueForReview(ctx context.Context, txQueries *db.Queries, txnResult db.Transaction, isNew bool, confidenceThreshold float64) {
	// Skip if transaction has category_override = true
	if txnResult.CategoryOverride {
		return
	}

	reviewType := ""

	// Priority: uncategorized > low_confidence > new_transaction
	hasCategory := txnResult.CategoryPrimary.Valid || txnResult.CategoryID.Valid
	if !hasCategory {
		reviewType = "uncategorized"
	} else if confidenceThreshold > 0 && txnResult.CategoryConfidence.Valid {
		conf, err := strconv.ParseFloat(txnResult.CategoryConfidence.String, 64)
		if err == nil && conf < confidenceThreshold {
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
	if txnResult.CategoryID.Valid {
		params.SuggestedCategoryID = txnResult.CategoryID
	}
	if txnResult.CategoryConfidence.Valid {
		if conf, err := strconv.ParseFloat(txnResult.CategoryConfidence.String, 64); err == nil {
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
