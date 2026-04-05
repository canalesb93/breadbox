//go:build integration

package sync_test

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/provider"
	"breadbox/internal/sync"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// mockProvider implements provider.Provider for testing the sync engine.
type mockProvider struct {
	syncResult  provider.SyncResult
	syncErr     error
	balances    []provider.AccountBalance
	balancesErr error
	syncCalls   int
}

func (m *mockProvider) CreateLinkSession(ctx context.Context, userID string) (provider.LinkSession, error) {
	return provider.LinkSession{}, fmt.Errorf("not implemented")
}

func (m *mockProvider) ExchangeToken(ctx context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	return provider.Connection{}, nil, fmt.Errorf("not implemented")
}

func (m *mockProvider) SyncTransactions(ctx context.Context, conn provider.Connection, cursor string) (provider.SyncResult, error) {
	m.syncCalls++
	if m.syncErr != nil {
		return provider.SyncResult{}, m.syncErr
	}
	return m.syncResult, nil
}

func (m *mockProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	if m.balancesErr != nil {
		return nil, m.balancesErr
	}
	return m.balances, nil
}

func (m *mockProvider) HandleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, fmt.Errorf("not implemented")
}

func (m *mockProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{}, fmt.Errorf("not implemented")
}

func (m *mockProvider) RemoveConnection(ctx context.Context, conn provider.Connection) error {
	return nil
}

// paginatingMockProvider returns HasMore=true on first call, then results on second call.
type paginatingMockProvider struct {
	pages     []provider.SyncResult
	pageIndex int
	balances  []provider.AccountBalance
}

func (m *paginatingMockProvider) CreateLinkSession(ctx context.Context, userID string) (provider.LinkSession, error) {
	return provider.LinkSession{}, fmt.Errorf("not implemented")
}
func (m *paginatingMockProvider) ExchangeToken(ctx context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	return provider.Connection{}, nil, fmt.Errorf("not implemented")
}
func (m *paginatingMockProvider) SyncTransactions(ctx context.Context, conn provider.Connection, cursor string) (provider.SyncResult, error) {
	if m.pageIndex >= len(m.pages) {
		return provider.SyncResult{}, fmt.Errorf("no more pages")
	}
	result := m.pages[m.pageIndex]
	m.pageIndex++
	return result, nil
}
func (m *paginatingMockProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	return m.balances, nil
}
func (m *paginatingMockProvider) HandleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, nil
}
func (m *paginatingMockProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{}, nil
}
func (m *paginatingMockProvider) RemoveConnection(ctx context.Context, conn provider.Connection) error {
	return nil
}

// retryMockProvider returns ErrSyncRetryable on first call, then succeeds.
type retryMockProvider struct {
	callCount int
	result    provider.SyncResult
	balances  []provider.AccountBalance
}

func (m *retryMockProvider) CreateLinkSession(ctx context.Context, userID string) (provider.LinkSession, error) {
	return provider.LinkSession{}, nil
}
func (m *retryMockProvider) ExchangeToken(ctx context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	return provider.Connection{}, nil, nil
}
func (m *retryMockProvider) SyncTransactions(ctx context.Context, conn provider.Connection, cursor string) (provider.SyncResult, error) {
	m.callCount++
	if m.callCount == 1 {
		return provider.SyncResult{}, provider.ErrSyncRetryable
	}
	return m.result, nil
}
func (m *retryMockProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	return m.balances, nil
}
func (m *retryMockProvider) HandleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, nil
}
func (m *retryMockProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{}, nil
}
func (m *retryMockProvider) RemoveConnection(ctx context.Context, conn provider.Connection) error {
	return nil
}

func newEngine(t *testing.T, pool *pgxpool.Pool, queries *db.Queries, providers map[string]provider.Provider) *sync.Engine {
	t.Helper()
	return sync.NewEngine(queries, pool, providers, slog.Default())
}

func seedCategories(t *testing.T, queries *db.Queries) db.Category {
	t.Helper()
	ctx := context.Background()
	uncat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized category: %v", err)
	}
	return uncat
}

func seedCategoriesWithFood(t *testing.T, queries *db.Queries) (uncat db.Category, food db.Category) {
	t.Helper()
	ctx := context.Background()
	uncat = seedCategories(t, queries)
	food, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create food category: %v", err)
	}
	return uncat, food
}

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// --- Tests ---

func TestSync_BasicAddTransactions(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")
	_ = acct

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_1",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(42.50),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Starbucks",
					ISOCurrencyCode:   "USD",
				},
				{
					ExternalID:        "txn_2",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(15.00),
					Date:              time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
					Name:              "Target",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_123",
		},
		balances: []provider.AccountBalance{
			{
				AccountExternalID: "ext_acct_1",
				Current:           decimal.NewFromFloat(500.00),
				ISOCurrencyCode:   "USD",
			},
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify transactions were created.
	var count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE account_id = $1 AND deleted_at IS NULL", acct.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 transactions, got %d", count)
	}

	// Verify sync log was created with success status.
	var logStatus string
	var addedCount int32
	err = pool.QueryRow(ctx, "SELECT status, added_count FROM sync_logs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1", conn.ID).Scan(&logStatus, &addedCount)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if logStatus != "success" {
		t.Errorf("expected sync log status 'success', got %q", logStatus)
	}
	if addedCount != 2 {
		t.Errorf("expected added_count 2, got %d", addedCount)
	}

	// Verify balance was updated.
	var balanceCurrent pgtype.Numeric
	err = pool.QueryRow(ctx, "SELECT balance_current FROM accounts WHERE id = $1", acct.ID).Scan(&balanceCurrent)
	if err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if !balanceCurrent.Valid {
		t.Fatal("expected balance_current to be set")
	}
	// Convert to float for comparison.
	f, _ := new(big.Float).SetInt(balanceCurrent.Int).Float64()
	for i := int32(0); i < -balanceCurrent.Exp; i++ {
		f /= 10
	}
	if f != 500.00 {
		t.Errorf("expected balance 500.00, got %f", f)
	}
}

func TestSync_ModifyTransactions(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	// First sync: add a transaction.
	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_1",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(42.50),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Starbucks",
					ISOCurrencyCode:   "USD",
					Pending:           true,
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second sync: modify the transaction (no longer pending, name changed).
	mock.syncResult = provider.SyncResult{
		Modified: []provider.Transaction{
			{
				ExternalID:        "txn_1",
				AccountExternalID: "ext_acct_1",
				Amount:            decimal.NewFromFloat(42.50),
				Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
				Name:              "Starbucks #1234",
				ISOCurrencyCode:   "USD",
				Pending:           false,
			},
		},
		Cursor: "cursor_2",
	}

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// Verify the transaction was updated.
	var name string
	var pending bool
	err := pool.QueryRow(ctx,
		"SELECT name, pending FROM transactions WHERE external_transaction_id = 'txn_1'",
	).Scan(&name, &pending)
	if err != nil {
		t.Fatalf("query transaction: %v", err)
	}
	if name != "Starbucks #1234" {
		t.Errorf("expected name 'Starbucks #1234', got %q", name)
	}
	if pending {
		t.Error("expected pending=false after modify")
	}

	// Verify second sync log has modified_count=1.
	var modifiedCount int32
	err = pool.QueryRow(ctx,
		"SELECT modified_count FROM sync_logs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1",
		conn.ID,
	).Scan(&modifiedCount)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if modifiedCount != 1 {
		t.Errorf("expected modified_count 1, got %d", modifiedCount)
	}
}

func TestSync_RemoveTransactions(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")
	testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Coffee", 500, "2025-03-01")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Removed: []string{"txn_1"},
			Cursor:  "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify transaction was soft-deleted.
	var deletedAt pgtype.Timestamptz
	err := pool.QueryRow(ctx,
		"SELECT deleted_at FROM transactions WHERE external_transaction_id = 'txn_1'",
	).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("query transaction: %v", err)
	}
	if !deletedAt.Valid {
		t.Error("expected transaction to be soft-deleted (deleted_at set)")
	}

	// Verify sync log records removed count.
	var removedCount int32
	err = pool.QueryRow(ctx,
		"SELECT removed_count FROM sync_logs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1",
		conn.ID,
	).Scan(&removedCount)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if removedCount != 1 {
		t.Errorf("expected removed_count 1, got %d", removedCount)
	}
}

func TestSync_ExcludedAccountsSkipped(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	included := testutil.MustCreateAccount(t, queries, conn.ID, "ext_incl", "Included")
	excluded := testutil.MustCreateAccount(t, queries, conn.ID, "ext_excl", "Excluded")

	// Mark one account as excluded.
	_, err := queries.UpdateAccountExcluded(ctx, db.UpdateAccountExcludedParams{
		ID: excluded.ID, Excluded: true,
	})
	if err != nil {
		t.Fatalf("exclude account: %v", err)
	}
	_ = included

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_incl",
					AccountExternalID: "ext_incl",
					Amount:            decimal.NewFromFloat(10.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Included Txn",
					ISOCurrencyCode:   "USD",
				},
				{
					ExternalID:        "txn_excl",
					AccountExternalID: "ext_excl",
					Amount:            decimal.NewFromFloat(20.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Excluded Txn",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Only the included transaction should exist.
	var count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE account_id = $1", included.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count included txns: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 included transaction, got %d", count)
	}

	err = pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE account_id = $1", excluded.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count excluded txns: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 excluded transactions, got %d", count)
	}
}

func TestSync_ErrReauthRequired_MarksConnection(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncErr: provider.ErrReauthRequired,
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual)
	if err == nil {
		t.Fatal("expected error from Sync() when provider returns ErrReauthRequired")
	}

	// Verify connection status was updated to pending_reauth.
	var status string
	var errorCode pgtype.Text
	err = pool.QueryRow(ctx,
		"SELECT status, error_code FROM bank_connections WHERE id = $1", conn.ID,
	).Scan(&status, &errorCode)
	if err != nil {
		t.Fatalf("query connection: %v", err)
	}
	if status != "pending_reauth" {
		t.Errorf("expected status 'pending_reauth', got %q", status)
	}
	if !errorCode.Valid || errorCode.String != "ITEM_LOGIN_REQUIRED" {
		t.Errorf("expected error_code 'ITEM_LOGIN_REQUIRED', got %v", errorCode)
	}

	// Verify sync log recorded the error.
	var logStatus string
	err = pool.QueryRow(ctx,
		"SELECT status FROM sync_logs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1",
		conn.ID,
	).Scan(&logStatus)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if logStatus != "error" {
		t.Errorf("expected sync log status 'error', got %q", logStatus)
	}
}

func TestSync_ProviderError_RecordedInSyncLog(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncErr: fmt.Errorf("provider is down"),
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual)
	if err == nil {
		t.Fatal("expected error from Sync()")
	}

	// Verify sync log recorded the error message.
	var logStatus string
	var errorMessage pgtype.Text
	err = pool.QueryRow(ctx,
		"SELECT status, error_message FROM sync_logs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1",
		conn.ID,
	).Scan(&logStatus, &errorMessage)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if logStatus != "error" {
		t.Errorf("expected sync log status 'error', got %q", logStatus)
	}
	if !errorMessage.Valid {
		t.Error("expected error_message to be set in sync log")
	}
}

func TestSync_UnknownProvider_Errors(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	// No providers registered.
	providers := map[string]provider.Provider{}
	engine := newEngine(t, pool, queries, providers)

	err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}

	// Sync log should record the error.
	var logStatus string
	err = pool.QueryRow(ctx,
		"SELECT status FROM sync_logs WHERE connection_id = $1",
		conn.ID,
	).Scan(&logStatus)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if logStatus != "error" {
		t.Errorf("expected sync log status 'error', got %q", logStatus)
	}
}

func TestSync_DisconnectedConnection_Errors(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	// Create a disconnected connection.
	disconnConn, err := queries.CreateBankConnection(ctx, db.CreateBankConnectionParams{
		Provider:             db.ProviderTypePlaid,
		ExternalID:           pgtype.Text{String: "item_disc", Valid: true},
		EncryptedCredentials: []byte("test"),
		Status:               db.ConnectionStatusDisconnected,
		UserID:               user.ID,
	})
	if err != nil {
		t.Fatalf("create disconnected connection: %v", err)
	}

	mock := &mockProvider{}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	err = engine.Sync(ctx, disconnConn.ID, db.SyncTriggerManual)
	if err == nil {
		t.Fatal("expected error for disconnected connection")
	}

	// Provider should not have been called.
	if mock.syncCalls > 0 {
		t.Errorf("provider should not be called for disconnected connection, but got %d calls", mock.syncCalls)
	}
}

func TestSync_RuleCategoryDuringSync(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	_, food := seedCategoriesWithFood(t, queries)

	// Create a transaction rule that matches "Restaurant" → food_and_drink.
	_, err := pool.Exec(ctx, `INSERT INTO transaction_rules (name, conditions, category_id, priority, enabled, created_by_type, created_by_name)
		VALUES ('Food Rule', '{"field":"name","op":"contains","value":"Restaurant"}', $1, 100, true, 'system', 'test')`, food.ID)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	catPrimary := "FOOD_AND_DRINK"
	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_food",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(25.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Restaurant",
					CategoryPrimary:   &catPrimary,
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify category was resolved via transaction rule.
	var categoryID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_food'",
	).Scan(&categoryID)
	if err != nil {
		t.Fatalf("query transaction: %v", err)
	}
	if !categoryID.Valid {
		t.Fatal("expected category_id to be set")
	}
	if categoryID != food.ID {
		t.Errorf("expected category_id %v, got %v", food.ID, categoryID)
	}
}

func TestSync_ReviewEnqueueNewTransaction(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	// Enable review auto-enqueue.
	if err := queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key: "review_auto_enqueue", Value: pgtype.Text{String: "true", Valid: true},
	}); err != nil {
		t.Fatalf("set app config: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_new",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(10.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "New Purchase",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify review was enqueued.
	var reviewCount int
	err := pool.QueryRow(ctx,
		"SELECT count(*) FROM review_queue WHERE status = 'pending'",
	).Scan(&reviewCount)
	if err != nil {
		t.Fatalf("count reviews: %v", err)
	}
	if reviewCount == 0 {
		t.Error("expected at least one review to be enqueued for new transaction")
	}
}

func TestSync_ReviewDisabled_NoEnqueue(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	// Disable review auto-enqueue.
	if err := queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key: "review_auto_enqueue", Value: pgtype.Text{String: "false", Valid: true},
	}); err != nil {
		t.Fatalf("set app config: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_new",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(10.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "New Purchase",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify no review was enqueued.
	var reviewCount int
	err := pool.QueryRow(ctx,
		"SELECT count(*) FROM review_queue WHERE status = 'pending'",
	).Scan(&reviewCount)
	if err != nil {
		t.Fatalf("count reviews: %v", err)
	}
	if reviewCount != 0 {
		t.Errorf("expected 0 reviews when auto-enqueue is disabled, got %d", reviewCount)
	}
}

func TestSync_Pagination_BuffersThenFlushes(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	paginatingMock := &paginatingMockProvider{
		pages: []provider.SyncResult{
			{
				Added: []provider.Transaction{
					{
						ExternalID:        "txn_page1",
						AccountExternalID: "ext_acct_1",
						Amount:            decimal.NewFromFloat(10.00),
						Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
						Name:              "Page 1 Transaction",
						ISOCurrencyCode:   "USD",
					},
				},
				Cursor:  "cursor_page2",
				HasMore: true,
			},
			{
				Added: []provider.Transaction{
					{
						ExternalID:        "txn_page2",
						AccountExternalID: "ext_acct_1",
						Amount:            decimal.NewFromFloat(20.00),
						Date:              time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
						Name:              "Page 2 Transaction",
						ISOCurrencyCode:   "USD",
					},
				},
				Cursor:  "cursor_final",
				HasMore: false,
			},
		},
	}

	providers := map[string]provider.Provider{"plaid": paginatingMock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Both pages' transactions should be committed.
	var count int
	err := pool.QueryRow(ctx,
		"SELECT count(*) FROM transactions WHERE account_id = $1 AND deleted_at IS NULL", acct.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 transactions from 2 pages, got %d", count)
	}

	// Verify cursor was saved as the final cursor.
	var cursor pgtype.Text
	err = pool.QueryRow(ctx,
		"SELECT sync_cursor FROM bank_connections WHERE id = $1", conn.ID,
	).Scan(&cursor)
	if err != nil {
		t.Fatalf("query cursor: %v", err)
	}
	if !cursor.Valid || cursor.String != "cursor_final" {
		t.Errorf("expected cursor 'cursor_final', got %v", cursor)
	}
}

func TestSync_RetryOnMutationDuringPagination(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	retryMock := &retryMockProvider{
		result: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_retry",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(30.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Retry Transaction",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_after_retry",
		},
	}

	providers := map[string]provider.Provider{"plaid": retryMock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify the provider was called twice (once failed with retry, once succeeded).
	if retryMock.callCount != 2 {
		t.Errorf("expected 2 provider calls (1 retry + 1 success), got %d", retryMock.callCount)
	}

	// Verify the transaction from the successful call was created.
	var count int
	err := pool.QueryRow(ctx,
		"SELECT count(*) FROM transactions WHERE account_id = $1 AND deleted_at IS NULL", acct.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 transaction after retry, got %d", count)
	}
}

func TestSync_SyncAll_MultpleConnections(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	alice := testutil.MustCreateUser(t, queries, "Alice")
	bob := testutil.MustCreateUser(t, queries, "Bob")

	connA := testutil.MustCreateConnection(t, queries, alice.ID, "item_alice")
	connB := testutil.MustCreateConnection(t, queries, bob.ID, "item_bob")

	testutil.MustCreateAccount(t, queries, connA.ID, "ext_acct_a", "Alice Checking")
	testutil.MustCreateAccount(t, queries, connB.ID, "ext_acct_b", "Bob Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_shared",
					AccountExternalID: "ext_acct_a",
					Amount:            decimal.NewFromFloat(5.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Shared Store",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.SyncAll(ctx, db.SyncTriggerCron); err != nil {
		t.Fatalf("SyncAll() error: %v", err)
	}

	// Verify sync logs were created for both connections.
	var logCount int
	err := pool.QueryRow(ctx, "SELECT count(*) FROM sync_logs").Scan(&logCount)
	if err != nil {
		t.Fatalf("count sync logs: %v", err)
	}
	if logCount < 2 {
		t.Errorf("expected at least 2 sync logs (one per connection), got %d", logCount)
	}
}

func TestSync_ConnectionStatusRestoredToActive(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	// Set connection to error status.
	if err := queries.UpdateBankConnectionStatus(ctx, db.UpdateBankConnectionStatusParams{
		ID:           conn.ID,
		Status:       db.ConnectionStatusError,
		ErrorCode:    pgtype.Text{String: "SOME_ERROR", Valid: true},
		ErrorMessage: pgtype.Text{String: "previous error", Valid: true},
	}); err != nil {
		t.Fatalf("set error status: %v", err)
	}

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify connection status was restored to active.
	var status string
	err := pool.QueryRow(ctx,
		"SELECT status FROM bank_connections WHERE id = $1", conn.ID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("query connection: %v", err)
	}
	if status != "active" {
		t.Errorf("expected status 'active' after successful sync, got %q", status)
	}
}

func TestSync_BalanceUpdateFailure_NonFatal(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_1",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(10.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Purchase",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
		balancesErr: fmt.Errorf("balance API temporarily down"),
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	// Sync should succeed even though balance update fails.
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() should succeed even when balance update fails: %v", err)
	}

	// Transaction should still be created.
	var count int
	err := pool.QueryRow(ctx,
		"SELECT count(*) FROM transactions WHERE external_transaction_id = 'txn_1'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 transaction despite balance failure, got %d", count)
	}

	// Sync log should be success (balance failure is non-fatal).
	var logStatus string
	err = pool.QueryRow(ctx,
		"SELECT status FROM sync_logs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1",
		conn.ID,
	).Scan(&logStatus)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if logStatus != "success" {
		t.Errorf("expected sync log status 'success' (balance failure is non-fatal), got %q", logStatus)
	}
}

func TestSync_TellerStalePendingCleanup(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	// Create a Teller connection with a previous cursor.
	tellerConn := testutil.MustCreateTellerConnection(t, queries, user.ID, "teller_item_1")
	acct := testutil.MustCreateAccount(t, queries, tellerConn.ID, "ext_acct_t", "Teller Checking")

	// Set a previous sync cursor so stale cleanup uses a date window.
	prevCursor := time.Now().AddDate(0, 0, -5).Format(time.RFC3339)
	if err := queries.UpdateBankConnectionCursor(ctx, db.UpdateBankConnectionCursorParams{
		ID:         tellerConn.ID,
		SyncCursor: pgtype.Text{String: prevCursor, Valid: true},
	}); err != nil {
		t.Fatalf("set cursor: %v", err)
	}

	// Create a stale pending transaction (within the sync window, not returned by API).
	staleTxn := testutil.MustCreateTransaction(t, queries, acct.ID, "stale_pending_1", "Stale Hold", 999, time.Now().AddDate(0, 0, -2).Format("2006-01-02"))
	// Mark it as pending.
	_, err := pool.Exec(ctx, "UPDATE transactions SET pending = true WHERE id = $1", staleTxn.ID)
	if err != nil {
		t.Fatalf("set pending: %v", err)
	}

	// Create a non-pending (posted) transaction that should NOT be deleted.
	postedTxn := testutil.MustCreateTransaction(t, queries, acct.ID, "posted_1", "Posted Purchase", 500, time.Now().AddDate(0, 0, -1).Format("2006-01-02"))
	_ = postedTxn

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_returned",
					AccountExternalID: "ext_acct_t",
					Amount:            decimal.NewFromFloat(10.00),
					Date:              time.Now().AddDate(0, 0, -1),
					Name:              "Returned Transaction",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: time.Now().Format(time.RFC3339),
		},
	}

	providers := map[string]provider.Provider{"teller": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, tellerConn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Stale pending transaction should be soft-deleted.
	var staleDeletedAt pgtype.Timestamptz
	err = pool.QueryRow(ctx,
		"SELECT deleted_at FROM transactions WHERE external_transaction_id = 'stale_pending_1'",
	).Scan(&staleDeletedAt)
	if err != nil {
		t.Fatalf("query stale txn: %v", err)
	}
	if !staleDeletedAt.Valid {
		t.Error("expected stale pending transaction to be soft-deleted")
	}

	// Posted transaction should NOT be deleted.
	var postedDeletedAt pgtype.Timestamptz
	err = pool.QueryRow(ctx,
		"SELECT deleted_at FROM transactions WHERE external_transaction_id = 'posted_1'",
	).Scan(&postedDeletedAt)
	if err != nil {
		t.Fatalf("query posted txn: %v", err)
	}
	if postedDeletedAt.Valid {
		t.Error("posted transaction should NOT be soft-deleted by stale cleanup")
	}
}

func TestSync_CursorPersisted(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Cursor: "new_cursor_abc",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify cursor was persisted.
	var cursor pgtype.Text
	err := pool.QueryRow(ctx,
		"SELECT sync_cursor FROM bank_connections WHERE id = $1", conn.ID,
	).Scan(&cursor)
	if err != nil {
		t.Fatalf("query cursor: %v", err)
	}
	if !cursor.Valid || cursor.String != "new_cursor_abc" {
		t.Errorf("expected cursor 'new_cursor_abc', got %v", cursor)
	}
}

func TestSync_RuleBasedCategorization(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	// Create a category for coffee shops.
	coffeeCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "coffee_shops", DisplayName: "Coffee Shops",
	})
	if err != nil {
		t.Fatalf("create coffee category: %v", err)
	}

	// Create a rule that matches "coffee" in name.
	_, err = pool.Exec(ctx,
		`INSERT INTO transaction_rules (name, conditions, category_id, priority, enabled, created_by_type, created_by_name)
		 VALUES ('Coffee Rule', '{"field":"name","op":"contains","value":"coffee"}', $1, 100, true, 'system', 'test')`,
		coffeeCat.ID,
	)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_coffee",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(5.50),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Starbucks Coffee",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify the transaction was categorized by the rule.
	var categoryID pgtype.UUID
	err = pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE external_transaction_id = 'txn_coffee'",
	).Scan(&categoryID)
	if err != nil {
		t.Fatalf("query transaction: %v", err)
	}
	if !categoryID.Valid {
		t.Fatal("expected category_id to be set by rule")
	}
	if categoryID != coffeeCat.ID {
		t.Errorf("expected coffee category ID, got %v", categoryID)
	}

	// Verify rule hit count was incremented.
	var hitCount int
	err = pool.QueryRow(ctx,
		"SELECT hit_count FROM transaction_rules WHERE name = 'Coffee Rule'",
	).Scan(&hitCount)
	if err != nil {
		t.Fatalf("query rule: %v", err)
	}
	if hitCount != 1 {
		t.Errorf("expected hit_count 1, got %d", hitCount)
	}
}

func TestSync_SyncTriggerTypes(t *testing.T) {
	_, _ = testutil.ServicePool(t) // truncate tables
	ctx := context.Background()

	triggers := []db.SyncTrigger{
		db.SyncTriggerCron,
		db.SyncTriggerWebhook,
		db.SyncTriggerManual,
		db.SyncTriggerInitial,
	}

	for _, trigger := range triggers {
		t.Run(string(trigger), func(t *testing.T) {
			// Re-truncate for each sub-test.
			pool, queries := testutil.ServicePool(t)
			seedCategories(t, queries)

			user := testutil.MustCreateUser(t, queries, "Alice")
			conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
			testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

			mock := &mockProvider{
				syncResult: provider.SyncResult{Cursor: "c"},
			}

			providers := map[string]provider.Provider{"plaid": mock}
			engine := newEngine(t, pool, queries, providers)

			if err := engine.Sync(ctx, conn.ID, trigger); err != nil {
				t.Fatalf("Sync(trigger=%s) error: %v", trigger, err)
			}

			var logTrigger string
			err := pool.QueryRow(ctx,
				"SELECT trigger FROM sync_logs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1",
				conn.ID,
			).Scan(&logTrigger)
			if err != nil {
				t.Fatalf("query sync log: %v", err)
			}
			if logTrigger != string(trigger) {
				t.Errorf("expected trigger %q in sync log, got %q", trigger, logTrigger)
			}
		})
	}
}

func TestSync_UncategorizedReviewType(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	// Enable review auto-enqueue.
	if err := queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key: "review_auto_enqueue", Value: pgtype.Text{String: "true", Valid: true},
	}); err != nil {
		t.Fatalf("set app config: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	// Transaction with no category mapping should be enqueued as "uncategorized".
	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_uncat",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(10.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Unknown Store",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify review type is "uncategorized" (since it resolves to the uncategorized fallback category).
	var reviewType string
	err := pool.QueryRow(ctx,
		`SELECT rq.review_type FROM review_queue rq
		 JOIN transactions t ON t.id = rq.transaction_id
		 WHERE t.external_transaction_id = 'txn_uncat' AND rq.status = 'pending'`,
	).Scan(&reviewType)
	if err != nil {
		t.Fatalf("query review: %v", err)
	}
	if reviewType != "uncategorized" {
		t.Errorf("expected review_type 'uncategorized', got %q", reviewType)
	}
}

func TestSync_CategorizedNewTransaction(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	_, food := seedCategoriesWithFood(t, queries)
	_ = food

	// Enable review auto-enqueue.
	if err := queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key: "review_auto_enqueue", Value: pgtype.Text{String: "true", Valid: true},
	}); err != nil {
		t.Fatalf("set app config: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	catPrimary := "FOOD_AND_DRINK"
	lowConfidence := "LOW"
	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:         "txn_categorized_new",
					AccountExternalID:  "ext_acct_1",
					Amount:             decimal.NewFromFloat(25.00),
					Date:               time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:               "Restaurant",
					CategoryPrimary:    &catPrimary,
					CategoryConfidence: &lowConfidence,
					ISOCurrencyCode:    "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// With confidence threshold removed, a categorized transaction gets "new_transaction" type.
	var reviewType string
	err := pool.QueryRow(ctx,
		`SELECT rq.review_type FROM review_queue rq
		 JOIN transactions t ON t.id = rq.transaction_id
		 WHERE t.external_transaction_id = 'txn_categorized_new' AND rq.status = 'pending'`,
	).Scan(&reviewType)
	if err != nil {
		t.Fatalf("query review: %v", err)
	}
	if reviewType != "new_transaction" {
		t.Errorf("expected review_type 'new_transaction', got %q", reviewType)
	}
}

func TestSync_UnchangedTransactions_SkipRuleReapplication(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	// Create a category and rule for coffee.
	coffeeCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "coffee_shops", DisplayName: "Coffee Shops",
	})
	if err != nil {
		t.Fatalf("create coffee category: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO transaction_rules (name, conditions, category_id, priority, enabled, created_by_type, created_by_name)
		 VALUES ('Coffee Rule', '{"field":"name","op":"contains","value":"coffee"}', $1, 100, true, 'system', 'test')`,
		coffeeCat.ID,
	)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	txns := []provider.Transaction{
		{
			ExternalID:        "txn_coffee",
			AccountExternalID: "ext_acct_1",
			Amount:            decimal.NewFromFloat(5.50),
			Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Name:              "Starbucks Coffee",
			ISOCurrencyCode:   "USD",
		},
	}

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added:  txns,
			Cursor: "cursor_1",
		},
	}

	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	// First sync: transaction is new, rule should apply.
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Verify rule was applied and record the applied_at timestamp.
	var appliedAt1 time.Time
	err = pool.QueryRow(ctx,
		`SELECT applied_at FROM transaction_rule_applications
		 WHERE transaction_id = (SELECT id FROM transactions WHERE external_transaction_id = 'txn_coffee')`,
	).Scan(&appliedAt1)
	if err != nil {
		t.Fatalf("query rule application after first sync: %v", err)
	}

	var hitCount1 int
	err = pool.QueryRow(ctx, "SELECT hit_count FROM transaction_rules WHERE name = 'Coffee Rule'").Scan(&hitCount1)
	if err != nil {
		t.Fatalf("query hit count: %v", err)
	}
	if hitCount1 != 1 {
		t.Errorf("expected hit_count 1 after first sync, got %d", hitCount1)
	}

	// Second sync: same transaction data (unchanged), rule should NOT be re-applied.
	// Provider returns same transaction in Added (simulating a re-sync).
	mock.syncResult = provider.SyncResult{
		Added:  txns,
		Cursor: "cursor_2",
	}

	// Wait long enough that classifyUpsertResult can distinguish the first
	// sync's created_at from the second sync's upsertStart (2s tolerance).
	time.Sleep(3 * time.Second)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// Verify applied_at was NOT bumped.
	var appliedAt2 time.Time
	err = pool.QueryRow(ctx,
		`SELECT applied_at FROM transaction_rule_applications
		 WHERE transaction_id = (SELECT id FROM transactions WHERE external_transaction_id = 'txn_coffee')`,
	).Scan(&appliedAt2)
	if err != nil {
		t.Fatalf("query rule application after second sync: %v", err)
	}
	if !appliedAt2.Equal(appliedAt1) {
		t.Errorf("rule application applied_at was bumped on unchanged transaction: first=%v, second=%v", appliedAt1, appliedAt2)
	}

	// Verify hit count was NOT incremented again.
	var hitCount2 int
	err = pool.QueryRow(ctx, "SELECT hit_count FROM transaction_rules WHERE name = 'Coffee Rule'").Scan(&hitCount2)
	if err != nil {
		t.Fatalf("query hit count: %v", err)
	}
	if hitCount2 != hitCount1 {
		t.Errorf("expected hit_count to stay at %d after unchanged re-sync, got %d", hitCount1, hitCount2)
	}

	// Verify the sync log shows 0 added, 0 modified, 1 unchanged.
	var addedCount, modifiedCount, unchangedCount int32
	err = pool.QueryRow(ctx,
		`SELECT added_count, modified_count, unchanged_count FROM sync_logs
		 WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 1`,
		conn.ID,
	).Scan(&addedCount, &modifiedCount, &unchangedCount)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if addedCount != 0 {
		t.Errorf("expected added_count 0 on re-sync, got %d", addedCount)
	}
	if modifiedCount != 0 {
		t.Errorf("expected modified_count 0 on re-sync, got %d", modifiedCount)
	}
	if unchangedCount != 1 {
		t.Errorf("expected unchanged_count 1 on re-sync, got %d", unchangedCount)
	}
}
