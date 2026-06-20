//go:build integration && !lite

package service_test

import (
	"context"
	"testing"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestReadModel_ListTransactionsSurfacesCounterparty asserts the
// LEFT JOIN counterparties in ListTransactions / GetTransaction surfaces the
// assigned counterparty's name+short_id, and leaves it nil for unbound rows.
func TestReadModel_ListTransactionsSurfacesCounterparty(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	bound := testutil.MustCreateTransaction(t, queries, acctID, "VENMO PAYMENT", "Venmo", 1599, "2026-03-15")
	loose := testutil.MustCreateTransaction(t, queries, acctID, "STARBUCKS", "Starbucks", 599, "2026-03-16")
	actor := service.Actor{Type: "user", Name: "Tester"}

	cp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name: "Venmo", CreateIfMissing: true, TransactionIDs: []string{bound.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("assign counterparty: %v", err)
	}

	res, err := svc.ListTransactions(ctx, service.TransactionListParams{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	var sawBound, sawLoose bool
	for _, tx := range res.Transactions {
		switch tx.ShortID {
		case bound.ShortID:
			sawBound = true
			if tx.CounterpartyName == nil || *tx.CounterpartyName != "Venmo" {
				t.Errorf("bound txn CounterpartyName = %v, want Venmo", tx.CounterpartyName)
			}
			if tx.CounterpartyShortID == nil || *tx.CounterpartyShortID != cp.ShortID {
				t.Errorf("bound txn CounterpartyShortID = %v, want %s", tx.CounterpartyShortID, cp.ShortID)
			}
		case loose.ShortID:
			sawLoose = true
			if tx.CounterpartyName != nil {
				t.Errorf("loose txn CounterpartyName = %v, want nil", tx.CounterpartyName)
			}
		}
	}
	if !sawBound || !sawLoose {
		t.Fatalf("did not see both txns in list (bound=%v loose=%v)", sawBound, sawLoose)
	}

	// GetTransaction (single detail) surfaces the same join.
	got, err := svc.GetTransaction(ctx, bound.ShortID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.CounterpartyName == nil || *got.CounterpartyName != "Venmo" {
		t.Errorf("GetTransaction CounterpartyName = %v, want Venmo", got.CounterpartyName)
	}
}

// TestReadModel_MerchantSummaryGroupsByCounterparty asserts the ListMerchants
// rollup collapses bound charges under COALESCE(cp.name, ...) and exposes the
// counterparty short_id on the group.
func TestReadModel_MerchantSummaryGroupsByCounterparty(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	// Two charges with DIFFERENT raw provider names that both map to one
	// counterparty — the rollup must collapse them into a single group keyed
	// by the counterparty name (not the raw provider strings).
	d1 := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	d2 := time.Now().AddDate(0, 0, -4).Format("2006-01-02")
	a := testutil.MustCreateTransaction(t, queries, acctID, "VENMO PAYMENT 1", "Venmo *Alice", 1500, d1)
	b := testutil.MustCreateTransaction(t, queries, acctID, "VENMO PAYMENT 2", "Venmo *Bob", 2500, d2)
	actor := service.Actor{Type: "user", Name: "Tester"}

	cp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
		Name: "Venmo", CreateIfMissing: true, TransactionIDs: []string{a.ShortID, b.ShortID},
	}, actor)
	if err != nil {
		t.Fatalf("assign counterparty: %v", err)
	}

	res, err := svc.GetMerchantSummary(ctx, service.MerchantSummaryParams{})
	if err != nil {
		t.Fatalf("GetMerchantSummary: %v", err)
	}
	var found *service.MerchantSummaryRow
	for i := range res.Merchants {
		if res.Merchants[i].Merchant == "Venmo" {
			found = &res.Merchants[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("merchant rollup did not contain a 'Venmo' counterparty group: %+v", res.Merchants)
	}
	if found.TransactionCount != 2 {
		t.Errorf("Venmo group count = %d, want 2 (both raw provider names collapsed)", found.TransactionCount)
	}
	if found.CounterpartyShortID == nil || *found.CounterpartyShortID != cp.ShortID {
		t.Errorf("Venmo group CounterpartyShortID = %v, want %s", found.CounterpartyShortID, cp.ShortID)
	}
}
