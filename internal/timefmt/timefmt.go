// Package timefmt provides shared human-readable time formatters used across
// the admin UI and service layer. Centralising these prevents the formatters
// drifting between callers (admin used to ship its own copy, service its own).
package timefmt

import (
	"fmt"
	"time"
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
