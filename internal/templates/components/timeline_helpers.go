package components

import (
	"fmt"
	"strings"
	"time"

	"breadbox/internal/timefmt"
)

// timelineHeadingID derives a stable id="..." from the heading string so the
// section's aria-labelledby has a deterministic target. Non-alphanumeric
// runes collapse to '-'. Empty headings yield "timeline-heading" (the section
// is still aria-labelled, even if no <h2> renders).
func timelineHeadingID(heading string) string {
	if heading == "" {
		return "timeline-heading"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(heading) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	id := strings.TrimRight(b.String(), "-")
	if id == "" {
		return "timeline-heading"
	}
	return id + "-heading"
}

// pluralEvents renders the "N events" / "1 event" suffix beside the heading.
func pluralEvents(n int) string {
	if n == 1 {
		return "1 event"
	}
	return fmt.Sprintf("%d events", n)
}

// timelineIconBg maps an IconTone string to the tailwind background classes
// for the 24px tile. Mirrors the legacy txdSystemIcon palette.
func timelineIconBg(tone string) string {
	switch tone {
	case "info":
		return "bg-info/15"
	case "success":
		return "bg-success/15"
	case "warning":
		return "bg-warning/15"
	case "error":
		return "bg-error/15"
	default:
		return "bg-base-200"
	}
}

// timelineIconColor maps an IconTone to the lucide icon color class.
func timelineIconColor(tone string) string {
	switch tone {
	case "info":
		return "text-info"
	case "success":
		return "text-success"
	case "warning":
		return "text-warning"
	case "error":
		return "text-error"
	default:
		return "text-base-content/40"
	}
}

// formatTimelineTimestamp parses an RFC3339 timestamp and renders it in the
// admin-standard "Jan 2, 2006 3:04 PM" local format. Unparseable input is
// returned unchanged.
func formatTimelineTimestamp(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Local().Format("Jan 2, 2006 3:04 PM")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Local().Format("Jan 2, 2006 3:04 PM")
	}
	return s
}

// relativeTimelineTimestamp renders an RFC3339 timestamp as a short relative
// phrase ("just now", "5 minutes ago", "2 days ago").
func relativeTimelineTimestamp(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return s
		}
	}
	return timefmt.Relative(t)
}
