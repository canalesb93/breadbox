// Package timefmt provides shared human-readable time formatters used across
// the admin UI and service layer. Centralising these prevents the formatters
// drifting between callers (admin used to ship its own copy, service its own).
package timefmt

import (
	"fmt"
	"time"
)

// Layouts shared by the local-time formatters. Kept as constants so handlers
// and templ helpers can render with the same wall-clock format.
const (
	// LocalDateTimeLayout is the admin "Jan 2, 2006 3:04 PM" format used in
	// detail pages, audit timelines, and tooltips that show absolute time.
	LocalDateTimeLayout = "Jan 2, 2006 3:04 PM"
	// LocalClockLayout is the "3:04 PM" clock-only form paired with a
	// same-day day separator on activity timelines.
	LocalClockLayout = "3:04 PM"
)

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

// parseRFC3339 accepts either RFC3339 or RFC3339Nano. Returns ok=false when
// the input is empty or unparseable so callers can decide whether to fall
// back to the raw string.
func parseRFC3339(s string) (time.Time, bool) {
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

// RelativeFromRFC3339 renders an RFC3339 (or RFC3339Nano) timestamp via
// Relative. Returns "" for empty input and the raw string when parsing fails,
// matching the pre-existing admin/templ helpers.
func RelativeFromRFC3339(s string) string {
	if s == "" {
		return ""
	}
	t, ok := parseRFC3339(s)
	if !ok {
		return s
	}
	return Relative(t)
}

// RelativeFromRFC3339Ptr is the *string overload of RelativeFromRFC3339.
// Returns "" when s is nil or points to the empty string.
func RelativeFromRFC3339Ptr(s *string) string {
	if s == nil {
		return ""
	}
	return RelativeFromRFC3339(*s)
}

// LocalDateTime renders t in the admin "Jan 2, 2006 3:04 PM" local-time format.
func LocalDateTime(t time.Time) string {
	return t.Local().Format(LocalDateTimeLayout)
}

// LocalDateTimeFromRFC3339 parses an RFC3339 timestamp and renders it via
// LocalDateTime. Returns "" for empty input and the raw string when parsing
// fails.
func LocalDateTimeFromRFC3339(s string) string {
	if s == "" {
		return ""
	}
	t, ok := parseRFC3339(s)
	if !ok {
		return s
	}
	return LocalDateTime(t)
}

// LocalDateTimeFromRFC3339Ptr is the *string overload of
// LocalDateTimeFromRFC3339. Returns "" when s is nil.
func LocalDateTimeFromRFC3339Ptr(s *string) string {
	if s == nil {
		return ""
	}
	return LocalDateTimeFromRFC3339(*s)
}

// LocalClock renders t as "3:04 PM" in the local timezone. Used on activity
// timelines where the day separator already carries the date.
func LocalClock(t time.Time) string {
	return t.Local().Format(LocalClockLayout)
}

// LocalClockFromRFC3339 parses an RFC3339 timestamp and renders it via
// LocalClock. Returns "" for empty input and the raw string when parsing fails.
func LocalClockFromRFC3339(s string) string {
	if s == "" {
		return ""
	}
	t, ok := parseRFC3339(s)
	if !ok {
		return s
	}
	return LocalClock(t)
}

// LocalClockFromRFC3339Ptr is the *string overload of LocalClockFromRFC3339.
// Returns "" when s is nil.
func LocalClockFromRFC3339Ptr(s *string) string {
	if s == nil {
		return ""
	}
	return LocalClockFromRFC3339(*s)
}
