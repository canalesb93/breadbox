package service

import (
	"context"

	"breadbox/internal/db"
)

func (s *Service) TriggerSync(ctx context.Context) error {
	go func() {
		bgCtx := context.Background()
		if err := s.SyncEngine.SyncAll(bgCtx, db.SyncTriggerManual); err != nil {
			s.Logger.Error("manual sync failed", "error", err)
		}
	}()
	return nil
}
