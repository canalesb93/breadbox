//go:build !lite

package sync

import (
	"context"
	"log/slog"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"
)

// Scheduler runs periodic transaction syncs using cron.
type Scheduler struct {
	cron        *cron.Cron
	engine      *Engine
	queries     *db.Queries
	logger      *slog.Logger
	syncTimeout time.Duration
}

// NewScheduler creates a new Scheduler.
func NewScheduler(engine *Engine, queries *db.Queries, logger *slog.Logger, syncTimeout time.Duration) *Scheduler {
	return &Scheduler{
		cron:        cron.New(),
		engine:      engine,
		queries:     queries,
		logger:      logger,
		syncTimeout: syncTimeout,
	}
}

// Start begins the cron scheduler. The tick fires every 15 minutes (the
// evaluation granularity) and checks each connection against its wall-clock sync
// schedules. fallbackIntervalMinutes is only consulted if no schedules exist.
// It also schedules a daily sync log cleanup job.
func (s *Scheduler) Start(fallbackIntervalMinutes int) {
	_, err := s.cron.AddFunc("@every 15m", func() {
		ctx := context.Background()
		s.logger.Info("cron sync starting")
		synced, skipped := s.syncAllScheduled(ctx, fallbackIntervalMinutes)
		s.logger.Info("cron sync completed", "synced", synced, "skipped", skipped)
	})
	if err != nil {
		s.logger.Error("failed to add cron job", "error", err)
		return
	}

	// Daily sync log cleanup at 3:00 AM.
	_, err = s.cron.AddFunc("0 3 * * *", func() {
		ctx := context.Background()
		s.cleanupSyncLogs(ctx)
	})
	if err != nil {
		s.logger.Error("failed to add sync log cleanup job", "error", err)
	}

	s.cron.Start()
	s.logger.Info("scheduler started", "check_interval", "15m", "fallback_interval_minutes", fallbackIntervalMinutes)
}

// IsRunning returns true if the scheduler has any active cron entries.
func (s *Scheduler) IsRunning() bool {
	return len(s.cron.Entries()) > 0
}

// NextRun returns the next scheduled cron fire time.
// Returns zero time if no entries are scheduled.
func (s *Scheduler) NextRun() time.Time {
	entries := s.cron.Entries()
	if len(entries) == 0 {
		return time.Time{}
	}
	return entries[0].Next
}

// AddFunc registers an additional cron job with the scheduler.
// spec is a standard cron expression (e.g., "0 2 * * *" for daily at 2am).
func (s *Scheduler) AddFunc(spec string, cmd func()) error {
	_, err := s.cron.AddFunc(spec, cmd)
	return err
}

// Stop gracefully stops the scheduler, waiting for any running jobs to finish.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.logger.Info("scheduler stopped")
}

// backoffInterval returns an adjusted sync interval in minutes based on
// consecutive failures. Uses exponential backoff (base * 2^failures) capped
// at 16x the base interval so a persistently failing connection doesn't
// retry every 15 minutes indefinitely.
func backoffInterval(baseMinutes int, consecutiveFailures int32) int {
	if consecutiveFailures <= 0 {
		return baseMinutes
	}
	// Cap the exponent at 4 so max multiplier is 2^4 = 16.
	exp := int(consecutiveFailures)
	if exp > 4 {
		exp = 4
	}
	return baseMinutes * (1 << exp)
}

// syncAllScheduled syncs all active, unpaused connections that are due
// according to their effective sync schedules (the union of `applies_to_all`
// schedules plus any explicitly targeting the connection). Wall-clock anchored:
// a connection is due when a scheduled fire time has passed since its last
// successful sync. fallbackIntervalMinutes is only used if no schedules exist.
func (s *Scheduler) syncAllScheduled(ctx context.Context, fallbackIntervalMinutes int) (synced, skipped int) {
	connections, err := s.queries.ListActiveUnpausedConnections(ctx)
	if err != nil {
		s.logger.Error("list active unpaused connections", "error", err)
		return 0, 0
	}

	if len(connections) == 0 {
		s.logger.Info("no active unpaused connections to sync")
		return 0, 0
	}

	resolver, err := s.loadScheduleResolver(ctx, fallbackIntervalMinutes)
	if err != nil {
		s.logger.Error("load sync schedules", "error", err)
		return 0, 0
	}

	now := time.Now()
	const maxWorkers = 5
	sem := make(chan struct{}, maxWorkers)

	type result struct{}
	done := make(chan result, len(connections))

	for _, conn := range connections {
		// Hold back a failing connection until its backoff window elapses,
		// regardless of how frequently its schedule would otherwise fire.
		if conn.LastErrorAt.Valid && backoffSuppressed(conn.ConsecutiveFailures, conn.LastErrorAt.Time, now) {
			skipped++
			done <- result{}
			continue
		}

		schedules := resolver.forConnection(conn.ID.Bytes)
		jitter := connectionJitter(conn.ID.Bytes, jitterWindow)
		lastSynced := time.Time{}
		if conn.LastSyncedAt.Valid {
			lastSynced = conn.LastSyncedAt.Time
		}
		if !scheduleDue(schedules, lastSynced, now, jitter) {
			skipped++
			done <- result{}
			continue
		}

		connID := conn.ID
		sem <- struct{}{}
		go func() {
			defer func() {
				<-sem
				done <- result{}
			}()

			syncCtx, cancel := context.WithTimeout(ctx, s.syncTimeout)
			defer cancel()
			if err := s.engine.Sync(syncCtx, connID, db.SyncTriggerCron); err != nil {
				s.logger.Error("scheduled sync failed", "connection_id", pgconv.FormatUUID(connID), "error", err)
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

// RunStartupSync checks all active, unpaused connections and syncs any that are
// due according to their schedules (a scheduled fire was missed while the server
// was down, or the connection has never synced). This is the catch-up path.
func (s *Scheduler) RunStartupSync(ctx context.Context, fallbackIntervalMinutes int) {
	connections, err := s.queries.ListActiveUnpausedConnections(ctx)
	if err != nil {
		s.logger.Error("startup sync: failed to list connections", "error", err)
		return
	}

	if len(connections) == 0 {
		s.logger.Info("startup sync: no active unpaused connections")
		return
	}

	resolver, err := s.loadScheduleResolver(ctx, fallbackIntervalMinutes)
	if err != nil {
		s.logger.Error("startup sync: load sync schedules", "error", err)
		return
	}

	now := time.Now()
	var staleCount int

	for _, conn := range connections {
		if conn.LastErrorAt.Valid && backoffSuppressed(conn.ConsecutiveFailures, conn.LastErrorAt.Time, now) {
			continue
		}
		schedules := resolver.forConnection(conn.ID.Bytes)
		jitter := connectionJitter(conn.ID.Bytes, jitterWindow)
		lastSynced := time.Time{}
		if conn.LastSyncedAt.Valid {
			lastSynced = conn.LastSyncedAt.Time
		}
		if scheduleDue(schedules, lastSynced, now, jitter) {
			staleCount++
			syncCtx, cancel := context.WithTimeout(ctx, s.syncTimeout)
			if err := s.engine.Sync(syncCtx, conn.ID, db.SyncTriggerCron); err != nil {
				s.logger.Error("startup sync: connection failed",
					"connection_id", pgconv.FormatUUID(conn.ID),
					"error", err,
				)
			}
			cancel()
		}
	}

	s.logger.Info("startup sync completed", "total", len(connections), "stale_synced", staleCount)
}

// defaultRetentionDays is used when sync_log_retention_days is not configured.
const defaultRetentionDays = 90

// cleanupSyncLogs deletes sync logs older than the configured retention period.
// Reads sync_log_retention_days from app_config (default: 90 days).
// A value of 0 disables cleanup.
func (s *Scheduler) cleanupSyncLogs(ctx context.Context) {
	retentionDays := appconfig.Int(ctx, s.queries, "sync_log_retention_days", defaultRetentionDays)
	if retentionDays < 0 {
		retentionDays = defaultRetentionDays
	}

	if retentionDays == 0 {
		s.logger.Debug("sync log cleanup disabled (retention_days=0)")
		return
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result, err := s.queries.DeleteSyncLogsOlderThan(ctx, pgtype.Timestamptz{
		Time:  cutoff,
		Valid: true,
	})
	if err != nil {
		s.logger.Error("sync log cleanup failed", "error", err)
		return
	}

	deleted := result.RowsAffected()
	if deleted > 0 {
		s.logger.Info("sync log cleanup completed", "deleted", deleted, "retention_days", retentionDays)
	} else {
		s.logger.Debug("sync log cleanup completed, no old logs to delete", "retention_days", retentionDays)
	}
}
