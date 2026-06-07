// Package textmatch holds pure, dependency-free string-similarity scoring used
// to decide whether two transactions describe the same thing. It is shared by
// the sync matcher (cross-connection duplicate reconciliation) and the CSV
// import classifier (dedup against existing transactions).
//
// This package is deliberately tag-free (no build constraints, no imports of
// server-only packages) so it compiles under every build tag and unit-tests
// fast without a DB.
package textmatch

import "strings"

// Score computes how similar two transactions' names are. It returns a score
// (0-3, higher is a stronger match) and which fields matched ("merchant_name"
// and/or "name"). The comparison is case-insensitive and symmetric.
//
//	3 — merchant names match exactly
//	2 — merchant names contain one another, OR names match exactly
//	1 — names contain one another
//	0 — no name similarity (caller may still treat as a match on date+amount)
//
// The first pair (a*) and second pair (b*) are interchangeable; the returned
// field labels reflect which field produced the match, not which side.
func Score(aName, aMerchant, bName, bMerchant string) (int, []string) {
	// Exact merchant_name match (highest signal). EqualFold covers
	// case-insensitive equality without allocating.
	if aMerchant != "" && bMerchant != "" &&
		strings.EqualFold(aMerchant, bMerchant) {
		return 3, []string{"merchant_name"}
	}

	// Merchant name contains or is contained.
	if aMerchant != "" && bMerchant != "" {
		aMerchantLower := strings.ToLower(aMerchant)
		bMerchantLower := strings.ToLower(bMerchant)
		if strings.Contains(aMerchantLower, bMerchantLower) ||
			strings.Contains(bMerchantLower, aMerchantLower) {
			return 2, []string{"merchant_name"}
		}
	}

	// Exact name match.
	if strings.EqualFold(aName, bName) {
		return 2, []string{"name"}
	}

	// Name contains or is contained.
	aNameLower := strings.ToLower(aName)
	bNameLower := strings.ToLower(bName)
	if strings.Contains(aNameLower, bNameLower) ||
		strings.Contains(bNameLower, aNameLower) {
		return 1, []string{"name"}
	}

	// No name similarity — still valid (caller matched on date + amount).
	return 0, nil
}

// ScoreLowered is identical to Score but accepts pre-lowered first-side strings
// so a caller iterating over many candidates can lower the (shared) first side
// exactly once. The scoring behavior is identical to Score.
func ScoreLowered(
	aName, aNameLower,
	aMerchant, aMerchantLower,
	bName, bMerchant string,
) (int, []string) {
	// Exact merchant_name match (highest signal).
	if aMerchant != "" && bMerchant != "" &&
		strings.EqualFold(aMerchant, bMerchant) {
		return 3, []string{"merchant_name"}
	}

	// Merchant name contains or is contained. We reuse aMerchantLower across
	// candidates and only lower bMerchant on demand.
	if aMerchant != "" && bMerchant != "" {
		bMerchantLower := strings.ToLower(bMerchant)
		if strings.Contains(aMerchantLower, bMerchantLower) ||
			strings.Contains(bMerchantLower, aMerchantLower) {
			return 2, []string{"merchant_name"}
		}
	}

	// Exact name match.
	if strings.EqualFold(aName, bName) {
		return 2, []string{"name"}
	}

	// Name contains or is contained. Reuse the pre-lowered aName and lower
	// bName on demand.
	bNameLower := strings.ToLower(bName)
	if strings.Contains(aNameLower, bNameLower) ||
		strings.Contains(bNameLower, aNameLower) {
		return 1, []string{"name"}
	}

	// No name similarity — still valid (caller matched on date + amount).
	return 0, nil
}
