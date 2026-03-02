package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"breadbox/internal/db"

	"github.com/robfig/cron/v3"
)

// Scheduler runs periodic transaction syncs using cron.
type Scheduler struct {
	cron   *cron.Cron
	engine *Engine
	logger *slog.Logger
}

// NewScheduler creates a new Scheduler.
func NewScheduler(engine *Engine, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:   cron.New(),
		engine: engine,
		logger: logger,
	}
}

// Start begins the cron scheduler with a sync job at the given interval in minutes.
func (s *Scheduler) Start(intervalMinutes int) {
	spec := fmt.Sprintf("@every %dm", intervalMinutes)
	_, err := s.cron.AddFunc(spec, func() {
		ctx := context.Background()
		s.logger.Info("cron sync starting")
		if err := s.engine.SyncAll(ctx, db.SyncTriggerCron); err != nil {
			s.logger.Error("cron sync failed", "error", err)
			return
		}
		s.logger.Info("cron sync completed")
	})
	if err != nil {
		s.logger.Error("failed to add cron job", "error", err)
		return
	}
	s.cron.Start()
	s.logger.Info("scheduler started", "interval_minutes", intervalMinutes)
}

// Stop gracefully stops the scheduler, waiting for any running jobs to finish.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.logger.Info("scheduler stopped")
}

// RunStartupSync checks all active connections and syncs any that are stale
// (last synced more than intervalMinutes ago or never synced).
func (s *Scheduler) RunStartupSync(ctx context.Context, queries *db.Queries, intervalMinutes int) {
	connections, err := queries.ListActiveConnections(ctx)
	if err != nil {
		s.logger.Error("startup sync: failed to list connections", "error", err)
		return
	}

	if len(connections) == 0 {
		s.logger.Info("startup sync: no active connections")
		return
	}

	threshold := time.Now().Add(-time.Duration(intervalMinutes) * time.Minute)
	var staleCount int

	for _, conn := range connections {
		if !conn.LastSyncedAt.Valid || conn.LastSyncedAt.Time.Before(threshold) {
			staleCount++
			if err := s.engine.Sync(ctx, conn.ID, db.SyncTriggerCron); err != nil {
				s.logger.Error("startup sync: connection failed",
					"connection_id", formatUUID(conn.ID),
					"error", err,
				)
			}
		}
	}

	s.logger.Info("startup sync completed", "total", len(connections), "stale_synced", staleCount)
}
