package admin

import (
	"net/url"
	"strconv"
)

// Helpers for parsing optional query-string parameters in admin handlers.
//
// Admin pages use silent-failure semantics: a malformed value is treated the
// same as an absent one so users don't get a 400 from a stale URL. The REST
// API takes a stricter approach (see internal/api/queryparse.go), which is why
// these helpers live here separately.
//
// Date parsing lives in helpers.go (parseDateParam / parseInclusiveDateParam).

// optStrQuery returns a pointer to the query value, or nil when the parameter
// is absent or empty.
func optStrQuery(q url.Values, key string) *string {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	return &v
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
