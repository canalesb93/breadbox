//go:build integration && !lite

package service_test

import (
	"context"
	"math"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedRecurring creates a user→connection→account and N monthly $9.99 charges
// with a shared provider_name, returning the account ID and the member short_ids.
func seedRecurring(t *testing.T, queries *db.Queries, name string, dates []string) (pgtype.UUID, []string) {
	t.Helper()
	acctID := seedTxnFixture(t, queries)
	ids := make([]string, 0, len(dates))
	for _, d := range dates {
		txn := testutil.MustCreateTransaction(t, queries, acctID, name+"_"+d, name, 999, d)
		ids = append(ids, txn.ShortID)
	}
	return acctID, ids
}

func countLinkedMembers(t *testing.T, pool *pgxpool.Pool, seriesID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM transactions WHERE series_id = $1`, seriesID).Scan(&n); err != nil {
		t.Fatalf("count linked members: %v", err)
	}
	return n
}

func seriesF64Ptr(f float64) *float64 { return &f }
func seriesI32Ptr(i int32) *int32     { return &i }
func seriesStrPtr(s string) *string   { return &s }

func spotifyUpsert(members []string) service.SeriesUpsert {
	return service.SeriesUpsert{
		Name:           "Spotify",
		MerchantKey:    "spotify",
		Cadence:        service.SeriesCadenceMonthly,
		ExpectedAmount: seriesF64Ptr(9.99),
		ExpectedDay:    seriesI32Ptr(15),
		Currency:       seriesStrPtr("USD"),
		Source:         service.SeriesSourceDeterministic,
		MemberTxnIDs:   members,
	}
}

func TestUpsertSeriesCandidate_InsertAndBacklink(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	resp, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("UpsertSeriesCandidate: %v", err)
	}

	if resp.Status != service.SeriesStatusCandidate {
		t.Errorf("status = %q, want candidate", resp.Status)
	}
	if resp.Confidence != service.SeriesConfidenceAuto {
		t.Errorf("confidence = %q, want auto", resp.Confidence)
	}
	if resp.OccurrenceCount != 3 {
		t.Errorf("occurrence_count = %d, want 3", resp.OccurrenceCount)
	}
	if resp.LastSeenDate == nil || *resp.LastSeenDate != "2026-04-15" {
		t.Errorf("last_seen_date = %v, want 2026-04-15", resp.LastSeenDate)
	}
	if resp.NextExpectedDate == nil || *resp.NextExpectedDate != "2026-05-15" {
		t.Errorf("next_expected_date = %v, want 2026-05-15", resp.NextExpectedDate)
	}
	if resp.LastAmount == nil || math.Abs(*resp.LastAmount-9.99) > 0.001 {
		t.Errorf("last_amount = %v, want 9.99", resp.LastAmount)
	}
	if n := countLinkedMembers(t, pool, resp.ID); n != 3 {
		t.Errorf("linked members = %d, want 3", n)
	}
}

func TestUpsertSeriesCandidate_Idempotent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	first, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if first.ShortID != second.ShortID {
		t.Errorf("re-upsert forked: %s != %s", first.ShortID, second.ShortID)
	}
	if second.OccurrenceCount != 3 {
		t.Errorf("occurrence_count after re-upsert = %d, want 3", second.OccurrenceCount)
	}
	if total, _ := queries.CountRecurringSeries(ctx); total != 1 {
		t.Errorf("series count = %d, want 1", total)
	}
}

func TestUpsertSeriesCandidate_RejectedIsSticky(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	created, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := svc.ReviewSeries(ctx, created.ShortID, service.VerdictReject, service.Actor{Type: "agent", Name: "Agent"}); err != nil {
		t.Fatalf("reject: %v", err)
	}

	// Re-detection must not resurrect a rejected series.
	again, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("re-upsert after reject: %v", err)
	}
	if again.Confidence != service.SeriesConfidenceRejected {
		t.Errorf("confidence = %q, want rejected (sticky)", again.Confidence)
	}
	if again.ShortID != created.ShortID {
		t.Errorf("re-upsert forked past a rejected series")
	}
	if total, _ := queries.CountRecurringSeries(ctx); total != 1 {
		t.Errorf("series count = %d, want 1", total)
	}
}

func TestReviewSeries_ConfirmPromotes(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	created, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	confirmed, err := svc.ReviewSeries(ctx, created.ShortID, service.VerdictConfirm, service.Actor{Type: "user", ID: "u1", Name: "Tester"})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if confirmed.Confidence != service.SeriesConfidenceConfirmed {
		t.Errorf("confidence = %q, want confirmed", confirmed.Confidence)
	}
	if confirmed.Status != service.SeriesStatusActive {
		t.Errorf("status = %q, want active", confirmed.Status)
	}
	if confirmed.ConfirmedByType == nil || *confirmed.ConfirmedByType != "user" {
		t.Errorf("confirmed_by_type = %v, want user", confirmed.ConfirmedByType)
	}
	_ = queries
}

func TestReviewSeries_UserOutranksAgent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	created, _ := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if _, err := svc.ReviewSeries(ctx, created.ShortID, service.VerdictConfirm, service.Actor{Type: "user", ID: "u1", Name: "Tester"}); err != nil {
		t.Fatalf("user confirm: %v", err)
	}
	// An agent cannot overturn a user's confirmation.
	after, err := svc.ReviewSeries(ctx, created.ShortID, service.VerdictReject, service.Actor{Type: "agent", Name: "Agent"})
	if err != nil {
		t.Fatalf("agent reject: %v", err)
	}
	if after.Confidence != service.SeriesConfidenceConfirmed {
		t.Errorf("confidence = %q, want confirmed (user outranks agent)", after.Confidence)
	}
	_ = queries
}

func TestUpsertSeriesCandidate_ConfirmedKeepsAdjudicatedFields(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	created, _ := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if _, err := svc.ReviewSeries(ctx, created.ShortID, service.VerdictConfirm, service.Actor{Type: "user", ID: "u1", Name: "Tester"}); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// A later detection pass adds a 4th charge but proposes a wrong cadence.
	fourth := testutil.MustCreateTransaction(t, queries, acctID, "SPOTIFY_2026-05-15", "SPOTIFY", 999, "2026-05-15")
	bad := spotifyUpsert(append(append([]string{}, members...), fourth.ShortID))
	bad.Cadence = service.SeriesCadenceWeekly // adjudicated field must NOT change

	refreshed, err := svc.UpsertSeriesCandidate(ctx, bad, service.SystemActor())
	if err != nil {
		t.Fatalf("re-upsert confirmed: %v", err)
	}
	if refreshed.Cadence != service.SeriesCadenceMonthly {
		t.Errorf("cadence = %q, want monthly (confirmed fields are sacred)", refreshed.Cadence)
	}
	if refreshed.OccurrenceCount != 4 {
		t.Errorf("occurrence_count = %d, want 4 (rollups always refresh)", refreshed.OccurrenceCount)
	}
	if refreshed.Confidence != service.SeriesConfidenceConfirmed {
		t.Errorf("confidence = %q, want confirmed (never downgraded)", refreshed.Confidence)
	}
}

func TestUpsertSeriesCandidate_HouseholdNullUser(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	in := spotifyUpsert(members) // UserID nil = household
	first, err := svc.UpsertSeriesCandidate(ctx, in, service.SystemActor())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.UserID != nil {
		t.Errorf("user_id = %v, want nil (household)", first.UserID)
	}
	second, err := svc.UpsertSeriesCandidate(ctx, in, service.SystemActor())
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ShortID != second.ShortID {
		t.Error("NULL-user signature did not match on re-upsert (IS NOT DISTINCT FROM)")
	}
	if total, _ := queries.CountRecurringSeries(ctx); total != 1 {
		t.Errorf("series count = %d, want 1", total)
	}
}

func TestUpsertSeriesCandidate_RequiresMerchantKey(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	_, err := svc.UpsertSeriesCandidate(ctx, service.SeriesUpsert{MerchantKey: "  "}, service.SystemActor())
	if err == nil {
		t.Fatal("expected error for empty merchant_key, got nil")
	}
}

// TestUpsertSeriesCandidate_SourcePrecedenceGuard pins PATCH A: a thin
// lower-or-equal-precedence write must not clobber a higher-precedence actor's
// proposed fields, and a write that passes no cadence must never downgrade a
// snapped cadence to "unknown" nor null detection_signals.
func TestUpsertSeriesCandidate_SourcePrecedenceGuard(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	// Detector lands a monthly candidate with real signals.
	det := spotifyUpsert(members)
	det.DetectionSignals = []byte(`{"cadence":"monthly","interval_cv":0.02}`)
	created, err := svc.UpsertSeriesCandidate(ctx, det, service.SystemActor())
	if err != nil {
		t.Fatalf("detector upsert: %v", err)
	}

	// A thin rule-source write (no cadence, no signals) links only — it must
	// NOT downgrade the snapped monthly cadence to "unknown" or null signals.
	ruleWrite := service.SeriesUpsert{
		MerchantKey:  "spotify",
		Currency:     seriesStrPtr("USD"),
		Source:       service.SeriesSourceRule,
		MemberTxnIDs: members,
	}
	afterRule, err := svc.UpsertSeriesCandidate(ctx, ruleWrite, service.Actor{Type: "agent", Name: "Rule"})
	if err != nil {
		t.Fatalf("rule-source upsert: %v", err)
	}
	if afterRule.ShortID != created.ShortID {
		t.Fatalf("rule write forked the series")
	}
	if afterRule.Cadence != service.SeriesCadenceMonthly {
		t.Errorf("cadence = %q, want monthly (a thin rule write must not downgrade to unknown)", afterRule.Cadence)
	}
	if len(afterRule.DetectionSignals) == 0 {
		t.Errorf("detection_signals nulled by a rule write; want preserved")
	}

	// An agent sharpens cadence to annual (higher precedence than deterministic).
	agentWrite := spotifyUpsert(members)
	agentWrite.Cadence = service.SeriesCadenceAnnual
	agentWrite.Source = service.SeriesSourceAgent
	if _, err := svc.UpsertSeriesCandidate(ctx, agentWrite, service.Actor{Type: "agent", Name: "Agent"}); err != nil {
		t.Fatalf("agent upsert: %v", err)
	}
	// A later deterministic re-detect proposing monthly must NOT overwrite the
	// agent's annual (lower rank than agent).
	redetect := spotifyUpsert(members) // source=deterministic, cadence=monthly
	afterRedetect, err := svc.UpsertSeriesCandidate(ctx, redetect, service.SystemActor())
	if err != nil {
		t.Fatalf("re-detect upsert: %v", err)
	}
	if afterRedetect.Cadence != service.SeriesCadenceAnnual {
		t.Errorf("cadence = %q, want annual (deterministic must not overwrite an agent's value)", afterRedetect.Cadence)
	}
}

func TestAssignSeries_CreateLinkConfirm(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID, members := seedRecurring(t, queries, "NETFLIX", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	_ = acctID

	resp, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey:     "netflix",
		CreateIfMissing: true,
		Name:            "Netflix",
		Cadence:         service.SeriesCadenceMonthly,
		ExpectedAmount:  seriesF64Ptr(9.99),
		Currency:        seriesStrPtr("USD"),
		TransactionIDs:  members,
		Confirm:         true,
	}, service.Actor{Type: "user", ID: "u1", Name: "Tester"})
	if err != nil {
		t.Fatalf("AssignSeries create+confirm: %v", err)
	}
	if resp.Status != service.SeriesStatusActive {
		t.Errorf("status = %q, want active (confirm:true)", resp.Status)
	}
	if resp.Confidence != service.SeriesConfidenceConfirmed {
		t.Errorf("confidence = %q, want confirmed", resp.Confidence)
	}
	if resp.OccurrenceCount != 3 {
		t.Errorf("occurrence_count = %d, want 3", resp.OccurrenceCount)
	}
	if n := countLinkedMembers(t, pool, resp.ID); n != 3 {
		t.Errorf("linked members = %d, want 3", n)
	}
}

func TestAssignSeries_LinkExisting(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	created, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("seed series: %v", err)
	}

	// A 4th charge the detector didn't group — link it by series_id.
	extra := testutil.MustCreateTransaction(t, queries, acctID, "SPOTIFY_2026-05-15", "SPOTIFY", 999, "2026-05-15")
	resp, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		SeriesID:       &created.ShortID,
		TransactionIDs: []string{extra.ShortID},
	}, service.Actor{Type: "user", ID: "u1", Name: "Tester"})
	if err != nil {
		t.Fatalf("AssignSeries link existing: %v", err)
	}
	if resp.ShortID != created.ShortID {
		t.Errorf("linked to wrong series: %s != %s", resp.ShortID, created.ShortID)
	}
	if resp.OccurrenceCount != 4 {
		t.Errorf("occurrence_count = %d, want 4", resp.OccurrenceCount)
	}
	if n := countLinkedMembers(t, pool, resp.ID); n != 4 {
		t.Errorf("linked members = %d, want 4", n)
	}
}

func TestAssignSeries_RejectsTooManyMembers(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = "deadbeef"
	}
	_, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey:     "netflix",
		CreateIfMissing: true,
		TransactionIDs:  ids,
	}, service.Actor{Type: "user", Name: "Tester"})
	if err == nil {
		t.Fatal("expected error for >50 transaction_ids, got nil")
	}
}

func TestAssignSeries_RequiresSeriesOrCreate(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	// Neither series_id nor (merchant_key + create_if_missing).
	_, err := svc.AssignSeries(ctx, service.AssignSeriesInput{MerchantKey: "netflix"}, service.Actor{Type: "user", Name: "Tester"})
	if err == nil {
		t.Fatal("expected error when create_if_missing is false and no series_id, got nil")
	}
}

// netflixSeriesRow finds a candidate/active series by merchant_key in the list.
func findSeriesByKey(t *testing.T, svc *service.Service, key string) *service.SeriesResponse {
	t.Helper()
	all, err := svc.ListSeries(context.Background(), nil)
	if err != nil {
		t.Fatalf("list series: %v", err)
	}
	for i := range all {
		if all[i].MerchantKey == key {
			return &all[i]
		}
	}
	return nil
}

func TestApplyRuleRetroactively_AssignSeries(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 1", "Netflix", 1599, "2026-03-15")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 2", "Netflix", 1599, "2026-04-15")
	other := testutil.MustCreateTransaction(t, queries, acctID, "STARBUCKS", "Starbucks", 599, "2026-04-16")

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Netflix → series",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "Netflix"},
		Actions:    []service.RuleAction{{Type: "assign_series", MerchantKey: "netflix", CreateIfMissing: true}},
		Actor:      service.Actor{Type: "user", ID: "u1", Name: "Tester"},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	n, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	if n != 2 {
		t.Errorf("matched = %d, want 2 (only the two Netflix charges)", n)
	}

	row := findSeriesByKey(t, svc, "netflix")
	if row == nil {
		t.Fatal("retroactive assign_series did not mint a netflix series")
	}
	if row.DetectionSource != service.SeriesSourceRule {
		t.Errorf("detection_source = %q, want rule", row.DetectionSource)
	}
	if row.OccurrenceCount != 2 {
		t.Errorf("occurrence_count = %d, want 2", row.OccurrenceCount)
	}
	if n := countLinkedMembers(t, pool, row.ID); n != 2 {
		t.Errorf("linked members = %d, want 2", n)
	}
	// The non-matching charge stays unlinked.
	var seriesID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT series_id FROM transactions WHERE id=$1`, other.ID).Scan(&seriesID); err != nil {
		t.Fatalf("query other txn: %v", err)
	}
	if seriesID.Valid {
		t.Error("non-matching transaction was wrongly linked to a series")
	}
}

// TestRuleConditions_InSeriesAndSeries exercises the read-half of the
// rules-engine composition: a rule conditioned on series membership
// (in_series) or a specific series (series eq short_id) matches only the
// linked transactions when applied retroactively. This validates the
// recurring_series JOIN + scan added to the retroactive context query.
func TestRuleConditions_InSeriesAndSeries(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	testutil.MustCreateTag(t, queries, "subscription-charge", "Subscription charge")
	testutil.MustCreateTag(t, queries, "is-netflix", "Is Netflix")
	acctID := seedTxnFixture(t, queries)
	n1 := testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_1", "Netflix", 1599, "2026-03-15")
	n2 := testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_2", "Netflix", 1599, "2026-04-15")
	loose := testutil.MustCreateTransaction(t, queries, acctID, "STARBUCKS", "Starbucks", 599, "2026-04-16")
	actor := service.Actor{Type: "user", Name: "Tester"}

	// Link the two Netflix charges to a series; Starbucks stays unlinked.
	series, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey: "netflix", CreateIfMissing: true, TransactionIDs: []string{n1.ShortID, n2.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("assign series: %v", err)
	}

	// Rule 1: in_series eq true → add_tag subscription-charge. Should hit the
	// two members, skip Starbucks.
	inSeriesRule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Members get a tag",
		Conditions: service.Condition{Field: "in_series", Op: "eq", Value: true},
		Actions:    []service.RuleAction{{Type: "add_tag", TagSlug: "subscription-charge"}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create in_series rule: %v", err)
	}
	matched, err := svc.ApplyRuleRetroactively(ctx, inSeriesRule.ID)
	if err != nil {
		t.Fatalf("apply in_series rule: %v", err)
	}
	if matched != 2 {
		t.Errorf("in_series matched = %d, want 2 (the two linked charges)", matched)
	}
	if !txnHasTag(t, pool, n1.ID, "subscription-charge") || !txnHasTag(t, pool, n2.ID, "subscription-charge") {
		t.Error("expected both series members to receive the subscription-charge tag")
	}
	if txnHasTag(t, pool, loose.ID, "subscription-charge") {
		t.Error("unlinked transaction was wrongly tagged by an in_series rule")
	}

	// Rule 2: series eq <short_id> → add_tag is-netflix. Targets the specific
	// series by its short_id, exercising the recurring_series JOIN.
	seriesRule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Netflix series tag",
		Conditions: service.Condition{Field: "series", Op: "eq", Value: series.ShortID},
		Actions:    []service.RuleAction{{Type: "add_tag", TagSlug: "is-netflix"}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create series rule: %v", err)
	}
	matched, err = svc.ApplyRuleRetroactively(ctx, seriesRule.ID)
	if err != nil {
		t.Fatalf("apply series rule: %v", err)
	}
	if matched != 2 {
		t.Errorf("series eq matched = %d, want 2", matched)
	}
	if !txnHasTag(t, pool, n1.ID, "is-netflix") {
		t.Error("expected the matched series member to receive the is-netflix tag")
	}
	if txnHasTag(t, pool, loose.ID, "is-netflix") {
		t.Error("unlinked transaction was wrongly tagged by a series eq rule")
	}
}

// TestExplainSeriesCandidates verifies the near-miss / explain feed: a 2-charge
// group is reported as too_few_occurrences, a clean 3-charge group qualifies but
// is untracked, and a merchant already represented by a series is excluded.
func TestExplainSeriesCandidates(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Near-miss: only 2 monthly charges → too_few_occurrences.
	testutil.MustCreateTransaction(t, queries, acctID, "HULU_1", "Hulu", 1799, "2026-03-10")
	testutil.MustCreateTransaction(t, queries, acctID, "HULU_2", "Hulu", 1799, "2026-04-10")
	// Qualifying but untracked: 3 clean monthly charges, steady amount.
	testutil.MustCreateTransaction(t, queries, acctID, "DISNEY_1", "Disney Plus", 1399, "2026-02-12")
	testutil.MustCreateTransaction(t, queries, acctID, "DISNEY_2", "Disney Plus", 1399, "2026-03-12")
	testutil.MustCreateTransaction(t, queries, acctID, "DISNEY_3", "Disney Plus", 1399, "2026-04-12")
	// Already a series: linked netflix charges must be excluded from the feed.
	n1 := testutil.MustCreateTransaction(t, queries, acctID, "NFLX_1", "Netflix", 1599, "2026-02-15")
	n2 := testutil.MustCreateTransaction(t, queries, acctID, "NFLX_2", "Netflix", 1599, "2026-03-15")
	n3 := testutil.MustCreateTransaction(t, queries, acctID, "NFLX_3", "Netflix", 1599, "2026-04-15")

	// Populate merchant_key the way sync would — explain is read-only and assumes
	// the key is already set (the engine sets it at upsert; backfill fills history).
	for key, name := range map[string]string{"hulu": "Hulu", "disneyplus": "Disney Plus", "netflix": "Netflix"} {
		if _, err := pool.Exec(ctx, `UPDATE transactions SET merchant_key=$1 WHERE provider_name=$2`, key, name); err != nil {
			t.Fatalf("set merchant_key: %v", err)
		}
	}

	if _, err := svc.UpsertSeriesCandidate(ctx, service.SeriesUpsert{
		Name: "Netflix", MerchantKey: "netflix", Cadence: service.SeriesCadenceMonthly,
		ExpectedAmount: seriesF64Ptr(15.99), Currency: seriesStrPtr("USD"),
		Source: service.SeriesSourceDeterministic, MemberTxnIDs: []string{n1.ShortID, n2.ShortID, n3.ShortID},
	}, service.SystemActor()); err != nil {
		t.Fatalf("seed netflix series: %v", err)
	}

	nm, err := svc.ExplainSeriesCandidates(ctx)
	if err != nil {
		t.Fatalf("ExplainSeriesCandidates: %v", err)
	}

	byKey := map[string]service.SeriesNearMiss{}
	for _, m := range nm {
		byKey[m.MerchantKey] = m
	}

	if _, ok := byKey["netflix"]; ok {
		t.Error("netflix is already a series; it must not appear in the near-miss feed")
	}

	hulu, ok := byKey["hulu"]
	if !ok {
		t.Fatalf("hulu near-miss missing from feed (got keys %v)", keysOf(byKey))
	}
	if hulu.Reason != "too_few_occurrences" {
		t.Errorf("hulu reason = %q, want too_few_occurrences", hulu.Reason)
	}
	if hulu.Qualifies {
		t.Error("hulu should not qualify with only 2 charges")
	}
	if hulu.OccurrenceCount != 2 {
		t.Errorf("hulu occurrence_count = %d, want 2", hulu.OccurrenceCount)
	}
	if hulu.Explanation == "" {
		t.Error("hulu near-miss should carry a human explanation")
	}

	disney, ok := byKey["disneyplus"]
	if !ok {
		t.Fatalf("disneyplus near-miss missing from feed (got keys %v)", keysOf(byKey))
	}
	if !disney.Qualifies {
		t.Errorf("disneyplus should qualify (3 clean monthly charges); reason=%q", disney.Reason)
	}
}

func keysOf(m map[string]service.SeriesNearMiss) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func txnHasTag(t *testing.T, pool *pgxpool.Pool, txnID pgtype.UUID, slug string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM transaction_tags tt JOIN tags t ON t.id = tt.tag_id WHERE tt.transaction_id = $1 AND t.slug = $2`,
		txnID, slug).Scan(&n); err != nil {
		t.Fatalf("txnHasTag: %v", err)
	}
	return n > 0
}

func TestSeriesTags_MembersInherit(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	testutil.MustCreateTag(t, queries, "subscriptions", "Subscriptions")
	acctID := seedTxnFixture(t, queries)
	t1 := testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_1", "Netflix", 1599, "2026-03-15")
	t2 := testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_2", "Netflix", 1599, "2026-04-15")
	actor := service.Actor{Type: "user", Name: "Tester"}

	series, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey: "netflix", CreateIfMissing: true, TransactionIDs: []string{t1.ShortID, t2.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("create series: %v", err)
	}

	// Attach a tag to the series → existing members are backfilled.
	if err := svc.AddSeriesTag(ctx, series.ShortID, "subscriptions", actor); err != nil {
		t.Fatalf("AddSeriesTag: %v", err)
	}
	if !txnHasTag(t, pool, t1.ID, "subscriptions") || !txnHasTag(t, pool, t2.ID, "subscriptions") {
		t.Error("existing members did not inherit the series tag on add (backfill)")
	}

	// A member linked AFTER the tag exists inherits it at link time.
	t3 := testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_3", "Netflix", 1599, "2026-05-15")
	if _, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		SeriesID: &series.ShortID, TransactionIDs: []string{t3.ShortID},
	}, actor); err != nil {
		t.Fatalf("link new member: %v", err)
	}
	if !txnHasTag(t, pool, t3.ID, "subscriptions") {
		t.Error("newly-linked member did not inherit the series tag")
	}

	// GetSeries reflects the series' tags.
	got, _ := svc.GetSeries(ctx, series.ShortID)
	if len(got.Tags) != 1 || got.Tags[0] != "subscriptions" {
		t.Errorf("GetSeries.Tags = %v, want [subscriptions]", got.Tags)
	}
}

func TestSeriesTags_RemoveStripsInheritedKeepsUserTags(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	testutil.MustCreateTag(t, queries, "subscriptions", "Subscriptions")
	userTag := testutil.MustCreateTag(t, queries, "important", "Important")
	acctID := seedTxnFixture(t, queries)
	tx1 := testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX_1", "Netflix", 1599, "2026-04-15")
	actor := service.Actor{Type: "user", Name: "Tester"}

	series, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey: "netflix", CreateIfMissing: true, TransactionIDs: []string{tx1.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("create series: %v", err)
	}
	if err := svc.AddSeriesTag(ctx, series.ShortID, "subscriptions", actor); err != nil {
		t.Fatalf("AddSeriesTag: %v", err)
	}
	// User manually adds a different tag to the member.
	if _, err := queries.AddTransactionTag(ctx, db.AddTransactionTagParams{
		TransactionID: tx1.ID, TagID: userTag.ID,
		AddedByType: "user", AddedByID: pgconv.Text("u1"), AddedByName: "Tester",
	}); err != nil {
		t.Fatalf("user add tag: %v", err)
	}

	if err := svc.RemoveSeriesTag(ctx, series.ShortID, "subscriptions"); err != nil {
		t.Fatalf("RemoveSeriesTag: %v", err)
	}
	if txnHasTag(t, pool, tx1.ID, "subscriptions") {
		t.Error("series-inherited tag was not stripped from member on remove")
	}
	if !txnHasTag(t, pool, tx1.ID, "important") {
		t.Error("user-added tag was wrongly stripped by RemoveSeriesTag")
	}
}

func TestAssignSeriesFromRuleTx_MintAndLink(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM", "Netflix", 1599, "2026-05-15")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := svc.AssignSeriesFromRuleTx(ctx, tx, txn.ID, "", "netflix", true); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("AssignSeriesFromRuleTx mint: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	row := findSeriesByKey(t, svc, "netflix")
	if row == nil {
		t.Fatal("rule did not mint a netflix series")
	}
	if row.DetectionSource != service.SeriesSourceRule {
		t.Errorf("detection_source = %q, want rule", row.DetectionSource)
	}
	if row.OccurrenceCount != 1 {
		t.Errorf("occurrence_count = %d, want 1", row.OccurrenceCount)
	}
	if n := countLinkedMembers(t, pool, row.ID); n != 1 {
		t.Errorf("linked members = %d, want 1", n)
	}
}

func TestAssignSeriesFromRuleTx_ExistingByShortID(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	created, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("seed series: %v", err)
	}
	extra := testutil.MustCreateTransaction(t, queries, acctID, "SPOTIFY_X", "SPOTIFY", 999, "2026-05-15")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := svc.AssignSeriesFromRuleTx(ctx, tx, extra.ID, created.ShortID, "", false); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("AssignSeriesFromRuleTx by short_id: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	refreshed, _ := svc.GetSeries(ctx, created.ShortID)
	if refreshed.OccurrenceCount != 4 {
		t.Errorf("occurrence_count = %d, want 4", refreshed.OccurrenceCount)
	}
	if n := countLinkedMembers(t, pool, created.ID); n != 4 {
		t.Errorf("linked members = %d, want 4", n)
	}
}

func TestAssignSeriesFromRuleTx_StickyRejectSkips(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	// Create at the SAME signature the rule mints under (NULL currency + user)
	// so the rule's match finds the rejected row and skips.
	householdUpsert := service.SeriesUpsert{
		Name:         "Spotify",
		MerchantKey:  "spotify",
		Cadence:      service.SeriesCadenceMonthly,
		Source:       service.SeriesSourceDeterministic,
		MemberTxnIDs: members,
	}
	created, _ := svc.UpsertSeriesCandidate(ctx, householdUpsert, service.SystemActor())
	if _, err := svc.ReviewSeries(ctx, created.ShortID, service.VerdictReject, service.Actor{Type: "user", Name: "Tester"}); err != nil {
		t.Fatalf("reject: %v", err)
	}

	extra := testutil.MustCreateTransaction(t, queries, acctID, "SPOTIFY_X", "SPOTIFY", 999, "2026-05-15")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// Rule tries to mint at the rejected signature — must be a no-op.
	if err := svc.AssignSeriesFromRuleTx(ctx, tx, extra.ID, "", "spotify", true); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("AssignSeriesFromRuleTx sticky: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// No new series, and the rejected one was not touched (still 3 members).
	if total, _ := queries.CountRecurringSeries(ctx); total != 1 {
		t.Errorf("series count = %d, want 1 (sticky reject must not mint)", total)
	}
	if n := countLinkedMembers(t, pool, created.ID); n != 3 {
		t.Errorf("rejected series members = %d, want 3 (extra txn must not link)", n)
	}
}
