// Package timefmt provides shared human-readable time formatters used across
// the admin UI and service layer. Centralising these prevents the formatters
// drifting between callers (admin used to ship its own copy, service its own).
package timefmt

import (
	"fmt"
	"time"
)

// Layout strings used by the admin UI to render absolute timestamps. Kept
// here so admin handlers, templ pages, and the funcMap entries all share
// one canonical format per shape.
const (
	// LayoutDateTime is the "Jan 2, 2006 3:04 PM" rendering used wherever
	// both the date and clock are shown together.
	LayoutDateTime = "Jan 2, 2006 3:04 PM"

	// LayoutClock is the clock-only "3:04 PM" rendering used on activity
	// rows where a day separator already carries the date.
	LayoutClock = "3:04 PM"

	// LayoutDateShort is the compact "Jan 2, 3:04 PM" rendering used on
	// dense list views (api keys, sessions).
	LayoutDateShort = "Jan 2, 3:04 PM"
)

// FormatRFC3339In parses an RFC3339 (or RFC3339Nano) timestamp string and
// renders it via layout anchored to loc. This is the canonical
// timezone-aware absolute-time formatter — pair it with admin.UserLocation(r)
// so a UTC-running server renders the viewer's wall clock, not its own. A nil
// loc falls back to time.Local. Empty input yields ""; an unparseable string
// is returned unchanged so callers don't display "0001-01-01..." on bad data.
func FormatRFC3339In(s string, loc *time.Location, layout string) string {
	if s == "" {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(loc).Format(layout)
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.In(loc).Format(layout)
	}
	return s
}

// FormatRFC3339At is FormatRFC3339In with the location taken from a `now`
// anchor (now.Location()). It's the shape used by page assemblers that already
// thread a single `now` built from admin.UserLocation(r) — the home feed and
// the per-tx activity timeline both render their absolute-time tooltips
// through this so the two surfaces can never disagree. A zero now falls back
// to time.Local.
func FormatRFC3339At(s string, now time.Time, layout string) string {
	loc := time.Local
	if !now.IsZero() {
		loc = now.Location()
	}
	return FormatRFC3339In(s, loc, layout)
}

// FormatTimeIn renders an already-parsed time.Time via layout anchored to
// loc — the time.Time twin of FormatRFC3339In for callers that hold a
// timestamptz value rather than an RFC3339 string. A zero t yields ""; a nil
// loc falls back to time.Local.
func FormatTimeIn(t time.Time, loc *time.Location, layout string) string {
	if t.IsZero() {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc).Format(layout)
}

// FormatRFC3339 parses an RFC3339 (or RFC3339Nano) timestamp string and
// renders it via layout in the SERVER's timezone. Deprecated for anything
// shown to a viewer: a UTC-running server renders every absolute time in UTC.
// Prefer FormatRFC3339In with a viewer location from admin.UserLocation(r).
// This server-local form remains only for contexts with no request/viewer
// (CLI output, server logs). Empty input yields ""; an unparseable string is
// returned unchanged.
func FormatRFC3339(s, layout string) string {
	return FormatRFC3339In(s, time.Local, layout)
}

// FormatRFC3339Ptr is FormatRFC3339 for nullable *string inputs. nil and
// empty both render as "". Server-local — see FormatRFC3339's caveat and
// prefer FormatRFC3339In for viewer-facing output.
func FormatRFC3339Ptr(s *string, layout string) string {
	if s == nil {
		return ""
	}
	return FormatRFC3339(*s, layout)
}

// FormatRFC3339PtrIn is FormatRFC3339In for nullable *string inputs — the
// viewer-timezone twin of FormatRFC3339Ptr. nil and empty both render as "".
func FormatRFC3339PtrIn(s *string, loc *time.Location, layout string) string {
	if s == nil {
		return ""
	}
	return FormatRFC3339In(*s, loc, layout)
}

// RelativeRFC3339 parses an RFC3339 (or RFC3339Nano) timestamp string and
// returns Relative(t). Empty input yields ""; an unparseable string is
// returned unchanged.
func RelativeRFC3339(s string) string {
	return RelativeRFC3339At(s, time.Now())
}

// RelativeRFC3339At is RelativeRFC3339 with an explicit now anchor. Pages
// that mix relative-time rendering with day-bucket labels share a single
// now via this entry point so the two paths can never disagree across
// midnight or timezone boundaries.
func RelativeRFC3339At(s string, now time.Time) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return RelativeAt(t, now)
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return RelativeAt(t, now)
	}
	return s
}

// RelativeRFC3339Ptr is RelativeRFC3339 for nullable *string inputs.
func RelativeRFC3339Ptr(s *string) string {
	if s == nil {
		return ""
	}
	return RelativeRFC3339(*s)
}

// RelativeRFC3339PtrAt is RelativeRFC3339Ptr with an explicit now anchor.
func RelativeRFC3339PtrAt(s *string, now time.Time) string {
	if s == nil {
		return ""
	}
	return RelativeRFC3339At(*s, now)
}

// Relative renders t as a human-readable "X ago" string relative to now.
// Output buckets: "just now" (<1m), "N minute(s) ago" (<1h),
// "N hour(s) ago" (<1d), "N day(s) ago" (>=1d). Future times collapse
// to the "just now" bucket. Delegates to RelativeAt with time.Now().
func Relative(t time.Time) string {
	return RelativeAt(t, time.Now())
}

// RelativeAt is Relative with an explicit now anchor. The page-level
// timeline assembler captures one now at the top and threads it through
// both the day-grouping and the per-row relative-time helpers via this
// function so labels ("Today" / "Yesterday") and timestamps
// ("yesterday" / "5 minutes ago") always agree.
func RelativeAt(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
