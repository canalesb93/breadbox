package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"breadbox/internal/db"

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

	uid, err := parseUUID(*connectionID)
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

func (s *Service) ListSyncLogsPaginated(ctx context.Context, params SyncLogListParams) (*SyncLogListResult, error) {
	query := "SELECT sl.id, sl.connection_id, bc.institution_name, sl.trigger, sl.status, " +
		"sl.added_count, sl.modified_count, sl.removed_count, sl.error_message, " +
		"sl.started_at, sl.completed_at " +
		"FROM sync_logs sl " +
		"JOIN bank_connections bc ON sl.connection_id = bc.id " +
		"WHERE 1=1"
	args := []any{}
	argN := 1

	if params.ConnectionID != nil {
		uid, err := parseUUID(*params.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("invalid connection id: %w", err)
		}
		query += fmt.Sprintf(" AND sl.connection_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.Status != nil {
		query += fmt.Sprintf(" AND sl.status = $%d::sync_status", argN)
		args = append(args, *params.Status)
		argN++
	}

	// Get total count with same filters.
	countQuery := "SELECT COUNT(*) FROM sync_logs sl " +
		"JOIN bank_connections bc ON sl.connection_id = bc.id " +
		"WHERE 1=1"
	countArgs := []any{}
	countArgN := 1

	if params.ConnectionID != nil {
		uid, _ := parseUUID(*params.ConnectionID) // already validated above
		countQuery += fmt.Sprintf(" AND sl.connection_id = $%d", countArgN)
		countArgs = append(countArgs, uid)
		countArgN++
	}
	if params.Status != nil {
		countQuery += fmt.Sprintf(" AND sl.status = $%d::sync_status", countArgN)
		countArgs = append(countArgs, *params.Status)
		countArgN++
	}

	var total int64
	if err := s.Pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
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
			errorMessage    pgtype.Text
			startedAt       pgtype.Timestamptz
			completedAt     pgtype.Timestamptz
		)

		if err := rows.Scan(
			&id, &connectionID, &institutionName, &trigger, &status,
			&addedCount, &modifiedCount, &removedCount, &errorMessage,
			&startedAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sync log: %w", err)
		}

		var duration *string
		if startedAt.Valid && completedAt.Valid {
			d := completedAt.Time.Sub(startedAt.Time).Round(time.Millisecond).String()
			duration = &d
		}

		instName := ""
		if institutionName.Valid {
			instName = institutionName.String
		}

		logs = append(logs, SyncLogRow{
			ID:              formatUUID(id),
			ConnectionID:    formatUUID(connectionID),
			InstitutionName: instName,
			Trigger:         trigger,
			Status:          status,
			AddedCount:      addedCount,
			ModifiedCount:   modifiedCount,
			RemovedCount:    removedCount,
			ErrorMessage:    textPtr(errorMessage),
			StartedAt:       timestampStr(startedAt),
			CompletedAt:     timestampStr(completedAt),
			Duration:        duration,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync logs: %w", err)
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
	query := "SELECT COUNT(*) FROM sync_logs sl " +
		"JOIN bank_connections bc ON sl.connection_id = bc.id " +
		"WHERE 1=1"
	args := []any{}
	argN := 1

	if params.ConnectionID != nil {
		uid, err := parseUUID(*params.ConnectionID)
		if err != nil {
			return 0, fmt.Errorf("invalid connection id: %w", err)
		}
		query += fmt.Sprintf(" AND sl.connection_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.Status != nil {
		query += fmt.Sprintf(" AND sl.status = $%d::sync_status", argN)
		args = append(args, *params.Status)
		argN++
	}

	var count int64
	if err := s.Pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sync logs: %w", err)
	}
	return count, nil
}

// SyncLogStats returns aggregate statistics about sync logs, optionally filtered.
func (s *Service) SyncLogStats(ctx context.Context, params SyncLogListParams) (*SyncLogStats, error) {
	query := `SELECT
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE sl.status = 'success') AS success_count,
		COUNT(*) FILTER (WHERE sl.status = 'error') AS error_count,
		COALESCE(AVG(EXTRACT(MILLISECONDS FROM (sl.completed_at - sl.started_at))) FILTER (WHERE sl.completed_at IS NOT NULL), 0) AS avg_duration_ms,
		COALESCE(SUM(sl.added_count), 0) AS total_added,
		COALESCE(SUM(sl.modified_count), 0) AS total_modified,
		COALESCE(SUM(sl.removed_count), 0) AS total_removed
	FROM sync_logs sl
	JOIN bank_connections bc ON sl.connection_id = bc.id
	WHERE 1=1`
	args := []any{}
	argN := 1

	if params.ConnectionID != nil {
		uid, err := parseUUID(*params.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("invalid connection id: %w", err)
		}
		query += fmt.Sprintf(" AND sl.connection_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.Status != nil {
		query += fmt.Sprintf(" AND sl.status = $%d::sync_status", argN)
		args = append(args, *params.Status)
		argN++
	}

	var stats SyncLogStats
	err := s.Pool.QueryRow(ctx, query, args...).Scan(
		&stats.TotalSyncs,
		&stats.SuccessCount,
		&stats.ErrorCount,
		&stats.AvgDurationMs,
		&stats.TotalAdded,
		&stats.TotalModified,
		&stats.TotalRemoved,
	)
	if err != nil {
		return nil, fmt.Errorf("sync log stats: %w", err)
	}

	if stats.TotalSyncs > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.TotalSyncs) * 100
	}

	return &stats, nil
}
