//go:build integration && !lite

// Regression coverage for P0-T4 (rules-as-universal-substrate): every
// retroactive apply path must materialize the full set of state-mutating rule
// actions. The bulk ApplyAllRulesRetroactively path historically dropped
// assign_series (and would silently drop any newly-added action), so this test
// asserts each action type lands on a matched row in the bulk path — with
// assign_series the load-bearing case.
//
// Both retroactive paths now flow through the shared applyRetroTxnIntent, so
// coverage cannot diverge again.
package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestApplyAllRulesRetroactively_AllStateMutatingActions creates one rule
// carrying every action type and asserts each state-mutating action is
// materialized by the bulk apply-all path, while add_comment is intentionally
// NOT replayed (it is sync-time narration only).
func TestApplyAllRulesRetroactively_AllStateMutatingActions(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Target transaction every action will apply to.
	target := testutil.MustCreateTransaction(t, queries, acctID, "txn_netflix", "Netflix Subscription", 1599, "2025-01-14")

	// A non-matching transaction used only to mint a series we can target by
	// short_id (its name does NOT contain "Netflix", so the rule won't touch it).
	seedMember := testutil.MustCreateTransaction(t, queries, acctID, "txn_seed", "Spotify Music", 999, "2025-01-15")
	seriesResp, err := svc.UpsertSeriesCandidate(ctx, spotifyUpsert([]string{seedMember.ShortID}), service.SystemActor())
	if err != nil {
		t.Fatalf("UpsertSeriesCandidate: %v", err)
	}
	seriesShortID := seriesResp.ShortID

	// Target category for set_category.
	testutil.MustCreateCategory(t, queries, "subscriptions", "Subscriptions")

	// Pre-attach a tag that remove_tag must strip, and pre-seed a metadata key
	// that remove_metadata must delete. Without a pre-existing value neither
	// removal would have anything to act on.
	oldTag := testutil.MustCreateTag(t, queries, "old-tag", "Old Tag")
	testutil.MustCreateTransactionTag(t, queries, target.ID, oldTag.ID)
	if _, err := pool.Exec(ctx,
		`UPDATE transactions SET metadata = '{"legacy":"yes"}'::jsonb WHERE id = $1`, target.ID); err != nil {
		t.Fatalf("seed legacy metadata: %v", err)
	}

	// One rule carrying every action type.
	_, err = svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "Netflix Everything",
		Conditions: service.Condition{
			Field: "provider_name",
			Op:    "contains",
			Value: "Netflix",
		},
		Actions: []service.RuleAction{
			{Type: "set_category", CategorySlug: "subscriptions"},
			{Type: "add_tag", TagSlug: "streaming"},
			{Type: "remove_tag", TagSlug: "old-tag"},
			{Type: "add_comment", Content: "reviewed by test"},
			{Type: "assign_series", SeriesShortID: seriesShortID},
			{Type: "set_metadata", MetadataKey: "plan", MetadataValue: "premium"},
			{Type: "remove_metadata", MetadataKey: "legacy"},
		},
		Priority: 100,
		Actor:    service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	if _, err := svc.ApplyAllRulesRetroactively(ctx); err != nil {
		t.Fatalf("ApplyAllRulesRetroactively: %v", err)
	}

	// set_category — category_id resolves to the subscriptions category.
	var catSlug pgtype.Text
	if err := pool.QueryRow(ctx,
		`SELECT c.slug FROM transactions t JOIN categories c ON c.id = t.category_id
		 WHERE t.id = $1`, target.ID).Scan(&catSlug); err != nil {
		t.Fatalf("query category: %v", err)
	}
	if catSlug.String != "subscriptions" {
		t.Errorf("set_category: expected category slug 'subscriptions', got %q", catSlug.String)
	}

	// assign_series — the load-bearing fix: the target must now be linked to the
	// rule-referenced series. Before P0-T4 the bulk path dropped this action.
	var seriesLinked int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions t JOIN recurring_series rs ON rs.id = t.series_id
		 WHERE t.id = $1 AND rs.short_id = $2`, target.ID, seriesShortID).Scan(&seriesLinked); err != nil {
		t.Fatalf("query series link: %v", err)
	}
	if seriesLinked != 1 {
		t.Errorf("assign_series: expected target linked to series %s, got %d matches", seriesShortID, seriesLinked)
	}

	// add_tag / remove_tag — 'streaming' added, 'old-tag' removed.
	tagSlugs := map[string]bool{}
	rows, err := pool.Query(ctx,
		`SELECT tg.slug FROM transaction_tags tt JOIN tags tg ON tg.id = tt.tag_id
		 WHERE tt.transaction_id = $1`, target.ID)
	if err != nil {
		t.Fatalf("query tags: %v", err)
	}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			rows.Close()
			t.Fatalf("scan tag: %v", err)
		}
		tagSlugs[slug] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tags: %v", err)
	}
	if !tagSlugs["streaming"] {
		t.Errorf("add_tag: expected 'streaming' tag on target, tags=%v", tagSlugs)
	}
	if tagSlugs["old-tag"] {
		t.Errorf("remove_tag: expected 'old-tag' stripped from target, tags=%v", tagSlugs)
	}

	// set_metadata / remove_metadata — 'plan' set, 'legacy' removed.
	var planVal pgtype.Text
	var hasLegacy bool
	if err := pool.QueryRow(ctx,
		`SELECT metadata->>'plan', metadata ? 'legacy' FROM transactions WHERE id = $1`, target.ID).
		Scan(&planVal, &hasLegacy); err != nil {
		t.Fatalf("query metadata: %v", err)
	}
	if planVal.String != "premium" {
		t.Errorf("set_metadata: expected metadata.plan='premium', got %q", planVal.String)
	}
	if hasLegacy {
		t.Errorf("remove_metadata: expected metadata key 'legacy' removed")
	}

	// add_comment — retroactive apply must NOT materialize a comment (it is
	// sync-time narration; replaying it across a backfill would be noise).
	if n := testutil.MustCountAnnotations(t, queries, target.ID, "comment"); n != 0 {
		t.Errorf("add_comment: expected 0 comment annotations on retroactive apply, got %d", n)
	}

	// The non-matching seed transaction keeps its own series and gains nothing
	// from the Netflix rule.
	var seedCat pgtype.UUID
	if err := pool.QueryRow(ctx,
		`SELECT category_id FROM transactions WHERE id = $1`, seedMember.ID).Scan(&seedCat); err != nil {
		t.Fatalf("query seed category: %v", err)
	}
	if seedCat.Valid {
		t.Errorf("seed transaction should be untouched, but got category %v", seedCat)
	}
}
