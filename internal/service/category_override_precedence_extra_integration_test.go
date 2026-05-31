//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// T13 — extra precedence-ladder cases for category_override (user > agent > rule).
//
// These tests complement TestCategoryOverridePrecedence in
// category_override_precedence_integration_test.go with edge cases not covered
// there. Each test is independently named with the "T13" prefix to avoid
// collisions with sibling tests that run in the same DB.
//
// Coverage:
//   1. Rule skips 'agent'-level rows — rules write only where override='none'.
//   2. Rule skips 'user'-locked rows — set_category UPDATE is gated on 'none'.
//   3. Agent batch on multiple 'none' rows — each op lands with status=ok.
//   4. Skipped agent write in abort-mode batch — skip != error; batch continues.
//   5. Unlock reopens a user-locked row to agent writes.
//   6. Hit count counts ALL condition matches regardless of override level.

// TestT13_ruleSkipsAgentRow proves that ApplyRuleRetroactively does NOT
// overwrite a row at category_override='agent'. The set_category UPDATE in the
// retroactive pipeline filters WHERE category_override='none' only.
func TestT13_ruleSkipsAgentRow(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	// Build fixture: user -> connection -> account with unique external IDs.
	user := testutil.MustCreateUser(t, queries, "T13rsa-user")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "T13rsa-conn-1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "T13rsa-acct-1", "T13 Checking")
	acctID := acct.ID

	testutil.MustCreateCategory(t, queries, "t13rsa_agent_cat", "T13 Agent Cat")
	testutil.MustCreateCategory(t, queries, "t13rsa_rule_cat", "T13 Rule Cat")

	// Two transactions with the same provider-name prefix.
	txnAgent := testutil.MustCreateTransaction(t, queries, acctID, "t13rsa_agent_row", "Bean & Brew", 600, "2026-04-01")
	txnNone := testutil.MustCreateTransaction(t, queries, acctID, "t13rsa_none_row", "Bean & Brew Fresh", 700, "2026-04-02")

	// Stamp the first transaction as agent-level override via UpdateTransactions.
	agent := service.Actor{Type: "agent", ID: "t13rsa-agent", Name: "T13rsa Agent"}
	res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txnAgent.ShortID, CategorySlug: strPtr("t13rsa_agent_cat")},
		},
		Actor: agent,
	})
	if err != nil || res[0].Status != "ok" {
		t.Fatalf("stamp agent: err=%v status=%q", err, res[0].Status)
	}

	// Create a rule that matches both transactions by name substring.
	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "T13rsa Rule",
		Conditions: service.Condition{
			Field: "provider_name",
			Op:    "contains",
			Value: "Bean & Brew",
		},
		CategorySlug: "t13rsa_rule_cat",
		Priority:     10,
		Actor:        service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	count, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	// Both rows match the condition (hit-count semantics: count ALL matches).
	if count != 2 {
		t.Errorf("hit count=%d, want 2 (both condition matches)", count)
	}

	// The agent-level row must NOT have been re-categorised by the rule.
	var agentSlug, agentOverride string
	if err := pool.QueryRow(ctx, `
		SELECT c.slug, t.category_override
		FROM transactions t JOIN categories c ON c.id = t.category_id
		WHERE t.provider_transaction_id = 't13rsa_agent_row'
	`).Scan(&agentSlug, &agentOverride); err != nil {
		t.Fatalf("query agent row: %v", err)
	}
	if agentSlug != "t13rsa_agent_cat" {
		t.Errorf("agent row category=%q, want t13rsa_agent_cat (rule must not overwrite agent override)", agentSlug)
	}
	if agentOverride != service.CategoryOverrideAgent {
		t.Errorf("agent row override=%q, want agent", agentOverride)
	}

	// The 'none' row must have picked up the rule category.
	var noneSlug string
	if err := pool.QueryRow(ctx, `
		SELECT c.slug FROM transactions t JOIN categories c ON c.id = t.category_id
		WHERE t.provider_transaction_id = 't13rsa_none_row'
	`).Scan(&noneSlug); err != nil {
		t.Fatalf("query none row: %v", err)
	}
	if noneSlug != "t13rsa_rule_cat" {
		t.Errorf("none row category=%q, want t13rsa_rule_cat", noneSlug)
	}
	_ = txnNone
}

// TestT13_ruleSkipsUserRow proves that a rule never overwrites a 'user'-locked
// row. The set_category UPDATE in ApplyRuleRetroactively is gated on
// category_override='none'.
func TestT13_ruleSkipsUserRow(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	u := testutil.MustCreateUser(t, queries, "T13rsu-user")
	c := testutil.MustCreateConnection(t, queries, u.ID, "T13rsu-conn-1")
	a := testutil.MustCreateAccount(t, queries, c.ID, "T13rsu-acct-1", "T13 Savings")
	acctID := a.ID

	testutil.MustCreateCategory(t, queries, "t13rsu_user_cat", "T13 User Cat")
	testutil.MustCreateCategory(t, queries, "t13rsu_rule_cat", "T13 Rule Cat 2")

	txnUser := testutil.MustCreateTransaction(t, queries, acctID, "t13rsu_user_row", "Trader Joes", 3000, "2026-04-01")
	txnNone := testutil.MustCreateTransaction(t, queries, acctID, "t13rsu_none_row", "Trader Joes Express", 500, "2026-04-02")

	// Lock the first transaction at user level.
	userActor := service.Actor{Type: "user", ID: "t13rsu-user", Name: "T13rsu User"}
	res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txnUser.ShortID, CategorySlug: strPtr("t13rsu_user_cat")},
		},
		Actor: userActor,
	})
	if err != nil || res[0].Status != "ok" {
		t.Fatalf("user lock: err=%v status=%q", err, res[0].Status)
	}

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "T13rsu Rule",
		Conditions: service.Condition{
			Field: "provider_name",
			Op:    "contains",
			Value: "Trader Joes",
		},
		CategorySlug: "t13rsu_rule_cat",
		Priority:     10,
		Actor:        service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	count, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	// Both rows match the condition; hit count = 2.
	if count != 2 {
		t.Errorf("hit count=%d, want 2", count)
	}

	// User-locked row must keep its user category and 'user' override.
	var userSlug, userOverride string
	if err := pool.QueryRow(ctx, `
		SELECT c.slug, t.category_override
		FROM transactions t JOIN categories c ON c.id = t.category_id
		WHERE t.provider_transaction_id = 't13rsu_user_row'
	`).Scan(&userSlug, &userOverride); err != nil {
		t.Fatalf("query user row: %v", err)
	}
	if userSlug != "t13rsu_user_cat" {
		t.Errorf("user row category=%q, want t13rsu_user_cat (rule must not overwrite user lock)", userSlug)
	}
	if userOverride != service.CategoryOverrideUser {
		t.Errorf("user row override=%q, want user", userOverride)
	}

	// The 'none' row must have the rule category.
	var noneSlug string
	if err := pool.QueryRow(ctx, `
		SELECT c.slug FROM transactions t JOIN categories c ON c.id = t.category_id
		WHERE t.provider_transaction_id = 't13rsu_none_row'
	`).Scan(&noneSlug); err != nil {
		t.Fatalf("query none row: %v", err)
	}
	if noneSlug != "t13rsu_rule_cat" {
		t.Errorf("none row category=%q, want t13rsu_rule_cat", noneSlug)
	}
	_ = txnNone
}

// TestT13_agentBatchOnMultipleNoneRows confirms that an agent batch on several
// 'none' rows all succeed with status=ok; each row lands at override=agent.
func TestT13_agentBatchOnMultipleNoneRows(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	u := testutil.MustCreateUser(t, queries, "T13ab-user")
	c := testutil.MustCreateConnection(t, queries, u.ID, "T13ab-conn-1")
	a := testutil.MustCreateAccount(t, queries, c.ID, "T13ab-acct-1", "T13 Batch Checking")
	acctID := a.ID

	testutil.MustCreateCategory(t, queries, "t13ab_batch_cat", "T13 Batch Cat")

	txn1 := testutil.MustCreateTransaction(t, queries, acctID, "t13ab_txn1", "Lyft A", 1000, "2026-04-01")
	txn2 := testutil.MustCreateTransaction(t, queries, acctID, "t13ab_txn2", "Lyft B", 1200, "2026-04-02")
	txn3 := testutil.MustCreateTransaction(t, queries, acctID, "t13ab_txn3", "Lyft C", 900, "2026-04-03")

	agent := service.Actor{Type: "agent", ID: "t13ab-agent", Name: "T13 Batch Agent"}
	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txn1.ShortID, CategorySlug: strPtr("t13ab_batch_cat")},
			{TransactionID: txn2.ShortID, CategorySlug: strPtr("t13ab_batch_cat")},
			{TransactionID: txn3.ShortID, CategorySlug: strPtr("t13ab_batch_cat")},
		},
		Actor: agent,
	})
	if err != nil {
		t.Fatalf("UpdateTransactions batch: %v", err)
	}
	for i, r := range results {
		if r.Status != "ok" {
			t.Errorf("op %d: status=%q, want ok", i, r.Status)
		}
	}

	// Verify all three rows are at agent level.
	for _, pair := range []struct {
		shortID string
		extID   string
	}{
		{txn1.ShortID, "t13ab_txn1"},
		{txn2.ShortID, "t13ab_txn2"},
		{txn3.ShortID, "t13ab_txn3"},
	} {
		got, err := svc.GetTransaction(ctx, pair.shortID)
		if err != nil {
			t.Fatalf("GetTransaction %s: %v", pair.extID, err)
		}
		if got.CategoryOverride != service.CategoryOverrideAgent {
			t.Errorf("%s: override=%q, want agent", pair.extID, got.CategoryOverride)
		}
	}
}

// TestT13_agentSkippedInAbortBatch verifies that a skipped agent write (blocked
// by a user lock) inside an abort-mode batch does NOT trigger the rollback path.
// A skip is a normal outcome, not an error. Tags on the skipped op still apply.
func TestT13_agentSkippedInAbortBatch(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	u := testutil.MustCreateUser(t, queries, "T13asab-user")
	conn := testutil.MustCreateConnection(t, queries, u.ID, "T13asab-conn-1")
	a := testutil.MustCreateAccount(t, queries, conn.ID, "T13asab-acct-1", "T13 Abort Checking")
	acctID := a.ID

	testutil.MustCreateCategory(t, queries, "t13asab_user_cat", "T13 Abort User Cat")
	testutil.MustCreateCategory(t, queries, "t13asab_agent_cat", "T13 Abort Agent Cat")

	txnLocked := testutil.MustCreateTransaction(t, queries, acctID, "t13asab_locked", "Amazon Prime", 1399, "2026-04-01")
	txnFree := testutil.MustCreateTransaction(t, queries, acctID, "t13asab_free", "Amazon Fresh", 2599, "2026-04-02")

	// Lock the first transaction at user level.
	userActor := service.Actor{Type: "user", ID: "t13asab-user", Name: "T13asab User"}
	res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txnLocked.ShortID, CategorySlug: strPtr("t13asab_user_cat")},
		},
		Actor: userActor,
	})
	if err != nil || res[0].Status != "ok" {
		t.Fatalf("user lock: err=%v status=%q", err, res[0].Status)
	}

	// Agent abort-mode batch:
	//   op[0]: tries to overwrite user lock -> skipped (NOT an error; batch continues).
	//   op[1]: writes the free row -> ok.
	agentActor := service.Actor{Type: "agent", ID: "t13asab-agent", Name: "T13asab Agent"}
	results, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{
				TransactionID: txnLocked.ShortID,
				CategorySlug:  strPtr("t13asab_agent_cat"),
				TagsToAdd:     []service.UpdateTransactionsTagOp{{Slug: "t13asab-lock-tag"}},
			},
			{
				TransactionID: txnFree.ShortID,
				CategorySlug:  strPtr("t13asab_agent_cat"),
			},
		},
		OnError: "abort",
		Actor:   agentActor,
	})
	if err != nil {
		t.Fatalf("abort-mode batch: unexpected error=%v (skip must not trigger abort)", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// op[0]: skipped (agent blocked by user lock).
	if results[0].Status != "skipped" {
		t.Errorf("op[0] status=%q, want skipped", results[0].Status)
	}

	// op[1]: ok (free row).
	if results[1].Status != "ok" {
		t.Errorf("op[1] status=%q, want ok", results[1].Status)
	}

	// The locked row must still carry the user category.
	lockedGot, err := svc.GetTransaction(ctx, txnLocked.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction locked: %v", err)
	}
	if lockedGot.CategoryOverride != service.CategoryOverrideUser {
		t.Errorf("locked override=%q, want user", lockedGot.CategoryOverride)
	}
	if lockedGot.Category == nil || lockedGot.Category.Slug == nil || *lockedGot.Category.Slug != "t13asab_user_cat" {
		t.Errorf("locked category=%v, want t13asab_user_cat", lockedGot.Category)
	}

	// Tag from the skipped op must have been applied (tags apply even on skips).
	var tagFound bool
	for _, tg := range lockedGot.Tags {
		if tg == "t13asab-lock-tag" {
			tagFound = true
		}
	}
	if !tagFound {
		t.Errorf("tag t13asab-lock-tag missing from skipped op; tags=%v", lockedGot.Tags)
	}

	// The free row must be agent-categorised.
	freeGot, err := svc.GetTransaction(ctx, txnFree.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction free: %v", err)
	}
	if freeGot.CategoryOverride != service.CategoryOverrideAgent {
		t.Errorf("free override=%q, want agent", freeGot.CategoryOverride)
	}
}

// TestT13_unlockReopensRowToAgent validates that unlocking a user-locked row
// (SetCategoryOverrideFlag false) resets category_override to 'none', which
// lets a subsequent agent write succeed.
func TestT13_unlockReopensRowToAgent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	u := testutil.MustCreateUser(t, queries, "T13urra-user")
	c := testutil.MustCreateConnection(t, queries, u.ID, "T13urra-conn-1")
	a := testutil.MustCreateAccount(t, queries, c.ID, "T13urra-acct-1", "T13 Unlock Checking")
	acctID := a.ID

	testutil.MustCreateCategory(t, queries, "t13urra_user_cat", "T13 Unlock User Cat")
	testutil.MustCreateCategory(t, queries, "t13urra_agent_cat", "T13 Unlock Agent Cat")

	txn := testutil.MustCreateTransaction(t, queries, acctID, "t13urra_txn", "Netflix", 1599, "2026-04-01")

	userActor := service.Actor{Type: "user", ID: "t13urra-user", Name: "T13urra User"}
	agentActor := service.Actor{Type: "agent", ID: "t13urra-agent", Name: "T13urra Agent"}

	// Step 1: User locks the row -> override=user.
	if res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txn.ShortID, CategorySlug: strPtr("t13urra_user_cat")},
		},
		Actor: userActor,
	}); err != nil || res[0].Status != "ok" {
		t.Fatalf("user lock: err=%v status=%q", err, res[0].Status)
	}
	if got := mustOverride(t, svc, txn.ShortID); got != service.CategoryOverrideUser {
		t.Fatalf("after user lock: override=%q, want user", got)
	}

	// Step 2: Agent write is blocked (user-locked) -> skipped.
	if res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txn.ShortID, CategorySlug: strPtr("t13urra_agent_cat")},
		},
		Actor: agentActor,
	}); err != nil || res[0].Status != "skipped" {
		t.Fatalf("pre-unlock agent write: err=%v status=%q, want skipped", err, res[0].Status)
	}

	// Step 3: Unlock (binary flag -> none).
	if err := svc.SetCategoryOverrideFlag(ctx, txn.ShortID, false, service.SystemActor()); err != nil {
		t.Fatalf("SetCategoryOverrideFlag(false): %v", err)
	}
	if got := mustOverride(t, svc, txn.ShortID); got != service.CategoryOverrideNone {
		t.Fatalf("after unlock: override=%q, want none", got)
	}

	// Step 4: Agent write now succeeds on the 'none' row.
	if res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txn.ShortID, CategorySlug: strPtr("t13urra_agent_cat")},
		},
		Actor: agentActor,
	}); err != nil || res[0].Status != "ok" {
		t.Fatalf("post-unlock agent write: err=%v status=%q, want ok", err, res[0].Status)
	}
	if got := mustOverride(t, svc, txn.ShortID); got != service.CategoryOverrideAgent {
		t.Errorf("after post-unlock agent write: override=%q, want agent", got)
	}
}

// TestT13_agentCannotOverwriteUserLock is the focused assertion of the most
// critical invariant: a user-locked row (override='user') is sacred -- an agent
// write is skipped and the category remains unchanged.
func TestT13_agentCannotOverwriteUserLock(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	u := testutil.MustCreateUser(t, queries, "T13acoul-user")
	c := testutil.MustCreateConnection(t, queries, u.ID, "T13acoul-conn-1")
	a := testutil.MustCreateAccount(t, queries, c.ID, "T13acoul-acct-1", "T13 Sacred Checking")
	acctID := a.ID

	testutil.MustCreateCategory(t, queries, "t13acoul_user_cat", "T13 Sacred User Cat")
	testutil.MustCreateCategory(t, queries, "t13acoul_agent_cat", "T13 Sacred Agent Cat")

	txn := testutil.MustCreateTransaction(t, queries, acctID, "t13acoul_txn", "Spotify", 999, "2026-04-01")

	userActor := service.Actor{Type: "user", ID: "t13acoul-user", Name: "T13acoul User"}
	agentActor := service.Actor{Type: "agent", ID: "t13acoul-agent", Name: "T13acoul Agent"}

	// User locks the row.
	if res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txn.ShortID, CategorySlug: strPtr("t13acoul_user_cat")},
		},
		Actor: userActor,
	}); err != nil || res[0].Status != "ok" {
		t.Fatalf("user lock: err=%v status=%q", err, res[0].Status)
	}

	// Agent attempts to overwrite -- must be skipped.
	res, err := svc.UpdateTransactions(ctx, service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{
			{TransactionID: txn.ShortID, CategorySlug: strPtr("t13acoul_agent_cat")},
		},
		Actor: agentActor,
	})
	if err != nil {
		t.Fatalf("agent write: unexpected error=%v", err)
	}
	if res[0].Status != "skipped" {
		t.Errorf("status=%q, want skipped -- user lock must be sacred", res[0].Status)
	}

	// Category and override must be unchanged.
	got, err := svc.GetTransaction(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.CategoryOverride != service.CategoryOverrideUser {
		t.Errorf("override=%q, want user (sacred lock)", got.CategoryOverride)
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != "t13acoul_user_cat" {
		t.Errorf("category=%v, want t13acoul_user_cat", got.Category)
	}
}

// TestT13_ruleHitCountAllMatchesRegardlessOfOverride verifies that
// ApplyRuleRetroactively counts ALL condition matches (including user-locked
// rows) in the returned hit count. This is the sync-parity contract: the
// counter reflects "how often this rule matches" not "how many rows changed."
func TestT13_ruleHitCountAllMatchesRegardlessOfOverride(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	u := testutil.MustCreateUser(t, queries, "T13hc-user")
	c := testutil.MustCreateConnection(t, queries, u.ID, "T13hc-conn-1")
	a := testutil.MustCreateAccount(t, queries, c.ID, "T13hc-acct-1", "T13 HitCount Checking")
	acctID := a.ID

	testutil.MustCreateCategory(t, queries, "t13hc_user_cat", "T13 HitCount User Cat")
	testutil.MustCreateCategory(t, queries, "t13hc_rule_cat", "T13 HitCount Rule Cat")

	// Two transactions with exactly the same provider name -- one will be
	// user-locked, the other stays at 'none'.
	txnLocked := testutil.MustCreateTransaction(t, queries, acctID, "t13hc_locked", "Whole Foods Market", 4500, "2026-04-01")
	testutil.MustCreateTransaction(t, queries, acctID, "t13hc_free", "Whole Foods Market", 6200, "2026-04-02")

	// Lock the first row directly via pool (mirrors the approach in
	// TestApplyRuleRetroactively_MatchesAndSkipsOverrides).
	userCatRow, err := queries.GetCategoryBySlug(ctx, "t13hc_user_cat")
	if err != nil {
		t.Fatalf("GetCategoryBySlug: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE transactions SET category_id = $1, category_override = 'user' WHERE id = $2",
		userCatRow.ID, txnLocked.ID,
	); err != nil {
		t.Fatalf("lock user row: %v", err)
	}

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "T13hc Rule",
		Conditions: service.Condition{
			Field: "provider_name",
			Op:    "eq",
			Value: "Whole Foods Market",
		},
		CategorySlug: "t13hc_rule_cat",
		Priority:     10,
		Actor:        service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	count, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	// Both rows match the condition, so hit count must be 2 (override level ignored).
	if count != 2 {
		t.Errorf("hit count=%d, want 2 (both condition matches regardless of override)", count)
	}

	// User-locked row must still have the user category.
	var lockedSlug, lockedOverride string
	if err := pool.QueryRow(ctx, `
		SELECT c.slug, t.category_override
		FROM transactions t JOIN categories c ON c.id = t.category_id
		WHERE t.provider_transaction_id = 't13hc_locked'
	`).Scan(&lockedSlug, &lockedOverride); err != nil {
		t.Fatalf("query locked row: %v", err)
	}
	if lockedSlug != "t13hc_user_cat" {
		t.Errorf("locked row category=%q, want t13hc_user_cat (rule must not overwrite user lock)", lockedSlug)
	}
	if lockedOverride != service.CategoryOverrideUser {
		t.Errorf("locked row override=%q, want user", lockedOverride)
	}

	// Free row must have the rule category.
	var freeSlug string
	if err := pool.QueryRow(ctx, `
		SELECT c.slug FROM transactions t JOIN categories c ON c.id = t.category_id
		WHERE t.provider_transaction_id = 't13hc_free'
	`).Scan(&freeSlug); err != nil {
		t.Fatalf("query free row: %v", err)
	}
	if freeSlug != "t13hc_rule_cat" {
		t.Errorf("free row category=%q, want t13hc_rule_cat", freeSlug)
	}
}
