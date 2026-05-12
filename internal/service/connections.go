//go:build !lite

package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/provider"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RegisterNewConnectionParams is the input for RegisterNewConnection. The
// caller (REST or admin handler) has already exchanged the provider's public
// token and produced an encrypted credentials blob — this method only
// performs the DB writes (CreateBankConnection + UpsertAccount per account)
// so both surfaces share one persistence path.
type RegisterNewConnectionParams struct {
	UserID          pgtype.UUID
	Provider        string
	InstitutionID   string
	InstitutionName string
	Conn            provider.Connection
	Accounts        []provider.Account
}

// RegisterNewConnectionResult carries the freshly-persisted connection's
// stable identifiers so the caller can shape its API response.
type RegisterNewConnectionResult struct {
	ID      pgtype.UUID
	ShortID string
}

// RegisterNewConnection persists a freshly-exchanged bank connection plus
// the accounts the provider returned. Account upsert errors are logged but
// do not fail the call — the connection itself is the source of truth and
// we'd rather have a connection with N-1 accounts than no connection at all
// (matches admin ExchangeTokenHandler behavior).
//
// The provider exchange (and the encryption of the access token) must
// already have happened — Conn.EncryptedCredentials and Conn.ExternalID are
// stored verbatim. This keeps the service layer provider-agnostic.
func (s *Service) RegisterNewConnection(ctx context.Context, p RegisterNewConnectionParams) (RegisterNewConnectionResult, error) {
	bankConn, err := s.Queries.CreateBankConnection(ctx, db.CreateBankConnectionParams{
		UserID:               p.UserID,
		Provider:             db.ProviderType(p.Provider),
		InstitutionID:        pgconv.Text(p.InstitutionID),
		InstitutionName:      pgconv.Text(p.InstitutionName),
		ExternalID:           pgconv.Text(p.Conn.ExternalID),
		EncryptedCredentials: p.Conn.EncryptedCredentials,
		Status:               db.ConnectionStatusActive,
	})
	if err != nil {
		return RegisterNewConnectionResult{}, fmt.Errorf("create bank connection: %w", err)
	}

	for _, acct := range p.Accounts {
		if _, err := s.Queries.UpsertAccount(ctx, db.UpsertAccountParams{
			ConnectionID:      bankConn.ID,
			ExternalAccountID: acct.ExternalID,
			Name:              acct.Name,
			OfficialName:      pgconv.TextIfNotEmpty(acct.OfficialName),
			Type:              acct.Type,
			Subtype:           pgconv.TextIfNotEmpty(acct.Subtype),
			Mask:              pgconv.TextIfNotEmpty(acct.Mask),
			IsoCurrencyCode:   pgconv.TextIfNotEmpty(acct.ISOCurrencyCode),
		}); err != nil {
			s.Logger.Error("upsert account during connection register",
				"error", err,
				"external_id", acct.ExternalID,
				"connection_id", pgconv.FormatUUID(bankConn.ID),
			)
		}
	}

	return RegisterNewConnectionResult{ID: bankConn.ID, ShortID: bankConn.ShortID}, nil
}

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
				UserID:          textPtr(r.UserShortID),
				UserName:        textPtr(r.UserName),
				Provider:        string(r.Provider),
				InstitutionID:   textPtr(r.InstitutionID),
				InstitutionName: textPtr(r.InstitutionName),
				Status:          string(r.Status),
				ErrorCode:       textPtr(r.ErrorCode),
				ErrorMessage:    textPtr(r.ErrorMessage),
				LastSyncedAt:    timestampStr(r.LastSyncedAt),
				CreatedAt:       pgconv.TimestampStr(r.CreatedAt),
				UpdatedAt:       pgconv.TimestampStr(r.UpdatedAt),
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
			UserID:          textPtr(r.UserShortID),
			UserName:        textPtr(r.UserName),
			Provider:        string(r.Provider),
			InstitutionID:   textPtr(r.InstitutionID),
			InstitutionName: textPtr(r.InstitutionName),
			Status:          string(r.Status),
			ErrorCode:       textPtr(r.ErrorCode),
			ErrorMessage:    textPtr(r.ErrorMessage),
			LastSyncedAt:    timestampStr(r.LastSyncedAt),
			CreatedAt:       pgconv.TimestampStr(r.CreatedAt),
			UpdatedAt:       pgconv.TimestampStr(r.UpdatedAt),
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
			UserID:          textPtr(conn.UserShortID),
			UserName:        textPtr(conn.UserName),
			Provider:        string(conn.Provider),
			InstitutionID:   textPtr(conn.InstitutionID),
			InstitutionName: textPtr(conn.InstitutionName),
			Status:          string(conn.Status),
			ErrorCode:       textPtr(conn.ErrorCode),
			ErrorMessage:    textPtr(conn.ErrorMessage),
			LastSyncedAt:    timestampStr(conn.LastSyncedAt),
			CreatedAt:       pgconv.TimestampStr(conn.CreatedAt),
			UpdatedAt:       pgconv.TimestampStr(conn.UpdatedAt),
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
			ConnectionID:  conn.ShortID,
			Trigger:       string(syncLog.Trigger),
			Status:        string(syncLog.Status),
			AddedCount:    syncLog.AddedCount,
			ModifiedCount: syncLog.ModifiedCount,
			RemovedCount:  syncLog.RemovedCount,
			ErrorMessage:  textPtr(syncLog.ErrorMessage),
			StartedAt:     timestampStr(syncLog.StartedAt),
			CompletedAt:   timestampStr(syncLog.CompletedAt),
		}
		if ms, ok := SyncLogDurationMs(syncLog.DurationMs, syncLog.StartedAt, syncLog.CompletedAt); ok {
			slResp.DurationMs = &ms
			d := FormatDurationMs(int64(ms))
			slResp.Duration = &d
		}
		resp.LastSyncLog = &slResp
	}

	return resp, nil
}

// GetConnection returns full per-connection detail for the management API.
// Accepts either UUID or short_id. Returns ErrNotFound when the row is
// missing — but, mirroring the existing GetConnectionStatus contract,
// disconnected connections are still surfaced (with status="disconnected"),
// not hidden. Callers wanting "live only" should filter on the response.
func (s *Service) GetConnection(ctx context.Context, id string) (*ConnectionDetailResponse, error) {
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

	return buildConnectionDetailResponse(conn), nil
}

// DeleteConnection performs the soft-disconnect flow: revokes the
// encrypted token cache, soft-deletes related transactions, and flips the
// connection's status to 'disconnected'. Provider-side credential
// revocation is NOT performed here (the service has no provider registry);
// admin handlers stay responsible for the provider.RemoveConnection() call
// when they need it.
//
// Idempotent on a per-row basis: calling on an already-disconnected
// connection returns ErrNotFound to give callers a deterministic 404.
func (s *Service) DeleteConnection(ctx context.Context, id string, _ Actor) error {
	uid, err := s.resolveConnectionID(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	// Verify the connection exists AND is not already disconnected before
	// touching any rows. Mirrors the read-filter behavior used elsewhere
	// (ListConnectionsForAPI hides disconnected; the sync resolver treats
	// disconnected as not-found).
	conn, err := s.Queries.GetBankConnection(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get bank connection: %w", err)
	}
	if conn.Status == db.ConnectionStatusDisconnected {
		return ErrNotFound
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete connection tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := s.Queries.WithTx(tx)

	if _, err := q.SoftDeleteTransactionsByConnectionID(ctx, uid); err != nil {
		return fmt.Errorf("soft delete transactions for connection: %w", err)
	}

	if err := q.DeleteBankConnection(ctx, uid); err != nil {
		return fmt.Errorf("delete bank connection: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete connection: %w", err)
	}

	return nil
}

// UpdateConnectionPaused flips the paused flag for a connection.
// Returns ErrNotFound when the connection doesn't exist.
func (s *Service) UpdateConnectionPaused(ctx context.Context, id string, paused bool, _ Actor) (*ConnectionDetailResponse, error) {
	uid, err := s.resolveConnectionID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	if _, err := s.Queries.UpdateConnectionPaused(ctx, db.UpdateConnectionPausedParams{
		ID:     uid,
		Paused: paused,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update connection paused: %w", err)
	}

	conn, err := s.Queries.GetConnectionForAPI(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get connection: %w", err)
	}
	return buildConnectionDetailResponse(conn), nil
}

// UpdateConnectionSyncInterval sets the per-connection sync interval
// override in minutes. Pass nil to revert to the global default. Returns
// ErrNotFound when the connection doesn't exist.
func (s *Service) UpdateConnectionSyncInterval(ctx context.Context, id string, intervalMinutes *int, _ Actor) (*ConnectionDetailResponse, error) {
	uid, err := s.resolveConnectionID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	var interval pgtype.Int4
	if intervalMinutes != nil && *intervalMinutes > 0 {
		interval = pgtype.Int4{Int32: int32(*intervalMinutes), Valid: true}
	}

	if _, err := s.Queries.UpdateConnectionSyncInterval(ctx, db.UpdateConnectionSyncIntervalParams{
		ID:                          uid,
		SyncIntervalOverrideMinutes: interval,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update connection sync interval: %w", err)
	}

	conn, err := s.Queries.GetConnectionForAPI(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get connection: %w", err)
	}
	return buildConnectionDetailResponse(conn), nil
}

// ResolveConnectionUUID resolves a UUID-or-short_id input into a pgtype.UUID
// without fetching the row. Returns ErrNotFound for an unknown short_id and a
// wrapped parse error for an invalid UUID. Public so non-service callers
// (admin handlers, REST handlers that need to call providers directly) can
// honor the same short-id contract the service layer enforces internally.
func (s *Service) ResolveConnectionUUID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveConnectionID(ctx, idOrShort)
}

// ResolveUserUUID resolves a UUID-or-short_id input into a pgtype.UUID
// without fetching the row. Same contract as ResolveConnectionUUID — public so
// non-service callers (admin and REST handlers) can honor the short-id
// contract uniformly when they need a user UUID for direct DB writes.
func (s *Service) ResolveUserUUID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveUserID(ctx, idOrShort)
}

// ReactivateConnection clears a broken connection's error state and flips
// status back to 'active'. Used by the reauth-complete flow after the user
// has completed the provider's re-authentication UI. Returns ErrNotFound
// when the row is missing.
func (s *Service) ReactivateConnection(ctx context.Context, id string, _ Actor) error {
	uid, err := s.resolveConnectionID(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	// UpdateBankConnectionStatus is :exec and silently succeeds against a
	// missing id, so verify existence first to keep the 404 contract.
	if _, err := s.Queries.GetBankConnection(ctx, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get bank connection: %w", err)
	}

	if err := s.Queries.UpdateBankConnectionStatus(ctx, db.UpdateBankConnectionStatusParams{
		ID:           uid,
		Status:       db.ConnectionStatusActive,
		ErrorCode:    pgtype.Text{},
		ErrorMessage: pgtype.Text{},
	}); err != nil {
		return fmt.Errorf("reactivate bank connection: %w", err)
	}
	return nil
}

func buildConnectionDetailResponse(conn db.GetConnectionForAPIRow) *ConnectionDetailResponse {
	resp := &ConnectionDetailResponse{
		ConnectionResponse: ConnectionResponse{
			ID:              formatUUID(conn.ID),
			ShortID:         conn.ShortID,
			UserID:          textPtr(conn.UserShortID),
			UserName:        textPtr(conn.UserName),
			Provider:        string(conn.Provider),
			InstitutionID:   textPtr(conn.InstitutionID),
			InstitutionName: textPtr(conn.InstitutionName),
			Status:          string(conn.Status),
			ErrorCode:       textPtr(conn.ErrorCode),
			ErrorMessage:    textPtr(conn.ErrorMessage),
			LastSyncedAt:    timestampStr(conn.LastSyncedAt),
			CreatedAt:       pgconv.TimestampStr(conn.CreatedAt),
			UpdatedAt:       pgconv.TimestampStr(conn.UpdatedAt),
		},
		Paused:              conn.Paused,
		ConsecutiveFailures: conn.ConsecutiveFailures,
		AccountCount:        int(conn.AccountCount),
	}
	if conn.SyncIntervalOverrideMinutes.Valid {
		v := conn.SyncIntervalOverrideMinutes.Int32
		resp.SyncIntervalOverrideMinutes = &v
	}
	return resp
}
