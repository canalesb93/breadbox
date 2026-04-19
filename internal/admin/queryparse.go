package admin

import (
	"net/url"
	"strconv"
	"time"
)

// Helpers for parsing optional query-string parameters in admin handlers.
//
// Admin pages use silent-failure semantics: a malformed value is treated the
// same as an absent one so users don't get a 400 from a stale URL. The REST
// API takes a stricter approach (see internal/api/queryparse.go), which is why
// these helpers live here separately.

// optStrQuery returns a pointer to the query value, or nil when the parameter
// is absent or empty.
func optStrQuery(q url.Values, key string) *string {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	return &v
}

// optDateQuery parses a YYYY-MM-DD query value. Returns nil when the parameter
// is absent, empty, or malformed.
func optDateQuery(q url.Values, key string) *time.Time {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil
	}
	return &t
}

// optEndDateQuery parses a YYYY-MM-DD query value and shifts it forward one
// day so the end date is inclusive in `date >= start AND date < end` filters.
// Returns nil when the parameter is absent, empty, or malformed.
func optEndDateQuery(q url.Values, key string) *time.Time {
	t := optDateQuery(q, key)
	if t == nil {
		return nil
	}
	next := t.AddDate(0, 0, 1)
	return &next
}

// optFloatQuery parses a floating-point query value. Returns nil when the
// parameter is absent, empty, or malformed.
func optFloatQuery(q url.Values, key string) *float64 {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &f
}
