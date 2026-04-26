// Package timefmt provides shared human-readable time formatters used across
// the admin UI and service layer. Centralising these prevents the formatters
// drifting between callers (admin used to ship its own copy, service its own).
package timefmt

import (
	"fmt"
	"time"
)

// Layout strings shared by admin and templ helpers. Use these instead of
// inlining the format string at each call site so date rendering stays
// aligned across pages.
const (
	// LayoutDateTimeLocal renders "Jan 2, 2006 3:04 PM" — the admin standard
	// for full timestamps (audit logs, metadata blocks, tooltips).
	LayoutDateTimeLocal = "Jan 2, 2006 3:04 PM"
	// LayoutDateShortLocal renders "Jan 2, 3:04 PM" — used in tight cells
	// (API key tables, log lists) where the year is implied.
	LayoutDateShortLocal = "Jan 2, 3:04 PM"
	// LayoutClockLocal renders "3:04 PM" — used on activity-timeline rows
	// where a same-day separator already carries the date.
	LayoutClockLocal = "3:04 PM"
)

// ParseRFC3339 parses s as RFC3339 and falls back to RFC3339Nano. Returns
// (zero, false) when s is empty or unparseable. Use when you need the parsed
// time.Time directly; prefer FormatRFC3339Local for the common format-and-
// render path.
func ParseRFC3339(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// FormatRFC3339Local parses s as RFC3339 (or RFC3339Nano) and renders the
// result in the local timezone using layout. Returns "" when s is empty,
// and the input verbatim when s fails to parse — matching the
// fall-through behavior the admin funcMap helpers have always used.
func FormatRFC3339Local(s, layout string) string {
	if s == "" {
		return ""
	}
	t, ok := ParseRFC3339(s)
	if !ok {
		return s
	}
	return t.Local().Format(layout)
}

// FormatRFC3339LocalPtr is the *string variant of FormatRFC3339Local.
// Returns "" for nil or empty input.
func FormatRFC3339LocalPtr(s *string, layout string) string {
	if s == nil {
		return ""
	}
	return FormatRFC3339Local(*s, layout)
}

// Relative renders t as a human-readable "X ago" string relative to now.
// Output buckets: "just now" (<1m), "N minute(s) ago" (<1h),
// "N hour(s) ago" (<1d), "N day(s) ago" (>=1d). Uses time.Since(t), so
// future times collapse to the "just now" bucket.
func Relative(t time.Time) string {
	d := time.Since(t)
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
