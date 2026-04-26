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

// FormatRFC3339 parses an RFC3339 (or RFC3339Nano) timestamp string and
// renders it via layout in the local timezone. Empty input yields ""; an
// unparseable string is returned unchanged so callers don't display
// "0001-01-01..." on bad data.
func FormatRFC3339(s, layout string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Local().Format(layout)
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Local().Format(layout)
	}
	return s
}

// FormatRFC3339Ptr is FormatRFC3339 for nullable *string inputs. nil and
// empty both render as "".
func FormatRFC3339Ptr(s *string, layout string) string {
	if s == nil {
		return ""
	}
	return FormatRFC3339(*s, layout)
}

// RelativeRFC3339 parses an RFC3339 (or RFC3339Nano) timestamp string and
// returns Relative(t). Empty input yields ""; an unparseable string is
// returned unchanged.
func RelativeRFC3339(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return Relative(t)
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return Relative(t)
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
