package components

import (
	"testing"
	"time"
)

// TestRelativeTimelineTimestamp_AnchorWiring covers the two contracts we
// promise the row primitives:
//
//  1. A zero now anchor falls back to wall-clock time.Now() so non-day-
//     grouped callers keep working without ceremony.
//  2. An explicit now anchor is honoured end-to-end, so day-grouped
//     callers can thread a single request-level now through the helper
//     and the day-bucket label without the two paths disagreeing across
//     midnight (the contract documented in docs/activity-timeline.md and
//     fixed for transaction-detail in PR #890).
func TestRelativeTimelineTimestamp_AnchorWiring(t *testing.T) {
	// Picked deliberately in the middle of a day in UTC so the local
	// timezone of the test runner can't shift it past a day boundary.
	ts := "2026-04-15T12:00:00Z"

	t.Run("zero anchor falls back to time.Now", func(t *testing.T) {
		// "just now" buckets to <1 minute, so passing a timestamp
		// effectively equal to wall-clock now exercises the
		// time.Now() fallback path.
		now := time.Now().UTC().Format(time.RFC3339)
		if got := relativeTimelineTimestamp(now, time.Time{}); got != "just now" {
			t.Fatalf("zero-anchor fallback: relativeTimelineTimestamp(now, zero) = %q, want %q", got, "just now")
		}
	})

	t.Run("explicit anchor is honoured", func(t *testing.T) {
		// Five hours after ts.
		anchor, err := time.Parse(time.RFC3339, "2026-04-15T17:00:00Z")
		if err != nil {
			t.Fatalf("parse anchor: %v", err)
		}
		if got := relativeTimelineTimestamp(ts, anchor); got != "5 hours ago" {
			t.Fatalf("explicit anchor: relativeTimelineTimestamp(ts, +5h) = %q, want %q", got, "5 hours ago")
		}
	})

	t.Run("explicit anchor disagrees with wall clock", func(t *testing.T) {
		// Anchor pinned 2 days after ts, even though wall-clock
		// time.Now() is far in the future. The explicit anchor must
		// win — that's the whole point of the parameter.
		anchor, err := time.Parse(time.RFC3339, "2026-04-17T12:00:00Z")
		if err != nil {
			t.Fatalf("parse anchor: %v", err)
		}
		if got := relativeTimelineTimestamp(ts, anchor); got != "2 days ago" {
			t.Fatalf("explicit anchor: relativeTimelineTimestamp(ts, +2d) = %q, want %q", got, "2 days ago")
		}
	})

	t.Run("empty input yields empty", func(t *testing.T) {
		if got := relativeTimelineTimestamp("", time.Time{}); got != "" {
			t.Fatalf("empty input: got %q, want empty", got)
		}
	})

	t.Run("unparseable input passes through", func(t *testing.T) {
		if got := relativeTimelineTimestamp("not-a-timestamp", time.Now()); got != "not-a-timestamp" {
			t.Fatalf("unparseable input: got %q, want passthrough", got)
		}
	})
}
