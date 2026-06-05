//go:build !lite

package sync

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func mustCron(t *testing.T, expr string) cron.Schedule {
	t.Helper()
	s, err := cron.ParseStandard(expr)
	if err != nil {
		t.Fatalf("parse %q: %v", expr, err)
	}
	return s
}

func TestScheduleDue(t *testing.T) {
	// Nightly at 03:00.
	nightly := mustCron(t, "0 3 * * *")
	// Reference "now" = 2026-06-04 09:00 local.
	now := time.Date(2026, 6, 4, 9, 0, 0, 0, time.Local)

	tests := []struct {
		name       string
		schedules  []cron.Schedule
		lastSynced time.Time
		want       bool
	}{
		{
			name:      "no schedules is never due",
			schedules: nil,
			want:      false,
		},
		{
			name:       "never synced is due when a schedule exists",
			schedules:  []cron.Schedule{nightly},
			lastSynced: time.Time{},
			want:       true,
		},
		{
			name:       "synced after last fire is not due",
			schedules:  []cron.Schedule{nightly},
			lastSynced: time.Date(2026, 6, 4, 3, 30, 0, 0, time.Local), // after 03:00 fire
			want:       false,
		},
		{
			name:       "synced before last fire is due",
			schedules:  []cron.Schedule{nightly},
			lastSynced: time.Date(2026, 6, 3, 23, 0, 0, 0, time.Local), // before the 03:00 fire
			want:       true,
		},
		{
			name: "union: due if ANY schedule fired since last sync",
			schedules: []cron.Schedule{
				mustCron(t, "0 3 * * *"), // last fired 03:00 — before lastSynced
				mustCron(t, "0 8 * * *"), // last fired 08:00 — after lastSynced → due
			},
			lastSynced: time.Date(2026, 6, 4, 7, 0, 0, 0, time.Local),
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scheduleDue(tt.schedules, tt.lastSynced, now, 0); got != tt.want {
				t.Errorf("scheduleDue = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScheduleDueNoDrift(t *testing.T) {
	// A 12h schedule fires at 00:00 and 12:00 regardless of when the last sync
	// landed — the wall-clock anchor, not last_synced + interval.
	twelveH := mustCron(t, "0 */12 * * *")
	// Last synced at an odd offset (02:47).
	lastSynced := time.Date(2026, 6, 4, 2, 47, 0, 0, time.Local)

	// At 11:59 the next fire (12:00) hasn't happened yet since 02:47 → not due.
	at1159 := time.Date(2026, 6, 4, 11, 59, 0, 0, time.Local)
	if scheduleDue([]cron.Schedule{twelveH}, lastSynced, at1159, 0) {
		t.Errorf("should not be due at 11:59 (next fire is 12:00)")
	}
	// At 12:01 the 12:00 fire has passed → due, anchored to the wall clock.
	at1201 := time.Date(2026, 6, 4, 12, 1, 0, 0, time.Local)
	if !scheduleDue([]cron.Schedule{twelveH}, lastSynced, at1201, 0) {
		t.Errorf("should be due at 12:01 (12:00 fire passed)")
	}
}

func TestScheduleDueJitterDelays(t *testing.T) {
	nightly := mustCron(t, "0 3 * * *")
	lastSynced := time.Date(2026, 6, 3, 23, 0, 0, 0, time.Local)
	// Exactly at the fire instant with a 5-minute jitter: not yet due.
	at0300 := time.Date(2026, 6, 4, 3, 0, 0, 0, time.Local)
	jitter := 5 * time.Minute
	if scheduleDue([]cron.Schedule{nightly}, lastSynced, at0300, jitter) {
		t.Errorf("should not be due at 03:00 with 5m jitter")
	}
	// Six minutes later, past the jitter window: due.
	at0306 := time.Date(2026, 6, 4, 3, 6, 0, 0, time.Local)
	if !scheduleDue([]cron.Schedule{nightly}, lastSynced, at0306, jitter) {
		t.Errorf("should be due at 03:06 with 5m jitter")
	}
}

func TestConnectionJitterDeterministic(t *testing.T) {
	id := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	a := connectionJitter(id, jitterWindow)
	b := connectionJitter(id, jitterWindow)
	if a != b {
		t.Errorf("jitter not deterministic: %v != %v", a, b)
	}
	if a < 0 || a >= jitterWindow {
		t.Errorf("jitter %v out of range [0, %v)", a, jitterWindow)
	}
	// Zero window → zero jitter.
	if connectionJitter(id, 0) != 0 {
		t.Errorf("zero window should give zero jitter")
	}
}

func TestScheduleNextRun(t *testing.T) {
	now := time.Date(2026, 6, 4, 9, 0, 0, 0, time.Local)
	schedules := []cron.Schedule{
		mustCron(t, "0 18 * * *"), // next fire 18:00
		mustCron(t, "0 12 * * *"), // next fire 12:00 — earliest
	}
	next := scheduleNextRun(schedules, now, 0)
	want := time.Date(2026, 6, 4, 12, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Errorf("scheduleNextRun = %v, want %v", next, want)
	}
	if !scheduleNextRun(nil, now, 0).IsZero() {
		t.Errorf("no schedules should give zero next run")
	}
}

func TestBackoffSuppressed(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.Local)
	// 1 failure → backoffInterval(15,1)=30m suppression.
	lastErr := now.Add(-10 * time.Minute)
	if !backoffSuppressed(1, lastErr, now) {
		t.Errorf("should be suppressed 10m after error with 1 failure (30m window)")
	}
	lastErrOld := now.Add(-31 * time.Minute)
	if backoffSuppressed(1, lastErrOld, now) {
		t.Errorf("should not be suppressed 31m after error with 1 failure (30m window)")
	}
	// No failures → never suppressed.
	if backoffSuppressed(0, lastErr, now) {
		t.Errorf("zero failures should never suppress")
	}
}
