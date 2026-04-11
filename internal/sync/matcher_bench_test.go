package sync

import "testing"

// Benchmarks for the account-link name similarity scorer used during
// post-sync reconciliation. These track the cost of the hot path that
// picks the best primary candidate for a dependent transaction when
// multiple candidates share the same (date, amount).
//
// The scenarios are intentionally shaped to exercise every branch of
// nameSimilarityScore so any allocation regression surfaces quickly.

// benchSmallCandidates: two candidates (the minimum that exercises
// pickBestCandidate, since ReconcileLink short-circuits single-candidate
// matches before invoking pickBestCandidate), simple merchant compare.
var benchSmallCandidates = []matchCandidate{
	{Name: "STARBUCKS STORE #1234", MerchantName: "Starbucks"},
	{Name: "PEET'S COFFEE #0087", MerchantName: "Peet's Coffee"},
}

// benchMediumCandidates: three candidates, requires substring comparison.
var benchMediumCandidates = []matchCandidate{
	{Name: "AMZN MKTP US*1234", MerchantName: "Amazon.com"},
	{Name: "SQ *BLUE BOTTLE COFFEE", MerchantName: "Blue Bottle"},
	{Name: "DD *DOORDASH CHIPOTLE SAN FRANCISCO", MerchantName: ""},
}

// benchLargeCandidates: ten candidates, mix of exact / partial matches
// with differing casing — exercises the full ToLower chain.
var benchLargeCandidates = []matchCandidate{
	{Name: "STARBUCKS STORE #5501", MerchantName: "Starbucks"},
	{Name: "amazon marketplace", MerchantName: "amazon.com"},
	{Name: "TRADER JOE'S #123", MerchantName: "Trader Joe's"},
	{Name: "COSTCO WHSE #0412", MerchantName: "Costco Wholesale"},
	{Name: "UBER EATS HELP.UBER.COM", MerchantName: "Uber Eats"},
	{Name: "Whole Foods Market 10231", MerchantName: "Whole Foods"},
	{Name: "SHELL OIL 12345678901", MerchantName: "Shell"},
	{Name: "DD *DOORDASH CHIPOTLE", MerchantName: ""},
	{Name: "APPLE.COM/BILL 866-712-7753", MerchantName: "Apple"},
	{Name: "TARGET STORE T-1234", MerchantName: "Target"},
}

func BenchmarkNameSimilarityScore_Single(b *testing.B) {
	// Covers the direct (single-call) API: buildMatchedOn takes this path
	// after ReconcileLink short-circuits the single-candidate branch.
	depName := "STARBUCKS #1234"
	depMerchant := "Starbucks"
	c := benchSmallCandidates[0]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = nameSimilarityScore(depName, depMerchant, c.Name, c.MerchantName)
	}
}

func BenchmarkPickBestCandidate_Small(b *testing.B) {
	depName := "STARBUCKS #1234"
	depMerchant := "Starbucks"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pickBestCandidate(depName, depMerchant, benchSmallCandidates)
	}
}

func BenchmarkPickBestCandidate_Medium(b *testing.B) {
	depName := "DOORDASH CHIPOTLE"
	depMerchant := ""
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pickBestCandidate(depName, depMerchant, benchMediumCandidates)
	}
}

func BenchmarkPickBestCandidate_Large(b *testing.B) {
	depName := "AMAZON MARKETPLACE"
	depMerchant := "Amazon"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pickBestCandidate(depName, depMerchant, benchLargeCandidates)
	}
}
