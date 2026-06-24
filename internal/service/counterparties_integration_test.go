//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func cpStrptr(s string) *string { return &s }

// seedCounterpartyCharges creates a user→connection→account and N charges with a
// shared provider_name, returning the account ID and the member db.Transaction
// rows.
func seedCounterpartyCharges(t *testing.T, queries *db.Queries, name string, dates []string) (pgtype.UUID, []db.Transaction) {
	t.Helper()
	acctID := seedTxnFixture(t, queries)
	rows := make([]db.Transaction, 0, len(dates))
	for _, d := range dates {
		rows = append(rows, testutil.MustCreateTransaction(t, queries, acctID, name+"_"+d, name, 999, d))
	}
	return acctID, rows
}

func shortIDs(rows []db.Transaction) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ShortID
	}
	return out
}

func countLinkedCounterparty(t *testing.T, pool *pgxpool.Pool, cpID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM transactions WHERE counterparty_id = $1`, cpID).Scan(&n); err != nil {
		t.Fatalf("count linked counterparty members: %v", err)
	}
	return n
}

// TestAssignCounterparty_CreateByName resolves-or-creates a counterparty by name,
// links members, and de-dupes — a second create-by-name resolves the SAME row
// (no UNIQUE on name, application-level de-dup).
func TestAssignCounterparty_CreateByName(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	_, members := seedCounterpartyCharges(t, queries, "VENMO", []string{"2026-02-15", "2026-03-15", "2026-04-15"})
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	resp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name:            "Venmo",
		CreateIfMissing: true,
		TransactionIDs:  shortIDs(members),
	}, actor)
	if err != nil {
		t.Fatalf("AssignCounterparty create: %v", err)
	}
	if resp.Name != "Venmo" {
		t.Errorf("got name=%q, want Venmo", resp.Name)
	}
	if n := countLinkedCounterparty(t, pool, resp.ID); n != 3 {
		t.Errorf("linked members = %d, want 3", n)
	}

	// Resolve-or-create de-dupes: same name → same surrogate, no duplicate row.
	again, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name: "Venmo", CreateIfMissing: true,
	}, actor)
	if err != nil {
		t.Fatalf("AssignCounterparty re-resolve: %v", err)
	}
	if again.ID != resp.ID {
		t.Errorf("re-resolve by name created a new counterparty: %s != %s", again.ID, resp.ID)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM counterparties WHERE name = 'Venmo' AND deleted_at IS NULL`).Scan(&count); err != nil {
		t.Fatalf("count counterparties: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 'Venmo' counterparty, got %d", count)
	}
}

// TestAssignCounterparty_EmitsEnrichedAnnotation proves the end-to-end timeline
// path: assigning a counterparty writes a counterparty_assigned annotation that
// EnrichAnnotations resolves into the human "set the counterparty to {name}"
// sentence — the parity twin of the series_assigned timeline event.
func TestAssignCounterparty_EmitsEnrichedAnnotation(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, members := seedCounterpartyCharges(t, queries, "NETFLIX", []string{"2026-04-15"})
	actor := service.Actor{Type: "user", ID: "u1", Name: "Alice"}

	if _, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name:            "Netflix",
		CreateIfMissing: true,
		TransactionIDs:  shortIDs(members),
	}, actor); err != nil {
		t.Fatalf("AssignCounterparty: %v", err)
	}

	anns, err := svc.ListAnnotations(ctx, members[0].ShortID, service.ListAnnotationsParams{})
	if err != nil {
		t.Fatalf("ListAnnotations: %v", err)
	}
	var found *service.Annotation
	for i := range anns {
		if anns[i].Kind == "counterparty_assigned" {
			found = &anns[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no counterparty_assigned annotation on the linked charge; got %d annotations", len(anns))
	}
	if found.Action != "set" {
		t.Errorf("Action = %q, want set", found.Action)
	}
	if found.Subject != "Netflix" {
		t.Errorf("Subject = %q, want Netflix", found.Subject)
	}
	if found.Summary != "Alice set the counterparty to Netflix" {
		t.Errorf("Summary = %q, want %q", found.Summary, "Alice set the counterparty to Netflix")
	}
}

// TestAssignCounterparty_FailIfExists makes the by-name path a strict create.
func TestAssignCounterparty_FailIfExists(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	actor := service.Actor{Type: "user", Name: "Tester"}

	if _, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name: "Uber", CreateIfMissing: true,
	}, actor); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name: "Uber", CreateIfMissing: true, FailIfExists: true,
	}, actor)
	if err == nil {
		t.Fatal("expected ErrConflict, got nil")
	}
}

// TestAssignCounterparty_NoAutoCreate confirms a counterparty is NOT created
// unless create_if_missing is set.
func TestAssignCounterparty_NoAutoCreate(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	actor := service.Actor{Type: "user", Name: "Tester"}
	if _, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{Name: "Stripe"}, actor); err == nil {
		t.Fatal("expected error when create_if_missing is false and no counterparty_short_id, got nil")
	}
}

// TestAssignCounterparty_LinkExisting binds a stray charge to an existing
// counterparty by short_id.
func TestAssignCounterparty_LinkExisting(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID, members := seedCounterpartyCharges(t, queries, "SPOTIFY", []string{"2026-02-15", "2026-03-15"})
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	created, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name: "Spotify", CreateIfMissing: true, TransactionIDs: shortIDs(members),
	}, actor)
	if err != nil {
		t.Fatalf("seed counterparty: %v", err)
	}

	extra := testutil.MustCreateTransaction(t, queries, acctID, "SPOTIFY_extra", "SPOTIFY", 999, "2026-05-15")
	resp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		CounterpartyShortID: cpStrptr(created.ShortID),
		TransactionIDs:      []string{extra.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("AssignCounterparty link existing: %v", err)
	}
	if resp.ShortID != created.ShortID {
		t.Errorf("linked to wrong counterparty: %s != %s", resp.ShortID, created.ShortID)
	}
	if n := countLinkedCounterparty(t, pool, resp.ID); n != 3 {
		t.Errorf("linked members = %d, want 3", n)
	}
}

// TestApplyRuleRetroactively_AssignCounterpartyByShortID covers the rule path
// (single-rule retroactive) binding by short_id.
func TestApplyRuleRetroactively_AssignCounterpartyByShortID(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	testutil.MustCreateTransaction(t, queries, acctID, "UBER TRIP 1", "Uber", 1599, "2026-03-15")
	testutil.MustCreateTransaction(t, queries, acctID, "UBER TRIP 2", "Uber", 1599, "2026-04-15")
	other := testutil.MustCreateTransaction(t, queries, acctID, "STARBUCKS", "Starbucks", 599, "2026-04-16")
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	cp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{Name: "Uber", CreateIfMissing: true}, actor)
	if err != nil {
		t.Fatalf("create counterparty: %v", err)
	}

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Uber → counterparty",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "Uber"},
		Actions:    []service.RuleAction{{Type: "assign_counterparty", CounterpartyShortID: cp.ShortID}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	n, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	if n != 2 {
		t.Errorf("matched = %d, want 2", n)
	}
	if got := countLinkedCounterparty(t, pool, cp.ID); got != 2 {
		t.Errorf("linked members = %d, want 2", got)
	}
	// The non-matching charge stays unbound.
	var cpID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT counterparty_id FROM transactions WHERE id=$1`, other.ID).Scan(&cpID); err != nil {
		t.Fatalf("query other txn: %v", err)
	}
	if cpID.Valid {
		t.Error("non-matching transaction was wrongly bound to a counterparty")
	}
}

// TestApplyRuleRetroactively_AssignCounterparty_FeedSingleEvent is the
// regression test for issue #1915: a retroactive assign_counterparty rule must
// surface as ONE feed event (the rule_applied row), not two. Before the fix the
// rule wrote both a rule_applied row (attributed to the rule) AND a
// counterparty_assigned side-effect row (attributed to SystemActor "Breadbox"),
// and the latter escaped the rule-source dedup — so the feed showed the same
// retroactive application twice ("… ran a rule …" + "Breadbox assigned …").
func TestApplyRuleRetroactively_AssignCounterparty_FeedSingleEvent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	// Three charges so the bucket clears the default bulk-action threshold.
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 1", "Netflix", 1599, "2026-03-15")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 2", "Netflix", 1599, "2026-04-15")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 3", "Netflix", 1599, "2026-05-15")
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	cp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{Name: "Netflix", CreateIfMissing: true}, actor)
	if err != nil {
		t.Fatalf("create counterparty: %v", err)
	}
	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Netflix Counterparty",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "Netflix"},
		Actions:    []service.RuleAction{{Type: "assign_counterparty", CounterpartyShortID: cp.ShortID}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := svc.ApplyRuleRetroactively(ctx, rule.ID); err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}

	events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
	if err != nil {
		t.Fatalf("ListFeedEvents: %v", err)
	}

	// Exactly one bulk_action event survives — the rule_applied bucket. The
	// counterparty_assigned side-effect (system "Breadbox" actor) is deduped.
	bulk := countEventsByType(events, "bulk_action")
	if bulk != 1 {
		for _, ev := range events {
			if ev.Type == "bulk_action" {
				t.Logf("bulk_action kind=%q actor=%q", ev.BulkAction.Kind, ev.BulkAction.ActorName)
			}
		}
		t.Fatalf("bulk_action events = %d, want 1 (rule_applied only; membership side-effect deduped)", bulk)
	}
	ev := findEventByType(events, "bulk_action").BulkAction
	if ev.Kind != "rule_applied" {
		t.Errorf("surviving bulk_action kind = %q, want rule_applied", ev.Kind)
	}
	if ev.ActorName == "Breadbox" {
		t.Errorf("surviving event attributed to %q — the system membership row should have been deduped", ev.ActorName)
	}
}

// TestApplyRuleRetroactively_AssignCounterpartyByName covers resolve-or-create by
// name on the single-rule retroactive path.
func TestApplyRuleRetroactively_AssignCounterpartyByName(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 1", "Netflix", 1599, "2026-03-15")
	testutil.MustCreateTransaction(t, queries, acctID, "NETFLIX.COM 2", "Netflix", 1599, "2026-04-15")
	actor := service.Actor{Type: "user", ID: "u1", Name: "Tester"}

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Netflix → counterparty",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "Netflix"},
		Actions:    []service.RuleAction{{Type: "assign_counterparty", CounterpartyName: "Netflix", CreateIfMissing: true}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := svc.ApplyRuleRetroactively(ctx, rule.ID); err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}

	cp := findCounterpartyByName(t, svc, "Netflix")
	if cp == nil {
		t.Fatal("retroactive assign_counterparty did not create a Netflix counterparty")
	}
	if got := countLinkedCounterparty(t, pool, cp.ID); got != 2 {
		t.Errorf("linked members = %d, want 2", got)
	}
}

// TestApplyAllRulesRetroactively_AssignCounterparty covers the BULK retroactive
// path (the one that historically dropped assign_series) materializing
// assign_counterparty.
func TestApplyAllRulesRetroactively_AssignCounterparty(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	testutil.MustCreateTransaction(t, queries, acctID, "DOORDASH 1", "DoorDash", 2599, "2026-03-15")
	testutil.MustCreateTransaction(t, queries, acctID, "DOORDASH 2", "DoorDash", 1899, "2026-04-15")
	actor := service.Actor{Type: "user", Name: "Tester"}

	if _, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "DoorDash → counterparty",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "DoorDash"},
		Actions:    []service.RuleAction{{Type: "assign_counterparty", CounterpartyName: "DoorDash", CreateIfMissing: true}},
		Actor:      actor,
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	if _, err := svc.ApplyAllRulesRetroactively(ctx); err != nil {
		t.Fatalf("ApplyAllRulesRetroactively: %v", err)
	}
	cp := findCounterpartyByName(t, svc, "DoorDash")
	if cp == nil {
		t.Fatal("bulk retroactive assign_counterparty did not create a DoorDash counterparty")
	}
	if got := countLinkedCounterparty(t, pool, cp.ID); got != 2 {
		t.Errorf("bulk path linked members = %d, want 2", got)
	}
}

// TestListCounterpartyGoverningRules returns the assign_counterparty rules whose
// target is this counterparty — by short_id or by name.
func TestListCounterpartyGoverningRules(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	actor := service.Actor{Type: "user", Name: "Tester"}

	cp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{Name: "Venmo", CreateIfMissing: true}, actor)
	if err != nil {
		t.Fatalf("create counterparty: %v", err)
	}

	byID, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "By short id",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "VENMO"},
		Actions:    []service.RuleAction{{Type: "assign_counterparty", CounterpartyShortID: cp.ShortID}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create rule A: %v", err)
	}
	byName, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "By name",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "VENMO PAY"},
		Actions:    []service.RuleAction{{Type: "assign_counterparty", CounterpartyName: "Venmo", CreateIfMissing: true}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create rule B: %v", err)
	}
	if _, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Unrelated",
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "Spotify"},
		Actions:    []service.RuleAction{{Type: "assign_counterparty", CounterpartyName: "Spotify", CreateIfMissing: true}},
		Actor:      actor,
	}); err != nil {
		t.Fatalf("create unrelated rule: %v", err)
	}

	governing, err := svc.ListCounterpartyGoverningRules(ctx, cp.ShortID)
	if err != nil {
		t.Fatalf("ListCounterpartyGoverningRules: %v", err)
	}
	got := map[string]bool{}
	for _, r := range governing {
		got[r.ID] = true
	}
	if !got[byID.ID] || !got[byName.ID] {
		t.Errorf("governing rules missing expected entries: %+v", got)
	}
	if len(governing) != 2 {
		t.Errorf("governing rules = %d, want 2 (unrelated excluded)", len(governing))
	}
}

// TestCounterpartyCascadeDelete_NullsCounterpartyID confirms a hard delete nulls
// members' counterparty_id (ON DELETE SET NULL).
func TestCounterpartyCascadeDelete_NullsCounterpartyID(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	_, members := seedCounterpartyCharges(t, queries, "VENMO", []string{"2026-02-15", "2026-03-15"})
	actor := service.Actor{Type: "user", Name: "Tester"}

	cp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name: "Venmo", CreateIfMissing: true, TransactionIDs: shortIDs(members),
	}, actor)
	if err != nil {
		t.Fatalf("create counterparty: %v", err)
	}
	if n := countLinkedCounterparty(t, pool, cp.ID); n != 2 {
		t.Fatalf("precondition: linked = %d, want 2", n)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM counterparties WHERE id = $1`, cp.ID); err != nil {
		t.Fatalf("delete counterparty: %v", err)
	}
	var nulled int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions WHERE counterparty_id IS NULL AND short_id = ANY($1)`,
		shortIDs(members)).Scan(&nulled); err != nil {
		t.Fatalf("count nulled: %v", err)
	}
	if nulled != 2 {
		t.Errorf("expected both members' counterparty_id nulled, got %d", nulled)
	}
}

// TestRuleConditions_CounterpartyAndHasCounterparty exercises the condition
// read-half: rules conditioned on has_counterparty / a specific counterparty
// match only the bound transactions when applied retroactively.
func TestRuleConditions_CounterpartyAndHasCounterparty(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	testutil.MustCreateTag(t, queries, "is-bound", "Is bound")
	testutil.MustCreateTag(t, queries, "is-venmo", "Is Venmo")
	acctID := seedTxnFixture(t, queries)
	v1 := testutil.MustCreateTransaction(t, queries, acctID, "VENMO 1", "Venmo", 1599, "2026-03-15")
	v2 := testutil.MustCreateTransaction(t, queries, acctID, "VENMO 2", "Venmo", 1599, "2026-04-15")
	loose := testutil.MustCreateTransaction(t, queries, acctID, "STARBUCKS", "Starbucks", 599, "2026-04-16")
	actor := service.Actor{Type: "user", Name: "Tester"}

	cp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name: "Venmo", CreateIfMissing: true, TransactionIDs: []string{v1.ShortID, v2.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("assign counterparty: %v", err)
	}

	hasRule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Bound get a tag",
		Conditions: service.Condition{Field: "has_counterparty", Op: "eq", Value: true},
		Actions:    []service.RuleAction{{Type: "add_tag", TagSlug: "is-bound"}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create has_counterparty rule: %v", err)
	}
	matched, err := svc.ApplyRuleRetroactively(ctx, hasRule.ID)
	if err != nil {
		t.Fatalf("apply has_counterparty rule: %v", err)
	}
	if matched != 2 {
		t.Errorf("has_counterparty matched = %d, want 2", matched)
	}
	if !txnHasTag(t, pool, v1.ID, "is-bound") || !txnHasTag(t, pool, v2.ID, "is-bound") {
		t.Error("expected both bound charges to receive is-bound")
	}
	if txnHasTag(t, pool, loose.ID, "is-bound") {
		t.Error("unbound transaction was wrongly tagged by a has_counterparty rule")
	}

	cpRule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Venmo counterparty tag",
		Conditions: service.Condition{Field: "counterparty", Op: "eq", Value: cp.ShortID},
		Actions:    []service.RuleAction{{Type: "add_tag", TagSlug: "is-venmo"}},
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("create counterparty rule: %v", err)
	}
	matched, err = svc.ApplyRuleRetroactively(ctx, cpRule.ID)
	if err != nil {
		t.Fatalf("apply counterparty rule: %v", err)
	}
	if matched != 2 {
		t.Errorf("counterparty eq matched = %d, want 2", matched)
	}
	if !txnHasTag(t, pool, v1.ID, "is-venmo") {
		t.Error("expected bound charge to receive is-venmo")
	}
	if txnHasTag(t, pool, loose.ID, "is-venmo") {
		t.Error("unbound transaction was wrongly tagged by a counterparty eq rule")
	}
}

// TestValidateActions_AssignCounterparty validates the write-time action checks.
func TestValidateActions_AssignCounterparty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if err := svc.ValidateActions(ctx, []service.RuleAction{
		{Type: "assign_counterparty", CounterpartyName: "Venmo"},
	}); err == nil {
		t.Error("expected error for counterparty_name without create_if_missing")
	}
	if err := svc.ValidateActions(ctx, []service.RuleAction{
		{Type: "assign_counterparty", CounterpartyShortID: "cp123abc", CounterpartyName: "Venmo", CreateIfMissing: true},
	}); err == nil {
		t.Error("expected error when both counterparty_short_id and counterparty_name set")
	}
	if err := svc.ValidateActions(ctx, []service.RuleAction{
		{Type: "assign_counterparty"},
	}); err == nil {
		t.Error("expected error when neither counterparty_short_id nor counterparty_name set")
	}
	if err := svc.ValidateActions(ctx, []service.RuleAction{
		{Type: "assign_counterparty", CounterpartyShortID: "nope0000"},
	}); err == nil {
		t.Error("expected error for unknown counterparty_short_id")
	}
	if err := svc.ValidateActions(ctx, []service.RuleAction{
		{Type: "assign_counterparty", CounterpartyName: "Venmo", CreateIfMissing: true},
	}); err != nil {
		t.Errorf("expected valid name+create_if_missing action, got %v", err)
	}
}

// findCounterpartyByName returns the live counterparty with the given name, or nil.
func findCounterpartyByName(t *testing.T, svc *service.Service, name string) *service.CounterpartyResponse {
	t.Helper()
	all, err := svc.ListCounterparties(context.Background())
	if err != nil {
		t.Fatalf("list counterparties: %v", err)
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}
