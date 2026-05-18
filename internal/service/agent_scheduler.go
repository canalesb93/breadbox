//go:build !lite

package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

	// reloadMu serializes the WHOLE Reload critical section
	// (remove-all + DB read + AddFunc loop) so concurrent CRUD mutations
	// can't interleave their reloads and produce duplicate cron entries
	// with leaked EntryIDs. Separate from `mu` (which protects the
	// entryIDs map for single-statement read/write) because Reload spans
	// multiple cron-library calls plus a DB query — holding `mu` for that
	// long would block fireCronJob lookups longer than needed.
	reloadMu sync.Mutex
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
		bg := context.Background()
		result := s.runCleanupAll(bg)
		s.logCleanupResult(result, "scheduled")
	})
	if err != nil {
		s.logger.Error("agent scheduler: add cleanup job failed", "error", err)
	}
	s.cron.Start()
	s.logger.Info("agent scheduler started", "enabled_count", len(s.entryIDs))
}

// AgentCleanupResult summarizes one cleanup pass — what was deleted from
// agent_runs and the transcript directory, plus the retention setting in
// effect at the time. Returned by RunCleanupNow so the HTTP handler can
// echo it back for operator toast/display.
type AgentCleanupResult struct {
	RunsDeleted        int64 `json:"runs_deleted"`
	TranscriptsDeleted int   `json:"transcripts_deleted"`
	TranscriptsScanned int   `json:"transcripts_scanned"`
	RetentionDays      int   `json:"retention_days"`
	TranscriptDir      string `json:"transcript_dir,omitempty"`
}

// RunCleanupNow runs the same prune pass the daily tick runs, synchronously,
// and returns the counts. Used by the Settings → Agents "Run cleanup now"
// button so an operator who just lowered retention can see the effect
// without waiting for 3:15 AM. Safe to call repeatedly — a no-op when
// nothing's eligible.
func (s *AgentScheduler) RunCleanupNow(ctx context.Context) AgentCleanupResult {
	result := s.runCleanupAll(ctx)
	s.logCleanupResult(result, "on-demand")
	return result
}

// runCleanupAll is the shared body: both the cron tick and RunCleanupNow
// go through it so they can't drift.
func (s *AgentScheduler) runCleanupAll(ctx context.Context) AgentCleanupResult {
	retentionDays := appconfig.Int(ctx, s.svc.Queries, appconfig.KeyAgentRunRetentionDays, 30)
	transcriptDir := appconfig.String(ctx, s.svc.Queries, appconfig.KeyAgentTranscriptDir, "")
	result := AgentCleanupResult{
		RetentionDays: retentionDays,
		TranscriptDir: transcriptDir,
	}
	if retentionDays <= 0 {
		return result
	}
	result.RunsDeleted = s.cleanupAgentRuns(ctx)
	if transcriptDir != "" {
		result.TranscriptsDeleted, result.TranscriptsScanned = s.cleanupTranscriptFiles(ctx)
	}
	return result
}

// logCleanupResult writes the structured slog line both cleanup paths share,
// tagged with how the pass was triggered.
func (s *AgentScheduler) logCleanupResult(r AgentCleanupResult, source string) {
	if r.RunsDeleted == 0 && r.TranscriptsDeleted == 0 {
		s.logger.Debug("agent cleanup pass: nothing to do",
			"source", source, "retention_days", r.RetentionDays)
		return
	}
	s.logger.Info("agent cleanup pass completed",
		"source", source,
		"runs_deleted", r.RunsDeleted,
		"transcripts_deleted", r.TranscriptsDeleted,
		"transcripts_scanned", r.TranscriptsScanned,
		"retention_days", r.RetentionDays)
}

// EntryCountForTest returns the live count of cron entries — agent
// registrations PLUS the singleton cleanup tick added in Start. Exposed
// for the concurrent-Reload regression test; the cron library has no
// other way to verify "no duplicate entries leaked." Do not call from
// production code.
func (s *AgentScheduler) EntryCountForTest() int {
	return len(s.cron.Entries())
}

// Stop gracefully halts the scheduler, waiting for any in-flight jobs.
func (s *AgentScheduler) Stop() {
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.logger.Info("agent scheduler stopped")
}

// Reload removes all per-definition cron entries and re-registers from DB.
// Called after any agent_definition CRUD mutation.
//
// Holds reloadMu for the entire span so two concurrent CRUD-triggered
// reloads can't both pass the "remove all" phase and then both call
// AddFunc, producing duplicate cron entries whose EntryIDs are leaked
// (entryIDs[slug] can only store one ID).
func (s *AgentScheduler) Reload(ctx context.Context) {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
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

// ComputeNextFire returns the next time the scheduler would fire `def`
// after `now`, accounting for quiet hours. Returns nil when the schedule
// is absent (manual-only), unparseable, or no non-quiet slot is found
// within the safety-limit window.
func ComputeNextFire(def *AgentDefinitionResponse, now time.Time) *time.Time {
	if def == nil || def.ScheduleCron == nil || *def.ScheduleCron == "" {
		return nil
	}
	schedule, err := cron.ParseStandard(*def.ScheduleCron)
	if err != nil {
		return nil
	}
	next := schedule.Next(now)
	// Up to 100 iterations of "next fire lands in quiet hours → advance
	// past the quiet window → re-ask cron." Bounded so a misconfigured
	// 24-hour quiet window can't infinite-loop.
	for i := 0; i < 100; i++ {
		if !IsWithinQuietHours(next, def.QuietHoursStart, def.QuietHoursEnd) {
			return &next
		}
		// Subtract one second so a cron that fires AT the quiet-end
		// minute still gets returned by schedule.Next (which excludes
		// the supplied time). Otherwise we'd skip past a valid first
		// fire — e.g. quiet 22-07 + hourly cron should report 07:00,
		// not 08:00.
		jumped := nextMinuteAfterQuietEnd(next, *def.QuietHoursEnd).Add(-time.Second)
		next = schedule.Next(jumped)
	}
	return nil
}

// nextMinuteAfterQuietEnd returns the first concrete time >= `now` that
// lands at the end of the quiet window. Used by ComputeNextFire to skip
// past the quiet period and resume cron evaluation.
func nextMinuteAfterQuietEnd(now time.Time, end string) time.Time {
	endMin, ok := parseHHMM(end)
	if !ok {
		// Fallback so the outer loop's bound prevents infinite recursion.
		return now.Add(time.Hour)
	}
	endHour := endMin / 60
	endMinute := endMin % 60
	candidate := time.Date(now.Year(), now.Month(), now.Day(),
		endHour, endMinute, 0, 0, now.Location())
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate
}

// cleanupTranscriptFiles prunes NDJSON transcript files older than the
// retention window. Reuses the same `agent.run_retention_days` setting as
// the agent_runs cleanup, so the two surfaces stay aligned. Returns the
// number deleted + number scanned so callers can surface counts. Caller
// is responsible for the transcript_dir + retention preflight.
func (s *AgentScheduler) cleanupTranscriptFiles(ctx context.Context) (deleted, scanned int) {
	transcriptDir := appconfig.String(ctx, s.svc.Queries, appconfig.KeyAgentTranscriptDir, "")
	retentionDays := appconfig.Int(ctx, s.svc.Queries, appconfig.KeyAgentRunRetentionDays, 30)
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	deleted, scanned, err := pruneTranscriptFiles(transcriptDir, cutoff)
	if err != nil {
		s.logger.Error("agent transcript cleanup failed",
			"dir", transcriptDir, "error", err)
		return 0, 0
	}
	return deleted, scanned
}

// pruneTranscriptFiles is the pure file-walking pass — split out so tests
// can exercise it against a tempdir without a scheduler. Deletes `*.ndjson`
// files in `dir` whose mtime is before `cutoff`. Returns (deleted, scanned,
// first-error). Non-NDJSON entries and subdirectories are left untouched
// so an operator who points transcript_dir at a shared folder doesn't lose
// adjacent files.
func pruneTranscriptFiles(dir string, cutoff time.Time) (deleted, scanned int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".ndjson") {
			continue
		}
		scanned++
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if rerr := os.Remove(filepath.Join(dir, e.Name())); rerr == nil {
				deleted++
			}
		}
	}
	return deleted, scanned, nil
}

// cleanupAgentRuns prunes completed agent_runs older than the retention
// period and returns the number deleted. Caller is responsible for the
// retention preflight (runCleanupAll short-circuits when retention<=0).
func (s *AgentScheduler) cleanupAgentRuns(ctx context.Context) int64 {
	retentionDays := appconfig.Int(ctx, s.svc.Queries, appconfig.KeyAgentRunRetentionDays, 30)
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result, err := s.svc.Queries.DeleteAgentRunsOlderThan(ctx,
		pgconv.Timestamptz(cutoff))
	if err != nil {
		s.logger.Error("agent run cleanup failed", "error", err)
		return 0
	}
	return result.RowsAffected()
}
