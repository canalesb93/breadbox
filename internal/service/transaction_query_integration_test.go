//go:build integration

// Integration tests for transaction query builder, filters, summary, and count.
// Run with: make test-integration
//
// IMPORTANT: Do NOT use t.Parallel() — tests share a database and truncate between runs.
package service_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// --- Transaction Summary: group_by=category ---

func TestGetTransactionSummary_GroupByCategory(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Create category
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "groceries", DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	// Create transactions: 2 with category, 1 without
	upsertTxnWithCategory(t, queries, acctID, "txn_g1", "Whole Foods", 2500, "2025-01-10", groceries.ID)
	upsertTxnWithCategory(t, queries, acctID, "txn_g2", "Trader Joes", 3500, "2025-01-11", groceries.ID)
	testutil.MustCreateTransaction(t, queries, acctID, "txn_nocat", "Random Store", 1000, "2025-01-12")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "category",
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Summary) == 0 {
		t.Fatal("expected at least one summary row")
	}

	// Should have at least a groceries row
	var foundGroceries bool
	for _, row := range result.Summary {
		if row.Category != nil && *row.Category == "Groceries" {
			foundGroceries = true
			if row.TransactionCount != 2 {
				t.Errorf("expected 2 grocery transactions, got %d", row.TransactionCount)
			}
			if row.TotalAmount != 60.0 { // 25 + 35
				t.Errorf("expected total 60.0, got %f", row.TotalAmount)
			}
		}
	}
	if !foundGroceries {
		t.Error("expected to find Groceries in summary")
	}

	// Totals
	if result.Totals.TransactionCount != 3 {
		t.Errorf("expected total count 3, got %d", result.Totals.TransactionCount)
	}
	if result.Totals.TotalAmount == nil {
		t.Error("expected total amount to be set (single currency)")
	}
}

// --- Transaction Summary: group_by=month ---

func TestGetTransactionSummary_GroupByMonth(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Create transactions in different months
	testutil.MustCreateTransaction(t, queries, acctID, "txn_jan1", "Jan Purchase 1", 1000, "2025-01-10")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_jan2", "Jan Purchase 2", 2000, "2025-01-20")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_feb1", "Feb Purchase", 3000, "2025-02-15")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-03-01")
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "month",
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Summary) != 2 {
		t.Fatalf("expected 2 month rows, got %d", len(result.Summary))
	}

	// Newest month first (DESC)
	if result.Summary[0].Period == nil || *result.Summary[0].Period != "2025-02" {
		t.Errorf("expected first row period=2025-02, got %v", result.Summary[0].Period)
	}
	if result.Summary[0].TransactionCount != 1 {
		t.Errorf("expected 1 Feb txn, got %d", result.Summary[0].TransactionCount)
	}
	if result.Summary[1].Period == nil || *result.Summary[1].Period != "2025-01" {
		t.Errorf("expected second row period=2025-01, got %v", result.Summary[1].Period)
	}
	if result.Summary[1].TransactionCount != 2 {
		t.Errorf("expected 2 Jan txns, got %d", result.Summary[1].TransactionCount)
	}
}

// --- Transaction Summary: group_by=day ---

func TestGetTransactionSummary_GroupByDay(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_d1", "Purchase A", 1000, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_d2", "Purchase B", 2000, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_d3", "Purchase C", 3000, "2025-01-16")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "day",
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Summary) != 2 {
		t.Fatalf("expected 2 day rows, got %d", len(result.Summary))
	}

	// Newest day first (DESC)
	if result.Summary[0].Period == nil || *result.Summary[0].Period != "2025-01-16" {
		t.Errorf("expected first row 2025-01-16, got %v", result.Summary[0].Period)
	}
	if result.Summary[0].TotalAmount != 30.0 {
		t.Errorf("expected 30.0 for Jan 16, got %f", result.Summary[0].TotalAmount)
	}
	if result.Summary[1].Period == nil || *result.Summary[1].Period != "2025-01-15" {
		t.Errorf("expected second row 2025-01-15, got %v", result.Summary[1].Period)
	}
	if result.Summary[1].TotalAmount != 30.0 { // 10 + 20
		t.Errorf("expected 30.0 for Jan 15, got %f", result.Summary[1].TotalAmount)
	}
}

// --- Transaction Summary: SpendingOnly filter ---

func TestGetTransactionSummary_SpendingOnly(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Positive amount = debit/spending, negative = credit/income
	testutil.MustCreateTransaction(t, queries, acctID, "txn_spend", "Starbucks", 1500, "2025-01-15")
	upsertTxnWithAmount(t, queries, acctID, "txn_income", "Payroll", -500000, "2025-01-15")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")

	// With SpendingOnly=true, should only see positive amounts
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:      "day",
		StartDate:    &start,
		EndDate:      &end,
		SpendingOnly: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Totals.TransactionCount != 1 {
		t.Errorf("expected 1 spending transaction, got %d", result.Totals.TransactionCount)
	}
	if result.Totals.TotalAmount == nil || *result.Totals.TotalAmount != 15.0 {
		t.Errorf("expected total 15.0, got %v", result.Totals.TotalAmount)
	}

	// Without SpendingOnly, should see both
	result2, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "day",
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.Totals.TransactionCount != 2 {
		t.Errorf("expected 2 total transactions, got %d", result2.Totals.TransactionCount)
	}
}

// --- Transaction Summary: pending filter ---

func TestGetTransactionSummary_ExcludesPendingByDefault(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_posted", "Posted Purchase", 1500, "2025-01-15")
	upsertTxnPending(t, queries, acctID, "txn_pending", "Pending Purchase", 2000, "2025-01-15")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")

	// Default: exclude pending
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "day",
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Totals.TransactionCount != 1 {
		t.Errorf("expected 1 (posted only), got %d", result.Totals.TransactionCount)
	}

	// Include pending
	result2, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:        "day",
		StartDate:      &start,
		EndDate:        &end,
		IncludePending: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.Totals.TransactionCount != 2 {
		t.Errorf("expected 2 (including pending), got %d", result2.Totals.TransactionCount)
	}
}

// --- Transaction Summary: AccountID filter ---

func TestGetTransactionSummary_AccountFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	acct2 := testutil.MustCreateAccount(t, queries, conn.ID, "ext_2", "Savings")

	testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_c1", "Checking Purchase", 1000, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_s1", "Savings Purchase", 2000, "2025-01-15")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	acct1ID := pgconv.FormatUUID(acct1.ID)

	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "day",
		StartDate: &start,
		EndDate:   &end,
		AccountID: &acct1ID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Totals.TransactionCount != 1 {
		t.Errorf("expected 1 (checking only), got %d", result.Totals.TransactionCount)
	}
}

// --- Transaction Summary: UserID filter ---

func TestGetTransactionSummary_UserFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice := testutil.MustCreateUser(t, queries, "Alice")
	bob := testutil.MustCreateUser(t, queries, "Bob")
	connA := testutil.MustCreateConnection(t, queries, alice.ID, "item_a")
	connB := testutil.MustCreateConnection(t, queries, bob.ID, "item_b")
	acctA := testutil.MustCreateAccount(t, queries, connA.ID, "ext_a", "Alice Checking")
	acctB := testutil.MustCreateAccount(t, queries, connB.ID, "ext_b", "Bob Checking")

	testutil.MustCreateTransaction(t, queries, acctA.ID, "txn_alice", "Alice Purchase", 1000, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctB.ID, "txn_bob", "Bob Purchase", 2000, "2025-01-15")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	aliceID := pgconv.FormatUUID(alice.ID)

	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "day",
		StartDate: &start,
		EndDate:   &end,
		UserID:    &aliceID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Totals.TransactionCount != 1 {
		t.Errorf("expected 1 (Alice only), got %d", result.Totals.TransactionCount)
	}
}

// --- Transaction Summary: empty result ---

func TestGetTransactionSummary_EmptyResult(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "category",
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Summary) != 0 {
		t.Errorf("expected empty summary, got %d rows", len(result.Summary))
	}
	if result.Totals.TransactionCount != 0 {
		t.Errorf("expected 0 total count, got %d", result.Totals.TransactionCount)
	}
}

// --- Transaction Summary: group_by=category_month ---

func TestGetTransactionSummary_GroupByCategoryMonth(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "groceries", DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	upsertTxnWithCategory(t, queries, acctID, "txn_g_jan", "Jan Groceries", 2000, "2025-01-15", groceries.ID)
	upsertTxnWithCategory(t, queries, acctID, "txn_g_feb", "Feb Groceries", 3000, "2025-02-15", groceries.ID)

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-03-01")
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "category_month",
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Summary) < 2 {
		t.Fatalf("expected at least 2 category_month rows, got %d", len(result.Summary))
	}

	// Verify both rows have category and period set
	for _, row := range result.Summary {
		if row.Category == nil {
			continue // uncategorized rows
		}
		if *row.Category == "Groceries" {
			if row.Period == nil {
				t.Error("expected period to be set on category_month row")
			}
		}
	}
}

// --- Transaction Summary: group_by=week ---

func TestGetTransactionSummary_GroupByWeek(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Two transactions in same ISO week, one in different week
	testutil.MustCreateTransaction(t, queries, acctID, "txn_w1", "Monday Purchase", 1000, "2025-01-13") // Monday
	testutil.MustCreateTransaction(t, queries, acctID, "txn_w2", "Tuesday Purchase", 2000, "2025-01-14")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_w3", "Next Week", 3000, "2025-01-20") // next Monday

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "week",
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Summary) != 2 {
		t.Fatalf("expected 2 week rows, got %d", len(result.Summary))
	}
	if result.Totals.TransactionCount != 3 {
		t.Errorf("expected 3 total transactions, got %d", result.Totals.TransactionCount)
	}
}

// --- Transaction Summary: default date range ---

func TestGetTransactionSummary_DefaultDateRange(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Create a transaction within the last 30 days
	recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_recent", "Recent Purchase", 1500, recentDate)

	// Create a transaction 60 days ago (outside default range)
	oldDate := time.Now().AddDate(0, 0, -60).Format("2006-01-02")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_old", "Old Purchase", 2500, oldDate)

	// No date params — should use default 30-day range
	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy: "day",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Totals.TransactionCount != 1 {
		t.Errorf("expected 1 transaction in default 30-day range, got %d", result.Totals.TransactionCount)
	}
}

// --- ListTransactions: date range filter ---

func TestListTransactions_DateRangeFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_early", "Early", 100, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_mid", "Mid", 200, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_late", "Late", 300, "2025-01-25")

	start := testutil.MustParseDate("2025-01-10")
	end := testutil.MustParseDate("2025-01-20")

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 transaction in date range, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Mid" {
		t.Errorf("expected Mid, got %s", result.Transactions[0].Name)
	}
}

// --- ListTransactions: pending filter ---

func TestListTransactions_PendingFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_posted", "Posted", 100, "2025-01-15")
	upsertTxnPending(t, queries, acctID, "txn_pending", "Pending", 200, "2025-01-15")

	// Filter for pending=true
	pendingTrue := true
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{Pending: &pendingTrue})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Pending" {
		t.Errorf("expected Pending, got %s", result.Transactions[0].Name)
	}

	// Filter for pending=false
	pendingFalse := false
	result2, err := svc.ListTransactions(ctx, service.TransactionListParams{Pending: &pendingFalse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result2.Transactions) != 1 {
		t.Fatalf("expected 1 posted, got %d", len(result2.Transactions))
	}
	if result2.Transactions[0].Name != "Posted" {
		t.Errorf("expected Posted, got %s", result2.Transactions[0].Name)
	}
}

// --- ListTransactions: user filter ---

func TestListTransactions_UserFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice := testutil.MustCreateUser(t, queries, "Alice")
	bob := testutil.MustCreateUser(t, queries, "Bob")
	connA := testutil.MustCreateConnection(t, queries, alice.ID, "item_a")
	connB := testutil.MustCreateConnection(t, queries, bob.ID, "item_b")
	acctA := testutil.MustCreateAccount(t, queries, connA.ID, "ext_a", "Alice Checking")
	acctB := testutil.MustCreateAccount(t, queries, connB.ID, "ext_b", "Bob Checking")

	testutil.MustCreateTransaction(t, queries, acctA.ID, "txn_alice", "Alice Purchase", 1000, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctB.ID, "txn_bob", "Bob Purchase", 2000, "2025-01-15")

	aliceID := pgconv.FormatUUID(alice.ID)
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{UserID: &aliceID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 transaction for Alice, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Alice Purchase" {
		t.Errorf("expected Alice Purchase, got %s", result.Transactions[0].Name)
	}
}

// --- ListTransactions: account filter ---

func TestListTransactions_AccountFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	acct2 := testutil.MustCreateAccount(t, queries, conn.ID, "ext_2", "Savings")

	testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_c", "Checking Txn", 1000, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_s", "Savings Txn", 2000, "2025-01-15")

	acct1ID := pgconv.FormatUUID(acct1.ID)
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{AccountID: &acct1ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 for checking account, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Checking Txn" {
		t.Errorf("expected Checking Txn, got %s", result.Transactions[0].Name)
	}
}

// --- ListTransactions: category_slug filter ---

func TestListTransactions_CategorySlugFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "groceries", DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	upsertTxnWithCategory(t, queries, acctID, "txn_g1", "Whole Foods", 2500, "2025-01-15", groceries.ID)
	testutil.MustCreateTransaction(t, queries, acctID, "txn_none", "Other Store", 1000, "2025-01-15")

	slug := "groceries"
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{CategorySlug: &slug})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 groceries transaction, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Whole Foods" {
		t.Errorf("expected Whole Foods, got %s", result.Transactions[0].Name)
	}
}

// --- ListTransactions: parent category slug includes children ---

func TestListTransactions_ParentCategorySlugIncludesChildren(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Create parent category
	foodDrink, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create parent category: %v", err)
	}

	// Create child category
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug:        "food_and_drink_groceries",
		DisplayName: "Groceries",
		ParentID:    foodDrink.ID,
	})
	if err != nil {
		t.Fatalf("create child category: %v", err)
	}

	// One with parent, one with child, one uncategorized
	upsertTxnWithCategory(t, queries, acctID, "txn_parent", "Restaurant", 3000, "2025-01-15", foodDrink.ID)
	upsertTxnWithCategory(t, queries, acctID, "txn_child", "Whole Foods", 2500, "2025-01-14", groceries.ID)
	testutil.MustCreateTransaction(t, queries, acctID, "txn_other", "Gas Station", 4000, "2025-01-13")

	// Filter by parent slug — should include both parent and child
	slug := "food_and_drink"
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{CategorySlug: &slug})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2 transactions (parent + child), got %d", len(result.Transactions))
	}
}

// --- ListTransactions: unknown category slug returns empty ---

func TestListTransactions_UnknownCategorySlugReturnsEmpty(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_1", "Purchase", 1000, "2025-01-15")

	slug := "nonexistent_category"
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{CategorySlug: &slug})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 0 {
		t.Errorf("expected 0 results for unknown slug, got %d", len(result.Transactions))
	}
}

// --- ListTransactions: sort by amount ---

func TestListTransactions_SortByAmount(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_small", "Small", 500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_big", "Big", 10000, "2025-01-14")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_mid", "Mid", 3000, "2025-01-13")

	sortBy := "amount"
	sortOrder := "desc"
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		SortBy:    &sortBy,
		SortOrder: &sortOrder,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 3 {
		t.Fatalf("expected 3, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Big" {
		t.Errorf("expected Big first (highest amount), got %s", result.Transactions[0].Name)
	}
	if result.Transactions[2].Name != "Small" {
		t.Errorf("expected Small last (lowest amount), got %s", result.Transactions[2].Name)
	}

	// No cursor when sorting by non-date
	if result.NextCursor != "" {
		t.Error("expected empty cursor when sorting by amount")
	}
}

// --- ListTransactions: sort by name ---

func TestListTransactions_SortByName(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_c", "Charlie", 100, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_a", "Alice", 200, "2025-01-14")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_b", "Bob", 300, "2025-01-13")

	sortBy := "name"
	sortOrder := "asc"
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		SortBy:    &sortBy,
		SortOrder: &sortOrder,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Transactions[0].Name != "Alice" {
		t.Errorf("expected Alice first, got %s", result.Transactions[0].Name)
	}
	if result.Transactions[1].Name != "Bob" {
		t.Errorf("expected Bob second, got %s", result.Transactions[1].Name)
	}
	if result.Transactions[2].Name != "Charlie" {
		t.Errorf("expected Charlie third, got %s", result.Transactions[2].Name)
	}
}

// --- ListTransactions: sort asc ---

func TestListTransactions_SortDateAsc(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_new", "New", 100, "2025-01-20")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_old", "Old", 200, "2025-01-10")

	sortOrder := "asc"
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{SortOrder: &sortOrder})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Transactions[0].Name != "Old" {
		t.Errorf("expected Old first (asc), got %s", result.Transactions[0].Name)
	}
}

// --- ListTransactions: max_amount filter ---

func TestListTransactions_MaxAmountFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_small", "Small", 500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_big", "Big", 10000, "2025-01-14")

	max := 10.0
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{MaxAmount: &max})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 (amount <= 10), got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Small" {
		t.Errorf("expected Small, got %s", result.Transactions[0].Name)
	}
}

// --- ListTransactions: combined min+max amount ---

func TestListTransactions_MinMaxAmountRange(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_1", "Tiny", 100, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_2", "Mid", 2000, "2025-01-14")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_3", "Big", 10000, "2025-01-13")

	min := 5.0
	max := 50.0
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		MinAmount: &min,
		MaxAmount: &max,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 (5 <= amount <= 50), got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Mid" {
		t.Errorf("expected Mid, got %s", result.Transactions[0].Name)
	}
}

// --- CountTransactionsFiltered: with filters ---

func TestCountTransactionsFiltered_DateRange(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_1", "Old", 100, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_2", "Mid", 200, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_3", "New", 300, "2025-01-25")

	start := testutil.MustParseDate("2025-01-10")
	end := testutil.MustParseDate("2025-01-20")
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 in range, got %d", count)
	}
}

func TestCountTransactionsFiltered_SearchFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_a", "Starbucks", 500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_b", "Shell", 3000, "2025-01-14")

	search := "starbucks"
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Search: &search})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 matching 'starbucks', got %d", count)
	}
}

func TestCountTransactionsFiltered_UserFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice := testutil.MustCreateUser(t, queries, "Alice")
	bob := testutil.MustCreateUser(t, queries, "Bob")
	connA := testutil.MustCreateConnection(t, queries, alice.ID, "item_a")
	connB := testutil.MustCreateConnection(t, queries, bob.ID, "item_b")
	acctA := testutil.MustCreateAccount(t, queries, connA.ID, "ext_a", "Alice Checking")
	acctB := testutil.MustCreateAccount(t, queries, connB.ID, "ext_b", "Bob Checking")

	testutil.MustCreateTransaction(t, queries, acctA.ID, "txn_a", "Alice Txn", 100, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctB.ID, "txn_b", "Bob Txn", 200, "2025-01-15")

	aliceID := pgconv.FormatUUID(alice.ID)
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{UserID: &aliceID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 for Alice, got %d", count)
	}
}

func TestCountTransactionsFiltered_CategorySlug(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "groceries", DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	upsertTxnWithCategory(t, queries, acctID, "txn_g", "Groceries Txn", 2500, "2025-01-15", groceries.ID)
	testutil.MustCreateTransaction(t, queries, acctID, "txn_o", "Other Txn", 1000, "2025-01-15")

	slug := "groceries"
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{CategorySlug: &slug})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 groceries, got %d", count)
	}
}

func TestCountTransactionsFiltered_PendingFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_posted", "Posted", 100, "2025-01-15")
	upsertTxnPending(t, queries, acctID, "txn_pending", "Pending", 200, "2025-01-15")

	pendingTrue := true
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Pending: &pendingTrue})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending, got %d", count)
	}
}

// --- ListTransactions: limit clamping ---

func TestListTransactions_LimitClamping(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_1", "Test", 100, "2025-01-15")

	// Limit 0 should default to 50
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 50 {
		t.Errorf("expected default limit 50, got %d", result.Limit)
	}

	// Limit > 500 should clamp to 500
	result2, err := svc.ListTransactions(ctx, service.TransactionListParams{Limit: 999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.Limit != 500 {
		t.Errorf("expected clamped limit 500, got %d", result2.Limit)
	}
}

// --- ListTransactions: invalid cursor ---

func TestListTransactions_InvalidCursor(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.ListTransactions(context.Background(), service.TransactionListParams{
		Cursor: "not-a-valid-cursor",
	})
	if err == nil {
		t.Fatal("expected error for invalid cursor")
	}
}

// --- ListTransactions: transaction response has category info ---

func TestListTransactions_CategoryInfoInResponse(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	parent, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries", ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	upsertTxnWithCategory(t, queries, acctID, "txn_cat", "Whole Foods", 2500, "2025-01-15", child.ID)

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1, got %d", len(result.Transactions))
	}

	txn := result.Transactions[0]
	if txn.Category == nil {
		t.Fatal("expected category info to be set")
	}
	if txn.Category.Slug == nil || *txn.Category.Slug != "food_and_drink_groceries" {
		t.Errorf("expected slug food_and_drink_groceries, got %v", txn.Category.Slug)
	}
	if txn.Category.PrimarySlug == nil || *txn.Category.PrimarySlug != "food_and_drink" {
		t.Errorf("expected primary slug food_and_drink, got %v", txn.Category.PrimarySlug)
	}
	if txn.Category.DisplayName == nil || *txn.Category.DisplayName != "Groceries" {
		t.Errorf("expected display name Groceries, got %v", txn.Category.DisplayName)
	}
}

// --- GetTransaction: returns full details including category ---

func TestGetTransaction_WithCategory(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	parent, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries", ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	txn := upsertTxnWithCategory(t, queries, acctID, "txn_get", "Whole Foods", 2500, "2025-01-15", child.ID)

	resp, err := svc.GetTransaction(ctx, pgconv.FormatUUID(txn.ID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Category == nil {
		t.Fatal("expected category info")
	}
	if resp.Category.Slug == nil || *resp.Category.Slug != "food_and_drink_groceries" {
		t.Errorf("expected slug food_and_drink_groceries, got %v", resp.Category.Slug)
	}
	if resp.Category.PrimarySlug == nil || *resp.Category.PrimarySlug != "food_and_drink" {
		t.Errorf("expected primary slug, got %v", resp.Category.PrimarySlug)
	}
}

// --- Helpers ---

func upsertTxnWithCategory(t *testing.T, q *db.Queries, acctID pgtype.UUID, extID, name string, amountCents int64, date string, categoryID pgtype.UUID) db.Transaction {
	t.Helper()
	txn, err := q.UpsertTransaction(context.Background(), db.UpsertTransactionParams{
		AccountID:             acctID,
		ProviderTransactionID: extID,
		Amount:                pgtype.Numeric{Int: big.NewInt(amountCents), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: testutil.MustParseDate(date), Valid: true},
		ProviderName:          name,
		CategoryID:            categoryID,
	})
	if err != nil {
		t.Fatalf("upsertTxnWithCategory(%q): %v", name, err)
	}
	return txn
}

func upsertTxnPending(t *testing.T, q *db.Queries, acctID pgtype.UUID, extID, name string, amountCents int64, date string) db.Transaction {
	t.Helper()
	txn, err := q.UpsertTransaction(context.Background(), db.UpsertTransactionParams{
		AccountID:             acctID,
		ProviderTransactionID: extID,
		Amount:                pgtype.Numeric{Int: big.NewInt(amountCents), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: testutil.MustParseDate(date), Valid: true},
		ProviderName:          name,
		Pending:               true,
	})
	if err != nil {
		t.Fatalf("upsertTxnPending(%q): %v", name, err)
	}
	return txn
}

// --- Transaction Summary: UUID validation for account_id and user_id ---

func TestGetTransactionSummary_InvalidAccountID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	badID := "not-a-uuid"

	_, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "month",
		StartDate: &start,
		EndDate:   &end,
		AccountID: &badID,
	})
	if err == nil {
		t.Fatal("expected error for invalid account_id, got nil")
	}
	if !isInvalidParam(err) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestGetTransactionSummary_InvalidUserID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	badID := "also-not-a-uuid"

	_, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "month",
		StartDate: &start,
		EndDate:   &end,
		UserID:    &badID,
	})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
	if !isInvalidParam(err) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestGetTransactionSummary_ValidAccountIDFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_1", "Coffee Shop", 500, "2025-01-15")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	acctIDStr := pgconv.FormatUUID(acctID)

	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "month",
		StartDate: &start,
		EndDate:   &end,
		AccountID: &acctIDStr,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Totals.TransactionCount != 1 {
		t.Errorf("expected 1 transaction, got %d", result.Totals.TransactionCount)
	}
}

func TestGetTransactionSummary_ValidUserIDFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Bob")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_user_sum")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_user_sum", "Checking")

	testutil.MustCreateTransaction(t, queries, acct.ID, "txn_bob", "Lunch", 1200, "2025-01-15")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	userIDStr := pgconv.FormatUUID(user.ID)

	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "month",
		StartDate: &start,
		EndDate:   &end,
		UserID:    &userIDStr,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Totals.TransactionCount != 1 {
		t.Errorf("expected 1 transaction, got %d", result.Totals.TransactionCount)
	}
}

func TestGetTransactionSummary_NonexistentAccountIDReturnsEmpty(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_1", "Coffee", 500, "2025-01-15")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	fakeID := "00000000-0000-0000-0000-000000000099"

	result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		GroupBy:   "month",
		StartDate: &start,
		EndDate:   &end,
		AccountID: &fakeID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Totals.TransactionCount != 0 {
		t.Errorf("expected 0 transactions for nonexistent account, got %d", result.Totals.TransactionCount)
	}
}

func upsertTxnWithAmount(t *testing.T, q *db.Queries, acctID pgtype.UUID, extID, name string, amountCents int64, date string) db.Transaction {
	t.Helper()
	txn, err := q.UpsertTransaction(context.Background(), db.UpsertTransactionParams{
		AccountID:             acctID,
		ProviderTransactionID: extID,
		Amount:                pgtype.Numeric{Int: big.NewInt(amountCents), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: testutil.MustParseDate(date), Valid: true},
		ProviderName:          name,
	})
	if err != nil {
		t.Fatalf("upsertTxnWithAmount(%q): %v", name, err)
	}
	return txn
}

// isInvalidParam checks if an error wraps service.ErrInvalidParameter.
func isInvalidParam(err error) bool {
	return errors.Is(err, service.ErrInvalidParameter)
}

func upsertTxnWithMerchant(t *testing.T, q *db.Queries, acctID pgtype.UUID, extID, name, merchant string, amountCents int64, date string) db.Transaction {
	t.Helper()
	txn, err := q.UpsertTransaction(context.Background(), db.UpsertTransactionParams{
		AccountID:             acctID,
		ProviderTransactionID: extID,
		Amount:                pgtype.Numeric{Int: big.NewInt(amountCents), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: testutil.MustParseDate(date), Valid: true},
		ProviderName:          name,
		ProviderMerchantName:  pgtype.Text{String: merchant, Valid: true},
	})
	if err != nil {
		t.Fatalf("upsertTxnWithMerchant(%q): %v", name, err)
	}
	return txn
}

// --- Merchant Summary ---

func TestGetMerchantSummary_Basic(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	upsertTxnWithMerchant(t, queries, acctID, "ms1", "STARBUCKS #1234", "Starbucks", 550, "2025-01-05")
	upsertTxnWithMerchant(t, queries, acctID, "ms2", "STARBUCKS #5678", "Starbucks", 650, "2025-01-10")
	upsertTxnWithMerchant(t, queries, acctID, "ms3", "STARBUCKS #9012", "Starbucks", 475, "2025-01-15")
	upsertTxnWithMerchant(t, queries, acctID, "ms4", "AMZN*1234", "Amazon", 2999, "2025-01-08")
	upsertTxnWithMerchant(t, queries, acctID, "ms5", "AMZN*5678", "Amazon", 1599, "2025-01-12")
	testutil.MustCreateTransaction(t, queries, acctID, "ms6", "TARGET #123", 4250, "2025-01-20")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	result, err := svc.GetMerchantSummary(ctx, service.MerchantSummaryParams{
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Merchants) != 3 {
		t.Fatalf("expected 3 merchants, got %d", len(result.Merchants))
	}

	// Ordered by count DESC: Starbucks (3), Amazon (2), Target (1)
	if result.Merchants[0].Merchant != "Starbucks" {
		t.Errorf("expected first merchant to be Starbucks, got %s", result.Merchants[0].Merchant)
	}
	if result.Merchants[0].TransactionCount != 3 {
		t.Errorf("expected 3 Starbucks transactions, got %d", result.Merchants[0].TransactionCount)
	}
	if result.Merchants[0].TotalAmount != 16.75 {
		t.Errorf("expected Starbucks total 16.75, got %f", result.Merchants[0].TotalAmount)
	}

	if result.Totals.MerchantCount != 3 {
		t.Errorf("expected 3 total merchants, got %d", result.Totals.MerchantCount)
	}
	if result.Totals.TotalAmount == nil {
		t.Error("expected total amount to be set (single currency)")
	}
}

func TestGetMerchantSummary_MinCount(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	upsertTxnWithMerchant(t, queries, acctID, "mc1", "SB1", "Starbucks", 550, "2025-01-05")
	upsertTxnWithMerchant(t, queries, acctID, "mc2", "SB2", "Starbucks", 650, "2025-01-10")
	upsertTxnWithMerchant(t, queries, acctID, "mc3", "SB3", "Starbucks", 475, "2025-01-15")
	upsertTxnWithMerchant(t, queries, acctID, "mc4", "AZ1", "Amazon", 2999, "2025-01-08")
	upsertTxnWithMerchant(t, queries, acctID, "mc5", "AZ2", "Amazon", 1599, "2025-01-12")
	testutil.MustCreateTransaction(t, queries, acctID, "mc6", "TARGET #123", 4250, "2025-01-20")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")

	result, err := svc.GetMerchantSummary(ctx, service.MerchantSummaryParams{
		StartDate: &start, EndDate: &end, MinCount: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Merchants) != 2 {
		t.Fatalf("expected 2 merchants with min_count=2, got %d", len(result.Merchants))
	}

	result, err = svc.GetMerchantSummary(ctx, service.MerchantSummaryParams{
		StartDate: &start, EndDate: &end, MinCount: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Merchants) != 1 {
		t.Fatalf("expected 1 merchant with min_count=3, got %d", len(result.Merchants))
	}
}

func TestGetMerchantSummary_EmptyResult(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	result, err := svc.GetMerchantSummary(ctx, service.MerchantSummaryParams{StartDate: &start, EndDate: &end})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Merchants) != 0 {
		t.Errorf("expected 0 merchants, got %d", len(result.Merchants))
	}
}

// --- ExcludeSearch ---

func TestListTransactions_ExcludeSearch(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "le1", "STARBUCKS #1234", 550, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "le2", "AMAZON PURCHASE", 2999, "2025-01-08")
	testutil.MustCreateTransaction(t, queries, acctID, "le3", "TARGET #123", 4250, "2025-01-20")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	exclude := "starbucks"

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		StartDate: &start, EndDate: &end, ExcludeSearch: &exclude,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2 transactions after excluding starbucks, got %d", len(result.Transactions))
	}
}

func TestCountTransactions_ExcludeSearch(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "ce1", "STARBUCKS #1234", 550, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "ce2", "AMAZON PURCHASE", 2999, "2025-01-08")
	testutil.MustCreateTransaction(t, queries, acctID, "ce3", "TARGET #123", 4250, "2025-01-20")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	exclude := "starbucks"

	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{
		StartDate: &start, EndDate: &end, ExcludeSearch: &exclude,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2 after excluding starbucks, got %d", count)
	}
}

func TestListTransactions_ExcludeSearch_WithPagination(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Create enough transactions to test pagination with exclude_search.
	// This regression test ensures exclude_search doesn't double-append SQL
	// parameters, which would corrupt the positional $N numbering for the
	// cursor clause that follows.
	testutil.MustCreateTransaction(t, queries, acctID, "ep1", "STARBUCKS #1234", 550, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "ep2", "AMAZON PURCHASE", 2999, "2025-01-08")
	testutil.MustCreateTransaction(t, queries, acctID, "ep3", "TARGET #123", 4250, "2025-01-10")
	testutil.MustCreateTransaction(t, queries, acctID, "ep4", "WALMART GROCERIES", 3500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "ep5", "COSTCO WHOLESALE", 8900, "2025-01-20")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	exclude := "starbucks"

	// First page with limit=2.
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		Limit: 2, StartDate: &start, EndDate: &end, ExcludeSearch: &exclude,
	})
	if err != nil {
		t.Fatalf("page 1 unexpected error: %v", err)
	}
	if !result.HasMore {
		t.Fatal("expected HasMore=true on first page")
	}
	if result.NextCursor == "" {
		t.Fatal("expected non-empty cursor on first page")
	}

	// Second page using cursor from first page.
	result2, err := svc.ListTransactions(ctx, service.TransactionListParams{
		Limit: 2, StartDate: &start, EndDate: &end, ExcludeSearch: &exclude,
		Cursor: result.NextCursor,
	})
	if err != nil {
		t.Fatalf("page 2 unexpected error: %v", err)
	}
	if len(result2.Transactions) == 0 {
		t.Fatal("expected transactions on second page")
	}

	// Verify no duplicates across pages.
	seen := make(map[string]bool)
	for _, txn := range result.Transactions {
		seen[txn.ID] = true
	}
	for _, txn := range result2.Transactions {
		if seen[txn.ID] {
			t.Errorf("duplicate transaction %s across pages", txn.ID)
		}
	}
}

// --- Search Modes ---

func TestListTransactions_SearchMode_Words(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "sw1", "CENTURY LINK PAYMENT", 5000, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "sw2", "CENTURYLINK", 5000, "2025-01-06")
	testutil.MustCreateTransaction(t, queries, acctID, "sw3", "AMAZON PURCHASE", 2999, "2025-01-08")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	search := "century link"
	mode := "words"

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		StartDate: &start, EndDate: &end, Search: &search, SearchMode: &mode,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "century link" in words mode: both "century" and "link" must appear
	// "CENTURY LINK PAYMENT" matches (both words present)
	// "CENTURYLINK" matches "century" and "link" as substrings
	// "AMAZON PURCHASE" does not match
	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2 transactions matching 'century link' in words mode, got %d", len(result.Transactions))
	}
}

func TestListTransactions_SearchMode_Fuzzy(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "sf1", "Starbucks Coffee", 550, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "sf2", "AMAZON PURCHASE", 2999, "2025-01-08")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	search := "starbuks" // typo
	mode := "fuzzy"

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		StartDate: &start, EndDate: &end, Search: &search, SearchMode: &mode,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fuzzy search should find "Starbucks Coffee" despite the typo
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 transaction matching 'starbuks' in fuzzy mode, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Starbucks Coffee" {
		t.Errorf("expected 'Starbucks Coffee', got %s", result.Transactions[0].Name)
	}
}

func TestListTransactions_Search_CommaOR(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "co1", "STARBUCKS #1234", 550, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "co2", "AMAZON PURCHASE", 2999, "2025-01-08")
	testutil.MustCreateTransaction(t, queries, acctID, "co3", "TARGET #123", 4250, "2025-01-20")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	search := "starbucks,amazon"

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		StartDate: &start, EndDate: &end, Search: &search,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Comma-separated: should match starbucks OR amazon, not target
	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2 transactions matching 'starbucks,amazon', got %d", len(result.Transactions))
	}
}

func TestListTransactions_ExcludeSearch_CommaOR(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "ec1", "STARBUCKS #1234", 550, "2025-01-05")
	testutil.MustCreateTransaction(t, queries, acctID, "ec2", "AMAZON PURCHASE", 2999, "2025-01-08")
	testutil.MustCreateTransaction(t, queries, acctID, "ec3", "TARGET #123", 4250, "2025-01-20")

	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-02-01")
	exclude := "starbucks,amazon"

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{
		StartDate: &start, EndDate: &end, ExcludeSearch: &exclude,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should exclude both starbucks and amazon, leaving only target
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 transaction after excluding starbucks,amazon, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "TARGET #123" {
		t.Errorf("expected TARGET #123, got %s", result.Transactions[0].Name)
	}
}
