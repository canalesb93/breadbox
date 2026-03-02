package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	gosync "sync"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/provider"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Engine orchestrates transaction syncing across all bank connections.
type Engine struct {
	db        *db.Queries
	pool      *pgxpool.Pool
	providers map[string]provider.Provider
	logger    *slog.Logger
	locks     gosync.Map // connection ID string -> *gosync.Mutex
}

// NewEngine creates a new sync engine.
func NewEngine(queries *db.Queries, pool *pgxpool.Pool, providers map[string]provider.Provider, logger *slog.Logger) *Engine {
	return &Engine{
		db:        queries,
		pool:      pool,
		providers: providers,
		logger:    logger,
	}
}

// Sync runs an incremental transaction sync for a single bank connection.
func (e *Engine) Sync(ctx context.Context, connectionID pgtype.UUID, trigger db.SyncTrigger) error {
	connIDStr := formatUUID(connectionID)
	logger := e.logger.With("connection_id", connIDStr, "trigger", string(trigger))

	// Acquire per-connection lock. If already locked, skip.
	mu := e.getOrCreateMutex(connIDStr)
	if !mu.TryLock() {
		logger.Info("sync already in progress, skipping")
		return nil
	}
	defer mu.Unlock()

	// Create sync_log entry.
	syncLog, err := e.db.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: connectionID,
		Trigger:      trigger,
		Status:       db.SyncStatusInProgress,
		StartedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create sync log: %w", err)
	}

	// Run the sync and capture results.
	added, modified, removed, syncErr := e.runSync(ctx, connectionID, logger)

	// Update sync_log with final status.
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	status := db.SyncStatusSuccess
	var errMsg pgtype.Text
	if syncErr != nil {
		status = db.SyncStatusError
		errMsg = pgtype.Text{String: syncErr.Error(), Valid: true}
	}

	if err := e.db.UpdateSyncLog(ctx, db.UpdateSyncLogParams{
		ID:            syncLog.ID,
		Status:        status,
		CompletedAt:   now,
		AddedCount:    int32(added),
		ModifiedCount: int32(modified),
		RemovedCount:  int32(removed),
		ErrorMessage:  errMsg,
	}); err != nil {
		logger.Error("failed to update sync log", "error", err)
	}

	if syncErr != nil {
		return fmt.Errorf("sync connection %s: %w", connIDStr, syncErr)
	}

	logger.Info("sync completed", "added", added, "modified", modified, "removed", removed)
	return nil
}

// runSync performs the actual sync loop for a connection. Returns counts and any error.
func (e *Engine) runSync(ctx context.Context, connectionID pgtype.UUID, logger *slog.Logger) (added, modified, removed int, err error) {
	// Load connection from DB.
	conn, err := e.db.GetBankConnectionForSync(ctx, connectionID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("load connection: %w", err)
	}

	// Look up the provider.
	prov, ok := e.providers[string(conn.Provider)]
	if !ok {
		return 0, 0, 0, fmt.Errorf("unknown provider: %s", conn.Provider)
	}

	// Build provider.Connection from DB row.
	provConn := provider.Connection{
		ProviderName:         string(conn.Provider),
		ExternalID:           conn.ExternalID.String,
		EncryptedCredentials: conn.EncryptedCredentials,
		UserID:               formatUUID(conn.UserID),
	}

	// Track cursors for pagination.
	previousCursor := ""
	if conn.SyncCursor.Valid {
		previousCursor = conn.SyncCursor.String
	}
	cursor := previousCursor

	// Account ID cache to avoid repeated lookups.
	accountIDCache := make(map[string]pgtype.UUID)

	// Buffer writes so we can discard them on ErrMutationDuringPagination.
	var pendingRemovals []string
	var pendingAdded []provider.Transaction
	var pendingModified []provider.Transaction

	// Pagination loop.
	for {
		result, syncErr := prov.SyncTransactions(ctx, provConn, cursor)
		if syncErr != nil {
			if errors.Is(syncErr, provider.ErrSyncRetryable) {
				logger.Warn("mutation during pagination, resetting cursor")
				cursor = previousCursor
				pendingRemovals = nil
				pendingAdded = nil
				pendingModified = nil
				continue
			}
			if errors.Is(syncErr, provider.ErrReauthRequired) {
				// Mark connection as needing re-auth.
				_ = e.db.UpdateBankConnectionStatus(ctx, db.UpdateBankConnectionStatusParams{
					ID:           connectionID,
					Status:       db.ConnectionStatusPendingReauth,
					ErrorCode:    pgtype.Text{String: "ITEM_LOGIN_REQUIRED", Valid: true},
					ErrorMessage: pgtype.Text{String: "Re-authentication required by institution", Valid: true},
				})
				return 0, 0, 0, syncErr
			}
			return 0, 0, 0, syncErr
		}

		// Buffer results — don't write to DB until pagination is complete.
		pendingRemovals = append(pendingRemovals, result.Removed...)
		pendingAdded = append(pendingAdded, result.Added...)
		pendingModified = append(pendingModified, result.Modified...)

		if result.HasMore {
			cursor = result.Cursor
			continue
		}

		// Pagination complete. Flush all buffered writes to DB.
		// Process removed FIRST.
		for _, externalID := range pendingRemovals {
			if err := e.db.SoftDeleteTransactionByExternalID(ctx, externalID); err != nil {
				logger.Error("soft delete transaction", "external_id", externalID, "error", err)
			}
		}
		removed = len(pendingRemovals)

		// Process added transactions.
		for i := range pendingAdded {
			if err := e.upsertTransaction(ctx, &pendingAdded[i], accountIDCache, logger); err != nil {
				logger.Error("upsert added transaction", "external_id", pendingAdded[i].ExternalID, "error", err)
			}
		}
		added = len(pendingAdded)

		// Process modified transactions (same upsert logic).
		for i := range pendingModified {
			if err := e.upsertTransaction(ctx, &pendingModified[i], accountIDCache, logger); err != nil {
				logger.Error("upsert modified transaction", "external_id", pendingModified[i].ExternalID, "error", err)
			}
		}
		modified = len(pendingModified)

		// Commit cursor.
		if err := e.db.UpdateBankConnectionCursor(ctx, db.UpdateBankConnectionCursorParams{
			ID:         connectionID,
			SyncCursor: pgtype.Text{String: result.Cursor, Valid: true},
		}); err != nil {
			return added, modified, removed, fmt.Errorf("update cursor: %w", err)
		}

		// Fetch and update balances.
		if err := e.updateBalances(ctx, prov, provConn, logger); err != nil {
			logger.Error("update balances failed", "error", err)
			// Non-fatal: balances are best-effort.
		}

		// Update connection status to active (clear any previous errors).
		_ = e.db.UpdateBankConnectionStatus(ctx, db.UpdateBankConnectionStatusParams{
			ID:     connectionID,
			Status: db.ConnectionStatusActive,
		})

		break
	}

	return added, modified, removed, nil
}

// upsertTransaction resolves the account ID and upserts a single transaction.
func (e *Engine) upsertTransaction(ctx context.Context, txn *provider.Transaction, cache map[string]pgtype.UUID, logger *slog.Logger) error {
	accountID, err := e.resolveAccountID(ctx, txn.AccountExternalID, cache)
	if err != nil {
		return fmt.Errorf("resolve account %s: %w", txn.AccountExternalID, err)
	}

	params := db.UpsertTransactionParams{
		AccountID:             accountID,
		ExternalTransactionID: txn.ExternalID,
		PendingTransactionID:  optionalText(txn.PendingExternalID),
		Amount:                decimalToNumeric(txn.Amount),
		IsoCurrencyCode:       pgtype.Text{String: txn.ISOCurrencyCode, Valid: txn.ISOCurrencyCode != ""},
		Date:                  pgtype.Date{Time: txn.Date, Valid: true},
		AuthorizedDate:        optionalDate(txn.AuthorizedDate),
		Datetime:              optionalTimestamptz(txn.Datetime),
		AuthorizedDatetime:    optionalTimestamptz(txn.AuthorizedDatetime),
		Name:                  txn.Name,
		MerchantName:          optionalText(txn.MerchantName),
		CategoryPrimary:       optionalText(txn.CategoryPrimary),
		CategoryDetailed:      optionalText(txn.CategoryDetailed),
		CategoryConfidence:    optionalText(txn.CategoryConfidence),
		PaymentChannel:        pgtype.Text{String: txn.PaymentChannel, Valid: txn.PaymentChannel != ""},
		Pending:               txn.Pending,
	}

	_, err = e.db.UpsertTransaction(ctx, params)
	return err
}

// updateBalances fetches current balances from the provider and updates the DB.
func (e *Engine) updateBalances(ctx context.Context, prov provider.Provider, conn provider.Connection, logger *slog.Logger) error {
	balances, err := prov.GetBalances(ctx, conn)
	if err != nil {
		return fmt.Errorf("get balances: %w", err)
	}

	for _, bal := range balances {
		params := db.UpdateAccountBalancesParams{
			ExternalAccountID: bal.AccountExternalID,
			BalanceCurrent:    decimalToNumeric(bal.Current),
			BalanceAvailable:  optionalDecimalToNumeric(bal.Available),
			BalanceLimit:      optionalDecimalToNumeric(bal.Limit),
			IsoCurrencyCode:   pgtype.Text{String: bal.ISOCurrencyCode, Valid: bal.ISOCurrencyCode != ""},
		}
		if err := e.db.UpdateAccountBalances(ctx, params); err != nil {
			logger.Error("update account balance", "account", bal.AccountExternalID, "error", err)
		}
	}
	return nil
}

// resolveAccountID looks up or caches the internal account UUID for an external account ID.
func (e *Engine) resolveAccountID(ctx context.Context, externalAccountID string, cache map[string]pgtype.UUID) (pgtype.UUID, error) {
	if id, ok := cache[externalAccountID]; ok {
		return id, nil
	}
	id, err := e.db.GetAccountIDByExternalAccountID(ctx, externalAccountID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	cache[externalAccountID] = id
	return id, nil
}

// getOrCreateMutex returns the per-connection mutex, creating one if needed.
func (e *Engine) getOrCreateMutex(connID string) *gosync.Mutex {
	val, _ := e.locks.LoadOrStore(connID, &gosync.Mutex{})
	return val.(*gosync.Mutex)
}

// SyncAll syncs all active bank connections with bounded concurrency.
func (e *Engine) SyncAll(ctx context.Context, trigger db.SyncTrigger) error {
	connections, err := e.db.ListActiveConnections(ctx)
	if err != nil {
		return fmt.Errorf("list active connections: %w", err)
	}

	if len(connections) == 0 {
		e.logger.Info("no active connections to sync")
		return nil
	}

	const maxWorkers = 5
	sem := make(chan struct{}, maxWorkers)
	var wg gosync.WaitGroup

	for _, conn := range connections {
		wg.Add(1)
		connID := conn.ID
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := e.Sync(ctx, connID, trigger); err != nil {
				e.logger.Error("sync connection failed", "connection_id", formatUUID(connID), "error", err)
			}
		}()
	}

	wg.Wait()
	return nil
}

// --- helpers ---

func optionalText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func optionalTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func optionalDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func decimalToNumeric(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func optionalDecimalToNumeric(d *decimal.Decimal) pgtype.Numeric {
	if d == nil {
		return pgtype.Numeric{}
	}
	return decimalToNumeric(*d)
}

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
