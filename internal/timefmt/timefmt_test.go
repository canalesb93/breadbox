package timefmt

import (
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

func TestParseRFC3339(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"empty", "", false},
		{"rfc3339", "2026-01-15T14:30:00Z", true},
		{"rfc3339 with offset", "2026-01-15T14:30:00-05:00", true},
		{"rfc3339nano", "2026-01-15T14:30:00.123456789Z", true},
		{"garbage", "not a date", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := ParseRFC3339(tc.in)
			if ok != tc.ok {
				t.Errorf("ParseRFC3339(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			}
		})
	}
}

func TestFormatRFC3339Local(t *testing.T) {
	// Pin the local zone so the test is deterministic regardless of where it runs.
	loc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load UTC: %v", err)
	}
	prev := time.Local
	time.Local = loc
	defer func() { time.Local = prev }()

	cases := []struct {
		name   string
		in     string
		layout string
		want   string
	}{
		{"empty returns empty", "", LayoutDateTimeLocal, ""},
		{"rfc3339 datetime", "2026-01-15T14:30:00Z", LayoutDateTimeLocal, "Jan 15, 2026 2:30 PM"},
		{"rfc3339 clock", "2026-01-15T14:30:00Z", LayoutClockLocal, "2:30 PM"},
		{"rfc3339 short", "2026-01-15T14:30:00Z", LayoutDateShortLocal, "Jan 15, 2:30 PM"},
		{"rfc3339nano", "2026-01-15T14:30:00.123Z", LayoutClockLocal, "2:30 PM"},
		{"unparseable returns input", "not a date", LayoutDateTimeLocal, "not a date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatRFC3339Local(tc.in, tc.layout); got != tc.want {
				t.Errorf("FormatRFC3339Local(%q, %q) = %q, want %q", tc.in, tc.layout, got, tc.want)
			}
		})
	}
}

func TestFormatRFC3339LocalPtr(t *testing.T) {
	loc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load UTC: %v", err)
	}
	prev := time.Local
	time.Local = loc
	defer func() { time.Local = prev }()

	if got := FormatRFC3339LocalPtr(nil, LayoutDateTimeLocal); got != "" {
		t.Errorf("FormatRFC3339LocalPtr(nil) = %q, want empty", got)
	}
	s := "2026-01-15T14:30:00Z"
	if got := FormatRFC3339LocalPtr(&s, LayoutDateTimeLocal); got != "Jan 15, 2026 2:30 PM" {
		t.Errorf("FormatRFC3339LocalPtr(&s) = %q, want Jan 15, 2026 2:30 PM", got)
	}
	empty := ""
	if got := FormatRFC3339LocalPtr(&empty, LayoutDateTimeLocal); got != "" {
		t.Errorf("FormatRFC3339LocalPtr(&\"\") = %q, want empty", got)
	}
}
