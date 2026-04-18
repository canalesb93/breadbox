package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ListConnections(ctx context.Context, userID *string) ([]ConnectionResponse, error) {
	if userID != nil {
		uid, err := s.resolveUserID(ctx, *userID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		rows, err := s.Queries.ListConnectionsByUserForAPI(ctx, uid)
		if err != nil {
			return nil, fmt.Errorf("list connections by user: %w", err)
		}
		result := make([]ConnectionResponse, len(rows))
		for i, r := range rows {
			result[i] = ConnectionResponse{
				ID:              formatUUID(r.ID),
				ShortID:         r.ShortID,
				UserID:          uuidPtr(r.UserID),
				UserName:        textPtr(r.UserName),
				Provider:        string(r.Provider),
				InstitutionID:   textPtr(r.InstitutionID),
				InstitutionName: textPtr(r.InstitutionName),
				Status:          string(r.Status),
				ErrorCode:       textPtr(r.ErrorCode),
				ErrorMessage:    textPtr(r.ErrorMessage),
				LastSyncedAt:    timestampStr(r.LastSyncedAt),
				CreatedAt:       r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
				UpdatedAt:       r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
			}
		}
		return result, nil
	}

	rows, err := s.Queries.ListConnectionsForAPI(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	result := make([]ConnectionResponse, len(rows))
	for i, r := range rows {
		result[i] = ConnectionResponse{
			ID:              formatUUID(r.ID),
			ShortID:         r.ShortID,
			UserID:          uuidPtr(r.UserID),
			UserName:        textPtr(r.UserName),
			Provider:        string(r.Provider),
			InstitutionID:   textPtr(r.InstitutionID),
			InstitutionName: textPtr(r.InstitutionName),
			Status:          string(r.Status),
			ErrorCode:       textPtr(r.ErrorCode),
			ErrorMessage:    textPtr(r.ErrorMessage),
			LastSyncedAt:    timestampStr(r.LastSyncedAt),
			CreatedAt:       r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:       r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	return result, nil
}

func (s *Service) GetConnectionStatus(ctx context.Context, id string) (*ConnectionStatusResponse, error) {
	uid, err := s.resolveConnectionID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	conn, err := s.Queries.GetConnectionForAPI(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get connection: %w", err)
	}

	resp := &ConnectionStatusResponse{
		ConnectionResponse: ConnectionResponse{
			ID:              formatUUID(conn.ID),
			ShortID:         conn.ShortID,
			UserID:          uuidPtr(conn.UserID),
			UserName:        textPtr(conn.UserName),
			Provider:        string(conn.Provider),
			InstitutionID:   textPtr(conn.InstitutionID),
			InstitutionName: textPtr(conn.InstitutionName),
			Status:          string(conn.Status),
			ErrorCode:       textPtr(conn.ErrorCode),
			ErrorMessage:    textPtr(conn.ErrorMessage),
			LastSyncedAt:    timestampStr(conn.LastSyncedAt),
			CreatedAt:       conn.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:       conn.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		},
	}

	syncLog, err := s.Queries.GetMostRecentSyncLog(ctx, uid)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get sync log: %w", err)
		}
		// No sync log found, that's fine
	} else {
		resp.LastAttemptedSyncAt = timestampStr(syncLog.StartedAt)
		slResp := SyncLogResponse{
			ID:            formatUUID(syncLog.ID),
			ShortID:       syncLog.ShortID,
			ConnectionID:  formatUUID(syncLog.ConnectionID),
			Trigger:       string(syncLog.Trigger),
			Status:        string(syncLog.Status),
			AddedCount:    syncLog.AddedCount,
			ModifiedCount: syncLog.ModifiedCount,
			RemovedCount:  syncLog.RemovedCount,
			ErrorMessage:  textPtr(syncLog.ErrorMessage),
			StartedAt:     timestampStr(syncLog.StartedAt),
			CompletedAt:   timestampStr(syncLog.CompletedAt),
		}
		if syncLog.DurationMs.Valid {
			slResp.DurationMs = &syncLog.DurationMs.Int32
			d := FormatDurationMs(int64(syncLog.DurationMs.Int32))
			slResp.Duration = &d
		} else if syncLog.StartedAt.Valid && syncLog.CompletedAt.Valid {
			ms := int32(syncLog.CompletedAt.Time.Sub(syncLog.StartedAt.Time).Milliseconds())
			slResp.DurationMs = &ms
			d := FormatDurationMs(int64(ms))
			slResp.Duration = &d
		}
		resp.LastSyncLog = &slResp
	}

	return resp, nil
}
