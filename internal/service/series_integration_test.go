//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedRecurring creates a user→connection→account and N $9.99 charges with a
// shared provider_name, returning the account ID and the member short_ids.
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

// findSeriesByName returns the live series with the given name, or nil.
func findSeriesByName(t *testing.T, svc *service.Service, name string) *service.SeriesResponse {
	t.Helper()
	all, err := svc.ListSeries(context.Background(), nil)
	if err != nil {
		t.Fatalf("list series: %v", err)
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

// TestAssignSeries_MintByName mints a series by name, links members, and is
// idempotent — a second assign with the same name resolves the SAME surrogate.
func TestAssignSeries_MintByName(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "NETFLIX", []string{"2026-02-15", "2026-03-15", "2026-04-15"})

	resp, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		Name:            "Netflix",
		CreateIfMissing: true,
		Type:            service.SeriesTypeSubscription,
		TransactionIDs:  members,
	}, service.Actor{Type: "user", ID: "u1", Name: "Tester"})
	if err != nil {
		t.Fatalf("AssignSeries mint: %v", err)
	}
	if resp.Name != "Netflix" || resp.Type != service.SeriesTypeSubscription {
		t.Errorf("got name=%q type=%q, want Netflix/subscription", resp.Name, resp.Type)
	}
	if n := countLinkedMembers(t, pool, resp.ID); n != 3 {
		t.Errorf("linked members = %d, want 3", n)
	}

	// Idempotent: same name → same series id, no duplicate row.
	again, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		Name:            "Netflix",
		CreateIfMissing: true,
	}, service.Actor{Type: "user", ID: "u1", Name: "Tester"})
	if err != nil {
		t.Fatalf("AssignSeries re-mint: %v", err)
	}
	if again.ID != resp.ID {
		t.Errorf("re-mint by name created a new series: %s != %s", again.ID, resp.ID)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM recurring_series WHERE name = 'Netflix' AND deleted_at IS NULL`).Scan(&count); err != nil {
		t.Fatalf("count series: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 'Netflix' series, got %d", count)
	}
}

// TestAssignSeries_FailIfExists makes a mint a strict create.
func TestAssignSeries_FailIfExists(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	actor := service.Actor{Type: "user", Name: "Tester"}

	if _, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		Name: "Spotify", CreateIfMissing: true,
	}, actor); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		Name: "Spotify", CreateIfMissing: true, FailIfExists: true,
	}, actor)
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

// TestAssignSeries_LinkExisting links a stray charge to an existing series.
func TestAssignSeries_LinkExisting(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID, members := seedRecurring(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15"})
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	created, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		Name: "Spotify", CreateIfMissing: true, TransactionIDs: members,
	}, actor)
	if err != nil {
		t.Fatalf("seed series: %v", err)
	}

	extra := testutil.MustCreateTransaction(t, queries, acctID, "SPOTIFY_2026-05-15", "SPOTIFY", 999, "2026-05-15")
	resp, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		SeriesID:       &created.ShortID,
		TransactionIDs: []string{extra.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("AssignSeries link existing: %v", err)
	}
	if resp.ShortID != created.ShortID {
		t.Errorf("linked to wrong series: %s != %s", resp.ShortID, created.ShortID)
	}
	if n := countLinkedMembers(t, pool, resp.ID); n != 3 {
		t.Errorf("linked members = %d, want 3", n)
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
		Name: "Netflix", CreateIfMissing: true, TransactionIDs: ids,
	}, service.Actor{Type: "user", Name: "Tester"})
	if err == nil {
		t.Fatal("expected error for >50 transaction_ids, got nil")
	}
}

func TestAssignSeries_RequiresSeriesOrName(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	// Neither series_id nor (name + create_if_missing).
	_, err := svc.AssignSeries(ctx, service.AssignSeriesInput{Name: "Netflix"}, service.Actor{Type: "user", Name: "Tester"})
	if err == nil {
		t.Fatal("expected error when create_if_missing is false and no series_id, got nil")
	}
}

// TestUpdateSeries edits the thin series' name and type.
func TestUpdateSeries(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	actor := service.Actor{Type: "user", Name: "Tester"}

	created, err := svc.AssignSeries(ctx, service.AssignSeriesInput{Name: "Rent", CreateIfMissing: true}, actor)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	newName := "Apartment Rent"
	newType := service.SeriesTypeBill
	updated, err := svc.UpdateSeries(ctx, created.ShortID, service.EditSeriesInput{
		Name: &newName, Type: &newType,
	}, actor)
	if err != nil {
		t.Fatalf("UpdateSeries: %v", err)
	}
	if updated.Name != newName || updated.Type != newType {
		t.Errorf("got name=%q type=%q, want %q/%q", updated.Name, updated.Type, newName, newType)
	}

	// Renaming onto an existing live name collides (unique index).
	other, err := svc.AssignSeries(ctx, service.AssignSeriesInput{Name: "Mortgage", CreateIfMissing: true}, actor)
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	clash := "Apartment Rent"
	if _, err := svc.UpdateSeries(ctx, other.ShortID, service.EditSeriesInput{Name: &clash}, actor); err == nil {
		t.Error("expected collision error renaming onto an existing live name, got nil")
	}
}

// TestAssignSeriesFromRuleTx_MintByName covers the rule/sync path: a rule whose
// assign_series action mints by name links matching charges retroactively.
func TestApplyRuleRetroactively_AssignSeriesByName(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 1", "Netflix", 1599, "2026-03-15")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 2", "Netflix", 1599, "2026-04-15")
	other := testutil.MustCreateTransaction(t, queries, acctID, "STARBUCKS", "Starbucks", 599, "2026-04-16")

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Netflix → series",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "Netflix"},
		Actions:    []service.RuleAction{{Type: "assign_series", SeriesName: "Netflix", CreateIfMissing: true}},
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

	row := findSeriesByName(t, svc, "Netflix")
	if row == nil {
		t.Fatal("retroactive assign_series did not mint a Netflix series")
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

// TestListGoverningRules returns the assign_series rules whose target is this
// series — by short_id or by name.
func TestListGoverningRules(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	actor := service.Actor{Type: "user", Name: "Tester"}

	series, err := svc.AssignSeries(ctx, service.AssignSeriesInput{Name: "Netflix", CreateIfMissing: true}, actor)
	if err != nil {
		t.Fatalf("create series: %v", err)
	}

	// Rule A targets the series by short_id.
	byID, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "By short id",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "Netflix"},
		Actions:    []service.RuleAction{{Type: "assign_series", SeriesShortID: series.ShortID}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create rule A: %v", err)
	}
	// Rule B mints/targets by the series name.
	byName, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "By name",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "NFLX"},
		Actions:    []service.RuleAction{{Type: "assign_series", SeriesName: "Netflix", CreateIfMissing: true}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create rule B: %v", err)
	}
	// An unrelated rule must NOT appear.
	if _, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Unrelated",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "Spotify"},
		Actions:    []service.RuleAction{{Type: "assign_series", SeriesName: "Spotify", CreateIfMissing: true}},
		Actor:      actor,
	}); err != nil {
		t.Fatalf("create unrelated rule: %v", err)
	}

	governing, err := svc.ListGoverningRules(ctx, series.ShortID)
	if err != nil {
		t.Fatalf("ListGoverningRules: %v", err)
	}
	got := map[string]bool{}
	for _, r := range governing {
		got[r.ID] = true
	}
	if !got[byID.ID] || !got[byName.ID] {
		t.Errorf("governing rules missing expected entries: %+v", got)
	}
	if len(governing) != 2 {
		t.Errorf("governing rules = %d, want 2 (the unrelated rule excluded)", len(governing))
	}
}

// TestSeriesCascadeDelete confirms that deleting a recurring_series row nulls
// its members' series_id (ON DELETE SET NULL).
func TestSeriesCascadeDelete_NullsSeriesID(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "NETFLIX", []string{"2026-02-15", "2026-03-15"})
	actor := service.Actor{Type: "user", Name: "Tester"}

	series, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		Name: "Netflix", CreateIfMissing: true, TransactionIDs: members,
	}, actor)
	if err != nil {
		t.Fatalf("create series: %v", err)
	}
	if n := countLinkedMembers(t, pool, series.ID); n != 2 {
		t.Fatalf("precondition: linked = %d, want 2", n)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM recurring_series WHERE id = $1`, series.ID); err != nil {
		t.Fatalf("delete series: %v", err)
	}
	var nullable int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions WHERE series_id IS NULL AND short_id = ANY($1)`,
		members).Scan(&nullable); err != nil {
		t.Fatalf("count nulled: %v", err)
	}
	if nullable != 2 {
		t.Errorf("expected both members' series_id nulled, got %d", nullable)
	}
}

// TestRuleConditions_InSeriesAndSeries exercises the read-half of the
// rules-engine composition: a rule conditioned on series membership (in_series)
// or a specific series (series eq short_id) matches only the linked
// transactions when applied retroactively.
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

	series, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		Name: "Netflix", CreateIfMissing: true, TransactionIDs: []string{n1.ShortID, n2.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("assign series: %v", err)
	}

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

// TestUnlinkSeriesTransactions detaches a member.
func TestUnlinkSeriesTransactions(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	_, members := seedRecurring(t, queries, "NETFLIX", []string{"2026-02-15", "2026-03-15"})
	actor := service.Actor{Type: "user", Name: "Tester"}

	series, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
		Name: "Netflix", CreateIfMissing: true, TransactionIDs: members,
	}, actor)
	if err != nil {
		t.Fatalf("create series: %v", err)
	}
	if _, err := svc.UnlinkSeriesTransactions(ctx, series.ShortID, []string{members[0]}, actor); err != nil {
		t.Fatalf("UnlinkSeriesTransactions: %v", err)
	}
	if n := countLinkedMembers(t, pool, series.ID); n != 1 {
		t.Errorf("linked members after unlink = %d, want 1", n)
	}
	// Unlinking a non-member errors.
	if _, err := svc.UnlinkSeriesTransactions(ctx, series.ShortID, []string{members[0]}, actor); err == nil {
		t.Error("expected error unlinking a non-member, got nil")
	}
}
