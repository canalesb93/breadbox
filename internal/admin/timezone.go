//go:build !headless && !lite

package admin

import (
	"net/http"
	"net/url"
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
//
// The JS shim writes the value through encodeURIComponent, so a slash-bearing
// IANA name like "America/Los_Angeles" arrives percent-encoded as
// "America%2FLos_Angeles". Go's net/http never percent-decodes cookie values,
// so we PathUnescape here before LoadLocation — otherwise every multi-segment
// zone (i.e. nearly all of them) fails to resolve and silently falls back to
// the server's timezone, which is exactly the bug this guards against.
// PathUnescape (not QueryUnescape) is used so a literal '+' in zones like
// "Etc/GMT+5" survives; values without a '%' pass through unchanged, so an
// already-decoded cookie still works.
func UserLocation(r *http.Request) *time.Location {
	if r == nil {
		return time.Local
	}
	c, err := r.Cookie(userTZCookie)
	if err != nil || c.Value == "" {
		return time.Local
	}
	name := c.Value
	if decoded, derr := url.PathUnescape(name); derr == nil {
		name = decoded
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}
