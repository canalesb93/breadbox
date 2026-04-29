package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/timefmt"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// relativeTime is a thin alias for timefmt.Relative kept so admin call sites
// (and the funcMap registration in templates.go) can refer to a short, local
// name. Concentrating the implementation in internal/timefmt prevents drift
// between admin and service copies.
func relativeTime(t time.Time) string {
	return timefmt.Relative(t)
}

// redirectGET returns an http.HandlerFunc that 301-redirects to dest while
// preserving any query string and fragment from the incoming request. Used
// to retire old GET paths after a URL migration.
func redirectGET(dest string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := dest
		if raw := r.URL.RawQuery; raw != "" {
			target = target + "?" + raw
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}
}

// redirectPreserveMethod returns an http.HandlerFunc that 308-redirects to
// dest. 308 (Permanent Redirect) preserves the request method, so POST/PUT/
// DELETE callers don't get downgraded to GET. Query string is preserved.
func redirectPreserveMethod(dest string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := dest
		if raw := r.URL.RawQuery; raw != "" {
			target = target + "?" + raw
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	}
}

// parseURLUUIDOrNotFound extracts the named chi URL param and decodes it as a
// UUID. On parse failure it renders the styled 404 page via tr and returns
// ok=false. Used by HTML detail/edit handlers.
func parseURLUUIDOrNotFound(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, key string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, key)); err != nil {
		tr.RenderNotFound(w, r)
		return pgtype.UUID{}, false
	}
	return id, true
}

// parseURLUUIDOrInvalid extracts the named chi URL param and decodes it as a
// UUID. On parse failure it writes a 400 response with the legacy
// {"error": label} envelope and returns ok=false. Used by admin JSON handlers
// that pre-date the canonical error envelope.
func parseURLUUIDOrInvalid(w http.ResponseWriter, r *http.Request, key, label string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, key)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": label})
		return pgtype.UUID{}, false
	}
	return id, true
}

// parsePage returns the 1-indexed page from ?page=, flooring at 1 when the
// param is missing, non-numeric, or less than 1.
func parsePage(r *http.Request) int {
	return parsePageKey(r, "page")
}

// parsePageKey is like parsePage but reads from an arbitrary query-param name.
// Useful on pages that host multiple independent lists (e.g. ?page=1&wh_page=2).
func parsePageKey(r *http.Request, key string) int {
	page, _ := strconv.Atoi(r.URL.Query().Get(key))
	if page < 1 {
		page = 1
	}
	return page
}

// parsePerPage returns the validated ?per_page= value. It falls back to
// defaultSize when the param is missing, non-numeric, or not in allowed.
// An empty allowed list means any positive integer is accepted.
func parsePerPage(r *http.Request, defaultSize int, allowed ...int) int {
	v, err := strconv.Atoi(r.URL.Query().Get("per_page"))
	if err != nil {
		return defaultSize
	}
	if len(allowed) == 0 {
		if v > 0 {
			return v
		}
		return defaultSize
	}
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return defaultSize
}

// parseDateParam parses the YYYY-MM-DD value of r.URL.Query().Get(key) into a
// *time.Time. Returns nil when the param is missing or malformed, matching the
// "silently drop invalid filters" convention used across admin handlers.
func parseDateParam(r *http.Request, key string) *time.Time {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil
	}
	return &t
}

// parseInclusiveDateParam behaves like parseDateParam but adds one day so the
// parsed value can be used as an exclusive upper bound that still includes the
// entire given calendar date (e.g. `date < end+1 day`).
func parseInclusiveDateParam(r *http.Request, key string) *time.Time {
	t := parseDateParam(r, key)
	if t == nil {
		return nil
	}
	next := t.AddDate(0, 0, 1)
	return &next
}

// pickValues returns a url.Values copy of the requested keys from
// r.URL.Query(). Empty values are skipped, so the result encodes only the
// filters actually in effect.
func pickValues(r *http.Request, keys []string) url.Values {
	src := r.URL.Query()
	out := make(url.Values, len(keys))
	for _, k := range keys {
		if v := src.Get(k); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// paginationBase builds a "<path>?<filters>&<pageParam>=" prefix (or
// "<path>?<pageParam>=" when no filters are present) so callers can append the
// page number directly. The pageParam key is dropped from params if present —
// the caller is appending a fresh value.
func paginationBase(path string, params url.Values, pageParam string) string {
	params.Del(pageParam)
	encoded := params.Encode()
	if encoded == "" {
		return path + "?" + pageParam + "="
	}
	return path + "?" + encoded + "&" + pageParam + "="
}

// splitCSV splits a comma-separated string into trimmed non-empty entries.
// Used by URL params that accept multi-value lists (e.g. ?tags=a,b,c).
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// IsLiabilityAccount returns true for account types that represent liabilities (credit, loan).
func IsLiabilityAccount(accountType string) bool {
	return accountType == "credit" || accountType == "loan"
}

// ConnectionStaleness computes whether a connection is stale based on the
// global sync interval and any per-connection override.
// A connection is stale when it hasn't synced within 2x its effective interval
// (minimum 1 hour for overrides, 24 hours for the global default).
func ConnectionStaleness(
	globalSyncIntervalMinutes int,
	overrideMinutes pgtype.Int4,
	lastSyncedAt pgtype.Timestamptz,
	now time.Time,
) bool {
	globalSyncInterval := time.Duration(globalSyncIntervalMinutes) * time.Minute
	threshold := globalSyncInterval * 2
	if threshold < 24*time.Hour {
		threshold = 24 * time.Hour
	}

	if overrideMinutes.Valid {
		connInterval := time.Duration(overrideMinutes.Int32) * time.Minute
		threshold = connInterval * 2
		if threshold < time.Hour {
			threshold = time.Hour
		}
	}

	if lastSyncedAt.Valid {
		return now.Sub(lastSyncedAt.Time) > threshold
	}
	// Never synced = stale.
	return true
}
