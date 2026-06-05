//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func TestSyncSchedules_CreateAppliesToAll(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	view, err := svc.CreateSyncSchedule(ctx, service.SyncScheduleInput{
		Name:         "Nightly",
		PresetKey:    "nightly",
		AppliesToAll: true,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.Cron != "0 3 * * *" {
		t.Errorf("expected nightly cron, got %q", view.Cron)
	}
	if !view.AppliesToAll || view.ConnectionCount != 0 {
		t.Errorf("applies_to_all schedule should have 0 explicit connections, got %d", view.ConnectionCount)
	}
	if view.CronHuman != "Nightly (3 AM)" {
		t.Errorf("unexpected human cron %q", view.CronHuman)
	}
}

func TestSyncSchedules_CreateWithConnectionTargets(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	c1 := testutil.MustCreateConnection(t, queries, user.ID, "item_a")
	c2 := testutil.MustCreateConnection(t, queries, user.ID, "item_b")

	view, err := svc.CreateSyncSchedule(ctx, service.SyncScheduleInput{
		Name:          "Checking hourly",
		PresetKey:     "hourly",
		AppliesToAll:  false,
		Enabled:       true,
		ConnectionIDs: []string{c1.ShortID, c2.ShortID},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.ConnectionCount != 2 {
		t.Errorf("expected 2 targeted connections, got %d", view.ConnectionCount)
	}

	shortIDs, err := svc.ListScheduleConnectionShortIDs(ctx, view.ShortID)
	if err != nil {
		t.Fatalf("list conn short ids: %v", err)
	}
	if len(shortIDs) != 2 {
		t.Errorf("expected 2 short ids, got %d", len(shortIDs))
	}
}

func TestSyncSchedules_UpdateReplacesTargets(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	c1 := testutil.MustCreateConnection(t, queries, user.ID, "item_a")
	c2 := testutil.MustCreateConnection(t, queries, user.ID, "item_b")

	view, err := svc.CreateSyncSchedule(ctx, service.SyncScheduleInput{
		Name: "S", PresetKey: "hourly", ConnectionIDs: []string{c1.ShortID},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update to target only c2.
	updated, err := svc.UpdateSyncSchedule(ctx, view.ShortID, service.SyncScheduleInput{
		Name: "S2", PresetKey: "every_4h", ConnectionIDs: []string{c2.ShortID},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "S2" || updated.Cron != "0 */4 * * *" {
		t.Errorf("update didn't persist fields: %+v", updated)
	}
	shortIDs, _ := svc.ListScheduleConnectionShortIDs(ctx, view.ShortID)
	if len(shortIDs) != 1 || shortIDs[0] != c2.ShortID {
		t.Errorf("expected targets replaced with [c2], got %v", shortIDs)
	}
}

func TestSyncSchedules_CustomCronValidation(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.CreateSyncSchedule(ctx, service.SyncScheduleInput{
		Name: "Bad", PresetKey: "custom", Cron: "not a cron",
	}); err == nil {
		t.Fatal("expected error for invalid custom cron")
	}

	view, err := svc.CreateSyncSchedule(ctx, service.SyncScheduleInput{
		Name: "Good", PresetKey: "custom", Cron: "0 6,18 * * *", AppliesToAll: true,
	})
	if err != nil {
		t.Fatalf("valid custom cron should succeed: %v", err)
	}
	if view.Preset != "custom" || view.Cron != "0 6,18 * * *" {
		t.Errorf("unexpected stored custom schedule: %+v", view)
	}
}

func TestSyncSchedules_ToggleAndDelete(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	view, err := svc.CreateSyncSchedule(ctx, service.SyncScheduleInput{
		Name: "Toggle me", PresetKey: "hourly", AppliesToAll: true, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.SetSyncScheduleEnabled(ctx, view.ShortID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	list, _ := svc.ListSyncSchedules(ctx)
	if len(list) != 1 || list[0].Enabled {
		t.Errorf("expected schedule disabled, got %+v", list)
	}

	if err := svc.DeleteSyncSchedule(ctx, view.ShortID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = svc.ListSyncSchedules(ctx)
	if len(list) != 0 {
		t.Errorf("expected 0 schedules after delete, got %d", len(list))
	}
}

func TestSyncSchedules_AssignToManagedSchedule(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	c1 := testutil.MustCreateConnection(t, queries, user.ID, "sf_a")
	c2 := testutil.MustCreateConnection(t, queries, user.ID, "sf_b")

	const name = "SimpleFIN (daily)"
	const cron = "0 6 * * *"

	// First assignment creates the shared schedule and adds c1.
	if err := svc.AssignConnectionToManagedSchedule(ctx, c1.ShortID, name, cron); err != nil {
		t.Fatalf("assign c1: %v", err)
	}
	// Second connection reuses the SAME schedule (no duplicate created).
	if err := svc.AssignConnectionToManagedSchedule(ctx, c2.ShortID, name, cron); err != nil {
		t.Fatalf("assign c2: %v", err)
	}
	// Re-assigning c1 is idempotent.
	if err := svc.AssignConnectionToManagedSchedule(ctx, c1.ShortID, name, cron); err != nil {
		t.Fatalf("reassign c1: %v", err)
	}

	list, _ := svc.ListSyncSchedules(ctx)
	managed := 0
	for _, s := range list {
		if s.Name == name {
			managed++
			if s.Cron != cron || s.AppliesToAll || !s.Enabled {
				t.Errorf("unexpected managed schedule: %+v", s)
			}
			if s.ConnectionCount != 2 {
				t.Errorf("expected 2 connections on managed schedule, got %d", s.ConnectionCount)
			}
		}
	}
	if managed != 1 {
		t.Errorf("expected exactly 1 managed schedule named %q, got %d", name, managed)
	}
}

func TestSyncSchedules_DeleteCascadesTargets(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	c1 := testutil.MustCreateConnection(t, queries, user.ID, "item_a")

	view, err := svc.CreateSyncSchedule(ctx, service.SyncScheduleInput{
		Name: "S", PresetKey: "hourly", ConnectionIDs: []string{c1.ShortID},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.DeleteSyncSchedule(ctx, view.ShortID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// The join rows must be gone (ON DELETE CASCADE).
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM sync_schedule_connections").Scan(&count); err != nil {
		t.Fatalf("count join rows: %v", err)
	}
	if count != 0 {
		t.Errorf("expected join rows cascaded away, got %d", count)
	}
}
