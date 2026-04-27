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

// TestRelativeAt locks the rendering to a fixed anchor so the bucket
// boundaries are deterministic — Relative()'s table uses time.Now() and
// thus drifts. RelativeAt is the entry point page-level callers use to
// share their now anchor with day-grouping logic; verifying it here means
// the shared anchor produces the strings tests downstream assert on.
func TestRelativeAt(t *testing.T) {
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"just now (zero)", now, "just now"},
		{"just now (30s)", now.Add(-30 * time.Second), "just now"},
		{"1 minute", now.Add(-1 * time.Minute), "1 minute ago"},
		{"15 minutes (midnight boundary)", now.Add(-15 * time.Minute), "15 minutes ago"},
		{"59 minutes", now.Add(-59 * time.Minute), "59 minutes ago"},
		{"1 hour", now.Add(-1 * time.Hour), "1 hour ago"},
		{"23 hours", now.Add(-23 * time.Hour), "23 hours ago"},
		{"1 day", now.Add(-24 * time.Hour), "1 day ago"},
		{"5 days", now.Add(-5 * 24 * time.Hour), "5 days ago"},
		{"future collapses to just now", now.Add(1 * time.Hour), "just now"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativeAt(tc.in, now); got != tc.want {
				t.Errorf("RelativeAt(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// Relative must delegate to RelativeAt with time.Now() — sanity check that
// the two entry points stay consistent at a coarse level.
func TestRelativeDelegatesToRelativeAt(t *testing.T) {
	in := time.Now().Add(-3 * time.Minute)
	if got, want := Relative(in), "3 minutes ago"; got != want {
		t.Errorf("Relative = %q, want %q", got, want)
	}
}

func TestFormatRFC3339(t *testing.T) {
	// Pin the inputs to UTC so the assertions are stable regardless of
	// the runner's local zone (formatter renders in local time).
	utc := time.FixedZone("UTC", 0)
	ts := time.Date(2026, 4, 26, 14, 5, 0, 0, utc)
	rfc := ts.Format(time.RFC3339)
	rfcNano := ts.Format(time.RFC3339Nano)

	want := ts.Local().Format(LayoutDateTime)
	if got := FormatRFC3339(rfc, LayoutDateTime); got != want {
		t.Errorf("FormatRFC3339(rfc) = %q, want %q", got, want)
	}
	if got := FormatRFC3339(rfcNano, LayoutDateTime); got != want {
		t.Errorf("FormatRFC3339(nano) = %q, want %q", got, want)
	}
	if got := FormatRFC3339("", LayoutDateTime); got != "" {
		t.Errorf("FormatRFC3339(empty) = %q, want \"\"", got)
	}
	if got := FormatRFC3339("not a date", LayoutDateTime); got != "not a date" {
		t.Errorf("FormatRFC3339(garbage) = %q, want input passthrough", got)
	}
}

func TestFormatRFC3339Ptr(t *testing.T) {
	utc := time.FixedZone("UTC", 0)
	ts := time.Date(2026, 4, 26, 14, 5, 0, 0, utc)
	rfc := ts.Format(time.RFC3339)
	want := ts.Local().Format(LayoutClock)

	if got := FormatRFC3339Ptr(&rfc, LayoutClock); got != want {
		t.Errorf("FormatRFC3339Ptr(&rfc) = %q, want %q", got, want)
	}
	if got := FormatRFC3339Ptr(nil, LayoutClock); got != "" {
		t.Errorf("FormatRFC3339Ptr(nil) = %q, want \"\"", got)
	}
}

func TestRelativeRFC3339(t *testing.T) {
	now := time.Now()
	rfc := now.Add(-3 * time.Minute).Format(time.RFC3339)
	if got := RelativeRFC3339(rfc); got != "3 minutes ago" {
		t.Errorf("RelativeRFC3339 = %q, want %q", got, "3 minutes ago")
	}
	if got := RelativeRFC3339(""); got != "" {
		t.Errorf("RelativeRFC3339(empty) = %q, want \"\"", got)
	}
	if got := RelativeRFC3339("garbage"); got != "garbage" {
		t.Errorf("RelativeRFC3339(garbage) = %q, want passthrough", got)
	}
}

func TestRelativeRFC3339Ptr(t *testing.T) {
	now := time.Now()
	rfc := now.Add(-1 * time.Hour).Format(time.RFC3339)
	if got := RelativeRFC3339Ptr(&rfc); got != "1 hour ago" {
		t.Errorf("RelativeRFC3339Ptr = %q, want %q", got, "1 hour ago")
	}
	if got := RelativeRFC3339Ptr(nil); got != "" {
		t.Errorf("RelativeRFC3339Ptr(nil) = %q, want \"\"", got)
	}
}
