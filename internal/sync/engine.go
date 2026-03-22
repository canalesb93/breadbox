package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	gosync "sync"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/provider"

	"github.com/jackc/pgx/v5"
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

	// Fetch excluded account IDs for this connection.
	excludedIDs, err := e.db.ListExcludedAccountIDsByConnection(ctx, connectionID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("list excluded accounts: %w", err)
	}
	excludedSet := make(map[pgtype.UUID]bool, len(excludedIDs))
	for _, id := range excludedIDs {
		excludedSet[id] = true
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

	// Load rule resolver (rules + category mappings) for this provider.
	resolver, err := NewRuleResolver(ctx, e.pool, string(conn.Provider), logger)
	if err != nil {
		logger.Warn("failed to load rule resolver, categories will be NULL", "error", err)
		resolver = nil
	}

	// Read review queue config.
	reviewAutoEnqueue := true
	if cfg, err := e.db.GetAppConfig(ctx, "review_auto_enqueue"); err == nil && cfg.Value.Valid {
		if v, err := strconv.ParseBool(cfg.Value.String); err == nil {
			reviewAutoEnqueue = v
		}
	}
	var confidenceThreshold float64
	if reviewAutoEnqueue {
		confidenceThreshold = 0.5 // default
		if cfg, err := e.db.GetAppConfig(ctx, "review_confidence_threshold"); err == nil && cfg.Value.Valid {
			if v, err := strconv.ParseFloat(cfg.Value.String, 64); err == nil {
				confidenceThreshold = v
			}
		}
	}

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

		// Start transaction for all data writes.
		tx, err := e.pool.Begin(ctx)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("begin transaction: %w", err)
		}
		defer tx.Rollback(ctx)

		txQueries := e.db.WithTx(tx)

		// Process removed FIRST.
		for _, externalID := range pendingRemovals {
			if err := txQueries.SoftDeleteTransactionByExternalID(ctx, externalID); err != nil {
				logger.Error("soft delete transaction", "external_id", externalID, "error", err)
			}
		}
		removed = len(pendingRemovals)

		// Process added transactions (skip excluded accounts).
		var skipped int
		for i := range pendingAdded {
			accountID, err := e.resolveAccountID(ctx, pendingAdded[i].AccountExternalID, accountIDCache)
			if err != nil {
				logger.Error("resolve account for added txn", "external_id", pendingAdded[i].ExternalID, "error", err)
				continue
			}
			if excludedSet[accountID] {
				skipped++
				continue
			}
			txnResult, err := e.upsertTransaction(ctx, txQueries, &pendingAdded[i], accountIDCache, string(conn.Provider), resolver, conn.UserID, logger)
			if err != nil {
				logger.Error("upsert added transaction", "external_id", pendingAdded[i].ExternalID, "error", err)
			} else if reviewAutoEnqueue {
				isNew := txnResult.CreatedAt.Time.Equal(txnResult.UpdatedAt.Time)
				e.enqueueForReview(ctx, txQueries, txnResult, isNew, confidenceThreshold, resolver)
			}
			added++
		}

		// Process modified transactions (skip excluded accounts).
		for i := range pendingModified {
			accountID, err := e.resolveAccountID(ctx, pendingModified[i].AccountExternalID, accountIDCache)
			if err != nil {
				logger.Error("resolve account for modified txn", "external_id", pendingModified[i].ExternalID, "error", err)
				continue
			}
			if excludedSet[accountID] {
				skipped++
				continue
			}
			txnResult, err := e.upsertTransaction(ctx, txQueries, &pendingModified[i], accountIDCache, string(conn.Provider), resolver, conn.UserID, logger)
			if err != nil {
				logger.Error("upsert modified transaction", "external_id", pendingModified[i].ExternalID, "error", err)
			} else if reviewAutoEnqueue {
				e.enqueueForReview(ctx, txQueries, txnResult, false, confidenceThreshold, resolver)
			}
			modified++
		}

		if skipped > 0 {
			logger.Debug("skipped transactions for excluded accounts", "skipped", skipped)
		}

		// Clean up stale pending transactions for Teller connections.
		if string(conn.Provider) == "teller" {
			staleCount := e.cleanStalePending(ctx, tx, connectionID, pendingAdded, previousCursor, logger)
			removed += staleCount
		}

		// Commit cursor.
		if err := txQueries.UpdateBankConnectionCursor(ctx, db.UpdateBankConnectionCursorParams{
			ID:         connectionID,
			SyncCursor: pgtype.Text{String: result.Cursor, Valid: true},
		}); err != nil {
			return added, modified, removed, fmt.Errorf("update cursor: %w", err)
		}

		// Fetch and update balances.
		if err := e.updateBalances(ctx, txQueries, prov, provConn, logger); err != nil {
			logger.Error("update balances failed", "error", err)
			// Non-fatal: balances are best-effort.
		}

		if err := tx.Commit(ctx); err != nil {
			return 0, 0, 0, fmt.Errorf("commit transaction: %w", err)
		}

		// Flush rule hit counts after commit (best-effort, non-fatal).
		if resolver != nil {
			if err := resolver.FlushHitCounts(ctx, e.pool); err != nil {
				logger.Warn("failed to flush rule hit counts", "error", err)
			}
		}

		// Update connection status to active (clear any previous errors).
		// Kept outside the transaction as an independent status update.
		_ = e.db.UpdateBankConnectionStatus(ctx, db.UpdateBankConnectionStatusParams{
			ID:     connectionID,
			Status: db.ConnectionStatusActive,
		})

		break
	}

	return added, modified, removed, nil
}

// upsertTransaction resolves the account ID and upserts a single transaction.
func (e *Engine) upsertTransaction(ctx context.Context, q *db.Queries, txn *provider.Transaction, cache map[string]pgtype.UUID, providerName string, resolver *RuleResolver, userID pgtype.UUID, logger *slog.Logger) (db.Transaction, error) {
	accountID, err := e.resolveAccountID(ctx, txn.AccountExternalID, cache)
	if err != nil {
		return db.Transaction{}, fmt.Errorf("resolve account %s: %w", txn.AccountExternalID, err)
	}

	// Resolve category ID using rules first, then category mappings as fallback.
	var categoryID pgtype.UUID
	if resolver != nil {
		tctx := TransactionContext{
			Name:       txn.Name,
			Amount:     txn.Amount.InexactFloat64(),
			Pending:    txn.Pending,
			Provider:   providerName,
			AccountID:  formatUUID(accountID),
			UserID:     formatUUID(userID),
		}
		if txn.MerchantName != nil {
			tctx.MerchantName = *txn.MerchantName
		}
		if txn.CategoryPrimary != nil {
			tctx.CategoryPrimary = *txn.CategoryPrimary
		}
		if txn.CategoryDetailed != nil {
			tctx.CategoryDetailed = *txn.CategoryDetailed
		}
		categoryID = resolver.ResolveWithContext(providerName, tctx)
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
		CategoryID:            categoryID,
	}

	return q.UpsertTransaction(ctx, params)
}

// updateBalances fetches current balances from the provider and updates the DB.
func (e *Engine) updateBalances(ctx context.Context, q *db.Queries, prov provider.Provider, conn provider.Connection, logger *slog.Logger) error {
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
		if err := q.UpdateAccountBalances(ctx, params); err != nil {
			logger.Error("update account balance", "account", bal.AccountExternalID, "error", err)
		}
	}
	return nil
}

// cleanStalePending soft-deletes pending transactions that were not returned by
// the Teller API during this sync window. This handles the case where a pending
// transaction disappears without posting (e.g., holds that expire).
func (e *Engine) cleanStalePending(ctx context.Context, tx pgx.Tx, connectionID pgtype.UUID, addedTxns []provider.Transaction, previousCursor string, logger *slog.Logger) int {
	// Calculate date window.
	toDate := time.Now()
	var fromDate time.Time
	if previousCursor != "" {
		t, err := time.Parse(time.RFC3339, previousCursor)
		if err != nil {
			logger.Error("parse previous cursor for stale cleanup", "cursor", previousCursor, "error", err)
			return 0
		}
		fromDate = t.AddDate(0, 0, -10)
	} else {
		// Initial sync: look back 2 years.
		fromDate = toDate.AddDate(-2, 0, 0)
	}

	// Collect ALL external_transaction_ids returned by the API (both pending and posted).
	// Any transaction that was returned still exists and should not be deleted.
	returnedIDs := make([]string, 0, len(addedTxns))
	for _, txn := range addedTxns {
		returnedIDs = append(returnedIDs, txn.ExternalID)
	}

	query := `
		UPDATE transactions SET deleted_at = NOW()
		WHERE account_id IN (SELECT id FROM accounts WHERE connection_id = $1)
		  AND date >= $2
		  AND date <= $3
		  AND pending = true
		  AND deleted_at IS NULL
		  AND external_transaction_id != ALL($4)
		RETURNING external_transaction_id`

	rows, err := tx.Query(ctx, query, connectionID, fromDate, toDate, returnedIDs)
	if err != nil {
		logger.Error("clean stale pending transactions", "error", err)
		return 0
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var externalID string
		if err := rows.Scan(&externalID); err != nil {
			logger.Error("scan stale pending transaction", "error", err)
			continue
		}
		logger.Info("soft-deleted stale pending transaction", "external_transaction_id", externalID)
		count++
	}
	if rows.Err() != nil {
		logger.Error("iterate stale pending rows", "error", rows.Err())
	}

	return count
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
