//go:build integration && !lite

package service_test

import (
	"context"
	"math"
	"testing"

	"breadbox/internal/db"
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
