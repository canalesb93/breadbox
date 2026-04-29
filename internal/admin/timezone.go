package admin

import (
	"net/http"
	"time"
)

// userTZCookie names the cookie populated by the small JS snippet in
// `internal/templates/layout/base.html`. The value is the browser's IANA
// timezone (e.g. "America/Los_Angeles") as reported by
// `Intl.DateTimeFormat().resolvedOptions().timeZone`.
const userTZCookie = "bb_tz"

// UserLocation returns the viewer's preferred *time.Location for rendering
// wall-clock-sensitive content (day buckets, "Today" labels, absolute-time
// tooltips). Reads the `bb_tz` cookie set by the base layout's JS shim and
// validates via `time.LoadLocation`. Falls back to `time.Local` when the
// cookie is missing, blank, or names a zone the runtime can't resolve —
// matches pre-cookie behaviour, so the very first request from a brand-new
// session still renders something sensible before the cookie round-trips.
func UserLocation(r *http.Request) *time.Location {
	if r == nil {
		return time.Local
	}
	c, err := r.Cookie(userTZCookie)
	if err != nil || c.Value == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(c.Value)
	if err != nil {
		return time.Local
	}
	return loc
}
