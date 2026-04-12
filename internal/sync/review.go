package sync

import (
	"context"
	"strings"

	"breadbox/internal/db"
)

// enqueueForReview adds a transaction to the review queue if it meets
// criteria (uncategorized or new).
func (e *Engine) enqueueForReview(ctx context.Context, txQueries *db.Queries, txnResult db.Transaction, isNew bool, resolver *RuleResolver) {
	// Skip if transaction has category_override = true
	if txnResult.CategoryOverride {
		return
	}

	reviewType := ""

	// Priority: uncategorized > new_transaction
	// Treat the "uncategorized" fallback category as no real category.
	isUncategorizedFallback := resolver != nil && txnResult.CategoryID.Valid &&
		txnResult.CategoryID.Bytes == resolver.UncategorizedID().Bytes
	hasCategory := (txnResult.CategoryPrimary.Valid || txnResult.CategoryID.Valid) && !isUncategorizedFallback
	if !hasCategory {
		reviewType = "uncategorized"
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
	// Suggest the category only if it's a real, specific category.
	// Suppress catch-all categories (slug ending in _other) and the uncategorized fallback
	// — these are misleading defaults that cause agents to approve bad categories.
	if txnResult.CategoryID.Valid && !isUncategorizedFallback {
		suggest := true
		if resolver != nil {
			if slug := resolver.CategorySlug(txnResult.CategoryID); strings.HasSuffix(slug, "_other") {
				suggest = false
			}
		}
		if suggest {
			params.SuggestedCategoryID = txnResult.CategoryID
		}
	}

	// ON CONFLICT DO NOTHING handles the unique pending constraint
	if _, err := txQueries.EnqueueReview(ctx, params); err != nil {
		e.logger.Error("enqueue review", "transaction_id", txnResult.ID, "review_type", reviewType, "error", err)
	}
}
