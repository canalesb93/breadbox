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
	"breadbox/internal/pgconv"
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

// mustCreateCategory is a test helper that inserts a category row and fatals
// on error.
func mustCreateCategory(t *testing.T, queries *db.Queries, slug, displayName string) db.Category {
	t.Helper()
	cat, err := queries.InsertCategory(context.Background(), db.InsertCategoryParams{
		Slug:        slug,
		DisplayName: displayName,
	})
	if err != nil {
		t.Fatalf("mustCreateCategory(%q): %v", slug, err)
	}
	return cat
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

	aliceID := pgconv.FormatUUID(alice.ID)
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
	expectedHeader := "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden\tmerge_into"
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

	result, err := svc.ImportCategoriesTSV(ctx, tsv, false)
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

	result, err := svc.ImportCategoriesTSV(ctx, tsv, false)
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
	result, err := svc.ImportCategoriesTSV(ctx, tsv, false)
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

func TestImportCategoriesTSV_ReplaceMode(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Seed the uncategorized system category (required for DeleteCategory).
	_, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}

	// Create three custom categories.
	_, err = svc.CreateCategory(ctx, service.CreateCategoryParams{
		DisplayName: "Keep Me",
	})
	if err != nil {
		t.Fatalf("create keep_me: %v", err)
	}
	_, err = svc.CreateCategory(ctx, service.CreateCategoryParams{
		DisplayName: "Delete Me",
	})
	if err != nil {
		t.Fatalf("create delete_me: %v", err)
	}
	_, err = svc.CreateCategory(ctx, service.CreateCategoryParams{
		DisplayName: "Also Delete",
	})
	if err != nil {
		t.Fatalf("create also_delete: %v", err)
	}

	// Import with only "keep_me" — replace mode should delete the other two.
	// System categories (uncategorized) are never deleted regardless.
	tsv := "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden\n"
	tsv += "keep_me\tKeep Me\t\t\t\t0\tfalse\n"

	result, err := svc.ImportCategoriesTSV(ctx, tsv, true)
	if err != nil {
		t.Fatalf("ImportCategoriesTSV replace: %v", err)
	}

	if result.Deleted != 2 {
		t.Errorf("expected Deleted=2, got %d; errors=%v", result.Deleted, result.Errors)
	}
	if result.Unchanged != 1 {
		t.Errorf("expected Unchanged=1, got %d (keep_me); created=%d updated=%d", result.Unchanged, result.Created, result.Updated)
	}

	// Verify deleted categories no longer exist.
	_, err = svc.GetCategoryBySlug(ctx, "delete_me")
	if err == nil {
		t.Error("expected delete_me to be deleted")
	}
	_, err = svc.GetCategoryBySlug(ctx, "also_delete")
	if err == nil {
		t.Error("expected also_delete to be deleted")
	}

	// Verify kept category still exists.
	_, err = svc.GetCategoryBySlug(ctx, "keep_me")
	if err != nil {
		t.Errorf("expected keep_me to still exist: %v", err)
	}
}

func TestImportCategoriesTSV_MergeInto(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	// Seed uncategorized (required for DeleteCategory inside MergeCategories).
	_, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}

	// Create source and target categories directly in DB.
	sourceCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "old_coffee", DisplayName: "Coffee Shops",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	targetCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	// Create a transaction categorized under the source.
	user := testutil.MustCreateUser(t, queries, "merge-user")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "merge-conn")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "merge-acct", "Merge Account")

	_, err = queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID:             acct.ID,
		ExternalTransactionID: "txn_merge_1",
		Amount:                pgtype.Numeric{Int: big.NewInt(550), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: testutil.MustParseDate("2025-01-15"), Valid: true},
		Name:                  "Starbucks",
		CategoryID:            sourceCat.ID,
	})
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	// Import with merge_into: old_coffee → food_and_drink
	tsv := "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden\tmerge_into\n"
	tsv += "food_and_drink\tFood & Drink\t\tutensils\t#f97316\t0\tfalse\t\n"
	tsv += "old_coffee\t\t\t\t\t\t\tfood_and_drink\n"

	result, err := svc.ImportCategoriesTSV(ctx, tsv, false)
	if err != nil {
		t.Fatalf("ImportCategoriesTSV merge: %v", err)
	}

	if result.Merged != 1 {
		t.Errorf("expected Merged=1, got %d; errors=%v", result.Merged, result.Errors)
	}
	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// Verify source category no longer exists.
	_, err = svc.GetCategoryBySlug(ctx, "old_coffee")
	if err == nil {
		t.Error("expected old_coffee to be deleted after merge")
	}

	// Verify transaction was reassigned to target.
	var gotCatID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_merge_1'",
	).Scan(&gotCatID)
	if err != nil {
		t.Fatalf("query transaction: %v", err)
	}
	if gotCatID.Bytes != targetCat.ID.Bytes {
		t.Errorf("expected transaction category_id=%v, got %v", targetCat.ID, gotCatID)
	}
}

func TestImportCategoriesTSV_MergeWithChildren(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Seed uncategorized.
	_, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}

	// Create parent with children.
	parent, err := svc.CreateCategory(ctx, service.CreateCategoryParams{
		Slug: "old_parent", DisplayName: "Old Parent",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	_, err = svc.CreateCategory(ctx, service.CreateCategoryParams{
		Slug: "old_child_1", DisplayName: "Old Child 1", ParentID: &parent.ID,
	})
	if err != nil {
		t.Fatalf("create child 1: %v", err)
	}
	_, err = svc.CreateCategory(ctx, service.CreateCategoryParams{
		Slug: "old_child_2", DisplayName: "Old Child 2", ParentID: &parent.ID,
	})
	if err != nil {
		t.Fatalf("create child 2: %v", err)
	}

	// Create target.
	_, err = svc.CreateCategory(ctx, service.CreateCategoryParams{
		Slug: "new_target", DisplayName: "New Target",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	// Merge parent (which has 2 children) → target. Should merge 3 total (2 children + 1 parent).
	tsv := "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden\tmerge_into\n"
	tsv += "new_target\tNew Target\t\t\t\t0\tfalse\t\n"
	tsv += "old_parent\t\t\t\t\t\t\tnew_target\n"

	result, err := svc.ImportCategoriesTSV(ctx, tsv, false)
	if err != nil {
		t.Fatalf("ImportCategoriesTSV merge with children: %v", err)
	}

	if result.Merged != 3 {
		t.Errorf("expected Merged=3 (2 children + 1 parent), got %d; errors=%v", result.Merged, result.Errors)
	}

	// All three should be gone.
	for _, slug := range []string{"old_parent", "old_child_1", "old_child_2"} {
		_, err := svc.GetCategoryBySlug(ctx, slug)
		if err == nil {
			t.Errorf("expected %s to be deleted after merge", slug)
		}
	}
}

func TestImportCategoriesTSV_MergeNonexistentSource(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Create target only — source doesn't exist. Should skip silently.
	_, err := svc.CreateCategory(ctx, service.CreateCategoryParams{
		Slug: "target_cat", DisplayName: "Target",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	tsv := "slug\tdisplay_name\tparent_slug\ticon\tcolor\tsort_order\thidden\tmerge_into\n"
	tsv += "target_cat\tTarget\t\t\t\t0\tfalse\t\n"
	tsv += "nonexistent\t\t\t\t\t\t\ttarget_cat\n"

	result, err := svc.ImportCategoriesTSV(ctx, tsv, false)
	if err != nil {
		t.Fatalf("ImportCategoriesTSV: %v", err)
	}

	// Should skip the nonexistent source silently (no error, no merge count).
	if result.Merged != 0 {
		t.Errorf("expected Merged=0, got %d", result.Merged)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %v", result.Errors)
	}
}

// --- Helpers ---

// --- Connection Deletion Soft-Deletes Transactions ---

func TestSoftDeleteTransactionsByConnectionID(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create two connections with accounts and transactions.
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn1 := testutil.MustCreateConnection(t, queries, user.ID, "item_del_1")
	conn2 := testutil.MustCreateConnection(t, queries, user.ID, "item_del_2")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_acct_1", "Checking")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_acct_2", "Savings")

	testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_c1_a", "Coffee", 5, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_c1_b", "Lunch", 15, "2025-01-16")
	testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_c2_a", "Groceries", 50, "2025-01-15")

	// Soft-delete transactions for connection 1 only.
	deleted, err := queries.SoftDeleteTransactionsByConnectionID(ctx, conn1.ID)
	if err != nil {
		t.Fatalf("soft delete by connection: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", deleted)
	}

	// Only connection 2's transaction should remain visible.
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{})
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 visible transaction, got %d", len(result.Transactions))
	}
	if result.Transactions[0].Name != "Groceries" {
		t.Errorf("expected Groceries, got %s", result.Transactions[0].Name)
	}
}

// --- BatchSetTransactionCategory ---

func TestBatchSetTransactionCategory_Success(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	txn1 := testutil.MustCreateTransaction(t, queries, acctID, "txn_batch_1", "Coffee", 500, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acctID, "txn_batch_2", "Lunch", 1200, "2025-01-16")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_batch_3", "Groceries", 5000, "2025-01-17")

	cat1, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create category 1: %v", err)
	}
	cat2, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "transportation", DisplayName: "Transportation",
	})
	if err != nil {
		t.Fatalf("create category 2: %v", err)
	}

	result, err := svc.BatchSetTransactionCategory(ctx, []service.BatchCategorizeItem{
		{TransactionID: pgconv.FormatUUID(txn1.ID), CategorySlug: "food_and_drink"},
		{TransactionID: pgconv.FormatUUID(txn2.ID), CategorySlug: "transportation"},
	})
	if err != nil {
		t.Fatalf("BatchSetTransactionCategory: %v", err)
	}
	if result.Succeeded != 2 {
		t.Errorf("expected succeeded=2, got %d", result.Succeeded)
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}

	// Verify txn1 has cat1
	var gotCatID pgtype.UUID
	err = pool.QueryRow(ctx, "SELECT category_id FROM transactions WHERE id = $1", txn1.ID).Scan(&gotCatID)
	if err != nil {
		t.Fatalf("query txn1 category: %v", err)
	}
	if gotCatID != cat1.ID {
		t.Errorf("txn1: expected category %v, got %v", cat1.ID, gotCatID)
	}

	// Verify txn2 has cat2
	err = pool.QueryRow(ctx, "SELECT category_id FROM transactions WHERE id = $1", txn2.ID).Scan(&gotCatID)
	if err != nil {
		t.Fatalf("query txn2 category: %v", err)
	}
	if gotCatID != cat2.ID {
		t.Errorf("txn2: expected category %v, got %v", cat2.ID, gotCatID)
	}

	// Verify txn3 is unchanged (no category_id set)
	var gotCatIDNullable pgtype.UUID
	err = pool.QueryRow(ctx, "SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_batch_3'").Scan(&gotCatIDNullable)
	if err != nil {
		t.Fatalf("query txn3 category: %v", err)
	}
	if gotCatIDNullable.Valid {
		t.Errorf("txn3 should have no category, got %v", gotCatIDNullable)
	}
}

func TestBatchSetTransactionCategory_InvalidTxnID(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	txn1 := testutil.MustCreateTransaction(t, queries, acctID, "txn_ok", "Coffee", 500, "2025-01-15")

	_, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	// Nonexistent transaction ID now correctly reports as failed (ErrNotFound).
	result, err := svc.BatchSetTransactionCategory(ctx, []service.BatchCategorizeItem{
		{TransactionID: pgconv.FormatUUID(txn1.ID), CategorySlug: "food_and_drink"},
		{TransactionID: "00000000-0000-0000-0000-000000000099", CategorySlug: "food_and_drink"},
	})
	if err != nil {
		t.Fatalf("BatchSetTransactionCategory: %v", err)
	}
	if result.Succeeded != 1 {
		t.Errorf("expected succeeded=1, got %d", result.Succeeded)
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestBatchSetTransactionCategory_MaxLimitExceeded(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	items := make([]service.BatchCategorizeItem, 501)
	for i := range items {
		items[i] = service.BatchCategorizeItem{
			TransactionID: fmt.Sprintf("00000000-0000-0000-0000-%012d", i),
			CategorySlug:  "food",
		}
	}

	_, err := svc.BatchSetTransactionCategory(ctx, items)
	if err == nil {
		t.Fatal("expected error for > 500 items, got nil")
	}
	if !strings.Contains(err.Error(), "maximum 500") {
		t.Errorf("expected 'maximum 500' error, got: %v", err)
	}
}

// --- BulkRecategorizeByFilter ---

func TestBulkRecategorizeByFilter_SearchFilter(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_uber_1", "UBER TRIP", 2500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_uber_2", "UBER EATS", 1800, "2025-01-16")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_other_1", "Starbucks", 500, "2025-01-17")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_other_2", "Shell Gas", 4000, "2025-01-18")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_other_3", "Amazon", 7500, "2025-01-19")

	targetCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "transportation", DisplayName: "Transportation",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	search := "UBER"
	result, err := svc.BulkRecategorizeByFilter(ctx, service.BulkRecategorizeParams{
		Search:             &search,
		TargetCategorySlug: "transportation",
	})
	if err != nil {
		t.Fatalf("BulkRecategorizeByFilter: %v", err)
	}
	if result.UpdatedCount != 2 {
		t.Errorf("expected updated_count=2, got %d", result.UpdatedCount)
	}

	// Verify UBER transactions got the target category and category_override=true
	rows, err := pool.Query(ctx,
		"SELECT external_transaction_id, category_id, category_override FROM transactions WHERE name ILIKE '%UBER%' ORDER BY external_transaction_id")
	if err != nil {
		t.Fatalf("query UBER txns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var extID string
		var catID pgtype.UUID
		var override bool
		if err := rows.Scan(&extID, &catID, &override); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if catID != targetCat.ID {
			t.Errorf("%s: expected category %v, got %v", extID, targetCat.ID, catID)
		}
		if !override {
			t.Errorf("%s: expected category_override=true", extID)
		}
	}

	// Verify non-UBER transactions are unchanged
	var unchangedCount int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions WHERE name NOT ILIKE '%UBER%' AND category_id IS NULL").Scan(&unchangedCount)
	if err != nil {
		t.Fatalf("query unchanged: %v", err)
	}
	if unchangedCount != 3 {
		t.Errorf("expected 3 unchanged transactions, got %d", unchangedCount)
	}
}

func TestBulkRecategorizeByFilter_NoFilterError(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	_, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "misc", DisplayName: "Misc",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	_, err = svc.BulkRecategorizeByFilter(ctx, service.BulkRecategorizeParams{
		TargetCategorySlug: "misc",
	})
	if err == nil {
		t.Fatal("expected error when no filter provided, got nil")
	}
	if !strings.Contains(err.Error(), "at least one filter") {
		t.Errorf("expected 'at least one filter' error, got: %v", err)
	}
}

// --- ApplyRuleRetroactively ---

func TestApplyRuleRetroactively_MatchesAndSkipsOverrides(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Create 5 transactions: 3 with "Starbucks" in name, 2 without
	testutil.MustCreateTransaction(t, queries, acctID, "txn_sb_1", "Starbucks Coffee", 500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_sb_2", "Starbucks Reserve", 700, "2025-01-16")
	txnSb3 := testutil.MustCreateTransaction(t, queries, acctID, "txn_sb_3", "Starbucks Drive", 450, "2025-01-17")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_other_1", "Shell Gas", 4000, "2025-01-18")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_other_2", "Amazon", 7500, "2025-01-19")

	// Set one Starbucks transaction with category_override=true (should be skipped)
	overrideCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "other_override", DisplayName: "Other Override",
	})
	if err != nil {
		t.Fatalf("create override category: %v", err)
	}
	_, err = pool.Exec(ctx,
		"UPDATE transactions SET category_id = $1, category_override = TRUE WHERE id = $2",
		overrideCat.ID, txnSb3.ID)
	if err != nil {
		t.Fatalf("set override: %v", err)
	}

	// Create target category
	coffeeCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "coffee", DisplayName: "Coffee",
	})
	if err != nil {
		t.Fatalf("create coffee category: %v", err)
	}

	// Create a rule: name contains "Starbucks"
	rule, err := svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "Starbucks Rule",
		Conditions: service.Condition{
			Field: "name",
			Op:    "contains",
			Value: "Starbucks",
		},
		CategorySlug: "coffee",
		Priority:     100,
		Actor:        service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("CreateTransactionRule: %v", err)
	}

	count, err := svc.ApplyRuleRetroactively(ctx, rule.ID)
	if err != nil {
		t.Fatalf("ApplyRuleRetroactively: %v", err)
	}
	// Should match 2 (txn_sb_1 and txn_sb_2), txn_sb_3 has override
	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}

	// Verify txn_sb_1 and txn_sb_2 have coffee category
	for _, extID := range []string{"txn_sb_1", "txn_sb_2"} {
		var catID pgtype.UUID
		err = pool.QueryRow(ctx,
			"SELECT category_id FROM transactions WHERE external_transaction_id = $1", extID).Scan(&catID)
		if err != nil {
			t.Fatalf("query %s: %v", extID, err)
		}
		if catID != coffeeCat.ID {
			t.Errorf("%s: expected coffee category, got %v", extID, catID)
		}
	}

	// Verify txn_sb_3 still has override category (not changed)
	var overrideCatID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_sb_3'").Scan(&overrideCatID)
	if err != nil {
		t.Fatalf("query txn_sb_3: %v", err)
	}
	if overrideCatID != overrideCat.ID {
		t.Errorf("txn_sb_3: expected override category %v, got %v", overrideCat.ID, overrideCatID)
	}

	// Verify non-matching transactions are unchanged
	for _, extID := range []string{"txn_other_1", "txn_other_2"} {
		var catID pgtype.UUID
		err = pool.QueryRow(ctx,
			"SELECT category_id FROM transactions WHERE external_transaction_id = $1", extID).Scan(&catID)
		if err != nil {
			t.Fatalf("query %s: %v", extID, err)
		}
		if catID.Valid {
			t.Errorf("%s: expected no category, got %v", extID, catID)
		}
	}
}

// --- ApplyAllRulesRetroactively ---

func TestApplyAllRulesRetroactively_PriorityWins(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Create transactions: "Starbucks Coffee" matches both rules, "Shell Gas" matches only gas rule
	testutil.MustCreateTransaction(t, queries, acctID, "txn_sb", "Starbucks Coffee", 500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_gas", "Shell Gas", 4000, "2025-01-16")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_unrelated", "Amazon Purchase", 7500, "2025-01-17")

	// Create categories
	coffeeCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "coffee", DisplayName: "Coffee",
	})
	if err != nil {
		t.Fatalf("create coffee category: %v", err)
	}
	gasCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "gas", DisplayName: "Gas",
	})
	if err != nil {
		t.Fatalf("create gas category: %v", err)
	}

	// Rule 1 (higher priority): name contains "Coffee"
	_, err = svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "Coffee Rule",
		Conditions: service.Condition{
			Field: "name",
			Op:    "contains",
			Value: "Coffee",
		},
		CategorySlug: "coffee",
		Priority:     200,
		Actor:        service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("create coffee rule: %v", err)
	}

	// Rule 2 (lower priority): name contains "Gas"
	_, err = svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name: "Gas Rule",
		Conditions: service.Condition{
			Field: "name",
			Op:    "contains",
			Value: "Gas",
		},
		CategorySlug: "gas",
		Priority:     100,
		Actor:        service.Actor{Type: "system", Name: "test"},
	})
	if err != nil {
		t.Fatalf("create gas rule: %v", err)
	}

	hitCounts, err := svc.ApplyAllRulesRetroactively(ctx)
	if err != nil {
		t.Fatalf("ApplyAllRulesRetroactively: %v", err)
	}

	// Coffee rule should match "Starbucks Coffee" (1 hit)
	// Gas rule should match "Shell Gas" (1 hit)
	totalHits := int64(0)
	for _, c := range hitCounts {
		totalHits += c
	}
	if totalHits != 2 {
		t.Errorf("expected total hits=2, got %d (map: %v)", totalHits, hitCounts)
	}

	// Verify "Starbucks Coffee" got coffee category (higher priority rule won)
	var catID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_sb'").Scan(&catID)
	if err != nil {
		t.Fatalf("query txn_sb: %v", err)
	}
	if catID != coffeeCat.ID {
		t.Errorf("txn_sb: expected coffee category %v, got %v", coffeeCat.ID, catID)
	}

	// Verify "Shell Gas" got gas category
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_gas'").Scan(&catID)
	if err != nil {
		t.Fatalf("query txn_gas: %v", err)
	}
	if catID != gasCat.ID {
		t.Errorf("txn_gas: expected gas category %v, got %v", gasCat.ID, catID)
	}

	// Verify "Amazon Purchase" is unchanged
	var unrelatedCatID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_unrelated'").Scan(&unrelatedCatID)
	if err != nil {
		t.Fatalf("query txn_unrelated: %v", err)
	}
	if unrelatedCatID.Valid {
		t.Errorf("txn_unrelated: expected no category, got %v", unrelatedCatID)
	}
}

// --- PreviewRule ---

func TestPreviewRule_MatchCount(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	testutil.MustCreateTransaction(t, queries, acctID, "txn_sb_1", "Starbucks", 500, "2025-01-15")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_sb_2", "Starbucks Reserve", 700, "2025-01-16")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_sb_3", "Starbucks Drive", 450, "2025-01-17")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_other_1", "Shell Gas", 4000, "2025-01-18")
	testutil.MustCreateTransaction(t, queries, acctID, "txn_other_2", "Amazon", 7500, "2025-01-19")

	result, err := svc.PreviewRule(ctx, service.Condition{
		Field: "name",
		Op:    "contains",
		Value: "Starbucks",
	}, 10)
	if err != nil {
		t.Fatalf("PreviewRule: %v", err)
	}
	if result.MatchCount != 3 {
		t.Errorf("expected match_count=3, got %d", result.MatchCount)
	}
	if result.TotalScanned != 5 {
		t.Errorf("expected total_scanned=5, got %d", result.TotalScanned)
	}
	if len(result.SampleMatches) != 3 {
		t.Errorf("expected 3 sample_matches, got %d", len(result.SampleMatches))
	}
}

func TestPreviewRule_SampleSizeLimit(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	// Create 5 transactions that all match
	for i := 0; i < 5; i++ {
		testutil.MustCreateTransaction(t, queries, acctID,
			fmt.Sprintf("txn_match_%d", i),
			fmt.Sprintf("Starbucks %d", i),
			int64(500+i*100), "2025-01-15")
	}

	result, err := svc.PreviewRule(ctx, service.Condition{
		Field: "name",
		Op:    "contains",
		Value: "Starbucks",
	}, 2) // limit to 2 samples
	if err != nil {
		t.Fatalf("PreviewRule: %v", err)
	}
	if result.MatchCount != 5 {
		t.Errorf("expected match_count=5, got %d", result.MatchCount)
	}
	if result.TotalScanned != 5 {
		t.Errorf("expected total_scanned=5, got %d", result.TotalScanned)
	}
	if len(result.SampleMatches) != 2 {
		t.Errorf("expected 2 sample_matches (limited by sampleSize), got %d", len(result.SampleMatches))
	}
}
