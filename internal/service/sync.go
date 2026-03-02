package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5"
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
