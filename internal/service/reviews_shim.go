package service

import (
	"context"
	"fmt"
)

// ReviewSummaryRow is the per-category-raw grouping used by the legacy
// pending_reviews_overview MCP tool. Phase 3 keeps the shape so deprecated
// callers continue to parse the response.
type ReviewSummaryRow struct {
	CategoryPrimaryRaw string   `json:"category_primary_raw"`
	Count              int64    `json:"count"`
	SampleNames        []string `json:"sample_names"`
}

// ReviewTypeCount mirrors the old review-queue breakdown. After Phase 3 there
// is only one bucket ("manual") — all tag-based reviews share a single type.
type ReviewTypeCount struct {
	ReviewType string `json:"review_type"`
	Count      int64  `json:"count"`
}

// PendingReviewsOverviewResult is the response payload for the MCP
// pending_reviews_overview shim.
type PendingReviewsOverviewResult struct {
	TotalPending   int64              `json:"total_pending"`
	CountsByType   []ReviewTypeCount  `json:"counts_by_type"`
	CategoryGroups []ReviewSummaryRow `json:"category_groups"`
	Note           string             `json:"note,omitempty"`
}

// PendingReviewsOverviewFromTags recomputes the old pending-reviews overview
// from the tag + transaction tables. Phase 3 retired review_queue; this
// function keeps the shape stable for the deprecated MCP shim.
func (s *Service) PendingReviewsOverviewFromTags(ctx context.Context) (*PendingReviewsOverviewResult, error) {
	// Total count of transactions with the needs-review tag (excluding matched
	// dependent transactions — same semantics as the old count).
	total, err := s.Queries.CountTransactionsWithTagSlug(ctx, "needs-review")
	if err != nil {
		return nil, fmt.Errorf("count needs-review tags: %w", err)
	}

	// By-category-raw breakdown with sample names.
	catRows, err := s.Pool.Query(ctx, `
		SELECT COALESCE(t.category_primary, 'NONE') AS cat_raw,
		       COUNT(*) AS cnt,
		       array_agg(DISTINCT LEFT(t.name, 40) ORDER BY LEFT(t.name, 40)) FILTER (WHERE t.name IS NOT NULL) AS sample_names
		FROM transaction_tags tt
		JOIN tags tag ON tag.id = tt.tag_id
		JOIN transactions t ON t.id = tt.transaction_id
		JOIN accounts a ON a.id = t.account_id
		WHERE tag.slug = 'needs-review'
		  AND t.deleted_at IS NULL
		  AND (a.is_dependent_linked = FALSE
		       OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))
		GROUP BY COALESCE(t.category_primary, 'NONE')
		ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("category breakdown: %w", err)
	}
	defer catRows.Close()

	groups := make([]ReviewSummaryRow, 0)
	for catRows.Next() {
		var row ReviewSummaryRow
		var sampleNames []string
		if err := catRows.Scan(&row.CategoryPrimaryRaw, &row.Count, &sampleNames); err != nil {
			return nil, fmt.Errorf("scan category group: %w", err)
		}
		if len(sampleNames) > 5 {
			sampleNames = sampleNames[:5]
		}
		row.SampleNames = sampleNames
		groups = append(groups, row)
	}
	if err := catRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate category groups: %w", err)
	}

	typeCounts := []ReviewTypeCount{}
	if total > 0 {
		typeCounts = append(typeCounts, ReviewTypeCount{ReviewType: "manual", Count: total})
	}

	return &PendingReviewsOverviewResult{
		TotalPending:   total,
		CountsByType:   typeCounts,
		CategoryGroups: groups,
		Note:           "DEPRECATED: the review_queue table is gone. This is a tag-based view of 'needs-review' transactions.",
	}, nil
}
