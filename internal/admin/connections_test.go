package admin

import (
	"testing"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSyncBackoffInterval(t *testing.T) {
	tests := []struct {
		name                string
		baseMinutes         int
		consecutiveFailures int32
		want                int
	}{
		{"no failures", 720, 0, 720},
		{"1 failure doubles", 720, 1, 1440},
		{"2 failures 4x", 720, 2, 2880},
		{"3 failures 8x", 60, 3, 480},
		{"4 failures 16x", 60, 4, 960},
		{"5 failures capped at 16x", 60, 5, 960},
		{"10 failures capped at 16x", 60, 10, 960},
		{"negative failures treated as zero", 720, -1, 720},
		{"15 min base no failures", 15, 0, 15},
		{"15 min base 2 failures", 15, 2, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := syncBackoffInterval(tt.baseMinutes, tt.consecutiveFailures)
			if got != tt.want {
				t.Errorf("syncBackoffInterval(%d, %d) = %d, want %d", tt.baseMinutes, tt.consecutiveFailures, got, tt.want)
			}
		})
	}
}

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
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	globalInterval := 720 // 12 hours

	t.Run("disconnected connection", func(t *testing.T) {
		p := syncScheduleParams{
			Status:   db.ConnectionStatusDisconnected,
			Provider: db.ProviderTypePlaid,
		}
		info := computeNextSync(p, globalInterval, now)
		if !info.IsDisconnected {
			t.Error("expected IsDisconnected=true")
		}
		if info.Label != "No schedule" {
			t.Errorf("expected label 'No schedule', got %q", info.Label)
		}
	})

	t.Run("CSV provider", func(t *testing.T) {
		p := syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypeCsv,
		}
		info := computeNextSync(p, globalInterval, now)
		if !info.IsDisconnected {
			t.Error("expected IsDisconnected=true for CSV")
		}
	})

	t.Run("paused connection", func(t *testing.T) {
		p := syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypePlaid,
			Paused:   true,
		}
		info := computeNextSync(p, globalInterval, now)
		if !info.IsPaused {
			t.Error("expected IsPaused=true")
		}
		if info.Label != "Paused" {
			t.Errorf("expected label 'Paused', got %q", info.Label)
		}
	})

	t.Run("never synced", func(t *testing.T) {
		p := syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypePlaid,
		}
		info := computeNextSync(p, globalInterval, now)
		if !info.IsOverdue {
			t.Error("expected IsOverdue=true for never-synced connection")
		}
		if info.Label != "Pending first sync" {
			t.Errorf("expected label 'Pending first sync', got %q", info.Label)
		}
	})

	t.Run("recently synced - next sync in future", func(t *testing.T) {
		lastSynced := now.Add(-6 * time.Hour) // 6h ago, interval is 12h => 6h remaining
		p := syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypePlaid,
			LastSyncedAt: pgtype.Timestamptz{
				Time:  lastSynced,
				Valid: true,
			},
		}
		info := computeNextSync(p, globalInterval, now)
		if info.IsOverdue {
			t.Error("expected IsOverdue=false")
		}
		if info.Label != "in 6h" {
			t.Errorf("expected label 'in 6h', got %q", info.Label)
		}
		if info.EffectiveIntervalMinutes != 720 {
			t.Errorf("expected EffectiveIntervalMinutes=720, got %d", info.EffectiveIntervalMinutes)
		}
	})

	t.Run("overdue - last synced long ago", func(t *testing.T) {
		lastSynced := now.Add(-24 * time.Hour) // 24h ago, interval is 12h => overdue
		p := syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypePlaid,
			LastSyncedAt: pgtype.Timestamptz{
				Time:  lastSynced,
				Valid: true,
			},
		}
		info := computeNextSync(p, globalInterval, now)
		if !info.IsOverdue {
			t.Error("expected IsOverdue=true")
		}
		if info.Label != "Due now" {
			t.Errorf("expected label 'Due now', got %q", info.Label)
		}
	})

	t.Run("per-connection override interval", func(t *testing.T) {
		lastSynced := now.Add(-10 * time.Minute) // 10m ago, override is 30m => 20m remaining
		p := syncScheduleParams{
			Status:   db.ConnectionStatusActive,
			Provider: db.ProviderTypeTeller,
			LastSyncedAt: pgtype.Timestamptz{
				Time:  lastSynced,
				Valid: true,
			},
			SyncIntervalOverrideMinutes: pgtype.Int4{Int32: 30, Valid: true},
		}
		info := computeNextSync(p, globalInterval, now)
		if info.IsOverdue {
			t.Error("expected IsOverdue=false")
		}
		if info.EffectiveIntervalMinutes != 30 {
			t.Errorf("expected EffectiveIntervalMinutes=30, got %d", info.EffectiveIntervalMinutes)
		}
		if info.Label != "in 20m" {
			t.Errorf("expected label 'in 20m', got %q", info.Label)
		}
	})

	t.Run("backoff doubles interval on failures", func(t *testing.T) {
		lastSynced := now.Add(-30 * time.Minute) // 30m ago, base=60m, 1 failure => effective=120m, remaining=90m
		p := syncScheduleParams{
			Status:              db.ConnectionStatusActive,
			Provider:            db.ProviderTypePlaid,
			ConsecutiveFailures: 1,
			LastSyncedAt: pgtype.Timestamptz{
				Time:  lastSynced,
				Valid: true,
			},
			SyncIntervalOverrideMinutes: pgtype.Int4{Int32: 60, Valid: true},
		}
		info := computeNextSync(p, globalInterval, now)
		if info.IsOverdue {
			t.Error("expected IsOverdue=false")
		}
		if info.EffectiveIntervalMinutes != 120 {
			t.Errorf("expected EffectiveIntervalMinutes=120, got %d", info.EffectiveIntervalMinutes)
		}
		if info.Label != "in 1h 30m" {
			t.Errorf("expected label 'in 1h 30m', got %q", info.Label)
		}
	})
}
