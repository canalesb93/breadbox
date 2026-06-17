//go:build integration && !lite

package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// readMeta loads a transaction's metadata blob as a Go map via the service.
func readMeta(t *testing.T, svc *service.Service, id string) map[string]any {
	t.Helper()
	txn, err := svc.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(txn.Metadata, &m); err != nil {
		t.Fatalf("unmarshal metadata %q: %v", string(txn.Metadata), err)
	}
	return m
}

// TestApplyRuleRetroactively_SetAndRemoveMetadata verifies that a rule's
// set_metadata / remove_metadata actions materialize onto existing transactions
// during retroactive apply, touching only the named keys.
func TestApplyRuleRetroactively_SetAndRemoveMetadata(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "IRS Payment", 5000, "2026-04-15")
	other := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_2", "Grocery Store", 4500, "2026-04-16")
	txnID := pgconv.FormatUUID(txn.ID)

	// Pre-seed a key that remove_metadata should delete, plus a sibling that must survive.
	if err := svc.SetTransactionMetadata(ctx, txnID, "needs_receipt", true); err != nil {
		t.Fatalf("seed needs_receipt: %v", err)
	}
	if err := svc.SetTransactionMetadata(ctx, txnID, "keep_me", "yes"); err != nil {
		t.Fatalf("seed keep_me: %v", err)
	}

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "IRS metadata",
		Actions: []service.RuleAction{
			{Type: "set_metadata", MetadataKey: "tax_deductible", MetadataValue: true},
			{Type: "set_metadata", MetadataKey: "category_hint", MetadataValue: "taxes"},
			{Type: "remove_metadata", MetadataKey: "needs_receipt"},
		},
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "irs"},
		Priority:   10,
		Actor:      service.Actor{Type: "user", Name: "Test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	matched, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	if matched != 1 {
		t.Fatalf("expected 1 matched txn, got %d", matched)
	}

	meta := readMeta(t, svc, txnID)
	if meta["tax_deductible"] != true {
		t.Errorf("tax_deductible: got %v, want true", meta["tax_deductible"])
	}
	if meta["category_hint"] != "taxes" {
		t.Errorf("category_hint: got %v, want taxes", meta["category_hint"])
	}
	if _, ok := meta["needs_receipt"]; ok {
		t.Errorf("needs_receipt should have been removed; meta=%v", meta)
	}
	if meta["keep_me"] != "yes" {
		t.Errorf("keep_me sibling should survive untouched; got %v", meta["keep_me"])
	}

	// The non-matching transaction must be untouched (empty blob).
	otherMeta := readMeta(t, svc, pgconv.FormatUUID(other.ID))
	if len(otherMeta) != 0 {
		t.Errorf("non-matching txn metadata should be empty, got %v", otherMeta)
	}
}

// TestApplyRuleRetroactively_SetThenRemoveSameKeyDeletes verifies last-writer-wins:
// a rule that sets a key and then removes it must delete a pre-existing value,
// not silently revert to it.
func TestApplyRuleRetroactively_SetThenRemoveSameKeyDeletes(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "IRS Payment", 5000, "2026-04-15")
	txnID := pgconv.FormatUUID(txn.ID)

	if err := svc.SetTransactionMetadata(ctx, txnID, "flag", "old"); err != nil {
		t.Fatalf("seed flag: %v", err)
	}

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "Set then remove",
		Actions: []service.RuleAction{
			{Type: "set_metadata", MetadataKey: "flag", MetadataValue: "new"},
			{Type: "remove_metadata", MetadataKey: "flag"},
		},
		Conditions: service.Condition{Field: "provider_name", Op: "contains", Value: "irs"},
		Priority:   10,
		Actor:      service.Actor{Type: "user", Name: "Test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	if _, err := svc.ApplyRuleRetroactively(ctx, rule.ID); err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}

	meta := readMeta(t, svc, txnID)
	if _, ok := meta["flag"]; ok {
		t.Errorf("set-then-remove of a pre-existing key must delete it; meta=%v", meta)
	}
}

// TestApplyRuleRetroactively_MetadataCondition verifies a rule whose condition
// reads metadata.<key> matches existing transactions by their stored metadata.
func TestApplyRuleRetroactively_MetadataCondition(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	match := testutil.MustCreateTransaction(t, queries, acct.ID, "m1", "Anything", 1000, "2026-04-15")
	noMatch := testutil.MustCreateTransaction(t, queries, acct.ID, "m2", "Anything", 1000, "2026-04-16")

	if err := svc.SetTransactionMetadata(ctx, pgconv.FormatUUID(match.ID), "reimbursable", true); err != nil {
		t.Fatalf("seed reimbursable: %v", err)
	}

	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:       "Flag reimbursables",
		Actions:    []service.RuleAction{{Type: "set_metadata", MetadataKey: "expense_report", MetadataValue: "Q2"}},
		Conditions: service.Condition{Field: "metadata.reimbursable", Op: "eq", Value: true},
		Priority:   10,
		Actor:      service.Actor{Type: "user", Name: "Test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	matched, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	if matched != 1 {
		t.Fatalf("expected the metadata condition to match exactly 1 txn, got %d", matched)
	}

	matchMeta := readMeta(t, svc, pgconv.FormatUUID(match.ID))
	if matchMeta["expense_report"] != "Q2" {
		t.Errorf("matched txn should get expense_report=Q2, got %v", matchMeta["expense_report"])
	}
	noMatchMeta := readMeta(t, svc, pgconv.FormatUUID(noMatch.ID))
	if _, ok := noMatchMeta["expense_report"]; ok {
		t.Errorf("non-reimbursable txn should not be touched; meta=%v", noMatchMeta)
	}
}
