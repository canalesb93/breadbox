//go:build integration

// Integration tests for the service layer. Require a running PostgreSQL with breadbox_test database.
// Run with: make test-integration
//
// IMPORTANT: Do NOT use t.Parallel() — tests share a database and truncate between runs.
package service_test

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

func newService(t *testing.T) (*service.Service, *db.Queries, *pgxpool.Pool) {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	return svc, queries, pool
}

// seedTxnFixture creates user → connection → account and returns the account ID.
func seedTxnFixture(t *testing.T, queries *db.Queries) pgtype.UUID {
	t.Helper()
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	return acct.ID
}

// --- Users ---

func TestListUsers_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	users, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 users, got %d", len(users))
	}
}

func TestListUsers_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	testutil.MustCreateUser(t, queries, "Alice")
	testutil.MustCreateUser(t, queries, "Bob")

	users, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Name != "Alice" {
		t.Errorf("expected first user Alice, got %s", users[0].Name)
	}
	if users[1].Name != "Bob" {
		t.Errorf("expected second user Bob, got %s", users[1].Name)
	}
}

// --- Accounts ---

func TestListAccounts_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	accounts, err := svc.ListAccounts(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(accounts))
	}
}

func TestListAccounts_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "test_item_1")

	_, err := queries.UpsertAccount(ctx, db.UpsertAccountParams{
		ConnectionID:      conn.ID,
		ExternalAccountID: "ext_acct_1",
		Name:              "Checking",
		Type:              "depository",
		Subtype:           pgtype.Text{String: "checking", Valid: true},
		IsoCurrencyCode:   pgtype.Text{String: "USD", Valid: true},
		BalanceCurrent:    pgtype.Numeric{Int: big.NewInt(100000), Exp: -2, Valid: true},
	})
	if err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	accounts, err := svc.ListAccounts(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Name != "Checking" {
		t.Errorf("expected account name Checking, got %s", accounts[0].Name)
	}
	if accounts[0].Type != "depository" {
		t.Errorf("expected type depository, got %s", accounts[0].Type)
	}
	if accounts[0].BalanceCurrent == nil || *accounts[0].BalanceCurrent != 1000.0 {
		t.Errorf("expected balance 1000.0, got %v", accounts[0].BalanceCurrent)
	}
}

func TestListAccounts_FilterByUser(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice := testutil.MustCreateUser(t, queries, "Alice")
	bob := testutil.MustCreateUser(t, queries, "Bob")

	connA := testutil.MustCreateConnection(t, queries, alice.ID, "item_a")
	connB := testutil.MustCreateConnection(t, queries, bob.ID, "item_b")

	testutil.MustCreateAccount(t, queries, connA.ID, "ext_a1", "Alice Checking")
	testutil.MustCreateAccount(t, queries, connB.ID, "ext_b1", "Bob Checking")

	aliceID := formatUUID(alice.ID)
	accounts, err := svc.ListAccounts(ctx, &aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account for Alice, got %d", len(accounts))
	}
	if accounts[0].Name != "Alice Checking" {
		t.Errorf("expected Alice Checking, got %s", accounts[0].Name)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetAccount(context.Background(), "00000000-0000-0000-0000-000000000099")
	if err != service.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- Connections ---

func TestListConnections_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	conns, err := svc.ListConnections(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conns) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(conns))
	}
}

func TestListConnections_ExcludesDisconnected(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	testutil.MustCreateConnection(t, queries, user.ID, "item_active")

	_, err := queries.CreateBankConnection(ctx, db.CreateBankConnectionParams{
		Provider:             db.ProviderTypePlaid,
		ExternalID:           pgtype.Text{String: "item_disc", Valid: true},
		EncryptedCredentials: []byte("e"),
		Status:               db.ConnectionStatusDisconnected,
		UserID:               user.ID,
	})
	if err != nil {
		t.Fatalf("create disconnected connection: %v", err)
	}

	conns, err := svc.ListConnections(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 active connection, got %d", len(conns))
	}
	if conns[0].Status != "active" {
		t.Errorf("expected status active, got %s", conns[0].Status)
	}
	if conns[0].Provider != "plaid" {
		t.Errorf("expected provider plaid, got %s", conns[0].Provider)
	}
}

func TestGetConnectionStatus_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetConnectionStatus(context.Background(), "00000000-0000-0000-0000-000000000099")
	if err != service.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- Transactions ---

func TestListTransactions_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	result, err := svc.ListTransactions(context.Background(), service.TransactionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 0 {
		t.Fatalf("expected 0 transactions, got %d", len(result.Transactions))
	}
	if result.HasMore {
		t.Error("expected HasMore=false")
	}
}

func TestListTransactions_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_001", "Starbucks", 450, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_002", "Shell Gas", 4215, "2025-01-14")

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(result.Transactions))
	}
	// Default sort is date DESC — newest first
	if result.Transactions[0].Name != "Starbucks" {
		t.Errorf("expected first transaction Starbucks (newer), got %s", result.Transactions[0].Name)
	}
	if result.Transactions[0].Amount != 4.50 {
		t.Errorf("expected amount 4.50, got %f", result.Transactions[0].Amount)
	}
	if result.Transactions[1].Name != "Shell Gas" {
		t.Errorf("expected second transaction Shell Gas, got %s", result.Transactions[1].Name)
	}
	if result.Transactions[1].Amount != 42.15 {
		t.Errorf("expected amount 42.15, got %f", result.Transactions[1].Amount)
	}
}

func TestListTransactions_Pagination(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	for i := 0; i < 5; i++ {
		testutil.MustCreateTransaction(t, queries, acctID,
			fmt.Sprintf("txn_%03d", i),
			fmt.Sprintf("Transaction %d", i),
			int64(i*100+100),
			fmt.Sprintf("2025-01-%02d", 15-i),
		)
	}

	// Page 1
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2, got %d", len(result.Transactions))
	}
	if !result.HasMore {
		t.Error("expected HasMore=true")
	}
	if result.NextCursor == "" {
		t.Error("expected non-empty cursor")
	}

	// Page 2
	result2, err := svc.ListTransactions(ctx, service.TransactionListParams{Limit: 2, Cursor: result.NextCursor})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result2.Transactions) != 2 {
		t.Fatalf("expected 2 on page 2, got %d", len(result2.Transactions))
	}

	// Page 3 (last)
	result3, err := svc.ListTransactions(ctx, service.TransactionListParams{Limit: 2, Cursor: result2.NextCursor})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result3.Transactions) != 1 {
		t.Fatalf("expected 1 on page 3, got %d", len(result3.Transactions))
	}
	if result3.HasMore {
		t.Error("expected HasMore=false on last page")
	}
}

func TestListTransactions_SearchFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_a", "Starbucks Coffee", 500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_b", "Shell Gas Station", 3000, "2025-01-14")

	search := "starbucks"
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{Search: &search})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 result for 'starbucks', got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Starbucks Coffee" {
		t.Errorf("expected Starbucks Coffee, got %s", result.Transactions[0].Name)
	}
}

func TestListTransactions_AmountFilter(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_small", "Small", 500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_big", "Big", 10000, "2025-01-14")

	min := 50.0
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{MinAmount: &min})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 result with min=50, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Big" {
		t.Errorf("expected Big, got %s", result.Transactions[0].Name)
	}
}

func TestCountTransactions(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	count, _ := svc.CountTransactions(ctx)
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}

	acctID := seedTxnFixture(t, queries)
	testutil.MustCreateTransaction(t, queries, acctID, "txn_1", "Test", 100, "2025-01-15")

	count, err := svc.CountTransactions(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
}

func TestGetTransaction_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetTransaction(context.Background(), "00000000-0000-0000-0000-000000000099")
	if err != service.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSoftDeletedTransactions_NotReturned(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_visible", "Visible", 100, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_deleted", "Deleted", 200, "2025-01-14")

	if err := queries.SoftDeleteTransactionByExternalID(ctx, "txn_deleted"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 visible transaction, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Visible" {
		t.Errorf("expected Visible, got %s", result.Transactions[0].Name)
	}
}

// --- Category Mapping Re-Resolution ---

func TestCreateMapping_ReResolvesUncategorizedTransactions(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	// Seed the uncategorized category (truncated between tests).
	uncategorized, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}

	// Create a target category for the mapping.
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("create groceries category: %v", err)
	}

	// Create user → connection → account.
	acctID := seedTxnFixture(t, queries)

	// Create a transaction with category_detailed that is currently uncategorized.
	_, err = queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID:             acctID,
		ExternalTransactionID: "txn_grocery",
		Amount:                pgtype.Numeric{Int: big.NewInt(2500), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: testutil.MustParseDate("2025-01-15"), Valid: true},
		Name:                  "Whole Foods",
		CategoryPrimary:       pgtype.Text{String: "FOOD_AND_DRINK", Valid: true},
		CategoryDetailed:      pgtype.Text{String: "FOOD_AND_DRINK_GROCERIES", Valid: true},
		CategoryID:            uncategorized.ID, // starts as uncategorized
	})
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	// Verify it shows as unmapped.
	unmapped, err := svc.ListUnmappedCategories(ctx)
	if err != nil {
		t.Fatalf("list unmapped: %v", err)
	}
	if len(unmapped) != 1 {
		t.Fatalf("expected 1 unmapped category, got %d", len(unmapped))
	}

	// Now create a mapping for FOOD_AND_DRINK_GROCERIES → groceries.
	_, err = svc.CreateMapping(ctx, "plaid", "FOOD_AND_DRINK_GROCERIES", formatUUID(groceries.ID))
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}

	// The transaction should now be re-resolved: no longer uncategorized.
	unmapped, err = svc.ListUnmappedCategories(ctx)
	if err != nil {
		t.Fatalf("list unmapped after mapping: %v", err)
	}
	if len(unmapped) != 0 {
		t.Errorf("expected 0 unmapped categories after mapping, got %d", len(unmapped))
	}

	// Verify the transaction now has the groceries category_id.
	var gotCategoryID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_grocery'",
	).Scan(&gotCategoryID)
	if err != nil {
		t.Fatalf("query transaction category: %v", err)
	}
	if gotCategoryID != groceries.ID {
		t.Errorf("expected category_id %v, got %v", groceries.ID, gotCategoryID)
	}

	// Ensure overridden transactions are NOT re-resolved.
	_, err = queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID:             acctID,
		ExternalTransactionID: "txn_override",
		Amount:                pgtype.Numeric{Int: big.NewInt(1000), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: testutil.MustParseDate("2025-01-16"), Valid: true},
		Name:                  "Override Txn",
		CategoryPrimary:       pgtype.Text{String: "FOOD_AND_DRINK", Valid: true},
		CategoryDetailed:      pgtype.Text{String: "FOOD_AND_DRINK_COFFEE", Valid: true},
		CategoryID:            uncategorized.ID,
	})
	if err != nil {
		t.Fatalf("create override txn: %v", err)
	}
	// Set override flag.
	pool.Exec(ctx, "UPDATE transactions SET category_override = TRUE WHERE external_transaction_id = 'txn_override'")

	// Create mapping for FOOD_AND_DRINK_COFFEE → groceries (reuse category).
	_, err = svc.CreateMapping(ctx, "plaid", "FOOD_AND_DRINK_COFFEE", formatUUID(groceries.ID))
	if err != nil {
		t.Fatalf("create mapping for coffee: %v", err)
	}

	// Overridden transaction should still be uncategorized.
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_override'",
	).Scan(&gotCategoryID)
	if err != nil {
		t.Fatalf("query override txn category: %v", err)
	}
	if gotCategoryID != uncategorized.ID {
		t.Errorf("overridden transaction should keep uncategorized, got %v", gotCategoryID)
	}
}

// --- Teller Category Mapping ---

// seedTellerTxnFixture creates user → teller connection → account and returns the account ID.
func seedTellerTxnFixture(t *testing.T, queries *db.Queries) pgtype.UUID {
	t.Helper()
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateTellerConnection(t, queries, user.ID, "teller_item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "teller_acct_1", "Teller Checking")
	return acct.ID
}

func TestCreateMapping_ReResolvesTellerRawCategories(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	// Seed categories.
	uncategorized, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("create groceries: %v", err)
	}

	acctID := seedTellerTxnFixture(t, queries)

	// Create a Teller transaction with raw category in category_primary (as the fixed provider does).
	_, err = queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID:             acctID,
		ExternalTransactionID: "teller_txn_grocery",
		Amount:                pgtype.Numeric{Int: big.NewInt(2500), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: testutil.MustParseDate("2025-01-15"), Valid: true},
		Name:                  "Whole Foods",
		CategoryPrimary:       pgtype.Text{String: "groceries", Valid: true}, // raw Teller category
		CategoryID:            uncategorized.ID,
	})
	if err != nil {
		t.Fatalf("create teller transaction: %v", err)
	}

	// Verify it shows as unmapped.
	unmapped, err := svc.ListUnmappedCategories(ctx)
	if err != nil {
		t.Fatalf("list unmapped: %v", err)
	}
	if len(unmapped) != 1 {
		t.Fatalf("expected 1 unmapped category, got %d", len(unmapped))
	}
	if unmapped[0].Provider != "teller" {
		t.Errorf("expected provider teller, got %s", unmapped[0].Provider)
	}

	// Create mapping for the raw Teller category "groceries".
	_, err = svc.CreateMapping(ctx, "teller", "groceries", formatUUID(groceries.ID))
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}

	// Transaction should now be resolved.
	unmapped, err = svc.ListUnmappedCategories(ctx)
	if err != nil {
		t.Fatalf("list unmapped after mapping: %v", err)
	}
	if len(unmapped) != 0 {
		t.Errorf("expected 0 unmapped categories after mapping, got %d", len(unmapped))
	}

	// Verify the transaction has the correct category_id.
	var gotCategoryID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'teller_txn_grocery'",
	).Scan(&gotCategoryID)
	if err != nil {
		t.Fatalf("query transaction category: %v", err)
	}
	if gotCategoryID != groceries.ID {
		t.Errorf("expected category_id %v, got %v", groceries.ID, gotCategoryID)
	}
}

func TestListUnmappedCategories_TellerShowsRawCategoryStrings(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Seed uncategorized category.
	uncategorized, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}

	acctID := seedTellerTxnFixture(t, queries)

	// Create transactions with raw Teller categories.
	for _, tc := range []struct {
		extID, name, category string
	}{
		{"teller_txn_1", "Whole Foods", "groceries"},
		{"teller_txn_2", "Chipotle", "dining"},
		{"teller_txn_3", "Shell Gas", "fuel"},
	} {
		_, err = queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
			AccountID:             acctID,
			ExternalTransactionID: tc.extID,
			Amount:                pgtype.Numeric{Int: big.NewInt(1000), Exp: -2, Valid: true},
			IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
			Date:                  pgtype.Date{Time: testutil.MustParseDate("2025-01-15"), Valid: true},
			Name:                  tc.name,
			CategoryPrimary:       pgtype.Text{String: tc.category, Valid: true},
			CategoryID:            uncategorized.ID,
		})
		if err != nil {
			t.Fatalf("create transaction %s: %v", tc.extID, err)
		}
	}

	unmapped, err := svc.ListUnmappedCategories(ctx)
	if err != nil {
		t.Fatalf("list unmapped: %v", err)
	}
	if len(unmapped) != 3 {
		t.Fatalf("expected 3 unmapped categories, got %d", len(unmapped))
	}

	// All should show raw Teller category strings, not Plaid taxonomy.
	rawCategories := make(map[string]bool)
	for _, u := range unmapped {
		if u.Provider != "teller" {
			t.Errorf("expected provider teller, got %s", u.Provider)
		}
		if u.Primary != nil {
			rawCategories[*u.Primary] = true
		}
	}
	for _, expected := range []string{"groceries", "dining", "fuel"} {
		if !rawCategories[expected] {
			t.Errorf("expected raw Teller category %q in unmapped list", expected)
		}
	}
	// Plaid-style values should NOT appear.
	for _, plaidVal := range []string{"FOOD_AND_DRINK_GROCERIES", "FOOD_AND_DRINK_RESTAURANT", "TRANSPORTATION_GAS"} {
		if rawCategories[plaidVal] {
			t.Errorf("Plaid-style value %q should NOT appear in Teller unmapped categories", plaidVal)
		}
	}
}

// --- Bulk TSV Export/Import ---

func TestExportCategoriesTSV(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create some categories since tables are truncated between tests.
	_, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create parent category: %v", err)
	}

	parent, err := queries.GetCategoryBySlug(ctx, "food_and_drink")
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}

	_, err = queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries", ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("create child category: %v", err)
	}

	tsv, err := svc.ExportCategoriesTSV(ctx)
	if err != nil {
		t.Fatalf("ExportCategoriesTSV: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(tsv), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (header + 2 categories), got %d", len(lines))
	}

	// Check header
	expectedHeader := "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden"
	if lines[0] != expectedHeader {
		t.Errorf("expected header %q, got %q", expectedHeader, lines[0])
	}

	// Check for at least one parent (empty parent_slug) and one child (non-empty parent_slug)
	hasParent := false
	hasChild := false
	for _, line := range lines[1:] {
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		if cols[2] == "" {
			hasParent = true
		} else {
			hasChild = true
		}
	}
	if !hasParent {
		t.Error("expected at least one parent category (empty parent_slug)")
	}
	if !hasChild {
		t.Error("expected at least one child category (non-empty parent_slug)")
	}
}

func TestImportCategoriesTSV_CreateAndUpdate(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Create a custom category via service
	_, err := svc.CreateCategory(ctx, service.CreateCategoryParams{
		Slug:        "test_parent",
		DisplayName: "Test Parent",
	})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	// Build TSV content
	tsv := "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden\n"
	tsv += "test_parent\tTest Parent Updated\t\tstar\t\t0\tfalse\n"
	tsv += "new_parent\tNew Parent\t\tfolder\t\t0\tfalse\n"
	tsv += "new_child\tNew Child\tnew_parent\tfile\t\t0\tfalse\n"

	result, err := svc.ImportCategoriesTSV(ctx, tsv)
	if err != nil {
		t.Fatalf("ImportCategoriesTSV: %v", err)
	}

	if result.Created != 2 {
		t.Errorf("expected Created=2, got %d", result.Created)
	}
	if result.Updated != 1 {
		t.Errorf("expected Updated=1, got %d", result.Updated)
	}
	if result.Unchanged != 0 {
		t.Errorf("expected Unchanged=0, got %d", result.Unchanged)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %v", result.Errors)
	}

	// Verify the updated category
	updated, err := svc.GetCategoryBySlug(ctx, "test_parent")
	if err != nil {
		t.Fatalf("get updated category: %v", err)
	}
	if updated.DisplayName != "Test Parent Updated" {
		t.Errorf("expected display_name 'Test Parent Updated', got %q", updated.DisplayName)
	}
	if updated.Icon == nil || *updated.Icon != "star" {
		t.Errorf("expected icon 'star', got %v", updated.Icon)
	}

	// Verify the new child category exists and has correct parent
	child, err := svc.GetCategoryBySlug(ctx, "new_child")
	if err != nil {
		t.Fatalf("get child category: %v", err)
	}
	if child.DisplayName != "New Child" {
		t.Errorf("expected display_name 'New Child', got %q", child.DisplayName)
	}
	// Verify parent by checking ParentID matches new_parent's ID
	newParent, err := svc.GetCategoryBySlug(ctx, "new_parent")
	if err != nil {
		t.Fatalf("get new_parent category: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != newParent.ID {
		t.Errorf("expected parent_id %q, got %v", newParent.ID, child.ParentID)
	}
}

func TestImportCategoriesTSV_ValidationErrors(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	tsv := "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden\n"
	tsv += "INVALID-SLUG!\tBad Slug\t\t\t\t0\tfalse\n"
	tsv += "empty_name\t\t\t\t\t0\tfalse\n"
	tsv += "orphan_child\tOrphan Child\tghost_parent\t\t\t0\tfalse\n"
	tsv += "valid_cat\tValid Category\t\t\t\t0\tfalse\n"

	result, err := svc.ImportCategoriesTSV(ctx, tsv)
	if err != nil {
		t.Fatalf("ImportCategoriesTSV: %v", err)
	}

	if result.Created != 1 {
		t.Errorf("expected Created=1, got %d", result.Created)
	}
	if len(result.Errors) != 3 {
		t.Errorf("expected 3 errors, got %d: %v", len(result.Errors), result.Errors)
	}

	// Verify valid_cat was created
	cat, err := svc.GetCategoryBySlug(ctx, "valid_cat")
	if err != nil {
		t.Fatalf("get valid_cat: %v", err)
	}
	if cat.DisplayName != "Valid Category" {
		t.Errorf("expected 'Valid Category', got %q", cat.DisplayName)
	}
}

func TestExportMappingsTSV(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create a category and mapping since tables are truncated.
	cat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	_, err = queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider:         db.ProviderTypePlaid,
		ProviderCategory: "FOOD_AND_DRINK",
		CategoryID:       cat.ID,
	})
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}

	tsv, err := svc.ExportMappingsTSV(ctx)
	if err != nil {
		t.Fatalf("ExportMappingsTSV: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(tsv), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines (header + 1 mapping), got %d", len(lines))
	}

	expectedHeader := "provider\tprovider_category\tcategory_slug"
	if lines[0] != expectedHeader {
		t.Errorf("expected header %q, got %q", expectedHeader, lines[0])
	}

	// Check that plaid mappings are present
	foundPlaid := false
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "plaid\t") {
			foundPlaid = true
			break
		}
	}
	if !foundPlaid {
		t.Error("expected at least one plaid mapping in export")
	}
}

func TestImportMappingsTSV_CreateAndUpdate(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create categories
	foodDrink, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create food_and_drink: %v", err)
	}
	_, err = queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries", ParentID: foodDrink.ID,
	})
	if err != nil {
		t.Fatalf("create groceries: %v", err)
	}
	_, err = queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_restaurant", DisplayName: "Restaurant", ParentID: foodDrink.ID,
	})
	if err != nil {
		t.Fatalf("create restaurant: %v", err)
	}

	// Create an existing mapping
	_, err = svc.CreateMappingBySlug(ctx, "csv", "test_food", "food_and_drink")
	if err != nil {
		t.Fatalf("create initial mapping: %v", err)
	}

	// Build TSV to update the existing and create a new one
	tsv := "provider\tprovider_category\tcategory_slug\n"
	tsv += "csv\ttest_food\tfood_and_drink_groceries\n"
	tsv += "csv\ttest_new\tfood_and_drink_restaurant\n"

	result, err := svc.ImportMappingsTSV(ctx, tsv, false)
	if err != nil {
		t.Fatalf("ImportMappingsTSV: %v", err)
	}

	if result.Created != 1 {
		t.Errorf("expected Created=1, got %d", result.Created)
	}
	if result.Updated != 1 {
		t.Errorf("expected Updated=1, got %d", result.Updated)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %v", result.Errors)
	}
}

func TestImportMappingsTSV_ApplyRetroactively(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	// Create categories
	foodDrink, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create food_and_drink: %v", err)
	}
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries", ParentID: foodDrink.ID,
	})
	if err != nil {
		t.Fatalf("create groceries: %v", err)
	}

	// Create user → connection (plaid) → account
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_retro")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "acct_retro", "Retro Checking")

	// Create existing mapping: plaid FOOD_TEST → food_and_drink
	_, err = svc.CreateMappingBySlug(ctx, "plaid", "FOOD_TEST", "food_and_drink")
	if err != nil {
		t.Fatalf("create initial mapping: %v", err)
	}

	// Create 3 non-overridden transactions with category_primary=FOOD_TEST, category_id=food_and_drink
	for i := 0; i < 3; i++ {
		extID := fmt.Sprintf("retro_txn_%d", i)
		_, err = pool.Exec(ctx, `INSERT INTO transactions
			(account_id, external_transaction_id, amount, iso_currency_code, date, name, category_primary, category_id, category_override)
			VALUES ($1, $2, 10.00, 'USD', '2024-01-15', 'Test', $3, $4, $5)`,
			acct.ID, extID, "FOOD_TEST", foodDrink.ID, false)
		if err != nil {
			t.Fatalf("insert transaction %d: %v", i, err)
		}
	}

	// Create 1 overridden transaction
	_, err = pool.Exec(ctx, `INSERT INTO transactions
		(account_id, external_transaction_id, amount, iso_currency_code, date, name, category_primary, category_id, category_override)
		VALUES ($1, $2, 10.00, 'USD', '2024-01-15', 'Override Test', $3, $4, $5)`,
		acct.ID, "retro_txn_override", "FOOD_TEST", foodDrink.ID, true)
	if err != nil {
		t.Fatalf("insert overridden transaction: %v", err)
	}

	// Build TSV: change plaid/FOOD_TEST to point to food_and_drink_groceries
	tsv := "provider\tprovider_category\tcategory_slug\n"
	tsv += "plaid\tFOOD_TEST\tfood_and_drink_groceries\n"

	result, err := svc.ImportMappingsTSV(ctx, tsv, true)
	if err != nil {
		t.Fatalf("ImportMappingsTSV: %v", err)
	}

	if result.Updated != 1 {
		t.Errorf("expected Updated=1, got %d", result.Updated)
	}
	if result.TransactionsUpdated != 3 {
		t.Errorf("expected TransactionsUpdated=3, got %d", result.TransactionsUpdated)
	}

	// Verify the overridden transaction still has the old category_id (food_and_drink)
	var overrideCatID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'retro_txn_override'",
	).Scan(&overrideCatID)
	if err != nil {
		t.Fatalf("query override transaction: %v", err)
	}
	if overrideCatID != foodDrink.ID {
		t.Errorf("overridden transaction should keep old category_id %v, got %v", foodDrink.ID, overrideCatID)
	}

	// Verify a non-overridden transaction now has groceries category_id
	var normalCatID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'retro_txn_0'",
	).Scan(&normalCatID)
	if err != nil {
		t.Fatalf("query normal transaction: %v", err)
	}
	if normalCatID != groceries.ID {
		t.Errorf("non-overridden transaction should have groceries category_id %v, got %v", groceries.ID, normalCatID)
	}
}

func TestRoundTrip_CategoriesTSV(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create some categories
	parent, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
		Icon: pgtype.Text{String: "utensils", Valid: true},
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	_, err = queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries", ParentID: parent.ID,
		Icon: pgtype.Text{String: "shopping-cart", Valid: true},
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	_, err = queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "income", DisplayName: "Income",
	})
	if err != nil {
		t.Fatalf("create income: %v", err)
	}

	// Export
	tsv, err := svc.ExportCategoriesTSV(ctx)
	if err != nil {
		t.Fatalf("ExportCategoriesTSV: %v", err)
	}

	// Count data lines
	lines := strings.Split(strings.TrimSpace(tsv), "\n")
	dataLines := len(lines) - 1 // exclude header

	// Import same content
	result, err := svc.ImportCategoriesTSV(ctx, tsv)
	if err != nil {
		t.Fatalf("ImportCategoriesTSV round-trip: %v", err)
	}

	if result.Created != 0 {
		t.Errorf("expected Created=0 on round-trip, got %d", result.Created)
	}
	if result.Updated != 0 {
		t.Errorf("expected Updated=0 on round-trip, got %d", result.Updated)
	}
	if result.Unchanged != dataLines {
		t.Errorf("expected Unchanged=%d on round-trip, got %d", dataLines, result.Unchanged)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors on round-trip, got %v", result.Errors)
	}
}

func TestRoundTrip_MappingsTSV(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create categories and mappings
	foodDrink, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create food_and_drink: %v", err)
	}
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries", ParentID: foodDrink.ID,
	})
	if err != nil {
		t.Fatalf("create groceries: %v", err)
	}

	_, err = queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider: db.ProviderTypePlaid, ProviderCategory: "FOOD_AND_DRINK", CategoryID: foodDrink.ID,
	})
	if err != nil {
		t.Fatalf("create mapping 1: %v", err)
	}
	_, err = queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider: db.ProviderTypePlaid, ProviderCategory: "FOOD_AND_DRINK_GROCERIES", CategoryID: groceries.ID,
	})
	if err != nil {
		t.Fatalf("create mapping 2: %v", err)
	}

	// Export
	tsv, err := svc.ExportMappingsTSV(ctx)
	if err != nil {
		t.Fatalf("ExportMappingsTSV: %v", err)
	}

	// Count data lines
	lines := strings.Split(strings.TrimSpace(tsv), "\n")
	dataLines := len(lines) - 1

	// Import same content
	result, err := svc.ImportMappingsTSV(ctx, tsv, false)
	if err != nil {
		t.Fatalf("ImportMappingsTSV round-trip: %v", err)
	}

	if result.Created != 0 {
		t.Errorf("expected Created=0 on round-trip, got %d", result.Created)
	}
	if result.Updated != 0 {
		t.Errorf("expected Updated=0 on round-trip, got %d", result.Updated)
	}
	if result.Unchanged != dataLines {
		t.Errorf("expected Unchanged=%d on round-trip, got %d", dataLines, result.Unchanged)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors on round-trip, got %v", result.Errors)
	}
}

// --- Helpers ---

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
