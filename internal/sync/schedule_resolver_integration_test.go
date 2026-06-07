//go:build integration && !lite

package sync

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/testutil"
)

// TestLoadScheduleResolver verifies the DB → resolver mapping: applies_to_all
// schedules reach every connection, targeted schedules reach only their
// connection, and disabled schedules are excluded.
func TestLoadScheduleResolver(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	c1 := testutil.MustCreateConnection(t, queries, user.ID, "item_a")
	c2 := testutil.MustCreateConnection(t, queries, user.ID, "item_b")

	// An applies_to_all nightly schedule.
	all, err := queries.CreateSyncSchedule(ctx, db.CreateSyncScheduleParams{
		Name: "All nightly", Cron: "0 3 * * *", Preset: pgconv.Text("nightly"),
		AppliesToAll: true, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create all schedule: %v", err)
	}
	// A targeted hourly schedule on c1 only.
	targeted, err := queries.CreateSyncSchedule(ctx, db.CreateSyncScheduleParams{
		Name: "C1 hourly", Cron: "0 * * * *", Preset: pgconv.Text("hourly"),
		AppliesToAll: false, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create targeted schedule: %v", err)
	}
	if err := queries.AddScheduleConnection(ctx, db.AddScheduleConnectionParams{
		ScheduleID: targeted.ID, ConnectionID: c1.ID,
	}); err != nil {
		t.Fatalf("add schedule connection: %v", err)
	}
	// A disabled schedule that must NOT appear.
	if _, err := queries.CreateSyncSchedule(ctx, db.CreateSyncScheduleParams{
		Name: "Disabled", Cron: "*/15 * * * *", Preset: pgconv.Text("every_15m"),
		AppliesToAll: true, Enabled: false,
	}); err != nil {
		t.Fatalf("create disabled schedule: %v", err)
	}
	_ = all

	s := &Scheduler{queries: queries, logger: slog.Default(), syncTimeout: time.Minute}
	resolver, err := s.loadScheduleResolver(ctx, 720)
	if err != nil {
		t.Fatalf("loadScheduleResolver: %v", err)
	}

	// c1 gets the all-schedule + its targeted one = 2.
	if got := len(resolver.forConnection(c1.ID.Bytes)); got != 2 {
		t.Errorf("c1 expected 2 schedules, got %d", got)
	}
	// c2 gets only the all-schedule = 1 (disabled one excluded).
	if got := len(resolver.forConnection(c2.ID.Bytes)); got != 1 {
		t.Errorf("c2 expected 1 schedule, got %d", got)
	}
	_ = pool
}
