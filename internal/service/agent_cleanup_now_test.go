//go:build integration && !lite

package service_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestRunCleanupNow exercises the on-demand cleanup path that backs the
// Settings → Agents "Run cleanup now" button. Seeds an old run row plus
// an old transcript file under transcript_dir; calls RunCleanupNow;
// asserts both surfaces report deletion counts.
func TestRunCleanupNow(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	// Configure retention + transcript dir via app_config.
	transcriptDir := t.TempDir()
	if err := q.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   appconfig.KeyAgentTranscriptDir,
		Value: pgtype.Text{String: transcriptDir, Valid: true},
	}); err != nil {
		t.Fatalf("set transcript_dir: %v", err)
	}
	if err := q.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   appconfig.KeyAgentRunRetentionDays,
		Value: pgtype.Text{String: "30", Valid: true},
	}); err != nil {
		t.Fatalf("set retention: %v", err)
	}

	// Seed: one old completed agent_run (40 days back), one fresh one.
	def := mustCreateAgentDefinition(t, svc, "svc-cleanup-now", true)
	defUUID, _ := pgconv.ParseUUID(def.ID)
	mustInsertCompletedRun(t, q, defUUID, "0.01")
	if _, err := pool.Exec(ctx,
		`UPDATE workflow_runs SET started_at = $1, completed_at = $1
		 WHERE workflow_id = $2`,
		time.Now().AddDate(0, 0, -40), defUUID); err != nil {
		t.Fatalf("backdate first row: %v", err)
	}
	mustInsertCompletedRun(t, q, defUUID, "0.01") // fresh — leaves started_at = NOW()

	// Seed transcript files matching.
	oldPath := filepath.Join(transcriptDir, "aaaa1111.ndjson")
	if err := os.WriteFile(oldPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write old transcript: %v", err)
	}
	if err := os.Chtimes(oldPath, time.Now().AddDate(0, 0, -40), time.Now().AddDate(0, 0, -40)); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(transcriptDir, "bbbb2222.ndjson"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write fresh transcript: %v", err)
	}

	orch := service.NewOrchestrator(svc, agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{}, nil
	}), 1, devEncKey, slog.Default())
	sched := service.NewAgentScheduler(orch, svc, slog.Default())

	result := sched.RunCleanupNow(ctx)

	if result.RunsDeleted < 1 {
		t.Errorf("RunsDeleted = %d, want >= 1 (the 40-day-old row)", result.RunsDeleted)
	}
	if result.TranscriptsDeleted != 1 {
		t.Errorf("TranscriptsDeleted = %d, want 1 (only the old .ndjson)", result.TranscriptsDeleted)
	}
	if result.TranscriptsScanned != 2 {
		t.Errorf("TranscriptsScanned = %d, want 2", result.TranscriptsScanned)
	}
	if result.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", result.RetentionDays)
	}
	if result.TranscriptDir != transcriptDir {
		t.Errorf("TranscriptDir = %q, want %q", result.TranscriptDir, transcriptDir)
	}

	// Verify the fresh transcript still exists.
	if _, err := os.Stat(filepath.Join(transcriptDir, "bbbb2222.ndjson")); err != nil {
		t.Errorf("fresh transcript should remain, stat err = %v", err)
	}
}

func TestRunCleanupNow_RetentionDisabled(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	if err := q.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   appconfig.KeyAgentRunRetentionDays,
		Value: pgtype.Text{String: "0", Valid: true},
	}); err != nil {
		t.Fatalf("set retention=0: %v", err)
	}

	orch := service.NewOrchestrator(svc, agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{}, nil
	}), 1, devEncKey, slog.Default())
	sched := service.NewAgentScheduler(orch, svc, slog.Default())

	result := sched.RunCleanupNow(ctx)
	if result.RunsDeleted != 0 || result.TranscriptsDeleted != 0 {
		t.Errorf("retention=0 should be a no-op, got %+v", result)
	}
	if result.RetentionDays != 0 {
		t.Errorf("RetentionDays = %d, want 0", result.RetentionDays)
	}
}

