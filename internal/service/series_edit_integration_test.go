//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestUpdateSeries_EditsAndSurvivesRedetect is the load-bearing edit test: a
// user edit changes name/amount/cadence, re-derives next_expected_date, stamps
// detection_source='user', and — critically — survives the next deterministic
// re-detect untouched (the precedence ladder protects the human's values).
func TestUpdateSeries_EditsAndSurvivesRedetect(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	created, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if created.DetectionSource != service.SeriesSourceDeterministic {
		t.Fatalf("precondition: detection_source = %q, want deterministic", created.DetectionSource)
	}

	user := service.Actor{Type: "user", ID: "u1", Name: "Tester"}
	edited, err := svc.UpdateSeries(ctx, created.ShortID, service.EditSeriesInput{
		Name:           seriesStrPtr("Spotify Family"),
		ExpectedAmount: seriesF64Ptr(17.99),
		Cadence:        seriesStrPtr(service.SeriesCadenceAnnual),
	}, user)
	if err != nil {
		t.Fatalf("UpdateSeries: %v", err)
	}
	if edited.Name != "Spotify Family" {
		t.Errorf("name = %q, want Spotify Family", edited.Name)
	}
	if edited.ExpectedAmount == nil || math.Abs(*edited.ExpectedAmount-17.99) > 0.001 {
		t.Errorf("expected_amount = %v, want 17.99", edited.ExpectedAmount)
	}
	if edited.Cadence != service.SeriesCadenceAnnual {
		t.Errorf("cadence = %q, want annual", edited.Cadence)
	}
	// Cadence change re-derived the projected next charge (annual from 2026-04-15).
	if edited.NextExpectedDate == nil || *edited.NextExpectedDate != "2027-04-15" {
		t.Errorf("next_expected_date = %v, want 2027-04-15 (re-derived from annual cadence)", edited.NextExpectedDate)
	}
	// The edit lifted the row to user-source so a re-detect can't revert it.
	if edited.DetectionSource != service.SeriesSourceUser {
		t.Errorf("detection_source = %q, want user", edited.DetectionSource)
	}

	// A deterministic re-detect proposes the ORIGINAL name/amount/cadence. The
	// user's values must survive (deterministic ranks below user on the ladder);
	// only the rollups refresh.
	afterRedetect, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("re-detect: %v", err)
	}
	if afterRedetect.Name != "Spotify Family" {
		t.Errorf("name after re-detect = %q, want Spotify Family (user edit must survive)", afterRedetect.Name)
	}
	if afterRedetect.Cadence != service.SeriesCadenceAnnual {
		t.Errorf("cadence after re-detect = %q, want annual (user edit must survive)", afterRedetect.Cadence)
	}
	if afterRedetect.ExpectedAmount == nil || math.Abs(*afterRedetect.ExpectedAmount-17.99) > 0.001 {
		t.Errorf("expected_amount after re-detect = %v, want 17.99 (user edit must survive)", afterRedetect.ExpectedAmount)
	}
	if afterRedetect.OccurrenceCount != 3 {
		t.Errorf("occurrence_count = %d, want 3 (rollups still refresh)", afterRedetect.OccurrenceCount)
	}
}

func TestUpdateSeries_ValidatesInput(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	created, _ := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	user := service.Actor{Type: "user", Name: "Tester"}

	if _, err := svc.UpdateSeries(ctx, created.ShortID, service.EditSeriesInput{Name: seriesStrPtr("   ")}, user); err == nil {
		t.Error("expected empty name to be rejected")
	}
	if _, err := svc.UpdateSeries(ctx, created.ShortID, service.EditSeriesInput{Cadence: seriesStrPtr("fortnightly")}, user); err == nil {
		t.Error("expected invalid cadence to be rejected")
	}
	if _, err := svc.UpdateSeries(ctx, "deadbeef", service.EditSeriesInput{Name: seriesStrPtr("x")}, user); err == nil {
		t.Error("expected unknown series to error")
	}
}

// TestUpdateSeries_CurrencyCollisionGuard proves a currency edit can't silently
// merge two distinct series: editing the USD Netflix series' currency to EUR,
// where an EUR Netflix series already exists, is refused.
func TestUpdateSeries_CurrencyCollisionGuard(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, _ = seedRecurring(t, queries, "NETFLIX", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	usd, err := svc.UpsertSeriesCandidate(ctx, service.SeriesUpsert{
		Name: "Netflix", MerchantKey: "netflix", Cadence: service.SeriesCadenceMonthly,
		Currency: seriesStrPtr("USD"), Source: service.SeriesSourceDeterministic,
	}, service.SystemActor())
	if err != nil {
		t.Fatalf("mint usd: %v", err)
	}
	if _, err := svc.UpsertSeriesCandidate(ctx, service.SeriesUpsert{
		Name: "Netflix EU", MerchantKey: "netflix", Cadence: service.SeriesCadenceMonthly,
		Currency: seriesStrPtr("EUR"), Source: service.SeriesSourceDeterministic,
	}, service.SystemActor()); err != nil {
		t.Fatalf("mint eur: %v", err)
	}

	if _, err := svc.UpdateSeries(ctx, usd.ShortID, service.EditSeriesInput{
		Currency: seriesStrPtr("EUR"),
	}, service.Actor{Type: "user", Name: "Tester"}); err == nil {
		t.Error("expected a currency edit that collides with another series to be refused")
	}
}

// TestUnlinkSeriesTransactions_DetachAndRollup detaches a member and verifies
// the charge is freed and the series' rollups (count + last seen + next charge)
// recompute from the remaining members.
func TestUnlinkSeriesTransactions_DetachAndRollup(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	created, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if created.OccurrenceCount != 3 {
		t.Fatalf("precondition: occurrence_count = %d, want 3", created.OccurrenceCount)
	}

	// Unlink the most-recent charge (2026-04-15, members[2]).
	updated, err := svc.UnlinkSeriesTransactions(ctx, created.ShortID, []string{members[2]}, service.SystemActor())
	if err != nil {
		t.Fatalf("UnlinkSeriesTransactions: %v", err)
	}
	if updated.OccurrenceCount != 2 {
		t.Errorf("occurrence_count = %d, want 2", updated.OccurrenceCount)
	}
	if updated.LastSeenDate == nil || *updated.LastSeenDate != "2026-03-15" {
		t.Errorf("last_seen_date = %v, want 2026-03-15 (recomputed)", updated.LastSeenDate)
	}
	if updated.NextExpectedDate == nil || *updated.NextExpectedDate != "2026-04-15" {
		t.Errorf("next_expected_date = %v, want 2026-04-15 (monthly from new last-seen)", updated.NextExpectedDate)
	}
	if n := countLinkedMembers(t, pool, created.ID); n != 2 {
		t.Errorf("linked members = %d, want 2", n)
	}

	// Unlinking a transaction that isn't a member errors (doesn't touch others).
	loose := testutil.MustCreateTransaction(t, queries, acctID, "LOOSE", "Loose Co", 100, "2026-01-01")
	if _, err := svc.UnlinkSeriesTransactions(ctx, created.ShortID, []string{loose.ShortID}, service.SystemActor()); err == nil {
		t.Error("expected unlinking a non-member to error")
	}
}

// TestUnlinkSeriesTransactions_StripsInheritedTags mirrors the split contract:
// detaching a charge drops the series' system-provenance inherited tags but
// keeps a tag the user added directly to the transaction.
func TestUnlinkSeriesTransactions_StripsInheritedTags(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	testutil.MustCreateTag(t, queries, "on-sub", "On Subscription")
	testutil.MustCreateTag(t, queries, "user-pin", "User Pin")
	acctID := seedTxnFixture(t, queries)
	keep := testutil.MustCreateTransaction(t, queries, acctID, "NFLX_1", "Netflix", 1599, "2026-03-15")
	drop := testutil.MustCreateTransaction(t, queries, acctID, "NFLX_2", "Netflix", 1599, "2026-04-15")
	actor := service.Actor{Type: "user", Name: "Tester"}

	series, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey: "netflix", CreateIfMissing: true, TransactionIDs: []string{keep.ShortID, drop.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("create series: %v", err)
	}
	if err := svc.AddSeriesTag(ctx, series.ShortID, "on-sub", actor); err != nil {
		t.Fatalf("add series tag: %v", err)
	}
	// A tag the user added directly to the charge being unlinked.
	if _, err := pool.Exec(ctx,
		`INSERT INTO transaction_tags (transaction_id, tag_id, added_by_type, added_by_name)
		 SELECT $1, id, 'user', 'Tester' FROM tags WHERE slug = 'user-pin'`, drop.ID); err != nil {
		t.Fatalf("insert user tag: %v", err)
	}
	if !txnHasTag(t, pool, drop.ID, "on-sub") || !txnHasTag(t, pool, drop.ID, "user-pin") {
		t.Fatal("precondition: dropped charge should carry both tags")
	}

	if _, err := svc.UnlinkSeriesTransactions(ctx, series.ShortID, []string{drop.ShortID}, actor); err != nil {
		t.Fatalf("UnlinkSeriesTransactions: %v", err)
	}

	if txnHasTag(t, pool, drop.ID, "on-sub") {
		t.Error("unlinked charge should lose the series' inherited tag")
	}
	if !txnHasTag(t, pool, drop.ID, "user-pin") {
		t.Error("a user-added tag must survive the unlink (provenance-scoped strip)")
	}
	if !txnHasTag(t, pool, keep.ID, "on-sub") {
		t.Error("a member that stayed should keep the series' inherited tag")
	}
}

// TestUpdateSeries_ClearExpectedDay proves the day anchor can be cleared back to
// NULL by passing 0 (the contract the Edit drawer relies on for a blank field).
func TestUpdateSeries_ClearExpectedDay(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	created, _ := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	user := service.Actor{Type: "user", Name: "Tester"}

	withDay, err := svc.UpdateSeries(ctx, created.ShortID, service.EditSeriesInput{ExpectedDay: seriesI32Ptr(15)}, user)
	if err != nil {
		t.Fatalf("set day: %v", err)
	}
	if withDay.ExpectedDay == nil || *withDay.ExpectedDay != 15 {
		t.Fatalf("expected_day = %v, want 15", withDay.ExpectedDay)
	}

	cleared, err := svc.UpdateSeries(ctx, created.ShortID, service.EditSeriesInput{ExpectedDay: seriesI32Ptr(0)}, user)
	if err != nil {
		t.Fatalf("clear day: %v", err)
	}
	if cleared.ExpectedDay != nil {
		t.Errorf("expected_day = %v, want nil (cleared)", *cleared.ExpectedDay)
	}
}

// TestUpdateSeries_CollisionGuardSkipsDeadRows proves the currency/owner guard
// only fires on a LIVE series at the target signature: a cancelled series there
// must not block an otherwise-valid currency edit (the user can't see it).
func TestUpdateSeries_CollisionGuardSkipsDeadRows(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, _ = seedRecurring(t, queries, "NETFLIX", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	usd, err := svc.UpsertSeriesCandidate(ctx, service.SeriesUpsert{
		Name: "Netflix", MerchantKey: "netflix", Cadence: service.SeriesCadenceMonthly,
		Currency: seriesStrPtr("USD"), Source: service.SeriesSourceDeterministic,
	}, service.SystemActor())
	if err != nil {
		t.Fatalf("mint usd: %v", err)
	}
	// An EUR series at the target signature, then cancelled — it's dead.
	eur, err := svc.UpsertSeriesCandidate(ctx, service.SeriesUpsert{
		Name: "Netflix EU", MerchantKey: "netflix", Cadence: service.SeriesCadenceMonthly,
		Currency: seriesStrPtr("EUR"), Source: service.SeriesSourceDeterministic,
	}, service.SystemActor())
	if err != nil {
		t.Fatalf("mint eur: %v", err)
	}
	if _, err := svc.ReviewSeries(ctx, eur.ShortID, service.VerdictCancel, service.Actor{Type: "user", Name: "Tester"}); err != nil {
		t.Fatalf("cancel eur: %v", err)
	}

	// Editing the USD series to EUR must now succeed — the only EUR row is dead.
	if _, err := svc.UpdateSeries(ctx, usd.ShortID, service.EditSeriesInput{
		Currency: seriesStrPtr("EUR"),
	}, service.Actor{Type: "user", Name: "Tester"}); err != nil {
		t.Errorf("currency edit blocked by a cancelled (dead) row: %v", err)
	}
}

// TestUnlinkSeriesTransactions_DuplicateIDs proves passing the same charge twice
// detaches it once and does NOT falsely report it as a non-member (the
// detached-count guard would otherwise misfire).
func TestUnlinkSeriesTransactions_DuplicateIDs(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	created, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	updated, err := svc.UnlinkSeriesTransactions(ctx, created.ShortID, []string{members[2], members[2]}, service.SystemActor())
	if err != nil {
		t.Fatalf("UnlinkSeriesTransactions with duplicate id: %v", err)
	}
	if updated.OccurrenceCount != 2 {
		t.Errorf("occurrence_count = %d, want 2", updated.OccurrenceCount)
	}
	if n := countLinkedMembers(t, pool, created.ID); n != 2 {
		t.Errorf("linked members = %d, want 2", n)
	}
}

// TestAssignSeries_FailIfExists proves a deliberate create (FailIfExists) refuses
// to adopt an existing live series or resurrect a rejected one, returning
// ErrConflict instead of silently mutating it.
func TestAssignSeries_FailIfExists(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, _ = seedRecurring(t, queries, "NETFLIX", []string{"2026-02-15", "2026-03-15"})
	user := service.Actor{Type: "user", Name: "Tester"}

	existing, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey: "netflix", CreateIfMissing: true, Confirm: true, Name: "Netflix",
	}, user)
	if err != nil {
		t.Fatalf("seed existing: %v", err)
	}

	// A strict create at the same signature must conflict, not adopt.
	if _, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey: "netflix", CreateIfMissing: true, Confirm: true, FailIfExists: true, Name: "Netflix",
	}, user); !errors.Is(err, service.ErrConflict) {
		t.Errorf("strict create over a live series: err = %v, want ErrConflict", err)
	}

	// And over a rejected signature it must conflict (not resurrect it).
	if _, err := svc.ReviewSeries(ctx, existing.ShortID, service.VerdictReject, user); err != nil {
		t.Fatalf("reject existing: %v", err)
	}
	if _, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		MerchantKey: "netflix", CreateIfMissing: true, Confirm: true, FailIfExists: true, Name: "Netflix",
	}, user); !errors.Is(err, service.ErrConflict) {
		t.Errorf("strict create over a rejected series: err = %v, want ErrConflict", err)
	}
}

// TestSeriesMembershipAnnotations proves the timeline events: linking members
// emits one series_assigned per newly-linked charge, a re-detect does NOT
// re-emit (NULL-fill only), and unlinking emits series_unlinked.
func TestSeriesMembershipAnnotations(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	countKind := func(kind string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM annotations WHERE kind = $1`, kind).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", kind, err)
		}
		return n
	}

	created, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if got := countKind("series_assigned"); got != 3 {
		t.Errorf("series_assigned after mint = %d, want 3 (one per linked member)", got)
	}

	// A deterministic re-detect NULL-fills already-linked members → no re-emit.
	if _, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor()); err != nil {
		t.Fatalf("re-detect: %v", err)
	}
	if got := countKind("series_assigned"); got != 3 {
		t.Errorf("series_assigned after re-detect = %d, want 3 (must not re-emit)", got)
	}

	// Unlinking a member emits exactly one series_unlinked.
	if _, err := svc.UnlinkSeriesTransactions(ctx, created.ShortID, []string{members[2]}, service.Actor{Type: "user", Name: "Tester"}); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if got := countKind("series_unlinked"); got != 1 {
		t.Errorf("series_unlinked after unlink = %d, want 1", got)
	}
}

// TestPatchSeries_EditAndVerdictAtomic proves the combined edit+verdict path
// applies both in one transaction: a rename + confirm lands the new name AND the
// confirmed/active verdict in a single call.
func TestPatchSeries_EditAndVerdictAtomic(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	created, _ := svc.UpsertSeriesCandidate(ctx, spotifyUpsert(members), service.SystemActor())
	user := service.Actor{Type: "user", Name: "Tester"}

	verdict := service.VerdictConfirm
	patched, err := svc.PatchSeries(ctx, created.ShortID, &service.EditSeriesInput{
		Name: seriesStrPtr("Spotify Family"),
	}, &verdict, user)
	if err != nil {
		t.Fatalf("PatchSeries: %v", err)
	}
	if patched.Name != "Spotify Family" {
		t.Errorf("name = %q, want Spotify Family (edit applied)", patched.Name)
	}
	if patched.Confidence != service.SeriesConfidenceConfirmed || patched.Status != service.SeriesStatusActive {
		t.Errorf("confidence/status = %q/%q, want confirmed/active (verdict applied)", patched.Confidence, patched.Status)
	}
}
