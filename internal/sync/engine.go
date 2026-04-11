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

// accountSyncCounts tracks per-account transaction counts during a sync.
type accountSyncCounts struct {
	AccountID   pgtype.UUID
	AccountName string
	Added       int
	Modified    int
	Removed     int
	Unchanged   int
}

// pendingApplication holds a deferred rule application record to be batch-inserted
// after the transaction processing loops complete.
type pendingApplication struct {
	txnID       pgtype.UUID
	ruleID      pgtype.UUID
	actionField string
	actionValue string
}

// Engine orchestrates transaction syncing across all bank connections.
type Engine struct {
	db                *db.Queries
	pool              *pgxpool.Pool
	providers         map[string]provider.Provider
	logger            *slog.Logger
	locks             gosync.Map // connection ID string -> *gosync.Mutex
	matcher           *Matcher
	balanceRetryDelay time.Duration // delay between balance fetch retries (default 2s)
}

// NewEngine creates a new sync engine.
func NewEngine(queries *db.Queries, pool *pgxpool.Pool, providers map[string]provider.Provider, logger *slog.Logger) *Engine {
	return &Engine{
		db:                queries,
		pool:              pool,
		providers:         providers,
		logger:            logger,
		matcher:           NewMatcher(queries, pool, logger.With("component", "matcher")),
		balanceRetryDelay: 2 * time.Second,
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
	added, modified, removed, unchanged, perAccount, ruleHits, warningMsg, syncErr := e.runSync(ctx, connectionID, logger)

	// Update sync_log with final status.
	completedAt := time.Now()
	now := pgtype.Timestamptz{Time: completedAt, Valid: true}
	status := db.SyncStatusSuccess
	var errMsg pgtype.Text
	if syncErr != nil {
		status = db.SyncStatusError
		errMsg = pgtype.Text{String: syncErr.Error(), Valid: true}
	}

	var warnMsg pgtype.Text
	if warningMsg != "" {
		warnMsg = pgtype.Text{String: warningMsg, Valid: true}
	}

	// Compute duration in milliseconds from started_at.
	var durationMs pgtype.Int4
	if syncLog.StartedAt.Valid {
		ms := completedAt.Sub(syncLog.StartedAt.Time).Milliseconds()
		durationMs = pgtype.Int4{Int32: int32(ms), Valid: true}
	}

	if err := e.db.UpdateSyncLog(ctx, db.UpdateSyncLogParams{
		ID:             syncLog.ID,
		Status:         status,
		CompletedAt:    now,
		AddedCount:     int32(added),
		ModifiedCount:  int32(modified),
		RemovedCount:   int32(removed),
		UnchangedCount: int32(unchanged),
		ErrorMessage:   errMsg,
		DurationMs:     durationMs,
		RuleHits:       ruleHits,
		WarningMessage: warnMsg,
	}); err != nil {
		logger.Error("failed to update sync log", "error", err)
	}

	// Save per-account breakdown (best-effort, non-fatal).
	e.saveSyncLogAccounts(ctx, syncLog.ID, perAccount, logger)

	// Update consecutive failure tracking on the connection.
	if syncErr != nil {
		if err := e.db.IncrementConsecutiveFailures(ctx, connectionID); err != nil {
			logger.Error("failed to increment consecutive failures", "error", err)
		}
		return fmt.Errorf("sync connection %s: %w", connIDStr, syncErr)
	}

	if err := e.db.ResetConsecutiveFailures(ctx, connectionID); err != nil {
		logger.Error("failed to reset consecutive failures", "error", err)
	}

	logger.Info("sync completed", "added", added, "modified", modified, "removed", removed, "unchanged", unchanged)
	return nil
}

// runSync performs the actual sync loop for a connection. Returns counts, per-account breakdown, rule hit counts JSON, warning message, and any error.
func (e *Engine) runSync(ctx context.Context, connectionID pgtype.UUID, logger *slog.Logger) (added, modified, removed, unchanged int, perAccount map[string]*accountSyncCounts, ruleHits []byte, warning string, err error) {
	// Initialize per-account tracking map.
	perAccount = make(map[string]*accountSyncCounts)

	// Load connection from DB.
	conn, err := e.db.GetBankConnectionForSync(ctx, connectionID)
	if err != nil {
		return 0, 0, 0, 0, nil, nil, "", fmt.Errorf("load connection: %w", err)
	}

	// Look up the provider.
	prov, ok := e.providers[string(conn.Provider)]
	if !ok {
		return 0, 0, 0, 0, nil, nil, "", fmt.Errorf("unknown provider: %s", conn.Provider)
	}

	// Fetch excluded account IDs for this connection.
	excludedIDs, err := e.db.ListExcludedAccountIDsByConnection(ctx, connectionID)
	if err != nil {
		return 0, 0, 0, 0, nil, nil, "", fmt.Errorf("list excluded accounts: %w", err)
	}
	excludedSet := make(map[pgtype.UUID]bool, len(excludedIDs))
	for _, id := range excludedIDs {
		excludedSet[id] = true
	}

	// Look up user name for rule matching.
	var userName string
	if conn.UserID.Valid {
		if user, err := e.db.GetUser(ctx, conn.UserID); err == nil {
			userName = user.Name
		}
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
	reviewAutoEnqueue := false // reviews are off by default
	if cfg, err := e.db.GetAppConfig(ctx, "review_auto_enqueue"); err == nil && cfg.Value.Valid {
		if v, err := strconv.ParseBool(cfg.Value.String); err == nil {
			reviewAutoEnqueue = v
		}
	}

	// Pre-fetch all account IDs and display names for this connection in one query.
	// This eliminates per-transaction DB lookups during the sync loop. The caches
	// still work as lazy fallbacks if a new account appears mid-sync.
	accountIDCache := make(map[string]pgtype.UUID)
	accountNameCache := make(map[string]string) // account UUID string -> display name

	connAccounts, err := e.db.ListAccountsByConnection(ctx, connectionID)
	if err != nil {
		logger.Warn("failed to pre-fetch accounts, will resolve lazily", "error", err)
	} else {
		for _, acct := range connAccounts {
			accountIDCache[acct.ExternalAccountID] = acct.ID
			key := formatUUID(acct.ID)
			if acct.DisplayName.Valid && acct.DisplayName.String != "" {
				accountNameCache[key] = acct.DisplayName.String
			} else {
				accountNameCache[key] = acct.Name
			}
		}
		logger.Debug("pre-fetched account caches", "accounts", len(connAccounts))
	}

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
				return 0, 0, 0, 0, nil, nil, "", syncErr
			}
			return 0, 0, 0, 0, nil, nil, "", syncErr
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
			return 0, 0, 0, 0, nil, nil, "", fmt.Errorf("begin transaction: %w", err)
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
		// Note: providers like Teller return ALL transactions as "Added" on every
		// sync (date-range polling). The upsert handles this correctly (ON CONFLICT
		// DO UPDATE), but we need to count accurately: only truly new rows count as
		// "added"; existing rows that got upserted count as "modified" or "unchanged".
		//
		// Classification logic uses timestamps from the upserted row:
		// - New: created_at ~= updated_at (within 1s) — row was just inserted.
		// - Modified: existing row where updated_at was bumped to NOW() by the
		//   conditional CASE in the upsert (key fields actually changed).
		// - Unchanged: existing row where updated_at was NOT bumped (all key
		//   fields identical to what was already stored).
		upsertStart := time.Now()
		var skipped int
		var ruleApplications []pendingApplication

		processOpts := processTransactionOpts{
			txQueries:        txQueries,
			tx:               tx,
			accountIDCache:   accountIDCache,
			accountNameCache: accountNameCache,
			excludedSet:      excludedSet,
			providerName:     string(conn.Provider),
			resolver:         resolver,
			userID:           conn.UserID,
			userName:         userName,
			reviewAutoEnqueue: reviewAutoEnqueue,
			upsertStart:      upsertStart,
			perAccount:       perAccount,
			logger:           logger,
		}

		for i := range pendingAdded {
			result := e.processTransaction(ctx, &pendingAdded[i], true, processOpts)
			if result.skipped {
				skipped++
				continue
			}
			if result.errored {
				continue
			}
			added += result.added
			modified += result.modified
			unchanged += result.unchanged
			ruleApplications = append(ruleApplications, result.ruleApplications...)
		}

		// Process modified transactions (skip excluded accounts).
		for i := range pendingModified {
			result := e.processTransaction(ctx, &pendingModified[i], false, processOpts)
			if result.skipped {
				skipped++
				continue
			}
			if result.errored {
				continue
			}
			added += result.added
			modified += result.modified
			unchanged += result.unchanged
			ruleApplications = append(ruleApplications, result.ruleApplications...)
		}

		// Batch-insert rule application records.
		for _, app := range ruleApplications {
			_, _ = tx.Exec(ctx,
				`INSERT INTO transaction_rule_applications (transaction_id, rule_id, action_field, action_value, applied_by)
				VALUES ($1, $2, $3, $4, 'sync')
				ON CONFLICT (transaction_id, rule_id, action_field) DO UPDATE
				SET applied_at = NOW(), action_value = EXCLUDED.action_value
				WHERE transaction_rule_applications.action_value IS DISTINCT FROM EXCLUDED.action_value`,
				app.txnID, app.ruleID, app.actionField, app.actionValue)
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
			return added, modified, removed, unchanged, perAccount, nil, "", fmt.Errorf("update cursor: %w", err)
		}

		// Fetch and update balances with retry.
		if balanceWarn := e.updateBalancesWithRetry(ctx, txQueries, prov, provConn, logger); balanceWarn != "" {
			warning = balanceWarn
		}

		if err := tx.Commit(ctx); err != nil {
			return 0, 0, 0, 0, nil, nil, "", fmt.Errorf("commit transaction: %w", err)
		}

		// Capture rule hit counts for the sync log before flushing.
		if resolver != nil {
			ruleHits = resolver.HitCountsJSON()
		}

		// Flush rule hit counts after commit (best-effort, non-fatal).
		if resolver != nil {
			if err := resolver.FlushHitCounts(ctx, e.pool); err != nil {
				logger.Warn("failed to flush rule hit counts", "error", err)
			}
		}

		// Reconcile linked account matches (best-effort, non-fatal).
		e.matcher.ReconcileForConnection(ctx, connectionID)

		// Update connection status to active (clear any previous errors).
		// Kept outside the transaction as an independent status update.
		_ = e.db.UpdateBankConnectionStatus(ctx, db.UpdateBankConnectionStatusParams{
			ID:     connectionID,
			Status: db.ConnectionStatusActive,
		})

		break
	}

	return added, modified, removed, unchanged, perAccount, ruleHits, warning, nil
}

// processTransactionOpts captures the shared context for processing a single
// transaction during sync. These values are identical across all transactions
// in a sync batch, so they are set once and reused.
type processTransactionOpts struct {
	txQueries        *db.Queries
	tx               pgx.Tx
	accountIDCache   map[string]pgtype.UUID
	accountNameCache map[string]string
	excludedSet      map[pgtype.UUID]bool
	providerName     string
	resolver         *RuleResolver
	userID           pgtype.UUID
	userName         string
	reviewAutoEnqueue bool
	upsertStart      time.Time
	perAccount       map[string]*accountSyncCounts
	logger           *slog.Logger
}

// processTransactionResult captures the outcome of processing a single transaction.
type processTransactionResult struct {
	added            int // 1 if newly inserted, 0 otherwise
	modified         int // 1 if existing row changed, 0 otherwise
	unchanged        int // 1 if existing row was identical, 0 otherwise
	skipped          bool // true if account is excluded
	errored          bool // true if resolve or upsert failed (already logged)
	ruleApplications []pendingApplication
}

// processTransaction handles the full lifecycle of a single transaction during
// sync: resolve account, check exclusion, upsert, classify, apply rules, enqueue
// review, and track per-account counts.
//
// When providerAdded is true, the provider classified this transaction as "added"
// (new from the provider's perspective). This allows newly inserted rows to be
// counted as "added" rather than "modified". When false (provider said "modified"),
// all changes are counted as "modified" regardless of DB classification.
func (e *Engine) processTransaction(ctx context.Context, txn *provider.Transaction, providerAdded bool, opts processTransactionOpts) processTransactionResult {
	var result processTransactionResult

	label := "modified"
	if providerAdded {
		label = "added"
	}

	accountID, err := e.resolveAccountID(ctx, txn.AccountExternalID, opts.accountIDCache)
	if err != nil {
		opts.logger.Error("resolve account for "+label+" txn", "external_id", txn.ExternalID, "error", err)
		result.errored = true
		return result
	}
	if opts.excludedSet[accountID] {
		result.skipped = true
		return result
	}

	dbTxn, err := e.upsertTransaction(ctx, opts.txQueries, txn, opts.accountIDCache)
	if err != nil {
		opts.logger.Error("upsert "+label+" transaction", "external_id", txn.ExternalID, "error", err)
		result.errored = true
		return result
	}

	isNew, isChanged := classifyUpsertResult(dbTxn, opts.upsertStart)

	// For provider-added transactions, a newly inserted row counts as "added".
	// For provider-modified transactions, isNew is not expected — all changes
	// are counted as "modified".
	if providerAdded && isNew {
		result.added = 1
	} else if isChanged {
		result.modified = 1
	} else {
		result.unchanged = 1
	}

	// Apply rules to new or changed transactions.
	if isNew || isChanged {
		sources, ruleErr := e.applyRulesToTransaction(ctx, opts.tx, txn, dbTxn, opts.accountIDCache, opts.providerName, opts.resolver, opts.userID, opts.userName)
		if ruleErr != nil {
			opts.logger.Error("apply rules to "+label+" txn", "external_id", txn.ExternalID, "error", ruleErr)
		}
		for _, src := range sources {
			result.ruleApplications = append(result.ruleApplications, pendingApplication{
				txnID: dbTxn.ID, ruleID: src.RuleID, actionField: src.ActionField, actionValue: src.ActionValue,
			})
		}
	}

	// Enqueue for review if enabled and transaction is new or changed.
	if opts.reviewAutoEnqueue && (isNew || isChanged) {
		e.enqueueForReview(ctx, opts.txQueries, dbTxn, providerAdded && isNew, opts.resolver)
	}

	// Track per-account counts.
	if providerAdded && isNew {
		e.trackAccountCount(ctx, opts.perAccount, accountID, opts.accountNameCache, "added")
	} else if isChanged {
		e.trackAccountCount(ctx, opts.perAccount, accountID, opts.accountNameCache, "modified")
	} else {
		e.trackAccountCount(ctx, opts.perAccount, accountID, opts.accountNameCache, "unchanged")
	}

	return result
}

// upsertTransaction upserts a single transaction without rule evaluation.
// Rules are applied separately only when the transaction is new or changed.
func (e *Engine) upsertTransaction(ctx context.Context, q *db.Queries, txn *provider.Transaction, cache map[string]pgtype.UUID) (db.Transaction, error) {
	accountID, err := e.resolveAccountID(ctx, txn.AccountExternalID, cache)
	if err != nil {
		return db.Transaction{}, fmt.Errorf("resolve account %s: %w", txn.AccountExternalID, err)
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

	return q.UpsertTransaction(ctx, params)
}

// applyRulesToTransaction evaluates rules against a transaction and updates the
// category if a rule matches. Called only for new or changed transactions.
func (e *Engine) applyRulesToTransaction(ctx context.Context, tx pgx.Tx, txn *provider.Transaction, dbTxn db.Transaction, cache map[string]pgtype.UUID, providerName string, resolver *RuleResolver, userID pgtype.UUID, userName string) ([]RuleActionSource, error) {
	if resolver == nil {
		return nil, nil
	}

	accountID, _ := e.resolveAccountID(ctx, txn.AccountExternalID, cache)
	tctx := TransactionContext{
		Name:       txn.Name,
		Amount:     txn.Amount.InexactFloat64(),
		Pending:    txn.Pending,
		Provider:   providerName,
		AccountID:  formatUUID(accountID),
		UserID:     formatUUID(userID),
		UserName:   userName,
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

	result := resolver.ResolveWithContext(providerName, tctx)
	if result == nil {
		return nil, nil
	}

	// Only update category_id when a rule explicitly matched and the
	// transaction doesn't have a manual override.
	if result.CategoryID.Valid && !dbTxn.CategoryOverride {
		_, err := tx.Exec(ctx,
			`UPDATE transactions SET category_id = $1 WHERE id = $2 AND NOT category_override`,
			result.CategoryID, dbTxn.ID)
		if err != nil {
			return result.Sources, fmt.Errorf("apply rule category: %w", err)
		}
	}

	return result.Sources, nil
}

// updateBalancesWithRetry fetches balances with 1 retry on transient errors.
// Returns a warning message if the retry succeeded (partial success) or if
// all attempts failed (non-fatal warning). Returns empty string on clean success.
func (e *Engine) updateBalancesWithRetry(ctx context.Context, q *db.Queries, prov provider.Provider, conn provider.Connection, logger *slog.Logger) string {
	err := e.updateBalances(ctx, q, prov, conn, logger)
	if err == nil {
		return ""
	}

	// First attempt failed. Log and retry once after a short delay.
	delay := e.balanceRetryDelay
	if delay <= 0 {
		delay = 2 * time.Second
	}
	logger.Warn("balance fetch failed, retrying", "error", err, "retry_delay", delay)
	time.Sleep(delay)

	retryErr := e.updateBalances(ctx, q, prov, conn, logger)
	if retryErr == nil {
		// Retry succeeded — record a warning that the first attempt failed.
		msg := fmt.Sprintf("Balance fetch succeeded on retry (first attempt: %s)", err.Error())
		logger.Info("balance fetch retry succeeded", "original_error", err)
		return msg
	}

	// Both attempts failed. Non-fatal: transaction sync data is still committed.
	msg := fmt.Sprintf("Balance fetch failed after retry: %s", retryErr.Error())
	logger.Error("balance fetch failed after retry", "original_error", err, "retry_error", retryErr)
	return msg
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

// Matcher returns the engine's matcher for external use (e.g., manual reconciliation).
func (e *Engine) Matcher() *Matcher {
	return e.matcher
}

// trackAccountCount increments the per-account counter for the given operation type.
func (e *Engine) trackAccountCount(ctx context.Context, perAccount map[string]*accountSyncCounts, accountID pgtype.UUID, nameCache map[string]string, op string) {
	key := formatUUID(accountID)
	if key == "" {
		return
	}

	counts, ok := perAccount[key]
	if !ok {
		// Resolve account display name (best-effort).
		name, exists := nameCache[key]
		if !exists {
			displayName, err := e.db.GetAccountDisplayNameByID(ctx, accountID)
			if err != nil {
				name = "Unknown"
			} else {
				name = displayName
			}
			nameCache[key] = name
		}
		counts = &accountSyncCounts{
			AccountID:   accountID,
			AccountName: name,
		}
		perAccount[key] = counts
	}

	switch op {
	case "added":
		counts.Added++
	case "modified":
		counts.Modified++
	case "removed":
		counts.Removed++
	case "unchanged":
		counts.Unchanged++
	}
}

// saveSyncLogAccounts persists per-account sync breakdown (best-effort, non-fatal).
func (e *Engine) saveSyncLogAccounts(ctx context.Context, syncLogID pgtype.UUID, perAccount map[string]*accountSyncCounts, logger *slog.Logger) {
	if len(perAccount) == 0 {
		return
	}

	for _, counts := range perAccount {
		if err := e.db.InsertSyncLogAccount(ctx, db.InsertSyncLogAccountParams{
			SyncLogID:      syncLogID,
			AccountID:      counts.AccountID,
			AccountName:    counts.AccountName,
			AddedCount:     int32(counts.Added),
			ModifiedCount:  int32(counts.Modified),
			RemovedCount:   int32(counts.Removed),
			UnchangedCount: int32(counts.Unchanged),
		}); err != nil {
			logger.Warn("failed to insert sync log account breakdown",
				"account_name", counts.AccountName,
				"error", err)
		}
	}

	logger.Debug("saved per-account sync breakdown", "accounts", len(perAccount))
}

// classifyUpsertResult determines whether an upserted transaction row is new,
// actually modified, or unchanged. It relies on the conditional updated_at in
// the UpsertTransaction SQL: updated_at is only set to NOW() when key fields
// actually changed. For new rows, created_at and updated_at are both set to
// NOW() by the INSERT.
//
// Returns (isNew, isChanged):
//   - (true, true):   newly inserted row
//   - (false, true):  existing row with changed values
//   - (false, false): existing row with identical values (unchanged)
func classifyUpsertResult(txn db.Transaction, upsertStart time.Time) (isNew bool, isChanged bool) {
	// New row: created_at was set during this upsert (>= upsertStart) AND
	// created_at ~= updated_at (both set to NOW() on INSERT).
	recentCreate := txn.CreatedAt.Time.After(upsertStart.Add(-2 * time.Second))
	if recentCreate && txn.CreatedAt.Time.Sub(txn.UpdatedAt.Time).Abs() < time.Second {
		return true, true
	}
	// Existing row: if updated_at was bumped to NOW() (>= upsertStart with
	// some tolerance), values actually changed. Otherwise unchanged.
	isChanged = txn.UpdatedAt.Time.After(upsertStart.Add(-2 * time.Second))
	return false, isChanged
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
