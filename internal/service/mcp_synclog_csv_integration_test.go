//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// ===================== MCP Config Service Tests =====================

func TestGetMCPConfig_Defaults(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	cfg, err := svc.GetMCPConfig(ctx)
	if err != nil {
		t.Fatalf("GetMCPConfig failed: %v", err)
	}
	if cfg.Mode != "read_write" {
		t.Errorf("default mode = %q, want %q", cfg.Mode, "read_write")
	}
	if len(cfg.DisabledTools) != 0 {
		t.Errorf("default disabled_tools should be empty, got %v", cfg.DisabledTools)
	}
	if cfg.Instructions != "" {
		t.Errorf("default instructions should be empty, got %q", cfg.Instructions)
	}
}

func TestSaveMCPMode_Valid(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Set to read_write
	if err := svc.SaveMCPMode(ctx, "read_write"); err != nil {
		t.Fatalf("SaveMCPMode(read_write) failed: %v", err)
	}
	cfg, err := svc.GetMCPConfig(ctx)
	if err != nil {
		t.Fatalf("GetMCPConfig failed: %v", err)
	}
	if cfg.Mode != "read_write" {
		t.Errorf("mode = %q, want %q", cfg.Mode, "read_write")
	}

	// Set back to read_only
	if err := svc.SaveMCPMode(ctx, "read_only"); err != nil {
		t.Fatalf("SaveMCPMode(read_only) failed: %v", err)
	}
	cfg, err = svc.GetMCPConfig(ctx)
	if err != nil {
		t.Fatalf("GetMCPConfig failed: %v", err)
	}
	if cfg.Mode != "read_only" {
		t.Errorf("mode = %q, want %q", cfg.Mode, "read_only")
	}
}

func TestSaveMCPMode_Invalid(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	err := svc.SaveMCPMode(ctx, "invalid_mode")
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
}

func TestSaveMCPDisabledTools_RoundTrip(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	tools := []string{"query_transactions", "batch_categorize_transactions"}
	if err := svc.SaveMCPDisabledTools(ctx, tools); err != nil {
		t.Fatalf("SaveMCPDisabledTools failed: %v", err)
	}

	cfg, err := svc.GetMCPConfig(ctx)
	if err != nil {
		t.Fatalf("GetMCPConfig failed: %v", err)
	}
	if len(cfg.DisabledTools) != 2 {
		t.Fatalf("disabled_tools length = %d, want 2", len(cfg.DisabledTools))
	}
	if cfg.DisabledTools[0] != "query_transactions" {
		t.Errorf("disabled_tools[0] = %q, want %q", cfg.DisabledTools[0], "query_transactions")
	}
	if cfg.DisabledTools[1] != "batch_categorize_transactions" {
		t.Errorf("disabled_tools[1] = %q, want %q", cfg.DisabledTools[1], "batch_categorize_transactions")
	}
}

func TestSaveMCPDisabledTools_NilBecomesEmpty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// First set some tools
	if err := svc.SaveMCPDisabledTools(ctx, []string{"tool1"}); err != nil {
		t.Fatalf("SaveMCPDisabledTools failed: %v", err)
	}

	// Now clear with nil
	if err := svc.SaveMCPDisabledTools(ctx, nil); err != nil {
		t.Fatalf("SaveMCPDisabledTools(nil) failed: %v", err)
	}

	cfg, err := svc.GetMCPConfig(ctx)
	if err != nil {
		t.Fatalf("GetMCPConfig failed: %v", err)
	}
	if len(cfg.DisabledTools) != 0 {
		t.Errorf("disabled_tools should be empty after nil, got %v", cfg.DisabledTools)
	}
}

func TestSaveMCPInstructions_RoundTrip(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	instructions := "# Server Instructions\nAnalyze spending by category monthly."
	if err := svc.SaveMCPInstructions(ctx, instructions); err != nil {
		t.Fatalf("SaveMCPInstructions failed: %v", err)
	}

	cfg, err := svc.GetMCPConfig(ctx)
	if err != nil {
		t.Fatalf("GetMCPConfig failed: %v", err)
	}
	if cfg.Instructions != instructions {
		t.Errorf("instructions mismatch: got %q", cfg.Instructions)
	}
}

func TestSaveMCPInstructions_TooLong(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// 20001 characters should fail
	longStr := make([]byte, 20001)
	for i := range longStr {
		longStr[i] = 'a'
	}
	err := svc.SaveMCPInstructions(ctx, string(longStr))
	if err == nil {
		t.Fatal("expected error for instructions > 20000 chars, got nil")
	}
}

func TestMCPConfig_FullRoundTrip(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Set all config values
	if err := svc.SaveMCPMode(ctx, "read_write"); err != nil {
		t.Fatalf("SaveMCPMode: %v", err)
	}
	if err := svc.SaveMCPDisabledTools(ctx, []string{"tool_a", "tool_b"}); err != nil {
		t.Fatalf("SaveMCPDisabledTools: %v", err)
	}
	if err := svc.SaveMCPInstructions(ctx, "Test instructions"); err != nil {
		t.Fatalf("SaveMCPInstructions: %v", err)
	}

	cfg, err := svc.GetMCPConfig(ctx)
	if err != nil {
		t.Fatalf("GetMCPConfig: %v", err)
	}
	if cfg.Mode != "read_write" {
		t.Errorf("mode = %q", cfg.Mode)
	}
	if len(cfg.DisabledTools) != 2 {
		t.Errorf("disabled_tools = %v", cfg.DisabledTools)
	}
	if cfg.Instructions != "Test instructions" {
		t.Errorf("instructions = %q", cfg.Instructions)
	}
}

// ===================== Sync Log Service Tests =====================

func TestListSyncLogsPaginated_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_sync_1")

	now := time.Now().UTC()
	_, err := queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateSyncLog: %v", err)
	}

	result, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated failed: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("total = %d, want 1", result.Total)
	}
	if len(result.Logs) != 1 {
		t.Fatalf("logs count = %d, want 1", len(result.Logs))
	}
	if result.Logs[0].Trigger != "manual" {
		t.Errorf("trigger = %q, want %q", result.Logs[0].Trigger, "manual")
	}
	if result.Logs[0].Status != "success" {
		t.Errorf("status = %q, want %q", result.Logs[0].Status, "success")
	}
}

func TestListSyncLogsPaginated_FilterByConnection(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn1 := testutil.MustCreateConnection(t, queries, user.ID, "item_sync_f1")
	conn2 := testutil.MustCreateConnection(t, queries, user.ID, "item_sync_f2")

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
			ConnectionID: conn1.ID,
			Trigger:      db.SyncTriggerManual,
			Status:       db.SyncStatusSuccess,
			StartedAt:    pgtype.Timestamptz{Time: now.Add(time.Duration(i) * time.Minute), Valid: true},
		})
	}
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn2.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusError,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})

	conn1ID := formatUUID(conn1.ID)
	result, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:         1,
		PageSize:     10,
		ConnectionID: &conn1ID,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("total = %d, want 3", result.Total)
	}
}

func TestListSyncLogsPaginated_FilterByStatus(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_sync_s1")

	now := time.Now().UTC()
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusError,
		StartedAt:    pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
	})

	status := "error"
	result, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
		Status:   &status,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1", result.Total)
	}
	if len(result.Logs) != 1 || result.Logs[0].Status != "error" {
		t.Errorf("expected 1 error log")
	}
}

func TestListSyncLogsPaginated_Pagination(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_sync_p1")

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
			ConnectionID: conn.ID,
			Trigger:      db.SyncTriggerManual,
			Status:       db.SyncStatusSuccess,
			StartedAt:    pgtype.Timestamptz{Time: now.Add(time.Duration(i) * time.Minute), Valid: true},
		})
	}

	// Page 1, size 2
	result, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated page 1: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("total = %d, want 5", result.Total)
	}
	if len(result.Logs) != 2 {
		t.Errorf("page 1 logs = %d, want 2", len(result.Logs))
	}
	if result.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", result.TotalPages)
	}

	// Page 3 (last page with 1 item)
	result, err = svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     3,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated page 3: %v", err)
	}
	if len(result.Logs) != 1 {
		t.Errorf("page 3 logs = %d, want 1", len(result.Logs))
	}
}

func TestListSyncLogsPaginated_InvalidConnectionID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	badID := "not-a-uuid"
	_, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:         1,
		PageSize:     10,
		ConnectionID: &badID,
	})
	if err == nil {
		t.Fatal("expected error for invalid connection ID")
	}
}

func TestCountSyncLogsFiltered_Basic(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_count_1")

	now := time.Now().UTC()
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusError,
		StartedAt:    pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
	})

	// Count all
	count, err := svc.CountSyncLogsFiltered(ctx, service.SyncLogListParams{})
	if err != nil {
		t.Fatalf("CountSyncLogsFiltered: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	// Count only errors
	status := "error"
	count, err = svc.CountSyncLogsFiltered(ctx, service.SyncLogListParams{
		Status: &status,
	})
	if err != nil {
		t.Fatalf("CountSyncLogsFiltered(error): %v", err)
	}
	if count != 1 {
		t.Errorf("error count = %d, want 1", count)
	}
}

func TestSyncLogStats_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_stats_1")

	now := time.Now().UTC()
	// Create 3 successes and 1 error
	for i := 0; i < 3; i++ {
		log, _ := queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
			ConnectionID: conn.ID,
			Trigger:      db.SyncTriggerManual,
			Status:       db.SyncStatusInProgress,
			StartedAt:    pgtype.Timestamptz{Time: now.Add(time.Duration(i) * time.Minute), Valid: true},
		})
		queries.UpdateSyncLog(ctx, db.UpdateSyncLogParams{
			ID:            log.ID,
			Status:        db.SyncStatusSuccess,
			CompletedAt:   pgtype.Timestamptz{Time: now.Add(time.Duration(i)*time.Minute + 5*time.Second), Valid: true},
			AddedCount:    int32(i + 1),
			ModifiedCount: 0,
			RemovedCount:  0,
		})
	}
	errLog, _ := queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusInProgress,
		StartedAt:    pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true},
	})
	queries.UpdateSyncLog(ctx, db.UpdateSyncLogParams{
		ID:            errLog.ID,
		Status:        db.SyncStatusError,
		CompletedAt:   pgtype.Timestamptz{Time: now.Add(5*time.Minute + 2*time.Second), Valid: true},
		ErrorMessage:  pgtype.Text{String: "provider error", Valid: true},
	})

	stats, err := svc.SyncLogStats(ctx, service.SyncLogListParams{})
	if err != nil {
		t.Fatalf("SyncLogStats: %v", err)
	}
	if stats.TotalSyncs != 4 {
		t.Errorf("total_syncs = %d, want 4", stats.TotalSyncs)
	}
	if stats.SuccessCount != 3 {
		t.Errorf("success_count = %d, want 3", stats.SuccessCount)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("error_count = %d, want 1", stats.ErrorCount)
	}
	if stats.SuccessRate != 75.0 {
		t.Errorf("success_rate = %f, want 75.0", stats.SuccessRate)
	}
	// Total added should be 1+2+3=6
	if stats.TotalAdded != 6 {
		t.Errorf("total_added = %d, want 6", stats.TotalAdded)
	}
	if stats.AvgDurationMs <= 0 {
		t.Errorf("avg_duration_ms should be > 0, got %f", stats.AvgDurationMs)
	}
}

func TestSyncLogStats_FilterByConnection(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn1 := testutil.MustCreateConnection(t, queries, user.ID, "item_stats_f1")
	conn2 := testutil.MustCreateConnection(t, queries, user.ID, "item_stats_f2")

	now := time.Now().UTC()
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn1.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn2.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})

	conn1ID := formatUUID(conn1.ID)
	stats, err := svc.SyncLogStats(ctx, service.SyncLogListParams{
		ConnectionID: &conn1ID,
	})
	if err != nil {
		t.Fatalf("SyncLogStats: %v", err)
	}
	if stats.TotalSyncs != 1 {
		t.Errorf("total_syncs = %d, want 1 (filtered to conn1)", stats.TotalSyncs)
	}
}

// ===================== Sync Log Filter by Trigger Tests =====================

func TestListSyncLogsPaginated_FilterByTrigger(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_trig_f1")

	now := time.Now().UTC()
	// Create logs with different triggers.
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusError,
		StartedAt:    pgtype.Timestamptz{Time: now.Add(2 * time.Minute), Valid: true},
	})

	// Filter by manual trigger.
	trigger := "manual"
	result, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
		Trigger:  &trigger,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated(trigger=manual): %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1", result.Total)
	}
	if len(result.Logs) != 1 || result.Logs[0].Trigger != "manual" {
		t.Errorf("expected 1 manual log, got %d logs", len(result.Logs))
	}

	// Filter by cron trigger.
	trigger = "cron"
	result, err = svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
		Trigger:  &trigger,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated(trigger=cron): %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
}

func TestCountSyncLogsFiltered_ByTrigger(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_trig_c1")

	now := time.Now().UTC()
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerWebhook,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
	})

	trigger := "webhook"
	count, err := svc.CountSyncLogsFiltered(ctx, service.SyncLogListParams{
		Trigger: &trigger,
	})
	if err != nil {
		t.Fatalf("CountSyncLogsFiltered(trigger=webhook): %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

// ===================== Sync Log Filter by Date Range Tests =====================

func TestListSyncLogsPaginated_FilterByDateRange(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_date_f1")

	// Create logs at specific dates.
	day1 := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	day3 := time.Date(2025, 3, 25, 10, 0, 0, 0, time.UTC)

	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: day1, Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: day2, Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusError,
		StartedAt:    pgtype.Timestamptz{Time: day3, Valid: true},
	})

	// Filter: from March 10 onward — should return day2 and day3.
	dateFrom := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	result, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
		DateFrom: &dateFrom,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated(date_from): %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2 (from March 10 onward)", result.Total)
	}

	// Filter: up to March 20 — should return day1 and day2.
	dateTo := time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC)
	result, err = svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
		DateTo:   &dateTo,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated(date_to): %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2 (before March 20)", result.Total)
	}

	// Filter: March 10 to March 20 — should return only day2.
	result, err = svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
		DateFrom: &dateFrom,
		DateTo:   &dateTo,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated(date range): %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1 (March 10-20)", result.Total)
	}
}

func TestListSyncLogsPaginated_CombinedFilters(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_combo_f1")

	day1 := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Manual success on day1.
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: day1, Valid: true},
	})
	// Cron success on day1.
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: day1.Add(time.Hour), Valid: true},
	})
	// Manual error on day2.
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusError,
		StartedAt:    pgtype.Timestamptz{Time: day2, Valid: true},
	})

	// Filter: manual + success + day1 range — should return 1.
	trigger := "manual"
	status := "success"
	dateFrom := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)

	result, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
		Status:   &status,
		Trigger:  &trigger,
		DateFrom: &dateFrom,
		DateTo:   &dateTo,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated(combined): %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1 (manual + success + day1)", result.Total)
	}
	if len(result.Logs) == 1 {
		if result.Logs[0].Trigger != "manual" {
			t.Errorf("trigger = %q, want manual", result.Logs[0].Trigger)
		}
		if result.Logs[0].Status != "success" {
			t.Errorf("status = %q, want success", result.Logs[0].Status)
		}
	}
}

func TestSyncLogStats_FilterByTriggerAndDate(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_stats_td")

	day1 := time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 4, 15, 10, 0, 0, 0, time.UTC)

	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: day1, Valid: true},
	})
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: day2, Valid: true},
	})

	// Stats for manual trigger only.
	trigger := "manual"
	stats, err := svc.SyncLogStats(ctx, service.SyncLogListParams{
		Trigger: &trigger,
	})
	if err != nil {
		t.Fatalf("SyncLogStats(trigger=manual): %v", err)
	}
	if stats.TotalSyncs != 1 {
		t.Errorf("total_syncs = %d, want 1", stats.TotalSyncs)
	}

	// Stats for date range covering only day1.
	dateFrom := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(2025, 4, 2, 0, 0, 0, 0, time.UTC)
	stats, err = svc.SyncLogStats(ctx, service.SyncLogListParams{
		DateFrom: &dateFrom,
		DateTo:   &dateTo,
	})
	if err != nil {
		t.Fatalf("SyncLogStats(date range): %v", err)
	}
	if stats.TotalSyncs != 1 {
		t.Errorf("total_syncs = %d, want 1 (day1 only)", stats.TotalSyncs)
	}
}

// ===================== Connection/Account Edge Cases =====================

func TestGetConnectionStatus_WithSyncLog(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_status_1")

	now := time.Now().UTC()
	queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})

	connID := formatUUID(conn.ID)
	status, err := svc.GetConnectionStatus(ctx, connID)
	if err != nil {
		t.Fatalf("GetConnectionStatus: %v", err)
	}
	if status.Provider != "plaid" {
		t.Errorf("provider = %q, want %q", status.Provider, "plaid")
	}
	if status.Status != "active" {
		t.Errorf("status = %q, want %q", status.Status, "active")
	}
	if status.LastSyncLog == nil {
		t.Fatal("expected last_sync_log to be set")
	}
	if status.LastSyncLog.Trigger != "manual" {
		t.Errorf("last_sync_log.trigger = %q, want %q", status.LastSyncLog.Trigger, "manual")
	}
}

func TestGetConnectionStatus_NoSyncLog(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_status_nosync")

	connID := formatUUID(conn.ID)
	status, err := svc.GetConnectionStatus(ctx, connID)
	if err != nil {
		t.Fatalf("GetConnectionStatus: %v", err)
	}
	if status.LastSyncLog != nil {
		t.Error("expected last_sync_log to be nil when no syncs exist")
	}
	if status.LastAttemptedSyncAt != nil {
		t.Error("expected last_attempted_sync_at to be nil")
	}
}

func TestGetConnectionStatus_InvalidUUID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.GetConnectionStatus(ctx, "not-a-uuid")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound for invalid UUID, got: %v", err)
	}
}

func TestGetConnectionStatus_NonExistent(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.GetConnectionStatus(ctx, "00000000-0000-0000-0000-000000000099")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestListConnections_WithMultipleProviders(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	testutil.MustCreateConnection(t, queries, user.ID, "plaid_conn_1")
	testutil.MustCreateTellerConnection(t, queries, user.ID, "teller_conn_1")

	conns, err := svc.ListConnections(ctx, nil)
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(conns))
	}

	providers := map[string]bool{}
	for _, c := range conns {
		providers[c.Provider] = true
	}
	if !providers["plaid"] {
		t.Error("expected plaid provider")
	}
	if !providers["teller"] {
		t.Error("expected teller provider")
	}
}

func TestListConnections_FilterByUser(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice := testutil.MustCreateUser(t, queries, "Alice")
	bob := testutil.MustCreateUser(t, queries, "Bob")

	testutil.MustCreateConnection(t, queries, alice.ID, "alice_conn_1")
	testutil.MustCreateConnection(t, queries, alice.ID, "alice_conn_2")
	testutil.MustCreateConnection(t, queries, bob.ID, "bob_conn_1")

	aliceID := formatUUID(alice.ID)
	conns, err := svc.ListConnections(ctx, &aliceID)
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(conns) != 2 {
		t.Errorf("expected 2 connections for Alice, got %d", len(conns))
	}
}

func TestListConnections_FilterByInvalidUser(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	badID := "not-a-uuid"
	_, err := svc.ListConnections(ctx, &badID)
	if err == nil {
		t.Fatal("expected error for invalid user ID")
	}
}

func TestGetAccount_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_acct_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_get", "Checking Account")

	acctID := formatUUID(acct.ID)
	resp, err := svc.GetAccount(ctx, acctID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if resp.Name != "Checking Account" {
		t.Errorf("name = %q, want %q", resp.Name, "Checking Account")
	}
	if resp.Type != "depository" {
		t.Errorf("type = %q, want %q", resp.Type, "depository")
	}
}

func TestGetAccount_InvalidUUID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.GetAccount(ctx, "bad-uuid")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetAccountDetail_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_detail_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_detail_1", "My Savings")

	acctID := formatUUID(acct.ID)
	detail, err := svc.GetAccountDetail(ctx, acctID)
	if err != nil {
		t.Fatalf("GetAccountDetail: %v", err)
	}
	if detail.Name != "My Savings" {
		t.Errorf("name = %q, want %q", detail.Name, "My Savings")
	}
	if detail.Provider != "plaid" {
		t.Errorf("provider = %q, want %q", detail.Provider, "plaid")
	}
	if detail.UserName != "Alice" {
		t.Errorf("user_name = %q, want %q", detail.UserName, "Alice")
	}
}

func TestGetAccountDetail_InvalidUUID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.GetAccountDetail(ctx, "bad-uuid")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetAccountDetail_NonExistent(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.GetAccountDetail(ctx, "00000000-0000-0000-0000-000000000099")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ===================== CSV Import Service Tests =====================

func TestImportCSV_BasicSuccess(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	userID := formatUUID(user.ID)

	result, err := svc.ImportCSV(ctx, service.CSVImportParams{
		UserID:      userID,
		AccountName: "Bank CSV",
		ColumnMapping: map[string]int{
			"date":        0,
			"amount":      1,
			"description": 2,
		},
		Rows: [][]string{
			{"2025-01-15", "25.50", "Coffee Shop"},
			{"2025-01-16", "-100.00", "Payroll"},
			{"2025-01-17", "12.99", "Lunch"},
		},
		DateFormat: "2006-01-02",
	})
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if result.TotalRows != 3 {
		t.Errorf("total_rows = %d, want 3", result.TotalRows)
	}
	if result.NewCount != 3 {
		t.Errorf("new_count = %d, want 3", result.NewCount)
	}
	if result.SkippedCount != 0 {
		t.Errorf("skipped_count = %d, want 0", result.SkippedCount)
	}
	if result.ConnectionID == "" {
		t.Error("connection_id should not be empty")
	}
	if result.AccountID == "" {
		t.Error("account_id should not be empty")
	}
}

func TestImportCSV_SkipsInvalidRows(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	userID := formatUUID(user.ID)

	result, err := svc.ImportCSV(ctx, service.CSVImportParams{
		UserID:      userID,
		AccountName: "Test CSV",
		ColumnMapping: map[string]int{
			"date":        0,
			"amount":      1,
			"description": 2,
		},
		Rows: [][]string{
			{"2025-01-15", "25.50", "Valid Row"},
			{"invalid-date", "10.00", "Bad Date"},
			{"2025-01-16", "not-a-number", "Bad Amount"},
			{"2025-01-17", "5.00", ""},   // empty description
			{"2025-01-18"},               // not enough columns
		},
		DateFormat: "2006-01-02",
	})
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if result.NewCount != 1 {
		t.Errorf("new_count = %d, want 1", result.NewCount)
	}
	if result.SkippedCount != 4 {
		t.Errorf("skipped_count = %d, want 4", result.SkippedCount)
	}
	if len(result.SkipReasons) != 4 {
		t.Errorf("skip_reasons length = %d, want 4", len(result.SkipReasons))
	}
}

func TestImportCSV_Deduplication(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	userID := formatUUID(user.ID)

	params := service.CSVImportParams{
		UserID:      userID,
		AccountName: "Dedup CSV",
		ColumnMapping: map[string]int{
			"date":        0,
			"amount":      1,
			"description": 2,
		},
		Rows: [][]string{
			{"2025-01-15", "25.50", "Coffee Shop"},
		},
		DateFormat: "2006-01-02",
	}

	// First import
	result1, err := svc.ImportCSV(ctx, params)
	if err != nil {
		t.Fatalf("ImportCSV first: %v", err)
	}
	if result1.NewCount != 1 {
		t.Errorf("first import new_count = %d, want 1", result1.NewCount)
	}

	// Re-import same data to same connection — verifies no error and no new row in DB.
	// Note: new_count vs updated_count detection relies on timestamp comparison
	// (updated_at - created_at < 1 second), which is timing-dependent. The important
	// invariant is that total processed = new + updated (no skips, no errors).
	params.ConnectionID = result1.ConnectionID
	result2, err := svc.ImportCSV(ctx, params)
	if err != nil {
		t.Fatalf("ImportCSV second: %v", err)
	}
	if result2.SkippedCount != 0 {
		t.Errorf("second import skipped_count = %d, want 0", result2.SkippedCount)
	}
	// Total processed should be 1 (either new or updated depending on timing)
	if result2.NewCount+result2.UpdatedCount != 1 {
		t.Errorf("second import new+updated = %d, want 1", result2.NewCount+result2.UpdatedCount)
	}
}

func TestImportCSV_InvalidUserID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.ImportCSV(ctx, service.CSVImportParams{
		UserID:      "bad-uuid",
		AccountName: "Test",
		ColumnMapping: map[string]int{
			"date":        0,
			"amount":      1,
			"description": 2,
		},
		Rows: [][]string{{"2025-01-15", "10.00", "Test"}},
	})
	if err == nil {
		t.Fatal("expected error for invalid user ID")
	}
}

func TestImportCSV_ReimportNonCSVConnection(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "plaid_conn")

	connID := formatUUID(conn.ID)
	_, err := svc.ImportCSV(ctx, service.CSVImportParams{
		UserID:       formatUUID(user.ID),
		ConnectionID: connID,
		AccountName:  "Test",
		ColumnMapping: map[string]int{
			"date":        0,
			"amount":      1,
			"description": 2,
		},
		Rows: [][]string{{"2025-01-15", "10.00", "Test"}},
	})
	if err == nil {
		t.Fatal("expected error when re-importing to non-CSV connection")
	}
}

func TestImportCSV_DefaultAccountName(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	userID := formatUUID(user.ID)

	result, err := svc.ImportCSV(ctx, service.CSVImportParams{
		UserID:      userID,
		AccountName: "", // should default to "CSV Import"
		ColumnMapping: map[string]int{
			"date":        0,
			"amount":      1,
			"description": 2,
		},
		Rows: [][]string{
			{"2025-01-15", "10.00", "Test Row"},
		},
		DateFormat: "2006-01-02",
	})
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}

	// Verify the account name defaults to "CSV Import"
	acct, err := svc.GetAccount(ctx, result.AccountID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if acct.Name != "CSV Import" {
		t.Errorf("account name = %q, want %q", acct.Name, "CSV Import")
	}
}

func TestImportCSV_DebitCreditColumns(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	userID := formatUUID(user.ID)

	result, err := svc.ImportCSV(ctx, service.CSVImportParams{
		UserID:      userID,
		AccountName: "Debit Credit CSV",
		ColumnMapping: map[string]int{
			"date":        0,
			"debit":       1,
			"credit":      2,
			"description": 3,
		},
		HasDebitCredit: true,
		Rows: [][]string{
			{"2025-01-15", "50.00", "", "Purchase"},
			{"2025-01-16", "", "100.00", "Payment"},
		},
		DateFormat: "2006-01-02",
	})
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if result.NewCount != 2 {
		t.Errorf("new_count = %d, want 2", result.NewCount)
	}
	if result.SkippedCount != 0 {
		t.Errorf("skipped_count = %d, want 0", result.SkippedCount)
	}
}

func TestImportCSV_WithCategoryAndMerchant(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	userID := formatUUID(user.ID)

	result, err := svc.ImportCSV(ctx, service.CSVImportParams{
		UserID:      userID,
		AccountName: "Rich CSV",
		ColumnMapping: map[string]int{
			"date":          0,
			"amount":        1,
			"description":   2,
			"category":      3,
			"merchant_name": 4,
		},
		Rows: [][]string{
			{"2025-01-15", "25.50", "Coffee", "food_and_drink", "Starbucks"},
		},
		DateFormat: "2006-01-02",
	})
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if result.NewCount != 1 {
		t.Errorf("new_count = %d, want 1", result.NewCount)
	}
}

func TestImportCSV_AllRowsSkipped_ErrorStatus(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	userID := formatUUID(user.ID)

	result, err := svc.ImportCSV(ctx, service.CSVImportParams{
		UserID:      userID,
		AccountName: "Bad CSV",
		ColumnMapping: map[string]int{
			"date":        0,
			"amount":      1,
			"description": 2,
		},
		Rows: [][]string{
			{"bad-date", "10.00", "Test"},
			{"also-bad", "20.00", "Test2"},
		},
		DateFormat: "2006-01-02",
	})
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if result.SkippedCount != 2 {
		t.Errorf("skipped = %d, want 2", result.SkippedCount)
	}
	if result.NewCount != 0 {
		t.Errorf("new = %d, want 0", result.NewCount)
	}

	// Verify sync log was marked as error
	connID, _ := parseUUIDForCSVTest(result.ConnectionID)
	var status string
	err = pool.QueryRow(ctx, "SELECT status FROM sync_logs WHERE connection_id = $1", connID).Scan(&status)
	if err != nil {
		t.Fatalf("query sync log: %v", err)
	}
	if status != "error" {
		t.Errorf("sync log status = %q, want %q", status, "error")
	}
}

// ===================== Overview Stats Tests =====================

func TestGetOverviewStats_WithConnections(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, alice.ID, "overview_conn_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_ov_1", "Checking")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_ov_2", "Savings")

	stats, err := svc.GetOverviewStats(ctx)
	if err != nil {
		t.Fatalf("GetOverviewStats: %v", err)
	}
	if stats.UserCount != 1 {
		t.Errorf("user_count = %d, want 1", stats.UserCount)
	}
	if stats.ConnectionCount != 1 {
		t.Errorf("connection_count = %d, want 1", stats.ConnectionCount)
	}
	if stats.AccountCount != 2 {
		t.Errorf("account_count = %d, want 2", stats.AccountCount)
	}
	if len(stats.Users) != 1 || stats.Users[0].Name != "Alice" {
		t.Errorf("users list unexpected: %+v", stats.Users)
	}
	if len(stats.Connections) != 1 {
		t.Errorf("connections count = %d, want 1", len(stats.Connections))
	}
	if stats.Connections[0].AccountCount != 2 {
		t.Errorf("connection account_count = %d, want 2", stats.Connections[0].AccountCount)
	}
}

// ===================== Sync Log Retention Tests =====================

func TestCountSyncLogs(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Count should be 0 with no logs.
	count, err := svc.CountSyncLogs(ctx)
	if err != nil {
		t.Fatalf("CountSyncLogs failed: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	// Create a sync log.
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "conn_count_test")
	now := time.Now()
	_, err = queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true},
		CompletedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateSyncLog failed: %v", err)
	}

	count, err = svc.CountSyncLogs(ctx)
	if err != nil {
		t.Fatalf("CountSyncLogs failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestCleanupSyncLogs_DeletesOldLogs(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "conn_cleanup_test")

	// Create an old log (100 days ago).
	oldTime := time.Now().AddDate(0, 0, -100)
	_, err := queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: oldTime, Valid: true},
		CompletedAt:  pgtype.Timestamptz{Time: oldTime.Add(time.Second), Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateSyncLog (old) failed: %v", err)
	}

	// Create a recent log (1 day ago).
	recentTime := time.Now().AddDate(0, 0, -1)
	_, err = queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusSuccess,
		StartedAt:    pgtype.Timestamptz{Time: recentTime, Valid: true},
		CompletedAt:  pgtype.Timestamptz{Time: recentTime.Add(time.Second), Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateSyncLog (recent) failed: %v", err)
	}

	// Cleanup with 90-day retention should delete the old log.
	deleted, err := svc.CleanupSyncLogs(ctx, 90)
	if err != nil {
		t.Fatalf("CleanupSyncLogs failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	// Should have 1 log remaining.
	count, err := svc.CountSyncLogs(ctx)
	if err != nil {
		t.Fatalf("CountSyncLogs failed: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining count = %d, want 1", count)
	}
}

func TestCleanupSyncLogs_SkipsInProgress(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "conn_cleanup_inprog")

	// Create an old in_progress log (should not be deleted).
	oldTime := time.Now().AddDate(0, 0, -100)
	_, err := queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: conn.ID,
		Trigger:      db.SyncTriggerCron,
		Status:       db.SyncStatusInProgress,
		StartedAt:    pgtype.Timestamptz{Time: oldTime, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateSyncLog (in_progress) failed: %v", err)
	}

	deleted, err := svc.CleanupSyncLogs(ctx, 90)
	if err != nil {
		t.Fatalf("CleanupSyncLogs failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (in_progress should be skipped)", deleted)
	}
}

func TestCleanupSyncLogs_InvalidRetention(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.CleanupSyncLogs(ctx, 0)
	if err == nil {
		t.Fatal("expected error for retention_days=0, got nil")
	}

	_, err = svc.CleanupSyncLogs(ctx, -5)
	if err == nil {
		t.Fatal("expected error for negative retention_days, got nil")
	}
}

func TestGetSyncLogRetentionDays_Default(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	days, err := svc.GetSyncLogRetentionDays(ctx)
	if err != nil {
		t.Fatalf("GetSyncLogRetentionDays failed: %v", err)
	}
	if days != 90 {
		t.Errorf("retention_days = %d, want 90 (default)", days)
	}
}

func TestGetSyncLogRetentionDays_Configured(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Set a custom retention period.
	err := queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "sync_log_retention_days",
		Value: pgtype.Text{String: "30", Valid: true},
	})
	if err != nil {
		t.Fatalf("SetAppConfig failed: %v", err)
	}

	days, err := svc.GetSyncLogRetentionDays(ctx)
	if err != nil {
		t.Fatalf("GetSyncLogRetentionDays failed: %v", err)
	}
	if days != 30 {
		t.Errorf("retention_days = %d, want 30", days)
	}
}

// ===================== Helpers =====================

func parseUUIDForCSVTest(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	err := u.Scan(s)
	return u, err
}
