package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	bsync "breadbox/internal/sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) TriggerSync(ctx context.Context, connectionID *string) error {
	if connectionID == nil {
		go func() {
			bgCtx := context.Background()
			if err := s.SyncEngine.SyncAll(bgCtx, db.SyncTriggerManual); err != nil {
				s.Logger.Error("manual sync failed", "error", err)
			}
		}()
		return nil
	}

	uid, err := s.resolveConnectionID(ctx, *connectionID)
	if err != nil {
		return fmt.Errorf("invalid connection id: %w", err)
	}

	_, err = s.Queries.GetBankConnectionForSync(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get bank connection: %w", err)
	}

	go func() {
		bgCtx := context.Background()
		if err := s.SyncEngine.Sync(bgCtx, uid, db.SyncTriggerManual); err != nil {
			s.Logger.Error("manual sync failed", "connection_id", *connectionID, "error", err)
		}
	}()
	return nil
}

// GetSyncLog returns a single sync log with connection info.
func (s *Service) GetSyncLog(ctx context.Context, syncLogID string) (*SyncLogRow, error) {
	uid, err := parseUUID(syncLogID)
	if err != nil {
		return nil, fmt.Errorf("invalid sync log id: %w", err)
	}

	query := "SELECT sl.id, sl.connection_id, bc.institution_name, bc.provider, sl.trigger, sl.status, " +
		"sl.added_count, sl.modified_count, sl.removed_count, sl.unchanged_count, sl.error_message, " +
		"sl.started_at, sl.completed_at, sl.rule_hits, sl.warning_message " +
		"FROM sync_logs sl " +
		"JOIN bank_connections bc ON sl.connection_id = bc.id " +
		"WHERE sl.id = $1"

	var (
		id              pgtype.UUID
		connectionID    pgtype.UUID
		institutionName pgtype.Text
		providerType    string
		trigger         string
		status          string
		addedCount      int32
		modifiedCount   int32
		removedCount    int32
		unchangedCount  int32
		errorMessage    pgtype.Text
		startedAt       pgtype.Timestamptz
		completedAt     pgtype.Timestamptz
		ruleHitsJSON    []byte
		warningMessage  pgtype.Text
	)

	if err := s.Pool.QueryRow(ctx, query, uid).Scan(
		&id, &connectionID, &institutionName, &providerType, &trigger, &status,
		&addedCount, &modifiedCount, &removedCount, &unchangedCount, &errorMessage,
		&startedAt, &completedAt, &ruleHitsJSON, &warningMessage,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get sync log: %w", err)
	}

	var duration *string
	if startedAt.Valid && completedAt.Valid {
		d := completedAt.Time.Sub(startedAt.Time).Round(time.Millisecond).String()
		duration = &d
	}

	instName := pgconv.TextOr(institutionName, "")

	// Get account count.
	accountCount, _ := s.Queries.CountAffectedAccountsBySyncLog(ctx, uid)

	row := &SyncLogRow{
		ID:               formatUUID(id),
		ConnectionID:     formatUUID(connectionID),
		InstitutionName:  instName,
		Provider:         providerType,
		Trigger:          trigger,
		Status:           status,
		AddedCount:       addedCount,
		ModifiedCount:    modifiedCount,
		RemovedCount:     removedCount,
		UnchangedCount:   unchangedCount,
		ErrorMessage:     textPtr(errorMessage),
		WarningMessage:   textPtr(warningMessage),
		StartedAt:        timestampStr(startedAt),
		CompletedAt:      timestampStr(completedAt),
		Duration:         duration,
		AccountsAffected: accountCount,
	}

	// Parse and resolve rule hits.
	row.RuleHits, row.TotalRuleHits = s.parseRuleHits(ctx, ruleHitsJSON)

	return row, nil
}

func (s *Service) ListSyncLogsPaginated(ctx context.Context, params SyncLogListParams) (*SyncLogListResult, error) {
	whereClause, args, argN, err := s.buildSyncLogWhereClause(params)
	if err != nil {
		return nil, err
	}

	query := "SELECT sl.id, sl.connection_id, bc.institution_name, sl.trigger, sl.status, " +
		"sl.added_count, sl.modified_count, sl.removed_count, sl.unchanged_count, sl.error_message, " +
		"sl.started_at, sl.completed_at, sl.duration_ms, sl.warning_message " +
		"FROM sync_logs sl " +
		"JOIN bank_connections bc ON sl.connection_id = bc.id " +
		whereClause

	// Get total count with same filters.
	countQuery := "SELECT COUNT(*) FROM sync_logs sl " +
		"JOIN bank_connections bc ON sl.connection_id = bc.id " +
		whereClause

	var total int64
	if err := s.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count sync logs: %w", err)
	}

	query += fmt.Sprintf(" ORDER BY sl.started_at DESC LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, params.PageSize, (params.Page-1)*params.PageSize)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sync logs: %w", err)
	}
	defer rows.Close()

	var logs []SyncLogRow
	for rows.Next() {
		var (
			id              pgtype.UUID
			connectionID    pgtype.UUID
			institutionName pgtype.Text
			trigger         string
			status          string
			addedCount      int32
			modifiedCount   int32
			removedCount    int32
			unchangedCount  int32
			errorMessage    pgtype.Text
			startedAt       pgtype.Timestamptz
			completedAt     pgtype.Timestamptz
			durationMs      pgtype.Int4
			warningMessage  pgtype.Text
		)

		if err := rows.Scan(
			&id, &connectionID, &institutionName, &trigger, &status,
			&addedCount, &modifiedCount, &removedCount, &unchangedCount, &errorMessage,
			&startedAt, &completedAt, &durationMs, &warningMessage,
		); err != nil {
			return nil, fmt.Errorf("scan sync log: %w", err)
		}

		var duration *string
		if durationMs.Valid {
			d := FormatDurationMs(int64(durationMs.Int32))
			duration = &d
		} else if startedAt.Valid && completedAt.Valid {
			// Fallback for logs before the duration_ms column was backfilled.
			d := FormatDurationMs(completedAt.Time.Sub(startedAt.Time).Milliseconds())
			duration = &d
		}

		var durationMsPtr *int32
		if durationMs.Valid {
			durationMsPtr = &durationMs.Int32
		} else if startedAt.Valid && completedAt.Valid {
			ms := int32(completedAt.Time.Sub(startedAt.Time).Milliseconds())
			durationMsPtr = &ms
		}

		instName := pgconv.TextOr(institutionName, "")

		row := SyncLogRow{
			ID:              formatUUID(id),
			ConnectionID:    formatUUID(connectionID),
			InstitutionName: instName,
			Trigger:         trigger,
			Status:          status,
			AddedCount:      addedCount,
			ModifiedCount:   modifiedCount,
			RemovedCount:    removedCount,
			UnchangedCount:  unchangedCount,
			ErrorMessage:    textPtr(errorMessage),
			WarningMessage:  textPtr(warningMessage),
			StartedAt:       timestampStr(startedAt),
			CompletedAt:     timestampStr(completedAt),
			Duration:        duration,
			DurationMs:      durationMsPtr,
		}
		if errorMessage.Valid {
			if friendly := bsync.FriendlyError(errorMessage.String); friendly != "" {
				row.FriendlyErrorMessage = &friendly
			}
		}
		logs = append(logs, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync logs: %w", err)
	}

	// Batch-fetch per-account counts for the returned sync log IDs.
	if len(logs) > 0 {
		syncLogIDs := make([]pgtype.UUID, 0, len(logs))
		for _, l := range logs {
			uid, err := parseUUID(l.ID)
			if err == nil {
				syncLogIDs = append(syncLogIDs, uid)
			}
		}
		accountCounts, err := s.Queries.CountAffectedAccountsBySyncLogIDs(ctx, syncLogIDs)
		if err == nil {
			countMap := make(map[string]int64, len(accountCounts))
			for _, ac := range accountCounts {
				countMap[formatUUID(ac.SyncLogID)] = ac.AccountCount
			}
			for i := range logs {
				if c, ok := countMap[logs[i].ID]; ok {
					logs[i].AccountsAffected = c
				}
			}
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(params.PageSize)))

	return &SyncLogListResult{
		Logs:       logs,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *Service) CountSyncLogsFiltered(ctx context.Context, params SyncLogListParams) (int64, error) {
	whereClause, args, _, err := s.buildSyncLogWhereClause(params)
	if err != nil {
		return 0, err
	}

	query := "SELECT COUNT(*) FROM sync_logs sl " +
		"JOIN bank_connections bc ON sl.connection_id = bc.id " +
		whereClause

	var count int64
	if err := s.Pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sync logs: %w", err)
	}
	return count, nil
}

// SyncLogStats returns aggregate statistics about sync logs, optionally filtered.
func (s *Service) SyncLogStats(ctx context.Context, params SyncLogListParams) (*SyncLogStats, error) {
	whereClause, args, _, err := s.buildSyncLogWhereClause(params)
	if err != nil {
		return nil, err
	}

	query := `SELECT
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE sl.status = 'success') AS success_count,
		COUNT(*) FILTER (WHERE sl.status = 'error') AS error_count,
		COUNT(*) FILTER (WHERE sl.warning_message IS NOT NULL AND sl.warning_message != '') AS warning_count,
		COALESCE(AVG(COALESCE(sl.duration_ms, EXTRACT(MILLISECONDS FROM (sl.completed_at - sl.started_at))::INTEGER)) FILTER (WHERE sl.completed_at IS NOT NULL), 0) AS avg_duration_ms,
		COALESCE(SUM(sl.added_count), 0) AS total_added,
		COALESCE(SUM(sl.modified_count), 0) AS total_modified,
		COALESCE(SUM(sl.removed_count), 0) AS total_removed,
		COALESCE(SUM(sl.unchanged_count), 0) AS total_unchanged
	FROM sync_logs sl
	JOIN bank_connections bc ON sl.connection_id = bc.id
	` + whereClause

	var stats SyncLogStats
	err = s.Pool.QueryRow(ctx, query, args...).Scan(
		&stats.TotalSyncs,
		&stats.SuccessCount,
		&stats.ErrorCount,
		&stats.WarningCount,
		&stats.AvgDurationMs,
		&stats.TotalAdded,
		&stats.TotalModified,
		&stats.TotalRemoved,
		&stats.TotalUnchanged,
	)
	if err != nil {
		return nil, fmt.Errorf("sync log stats: %w", err)
	}

	if stats.TotalSyncs > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.TotalSyncs) * 100
	}

	return &stats, nil
}

// CountSyncLogs returns the total number of sync log entries.
func (s *Service) CountSyncLogs(ctx context.Context) (int64, error) {
	return s.Queries.CountSyncLogs(ctx)
}

// CleanupSyncLogs deletes sync logs older than the given number of days.
// It skips in_progress logs for safety. Returns the number of deleted rows.
func (s *Service) CleanupSyncLogs(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, fmt.Errorf("retention days must be positive, got %d", retentionDays)
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result, err := s.Queries.DeleteSyncLogsOlderThan(ctx, pgtype.Timestamptz{
		Time:  cutoff,
		Valid: true,
	})
	if err != nil {
		return 0, fmt.Errorf("delete old sync logs: %w", err)
	}

	return result.RowsAffected(), nil
}

// GetSyncLogRetentionDays reads the sync_log_retention_days setting from app_config.
// Returns 90 (default) if not set or not a positive value.
func (s *Service) GetSyncLogRetentionDays(ctx context.Context) (int, error) {
	days := appconfig.Int(ctx, s.Queries, "sync_log_retention_days", 90)
	if days <= 0 {
		return 90, nil
	}
	return days, nil
}

// GetSyncHealthSummary returns a dashboard-oriented summary of recent sync health.
// It queries the last 24 hours of sync activity.
func (s *Service) GetSyncHealthSummary(ctx context.Context) (*SyncHealthSummary, error) {
	query := `SELECT
		COUNT(*) AS total_syncs,
		COUNT(*) FILTER (WHERE status = 'success') AS success_count,
		COUNT(*) FILTER (WHERE status = 'error') AS error_count,
		MAX(COALESCE(completed_at, started_at)) AS last_sync_time,
		(SELECT status FROM sync_logs ORDER BY started_at DESC LIMIT 1) AS last_status
	FROM sync_logs
	WHERE started_at >= NOW() - INTERVAL '24 hours'`

	var (
		totalSyncs   int64
		successCount int64
		errorCount   int64
		lastSyncTime pgtype.Timestamptz
		lastStatus   pgtype.Text
	)

	err := s.Pool.QueryRow(ctx, query).Scan(
		&totalSyncs,
		&successCount,
		&errorCount,
		&lastSyncTime,
		&lastStatus,
	)
	if err != nil {
		return nil, fmt.Errorf("sync health summary: %w", err)
	}

	summary := &SyncHealthSummary{
		RecentSyncCount:  totalSyncs,
		RecentErrorCount: errorCount,
	}

	if totalSyncs > 0 {
		summary.RecentSuccessRate = float64(successCount) / float64(totalSyncs) * 100
	}

	if lastSyncTime.Valid {
		t := relativeTimeStr(lastSyncTime.Time)
		summary.LastSyncTime = &t
	}

	summary.LastSyncStatus = pgconv.TextOr(lastStatus, "")

	// Determine overall health: healthy, degraded, or unhealthy.
	// - unhealthy: success rate < 50% with 2+ syncs, or all recent syncs failed
	// - degraded: success rate < 100% or no syncs in 24h
	// - healthy: everything ok
	switch {
	case totalSyncs == 0:
		summary.OverallHealth = "degraded" // no recent syncs
	case summary.RecentSuccessRate < 50 && totalSyncs >= 2:
		summary.OverallHealth = "unhealthy"
	case errorCount == totalSyncs && totalSyncs > 0:
		summary.OverallHealth = "unhealthy"
	case errorCount > 0:
		summary.OverallHealth = "degraded"
	default:
		summary.OverallHealth = "healthy"
	}

	return summary, nil
}

// relativeTimeStr converts a time to a human-readable relative string.
func relativeTimeStr(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// ListSyncLogAccounts returns the per-account breakdown for a specific sync log.
func (s *Service) ListSyncLogAccounts(ctx context.Context, syncLogID string) ([]SyncLogAccountRow, error) {
	uid, err := parseUUID(syncLogID)
	if err != nil {
		return nil, fmt.Errorf("invalid sync log id: %w", err)
	}

	rows, err := s.Queries.ListSyncLogAccounts(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("list sync log accounts: %w", err)
	}

	result := make([]SyncLogAccountRow, 0, len(rows))
	for _, row := range rows {
		r := SyncLogAccountRow{
			ID:             formatUUID(row.ID),
			SyncLogID:      formatUUID(row.SyncLogID),
			AccountName:    row.AccountName,
			AddedCount:     row.AddedCount,
			ModifiedCount:  row.ModifiedCount,
			RemovedCount:   row.RemovedCount,
			UnchangedCount: row.UnchangedCount,
		}
		if row.AccountID.Valid {
			id := formatUUID(row.AccountID)
			r.AccountID = &id
		}
		result = append(result, r)
	}
	return result, nil
}

// parseRuleHits parses the JSONB rule_hits column and resolves rule names.
// Returns the entries sorted by hit count descending, and the total hit count.
func (s *Service) parseRuleHits(ctx context.Context, ruleHitsJSON []byte) ([]RuleHitEntry, int) {
	if len(ruleHitsJSON) == 0 {
		return nil, 0
	}

	var hitMap map[string]int
	if err := json.Unmarshal(ruleHitsJSON, &hitMap); err != nil {
		s.Logger.Warn("failed to parse rule_hits JSON", "error", err)
		return nil, 0
	}

	if len(hitMap) == 0 {
		return nil, 0
	}

	// Batch-fetch rule names and conditions for all rule IDs.
	type ruleInfo struct {
		name       string
		conditions *Condition
	}
	ruleInfoMap := make(map[string]ruleInfo, len(hitMap))
	for ruleID := range hitMap {
		uid, err := parseUUID(ruleID)
		if err != nil {
			continue
		}
		var name string
		var condJSON []byte
		err = s.Pool.QueryRow(ctx, "SELECT name, conditions FROM transaction_rules WHERE id = $1", uid).Scan(&name, &condJSON)
		if err != nil {
			ruleInfoMap[ruleID] = ruleInfo{name: "Deleted rule"}
			continue
		}
		info := ruleInfo{name: name}
		if len(condJSON) > 0 {
			var cond Condition
			if json.Unmarshal(condJSON, &cond) == nil {
				info.conditions = &cond
			}
		}
		ruleInfoMap[ruleID] = info
	}

	entries := make([]RuleHitEntry, 0, len(hitMap))
	total := 0
	for ruleID, count := range hitMap {
		info := ruleInfoMap[ruleID]
		entries = append(entries, RuleHitEntry{
			RuleID:     ruleID,
			RuleName:   info.name,
			Count:      count,
			Conditions: info.conditions,
		})
		total += count
	}

	// Sort by hit count descending, then by rule name for stability.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].RuleName < entries[j].RuleName
	})

	return entries, total
}

// GetProviderHealthSummaries returns per-provider health info (connection count, last sync status/time).
func (s *Service) GetProviderHealthSummaries(ctx context.Context) (map[string]*ProviderHealthSummary, error) {
	// Query 1: connection and account counts per provider.
	countQuery := `
		SELECT bc.provider::text,
			COUNT(DISTINCT bc.id) AS connection_count,
			COUNT(DISTINCT a.id) AS account_count
		FROM bank_connections bc
		LEFT JOIN accounts a ON a.connection_id = bc.id
		WHERE bc.status != 'disconnected'
		GROUP BY bc.provider`

	countRows, err := s.Pool.Query(ctx, countQuery)
	if err != nil {
		return nil, fmt.Errorf("provider counts: %w", err)
	}
	defer countRows.Close()

	result := map[string]*ProviderHealthSummary{}
	for countRows.Next() {
		var provider string
		var connCount, acctCount int64
		if err := countRows.Scan(&provider, &connCount, &acctCount); err != nil {
			return nil, fmt.Errorf("scan provider counts: %w", err)
		}
		result[provider] = &ProviderHealthSummary{
			Provider:        provider,
			ConnectionCount: connCount,
			AccountCount:    acctCount,
		}
	}
	if err := countRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider counts: %w", err)
	}

	// Query 2: latest sync per provider using DISTINCT ON.
	syncQuery := `
		SELECT DISTINCT ON (bc.provider)
			bc.provider::text,
			sl.status::text,
			COALESCE(sl.completed_at, sl.started_at) AS sync_time,
			sl.error_message
		FROM sync_logs sl
		JOIN bank_connections bc ON sl.connection_id = bc.id
		ORDER BY bc.provider, sl.started_at DESC`

	syncRows, err := s.Pool.Query(ctx, syncQuery)
	if err != nil {
		return nil, fmt.Errorf("provider sync status: %w", err)
	}
	defer syncRows.Close()

	for syncRows.Next() {
		var (
			provider   string
			status     string
			syncTime   pgtype.Timestamptz
			errMessage pgtype.Text
		)
		if err := syncRows.Scan(&provider, &status, &syncTime, &errMessage); err != nil {
			return nil, fmt.Errorf("scan provider sync: %w", err)
		}
		summary, ok := result[provider]
		if !ok {
			summary = &ProviderHealthSummary{Provider: provider}
			result[provider] = summary
		}
		summary.LastSyncStatus = status
		if syncTime.Valid {
			t := relativeTimeStr(syncTime.Time)
			summary.LastSyncTime = &t
		}
		if errMessage.Valid && errMessage.String != "" {
			summary.LastSyncError = &errMessage.String
		}
	}
	if err := syncRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider sync: %w", err)
	}

	return result, nil
}

// buildSyncLogWhereClause constructs a WHERE clause and args from SyncLogListParams.
// Returns the clause string (starting with "WHERE 1=1"), the args slice, and the next argN.
func (s *Service) buildSyncLogWhereClause(params SyncLogListParams) (string, []any, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argN := 1

	if params.ConnectionID != nil {
		uid, err := parseUUID(*params.ConnectionID)
		if err != nil {
			return "", nil, 0, fmt.Errorf("invalid connection id: %w", err)
		}
		where += fmt.Sprintf(" AND sl.connection_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.Status != nil {
		where += fmt.Sprintf(" AND sl.status = $%d::sync_status", argN)
		args = append(args, *params.Status)
		argN++
	}

	if params.Trigger != nil {
		where += fmt.Sprintf(" AND sl.trigger = $%d::sync_trigger", argN)
		args = append(args, *params.Trigger)
		argN++
	}

	if params.DateFrom != nil {
		where += fmt.Sprintf(" AND sl.started_at >= $%d", argN)
		args = append(args, *params.DateFrom)
		argN++
	}

	if params.DateTo != nil {
		where += fmt.Sprintf(" AND sl.started_at < $%d", argN)
		args = append(args, *params.DateTo)
		argN++
	}

	return where, args, argN, nil
}
