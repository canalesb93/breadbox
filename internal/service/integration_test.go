package service_test

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"testing"
	"time"

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

// helper to build pgtype.Text
func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

// helper to build pgtype.Numeric from cents (e.g., 450 → 4.50)
func cents(v int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(v), Exp: -2, Valid: true}
}

// helper to build a connection params struct with defaults
func connParams(userID pgtype.UUID, extID string) db.CreateBankConnectionParams {
	return db.CreateBankConnectionParams{
		Provider:             db.ProviderTypePlaid,
		ExternalID:           text(extID),
		EncryptedCredentials: []byte("encrypted"),
		Status:               db.ConnectionStatusActive,
		UserID:               userID,
	}
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
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

	queries.CreateUser(ctx, db.CreateUserParams{Name: "Alice", Email: text("alice@test.com")})
	queries.CreateUser(ctx, db.CreateUserParams{Name: "Bob", Email: text("bob@test.com")})

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

	user, _ := queries.CreateUser(ctx, db.CreateUserParams{Name: "Alice"})
	cp := connParams(user.ID, "test_item_1")
	cp.InstitutionName = text("Chase")
	conn, _ := queries.CreateBankConnection(ctx, cp)

	queries.UpsertAccount(ctx, db.UpsertAccountParams{
		ConnectionID:      conn.ID,
		ExternalAccountID: "ext_acct_1",
		Name:              "Checking",
		Type:              "depository",
		Subtype:           text("checking"),
		IsoCurrencyCode:   text("USD"),
		BalanceCurrent:    cents(100000),
	})

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
	if accounts[0].BalanceCurrent == nil || *accounts[0].BalanceCurrent != 1000.0 {
		t.Errorf("expected balance 1000.0, got %v", accounts[0].BalanceCurrent)
	}
}

func TestListAccounts_FilterByUser(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice, _ := queries.CreateUser(ctx, db.CreateUserParams{Name: "Alice"})
	bob, _ := queries.CreateUser(ctx, db.CreateUserParams{Name: "Bob"})

	connA, _ := queries.CreateBankConnection(ctx, connParams(alice.ID, "item_a"))
	connB, _ := queries.CreateBankConnection(ctx, connParams(bob.ID, "item_b"))

	queries.UpsertAccount(ctx, db.UpsertAccountParams{
		ConnectionID: connA.ID, ExternalAccountID: "ext_a1", Name: "Alice Checking", Type: "depository",
	})
	queries.UpsertAccount(ctx, db.UpsertAccountParams{
		ConnectionID: connB.ID, ExternalAccountID: "ext_b1", Name: "Bob Checking", Type: "depository",
	})

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

	user, _ := queries.CreateUser(ctx, db.CreateUserParams{Name: "Alice"})
	queries.CreateBankConnection(ctx, connParams(user.ID, "item_active"))
	dp := connParams(user.ID, "item_disc")
	dp.Status = db.ConnectionStatusDisconnected
	queries.CreateBankConnection(ctx, dp)

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
}

// --- Transactions ---

// seedTxnFixture creates a user → connection → account chain and returns the account ID.
func seedTxnFixture(t *testing.T, queries *db.Queries) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	user, _ := queries.CreateUser(ctx, db.CreateUserParams{Name: "Alice"})
	conn, _ := queries.CreateBankConnection(ctx, connParams(user.ID, "item_1"))
	acct, _ := queries.UpsertAccount(ctx, db.UpsertAccountParams{
		ConnectionID: conn.ID, ExternalAccountID: "ext_1", Name: "Checking", Type: "depository",
	})
	return acct.ID
}

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

	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_001",
		Amount: cents(450), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-15"), Valid: true},
		Name: "Starbucks", MerchantName: text("Starbucks"),
		CategoryPrimary: text("FOOD_AND_DRINK"), PaymentChannel: text("in_store"),
	})
	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_002",
		Amount: cents(4215), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-14"), Valid: true},
		Name: "Shell Gas", PaymentChannel: text("in_store"),
	})

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(result.Transactions))
	}
	// Default sort is date DESC
	if result.Transactions[0].Name != "Starbucks" {
		t.Errorf("expected first transaction Starbucks (newer), got %s", result.Transactions[0].Name)
	}
	if result.Transactions[0].Amount != 4.50 {
		t.Errorf("expected amount 4.50, got %f", result.Transactions[0].Amount)
	}
}

func TestListTransactions_Pagination(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)

	for i := 0; i < 5; i++ {
		queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
			AccountID: acctID, ExternalTransactionID: fmt.Sprintf("txn_%03d", i),
			Amount: cents(int64(i*100 + 100)), IsoCurrencyCode: text("USD"),
			Date: pgtype.Date{Time: mustDate(fmt.Sprintf("2025-01-%02d", 15-i)), Valid: true},
			Name: fmt.Sprintf("Transaction %d", i),
		})
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

	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_a",
		Amount: cents(500), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-15"), Valid: true}, Name: "Starbucks Coffee",
	})
	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_b",
		Amount: cents(3000), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-14"), Valid: true}, Name: "Shell Gas Station",
	})

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

	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_small",
		Amount: cents(500), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-15"), Valid: true}, Name: "Small",
	})
	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_big",
		Amount: cents(10000), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-14"), Valid: true}, Name: "Big",
	})

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
	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_1",
		Amount: cents(100), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-15"), Valid: true}, Name: "Test",
	})

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

	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_visible",
		Amount: cents(100), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-15"), Valid: true}, Name: "Visible",
	})
	queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
		AccountID: acctID, ExternalTransactionID: "txn_deleted",
		Amount: cents(200), IsoCurrencyCode: text("USD"),
		Date: pgtype.Date{Time: mustDate("2025-01-14"), Valid: true}, Name: "Deleted",
	})

	queries.SoftDeleteTransactionByExternalID(ctx, "txn_deleted")

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
