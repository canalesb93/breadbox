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

// --- Helpers ---

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
