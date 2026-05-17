//go:build !lite

package service

import (
	"testing"
	"time"
)

func TestIsWithinQuietHours(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	at := func(h, m int) time.Time {
		return time.Date(2026, 5, 17, h, m, 0, 0, time.Local)
	}
	tests := []struct {
		name      string
		now       time.Time
		start     *string
		end       *string
		want      bool
	}{
		{"nil bounds = never quiet", at(3, 0), nil, nil, false},
		{"empty bounds = never quiet", at(3, 0), strPtr(""), strPtr(""), false},
		{"unparseable = never quiet", at(3, 0), strPtr("bogus"), strPtr("07:00"), false},
		{"equal bounds = never quiet", at(3, 0), strPtr("07:00"), strPtr("07:00"), false},

		// Same-day window 09:00–17:00 (work hours).
		{"same-day before window", at(8, 0), strPtr("09:00"), strPtr("17:00"), false},
		{"same-day at start (inclusive)", at(9, 0), strPtr("09:00"), strPtr("17:00"), true},
		{"same-day middle", at(13, 30), strPtr("09:00"), strPtr("17:00"), true},
		{"same-day at end (exclusive)", at(17, 0), strPtr("09:00"), strPtr("17:00"), false},
		{"same-day after window", at(17, 30), strPtr("09:00"), strPtr("17:00"), false},

		// Wrap-midnight window 22:00–07:00 (overnight quiet).
		{"wraps at evening start (inclusive)", at(22, 0), strPtr("22:00"), strPtr("07:00"), true},
		{"wraps middle of night", at(2, 30), strPtr("22:00"), strPtr("07:00"), true},
		{"wraps at morning end (exclusive)", at(7, 0), strPtr("22:00"), strPtr("07:00"), false},
		{"wraps during day", at(13, 0), strPtr("22:00"), strPtr("07:00"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsWithinQuietHours(tc.now, tc.start, tc.end); got != tc.want {
				t.Errorf("got %v, want %v (now=%s start=%v end=%v)", got, tc.want,
					tc.now.Format("15:04"), tc.start, tc.end)
			}
		})
	}
}

func TestComputeNextFire(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	at := func(year int, month time.Month, day, h, m int) time.Time {
		return time.Date(year, month, day, h, m, 0, 0, time.Local)
	}

	t.Run("nil schedule returns nil", func(t *testing.T) {
		def := &AgentDefinitionResponse{ScheduleCron: nil}
		if got := ComputeNextFire(def, at(2026, 5, 17, 12, 0)); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("empty schedule returns nil", func(t *testing.T) {
		def := &AgentDefinitionResponse{ScheduleCron: strPtr("")}
		if got := ComputeNextFire(def, at(2026, 5, 17, 12, 0)); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("malformed cron returns nil", func(t *testing.T) {
		def := &AgentDefinitionResponse{ScheduleCron: strPtr("not a cron")}
		if got := ComputeNextFire(def, at(2026, 5, 17, 12, 0)); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("hourly cron without quiet hours fires next hour", func(t *testing.T) {
		def := &AgentDefinitionResponse{ScheduleCron: strPtr("0 * * * *")}
		now := at(2026, 5, 17, 12, 30)
		got := ComputeNextFire(def, now)
		if got == nil {
			t.Fatal("got nil")
		}
		want := at(2026, 5, 17, 13, 0)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("daily 9am cron lands at 9am", func(t *testing.T) {
		def := &AgentDefinitionResponse{ScheduleCron: strPtr("0 9 * * *")}
		now := at(2026, 5, 17, 7, 0)
		got := ComputeNextFire(def, now)
		if got == nil {
			t.Fatal("got nil")
		}
		want := at(2026, 5, 17, 9, 0)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("hourly cron skips past quiet hours 22-07", func(t *testing.T) {
		// Hourly fires at :00; quiet 22:00-07:00. From 23:30, the next
		// schedule.Next is 00:00 (in quiet) → jump to 07:00 → cron Next
		// from there yields 07:00 (exclusive end means 07:00 is OK).
		def := &AgentDefinitionResponse{
			ScheduleCron:    strPtr("0 * * * *"),
			QuietHoursStart: strPtr("22:00"),
			QuietHoursEnd:   strPtr("07:00"),
		}
		now := at(2026, 5, 17, 23, 30)
		got := ComputeNextFire(def, now)
		if got == nil {
			t.Fatal("got nil")
		}
		want := at(2026, 5, 18, 7, 0)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("hourly cron outside quiet hours fires normally", func(t *testing.T) {
		def := &AgentDefinitionResponse{
			ScheduleCron:    strPtr("0 * * * *"),
			QuietHoursStart: strPtr("22:00"),
			QuietHoursEnd:   strPtr("07:00"),
		}
		now := at(2026, 5, 17, 10, 30)
		got := ComputeNextFire(def, now)
		if got == nil {
			t.Fatal("got nil")
		}
		want := at(2026, 5, 17, 11, 0)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}
