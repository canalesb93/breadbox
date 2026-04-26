package timefmt

import (
	"strings"
	"testing"
	"time"
)

func TestRelative(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"just now (zero)", now, "just now"},
		{"just now (30s)", now.Add(-30 * time.Second), "just now"},
		{"1 minute", now.Add(-1 * time.Minute), "1 minute ago"},
		{"5 minutes", now.Add(-5 * time.Minute), "5 minutes ago"},
		{"59 minutes", now.Add(-59 * time.Minute), "59 minutes ago"},
		{"1 hour", now.Add(-1 * time.Hour), "1 hour ago"},
		{"3 hours", now.Add(-3 * time.Hour), "3 hours ago"},
		{"23 hours", now.Add(-23 * time.Hour), "23 hours ago"},
		{"1 day", now.Add(-24 * time.Hour), "1 day ago"},
		{"5 days", now.Add(-5 * 24 * time.Hour), "5 days ago"},
		{"future collapses to just now", now.Add(1 * time.Hour), "just now"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Relative(tc.in); got != tc.want {
				t.Errorf("Relative(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestRelativeFromRFC3339(t *testing.T) {
	now := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"unparseable falls through", "not-a-timestamp", "not-a-timestamp"},
		{"5 minutes", now, "5 minutes ago"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativeFromRFC3339(tc.in); got != tc.want {
				t.Errorf("RelativeFromRFC3339(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("nil ptr → empty", func(t *testing.T) {
		if got := RelativeFromRFC3339Ptr(nil); got != "" {
			t.Errorf("RelativeFromRFC3339Ptr(nil) = %q, want \"\"", got)
		}
	})
	t.Run("non-nil ptr delegates", func(t *testing.T) {
		s := now
		if got := RelativeFromRFC3339Ptr(&s); got != "5 minutes ago" {
			t.Errorf("RelativeFromRFC3339Ptr(&now) = %q, want %q", got, "5 minutes ago")
		}
	})
}

func TestLocalDateTimeFromRFC3339(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := LocalDateTimeFromRFC3339(""); got != "" {
			t.Errorf("LocalDateTimeFromRFC3339(\"\") = %q, want \"\"", got)
		}
	})
	t.Run("unparseable falls through", func(t *testing.T) {
		if got := LocalDateTimeFromRFC3339("nope"); got != "nope" {
			t.Errorf("LocalDateTimeFromRFC3339(\"nope\") = %q, want \"nope\"", got)
		}
	})
	t.Run("RFC3339 parses", func(t *testing.T) {
		// Don't pin a timezone: just confirm the layout is the admin format
		// (contains comma-year and AM/PM marker).
		got := LocalDateTimeFromRFC3339("2026-04-01T15:04:05Z")
		if !strings.Contains(got, "2026") || (!strings.Contains(got, "AM") && !strings.Contains(got, "PM")) {
			t.Errorf("LocalDateTimeFromRFC3339 = %q, want admin Jan 2, 2006 3:04 PM format", got)
		}
	})
	t.Run("RFC3339Nano parses", func(t *testing.T) {
		got := LocalDateTimeFromRFC3339("2026-04-01T15:04:05.123456789Z")
		if !strings.Contains(got, "2026") {
			t.Errorf("LocalDateTimeFromRFC3339 (nano) = %q, want admin format", got)
		}
	})
	t.Run("nil ptr", func(t *testing.T) {
		if got := LocalDateTimeFromRFC3339Ptr(nil); got != "" {
			t.Errorf("LocalDateTimeFromRFC3339Ptr(nil) = %q, want \"\"", got)
		}
	})
}

func TestLocalClockFromRFC3339(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := LocalClockFromRFC3339(""); got != "" {
			t.Errorf("LocalClockFromRFC3339(\"\") = %q, want \"\"", got)
		}
	})
	t.Run("unparseable falls through", func(t *testing.T) {
		if got := LocalClockFromRFC3339("nope"); got != "nope" {
			t.Errorf("LocalClockFromRFC3339(\"nope\") = %q, want \"nope\"", got)
		}
	})
	t.Run("RFC3339 parses to short clock", func(t *testing.T) {
		got := LocalClockFromRFC3339("2026-04-01T15:04:05Z")
		if !strings.Contains(got, ":") || (!strings.Contains(got, "AM") && !strings.Contains(got, "PM")) {
			t.Errorf("LocalClockFromRFC3339 = %q, want H:MM AM/PM", got)
		}
	})
}
