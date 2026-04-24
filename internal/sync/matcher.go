package sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Matcher reconciles transactions between linked accounts (primary + dependent).
// It finds unmatched dependent transactions and pairs them with primary transactions
// based on date + exact amount matching.
type Matcher struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	logger  *slog.Logger
}

// NewMatcher creates a new Matcher.
func NewMatcher(queries *db.Queries, pool *pgxpool.Pool, logger *slog.Logger) *Matcher {
	return &Matcher{queries: queries, pool: pool, logger: logger}
}

// matchCandidate holds a primary transaction that could match a dependent one.
type matchCandidate struct {
	ID           pgtype.UUID
	Name         string
	MerchantName string
}

// ReconcileLink runs the matching algorithm for a single account link.
// Returns the number of new matches created.
func (m *Matcher) ReconcileLink(ctx context.Context, link db.AccountLink) (int, error) {
	if !link.Enabled {
		return 0, nil
	}

	logger := m.logger.With("link_id", pgconv.FormatUUID(link.ID))

	// Resolve the dependent account's user ID for attribution.
	depUserID, err := m.queries.GetDependentUserID(ctx, link.DependentAccountID)
	if err != nil {
		return 0, fmt.Errorf("get dependent user id: %w", err)
	}

	// Find unmatched dependent transactions.
	unmatchedQuery := `
		SELECT t.id, t.date, t.amount, t.provider_name, COALESCE(t.provider_merchant_name, '') AS provider_merchant_name
		FROM transactions t
		WHERE t.account_id = $1
		  AND t.deleted_at IS NULL
		  AND NOT EXISTS (
			SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id
		  )
		ORDER BY t.date DESC`

	unmatchedRows, err := m.pool.Query(ctx, unmatchedQuery, link.DependentAccountID)
	if err != nil {
		return 0, fmt.Errorf("query unmatched dependent txns: %w", err)
	}
	defer unmatchedRows.Close()

	type depTxn struct {
		ID           pgtype.UUID
		Date         pgtype.Date
		Amount       pgtype.Numeric
		Name         string
		MerchantName string
	}

	var unmatched []depTxn
	for unmatchedRows.Next() {
		var t depTxn
		if err := unmatchedRows.Scan(&t.ID, &t.Date, &t.Amount, &t.Name, &t.MerchantName); err != nil {
			return 0, fmt.Errorf("scan unmatched txn: %w", err)
		}
		unmatched = append(unmatched, t)
	}
	if err := unmatchedRows.Err(); err != nil {
		return 0, fmt.Errorf("iterate unmatched txns: %w", err)
	}

	if len(unmatched) == 0 {
		return 0, nil
	}

	newMatches := 0
	tolerance := int(link.MatchToleranceDays)

	for _, dep := range unmatched {
		// Find candidate primary transactions: same amount, date within tolerance, not already matched.
		candidateQuery := `
			SELECT t.id, t.provider_name, COALESCE(t.provider_merchant_name, '') AS provider_merchant_name
			FROM transactions t
			WHERE t.account_id = $1
			  AND t.deleted_at IS NULL
			  AND t.amount = $2
			  AND t.date BETWEEN ($3::date - $4::integer) AND ($3::date + $4::integer)
			  AND NOT EXISTS (
				SELECT 1 FROM transaction_matches tm WHERE tm.primary_transaction_id = t.id
			  )`

		candidateRows, err := m.pool.Query(ctx, candidateQuery,
			link.PrimaryAccountID, dep.Amount, dep.Date, tolerance)
		if err != nil {
			logger.Error("query candidates", "dep_id", pgconv.FormatUUID(dep.ID), "error", err)
			continue
		}

		var candidates []matchCandidate
		for candidateRows.Next() {
			var c matchCandidate
			if err := candidateRows.Scan(&c.ID, &c.Name, &c.MerchantName); err != nil {
				logger.Error("scan candidate", "error", err)
				continue
			}
			candidates = append(candidates, c)
		}
		candidateRows.Close()

		if len(candidates) == 0 {
			continue
		}

		// Single candidate → auto-match.
		var bestCandidate *matchCandidate
		var matchedFields []string

		if len(candidates) == 1 {
			bestCandidate = &candidates[0]
			matchedFields = buildMatchedOn(dep.Name, dep.MerchantName, bestCandidate.Name, bestCandidate.MerchantName)
		} else {
			// Multiple candidates — use name similarity as tiebreaker.
			bestCandidate, matchedFields = pickBestCandidate(dep.Name, dep.MerchantName, candidates)
		}

		if bestCandidate == nil {
			logger.Debug("ambiguous match, skipping",
				"dep_id", pgconv.FormatUUID(dep.ID),
				"candidates", len(candidates))
			continue
		}

		// Always include date and amount in matched_on.
		matchedFields = append([]string{"date", "amount"}, matchedFields...)

		// Create the match record.
		_, err = m.queries.CreateTransactionMatch(ctx, db.CreateTransactionMatchParams{
			AccountLinkID:          link.ID,
			PrimaryTransactionID:   bestCandidate.ID,
			DependentTransactionID: dep.ID,
			MatchConfidence:        "auto",
			MatchedOn:              matchedFields,
		})
		if err != nil {
			// ON CONFLICT DO NOTHING — already matched.
			logger.Debug("match insert skipped (conflict)", "dep_id", pgconv.FormatUUID(dep.ID))
			continue
		}

		// Set attribution on the primary transaction.
		if err := m.queries.SetTransactionAttributedUser(ctx, db.SetTransactionAttributedUserParams{
			ID:               bestCandidate.ID,
			AttributedUserID: depUserID,
		}); err != nil {
			logger.Error("set attribution", "primary_id", pgconv.FormatUUID(bestCandidate.ID), "error", err)
		}

		newMatches++
	}

	if newMatches > 0 {
		logger.Info("reconciliation complete", "new_matches", newMatches)
	}

	return newMatches, nil
}

// ReconcileForConnection runs matching for all account links involving accounts
// under the given connection. Called post-sync.
func (m *Matcher) ReconcileForConnection(ctx context.Context, connectionID pgtype.UUID) {
	links, err := m.queries.ListAccountLinksByConnectionID(ctx, connectionID)
	if err != nil {
		m.logger.Error("list account links for reconciliation", "error", err)
		return
	}

	for _, link := range links {
		if _, err := m.ReconcileLink(ctx, link); err != nil {
			m.logger.Error("reconcile link", "link_id", pgconv.FormatUUID(link.ID), "error", err)
		}
	}
}

// pickBestCandidate selects the best matching candidate when there are multiple
// with the same date+amount. Returns nil if ambiguous.
func pickBestCandidate(depName, depMerchant string, candidates []matchCandidate) (*matchCandidate, []string) {
	type scored struct {
		idx    int
		score  int
		fields []string
	}

	// Hoist ToLower on the dependent strings out of the per-candidate loop.
	// pickBestCandidate is called with the same depName / depMerchant against
	// every candidate, so lowering them once avoids O(candidates) redundant
	// allocations when the substring branches are exercised.
	depNameLower := strings.ToLower(depName)
	depMerchantLower := strings.ToLower(depMerchant)

	var best []scored

	for i, c := range candidates {
		s, fields := nameSimilarityScoreLowered(
			depName, depNameLower,
			depMerchant, depMerchantLower,
			c.Name, c.MerchantName,
		)
		if len(best) == 0 || s > best[0].score {
			best = []scored{{idx: i, score: s, fields: fields}}
		} else if s == best[0].score {
			best = append(best, scored{idx: i, score: s, fields: fields})
		}
	}

	// If exactly one best candidate, return it.
	if len(best) == 1 {
		return &candidates[best[0].idx], best[0].fields
	}

	return nil, nil
}

// nameSimilarityScore computes how similar two transactions' names are.
// Returns score (0-3) and which fields matched.
func nameSimilarityScore(depName, depMerchant, priName, priMerchant string) (int, []string) {
	// Exact merchant_name match (highest signal).
	// EqualFold covers case-insensitive equality without allocating.
	if depMerchant != "" && priMerchant != "" &&
		strings.EqualFold(depMerchant, priMerchant) {
		return 3, []string{"merchant_name"}
	}

	// Merchant name contains or is contained.
	if depMerchant != "" && priMerchant != "" {
		depMerchantLower := strings.ToLower(depMerchant)
		priMerchantLower := strings.ToLower(priMerchant)
		if strings.Contains(depMerchantLower, priMerchantLower) ||
			strings.Contains(priMerchantLower, depMerchantLower) {
			return 2, []string{"merchant_name"}
		}
	}

	// Exact name match.
	if strings.EqualFold(depName, priName) {
		return 2, []string{"name"}
	}

	// Name contains or is contained.
	depNameLower := strings.ToLower(depName)
	priNameLower := strings.ToLower(priName)
	if strings.Contains(depNameLower, priNameLower) ||
		strings.Contains(priNameLower, depNameLower) {
		return 1, []string{"name"}
	}

	// No name similarity — still valid (date + amount matched).
	return 0, nil
}

// nameSimilarityScoreLowered is the same as nameSimilarityScore but accepts
// pre-lowered dependent strings so callers iterating over many candidates
// can lower the dep side exactly once. The scoring behavior is identical to
// nameSimilarityScore.
func nameSimilarityScoreLowered(
	depName, depNameLower,
	depMerchant, depMerchantLower,
	priName, priMerchant string,
) (int, []string) {
	// Exact merchant_name match (highest signal).
	if depMerchant != "" && priMerchant != "" &&
		strings.EqualFold(depMerchant, priMerchant) {
		return 3, []string{"merchant_name"}
	}

	// Merchant name contains or is contained. We reuse depMerchantLower
	// across candidates and only lower priMerchant on demand.
	if depMerchant != "" && priMerchant != "" {
		priMerchantLower := strings.ToLower(priMerchant)
		if strings.Contains(depMerchantLower, priMerchantLower) ||
			strings.Contains(priMerchantLower, depMerchantLower) {
			return 2, []string{"merchant_name"}
		}
	}

	// Exact name match.
	if strings.EqualFold(depName, priName) {
		return 2, []string{"name"}
	}

	// Name contains or is contained. Reuse the pre-lowered depName and
	// lower priName on demand.
	priNameLower := strings.ToLower(priName)
	if strings.Contains(depNameLower, priNameLower) ||
		strings.Contains(priNameLower, depNameLower) {
		return 1, []string{"name"}
	}

	// No name similarity — still valid (date + amount matched).
	return 0, nil
}

// buildMatchedOn returns which name fields matched between two transactions.
func buildMatchedOn(depName, depMerchant, priName, priMerchant string) []string {
	_, fields := nameSimilarityScore(depName, depMerchant, priName, priMerchant)
	return fields
}
