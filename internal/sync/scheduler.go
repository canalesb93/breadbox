package sync

import (
	"context"
	"log/slog"
	"time"

	"breadbox/internal/db"

	"github.com/robfig/cron/v3"
)

// Scheduler runs periodic transaction syncs using cron.
type Scheduler struct {
	cron    *cron.Cron
	engine  *Engine
	queries *db.Queries
	logger  *slog.Logger
}

// NewScheduler creates a new Scheduler.
func NewScheduler(engine *Engine, queries *db.Queries, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:    cron.New(),
		engine:  engine,
		queries: queries,
		logger:  logger,
	}
}

// Start begins the cron scheduler. Cron fires every 15 minutes (the minimum
// supported interval) and checks each connection's staleness individually.
func (s *Scheduler) Start(globalIntervalMinutes int) {
	_, err := s.cron.AddFunc("@every 15m", func() {
		ctx := context.Background()
		s.logger.Info("cron sync starting")
		synced, skipped := s.syncAllScheduled(ctx, globalIntervalMinutes)
		s.logger.Info("cron sync completed", "synced", synced, "skipped", skipped)
	})
	if err != nil {
		s.logger.Error("failed to add cron job", "error", err)
		return
	}
	s.cron.Start()
	s.logger.Info("scheduler started", "check_interval", "15m", "global_sync_interval_minutes", globalIntervalMinutes)
}

// Stop gracefully stops the scheduler, waiting for any running jobs to finish.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.logger.Info("scheduler stopped")
}

// syncAllScheduled syncs all active, unpaused connections that are stale
// according to their effective interval (per-connection override or global).
func (s *Scheduler) syncAllScheduled(ctx context.Context, globalIntervalMinutes int) (synced, skipped int) {
	connections, err := s.queries.ListActiveUnpausedConnections(ctx)
	if err != nil {
		s.logger.Error("list active unpaused connections", "error", err)
		return 0, 0
	}

	if len(connections) == 0 {
		s.logger.Info("no active unpaused connections to sync")
		return 0, 0
	}

	now := time.Now()
	const maxWorkers = 5
	sem := make(chan struct{}, maxWorkers)

	type result struct{}
	done := make(chan result, len(connections))

	for _, conn := range connections {
		// Compute effective interval.
		effectiveMinutes := globalIntervalMinutes
		if conn.SyncIntervalOverrideMinutes.Valid {
			effectiveMinutes = int(conn.SyncIntervalOverrideMinutes.Int32)
		}

		// Skip if not stale.
		if conn.LastSyncedAt.Valid {
			nextSync := conn.LastSyncedAt.Time.Add(time.Duration(effectiveMinutes) * time.Minute)
			if nextSync.After(now) {
				skipped++
				done <- result{}
				continue
			}
		}

		connID := conn.ID
		sem <- struct{}{}
		go func() {
			defer func() {
				<-sem
				done <- result{}
			}()

			if err := s.engine.Sync(ctx, connID, db.SyncTriggerCron); err != nil {
				s.logger.Error("scheduled sync failed", "connection_id", formatUUID(connID), "error", err)
			}
		}()
		synced++
	}

	// Wait for all goroutines.
	for range connections {
		<-done
	}

	return synced, skipped
}

// RunStartupSync checks all active, unpaused connections and syncs any that
// are stale (last synced more than their effective interval ago or never synced).
func (s *Scheduler) RunStartupSync(ctx context.Context, globalIntervalMinutes int) {
	connections, err := s.queries.ListActiveUnpausedConnections(ctx)
	if err != nil {
		s.logger.Error("startup sync: failed to list connections", "error", err)
		return
	}

	if len(connections) == 0 {
		s.logger.Info("startup sync: no active unpaused connections")
		return
	}

	now := time.Now()
	var staleCount int

	for _, conn := range connections {
		effectiveMinutes := globalIntervalMinutes
		if conn.SyncIntervalOverrideMinutes.Valid {
			effectiveMinutes = int(conn.SyncIntervalOverrideMinutes.Int32)
		}

		threshold := now.Add(-time.Duration(effectiveMinutes) * time.Minute)
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
