//go:build !headless && !lite

package admin

import (
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRelativeTimeUntil(t *testing.T) {
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		target time.Time
		want   string
	}{
		{"in the past", now.Add(-1 * time.Minute), "now"},
		{"exactly now", now, "now"},
		{"less than 1 minute", now.Add(30 * time.Second), "in <1m"},
		{"5 minutes", now.Add(5 * time.Minute), "in 5m"},
		{"1 hour", now.Add(1 * time.Hour), "in 1h"},
		{"1 hour 30 minutes", now.Add(90 * time.Minute), "in 1h 30m"},
		{"2 hours", now.Add(2 * time.Hour), "in 2h"},
		{"2 hours 15 minutes", now.Add(135 * time.Minute), "in 2h 15m"},
		{"1 day", now.Add(24 * time.Hour), "in 1d"},
		{"1 day 3 hours", now.Add(27 * time.Hour), "in 1d 3h"},
		{"3 days", now.Add(72 * time.Hour), "in 3d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relativeTimeUntil(tt.target, now)
			if got != tt.want {
				t.Errorf("relativeTimeUntil(%v, %v) = %q, want %q", tt.target, now, got, tt.want)
			}
		})
	}
}

func TestComputeNextSync(t *testing.T) {
	// 2026-03-26 09:00 UTC. A nightly (03:00) + hourly schedule give clear
	// fire boundaries to assert against.
	now := time.Date(2026, 3, 26, 9, 0, 0, 0, time.UTC)
	nightly := []service.ScheduleRef{{Name: "Nightly", Cron: "0 3 * * *"}}
	hourly := []service.ScheduleRef{{Name: "Hourly", Cron: "0 * * * *"}}

	tsAt := func(tt time.Time) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: tt, Valid: true}
	}

	t.Run("disconnected connection", func(t *testing.T) {
		info := computeNextSync(syncScheduleParams{
			Status:   db.ConnectionStatusDisconnected,
			Provider: db.ProviderTypePlaid,
		}, nightly, now)
		if !info.IsDisconnected || info.Label != "No schedule" {
			t.Errorf("expected disconnected/No schedule, got %+v", info)
		}
	})

	t.Run("CSV provider", func(t *testing.T) {
		info := computeNextSync(syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypeCsv,
		}, nightly, now)
		if !info.IsDisconnected {
			t.Error("expected IsDisconnected=true for CSV")
		}
	})

	t.Run("paused connection", func(t *testing.T) {
		info := computeNextSync(syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypePlaid,
			Paused:   true,
		}, nightly, now)
		if !info.IsPaused || info.Label != "Paused" {
			t.Errorf("expected paused, got %+v", info)
		}
	})

	t.Run("no schedules covers connection", func(t *testing.T) {
		info := computeNextSync(syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypePlaid,
		}, nil, now)
		if info.Label != "No schedule" || info.IsOverdue {
			t.Errorf("expected 'No schedule' with no schedules, got %+v", info)
		}
	})

	t.Run("never synced is overdue", func(t *testing.T) {
		info := computeNextSync(syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypePlaid,
		}, nightly, now)
		if !info.IsOverdue || info.Label != "Pending first sync" {
			t.Errorf("expected pending first sync, got %+v", info)
		}
		if len(info.ScheduleNames) != 1 || info.ScheduleNames[0] != "Nightly" {
			t.Errorf("expected ScheduleNames=[Nightly], got %v", info.ScheduleNames)
		}
	})

	t.Run("synced after last fire - next in future", func(t *testing.T) {
		// Last synced 04:00 (after the 03:00 nightly fire) → next fire is
		// tomorrow 03:00, 18h away.
		info := computeNextSync(syncScheduleParams{
			Status:       db.ConnectionStatusActive,
			Provider:     db.ProviderTypePlaid,
			LastSyncedAt: tsAt(time.Date(2026, 3, 26, 4, 0, 0, 0, time.UTC)),
		}, nightly, now)
		if info.IsOverdue {
			t.Errorf("expected not overdue, got %+v", info)
		}
		if info.Label != "in 18h" {
			t.Errorf("expected 'in 18h', got %q", info.Label)
		}
	})

	t.Run("scheduled fire passed since last sync - due now", func(t *testing.T) {
		// Last synced 02:00 (before the 03:00 nightly fire), now is 09:00 →
		// the 03:00 fire was missed → due.
		info := computeNextSync(syncScheduleParams{
			Status:       db.ConnectionStatusActive,
			Provider:     db.ProviderTypePlaid,
			LastSyncedAt: tsAt(time.Date(2026, 3, 26, 2, 0, 0, 0, time.UTC)),
		}, nightly, now)
		if !info.IsOverdue || info.Label != "Due now" {
			t.Errorf("expected due now, got %+v", info)
		}
	})

	t.Run("hourly - next fire within the hour", func(t *testing.T) {
		// Synced at 09:00 exactly; next hourly fire is 10:00, 1h away.
		info := computeNextSync(syncScheduleParams{
			Status:       db.ConnectionStatusActive,
			Provider:     db.ProviderTypeTeller,
			LastSyncedAt: tsAt(now),
		}, hourly, now)
		if info.IsOverdue {
			t.Errorf("expected not overdue, got %+v", info)
		}
		if info.Label != "in 1h" {
			t.Errorf("expected 'in 1h', got %q", info.Label)
		}
	})

	t.Run("union of multiple schedules picks earliest", func(t *testing.T) {
		// Synced at 09:00 with both nightly (next 03:00) and hourly (next
		// 10:00) → earliest next fire is 10:00, 1h away.
		info := computeNextSync(syncScheduleParams{
			Status:       db.ConnectionStatusActive,
			Provider:     db.ProviderTypePlaid,
			LastSyncedAt: tsAt(now),
		}, append(append([]service.ScheduleRef{}, nightly...), hourly...), now)
		if info.Label != "in 1h" {
			t.Errorf("expected earliest 'in 1h', got %q", info.Label)
		}
		if len(info.ScheduleNames) != 2 {
			t.Errorf("expected 2 schedule names, got %v", info.ScheduleNames)
		}
	})
}
