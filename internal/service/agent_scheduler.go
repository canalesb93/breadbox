//go:build !lite

package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/appconfig"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"
)

// AgentScheduler registers one cron entry per enabled agent_definition that
// has a non-null schedule_cron. Reload re-syncs from DB.
type AgentScheduler struct {
	cron   *cron.Cron
	orch   *Orchestrator
	svc    *Service
	logger *slog.Logger

	mu       sync.Mutex
	entryIDs map[string]cron.EntryID // slug → entry id
}

// NewAgentScheduler constructs the scheduler. Call Start to begin firing.
func NewAgentScheduler(orch *Orchestrator, svc *Service, logger *slog.Logger) *AgentScheduler {
	return &AgentScheduler{
		cron:     cron.New(),
		orch:     orch,
		svc:      svc,
		logger:   logger,
		entryIDs: make(map[string]cron.EntryID),
	}
}

// Start registers all enabled definitions and the daily cleanup job, then
// kicks off the cron loop.
func (s *AgentScheduler) Start(ctx context.Context) {
	s.registerAll(ctx)
	_, err := s.cron.AddFunc("15 3 * * *", func() {
		s.cleanupAgentRuns(context.Background())
	})
	if err != nil {
		s.logger.Error("agent scheduler: add cleanup job failed", "error", err)
	}
	s.cron.Start()
	s.logger.Info("agent scheduler started", "enabled_count", len(s.entryIDs))
}

// Stop gracefully halts the scheduler, waiting for any in-flight jobs.
func (s *AgentScheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.logger.Info("agent scheduler stopped")
}

// Reload removes all per-definition cron entries and re-registers from DB.
// Called after any agent_definition CRUD mutation.
func (s *AgentScheduler) Reload(ctx context.Context) {
	s.mu.Lock()
	for slug, id := range s.entryIDs {
		s.cron.Remove(id)
		delete(s.entryIDs, slug)
	}
	s.mu.Unlock()
	s.registerAll(ctx)
	s.logger.Info("agent scheduler reloaded", "registered", len(s.entryIDs))
}

// registerAll reads enabled definitions and registers one cron entry per
// definition with a non-empty schedule_cron.
func (s *AgentScheduler) registerAll(ctx context.Context) {
	defs, err := s.svc.Queries.ListEnabledAgentDefinitions(ctx)
	if err != nil {
		s.logger.Error("agent scheduler: list enabled defs failed", "error", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, def := range defs {
		if !def.ScheduleCron.Valid || def.ScheduleCron.String == "" {
			continue
		}
		slug := def.Slug
		schedCron := def.ScheduleCron.String
		defID := def.ID
		id, err := s.cron.AddFunc(schedCron, func() {
			s.fireCronJob(defID, slug)
		})
		if err != nil {
			s.logger.Error("agent scheduler: invalid cron expression",
				"agent", slug, "cron", schedCron, "error", err)
			continue
		}
		s.entryIDs[slug] = id
		s.logger.Info("agent scheduler: registered", "agent", slug, "cron", schedCron)
	}
}

// fireCronJob is the cron callback for one definition. It resolves the
// definition fresh, then calls RunOrSkip on the orchestrator.
func (s *AgentScheduler) fireCronJob(defID pgtype.UUID, slug string) {
	ctx := context.Background()
	def, err := s.svc.GetAgentDefinition(ctx, pgconv.FormatUUID(defID))
	if err != nil {
		s.logger.Error("agent scheduler: resolve definition failed",
			"agent", slug, "error", err)
		return
	}
	if !def.Enabled {
		return
	}
	if IsWithinQuietHours(time.Now(), def.QuietHoursStart, def.QuietHoursEnd) {
		s.logger.Info("agent scheduler: run skipped (quiet hours)",
			"agent", slug,
			"quiet_start", derefString(def.QuietHoursStart),
			"quiet_end", derefString(def.QuietHoursEnd),
		)
		return
	}
	_, runErr := s.orch.RunOrSkip(ctx, def, "cron")
	if errors.Is(runErr, agent.ErrConcurrencyLocked) {
		s.logger.Warn("agent scheduler: run skipped (concurrency locked)", "agent", slug)
		return
	}
	if runErr != nil {
		s.logger.Error("agent scheduler: run failed", "agent", slug, "error", runErr)
	}
}

// cleanupAgentRuns prunes completed agent_runs older than the retention
// period (default 30 days; 0 disables). Transcript file GC is deferred
// to a later iteration.
func (s *AgentScheduler) cleanupAgentRuns(ctx context.Context) {
	retentionDays := appconfig.Int(ctx, s.svc.Queries, appconfig.KeyAgentRunRetentionDays, 30)
	if retentionDays <= 0 {
		s.logger.Debug("agent run cleanup disabled", "retention_days", retentionDays)
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result, err := s.svc.Queries.DeleteAgentRunsOlderThan(ctx,
		pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		s.logger.Error("agent run cleanup failed", "error", err)
		return
	}
	if n := result.RowsAffected(); n > 0 {
		s.logger.Info("agent run cleanup completed",
			"deleted", n, "retention_days", retentionDays)
	}
}
