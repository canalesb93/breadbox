package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// parseIntParam parses an integer query parameter with bounds checking.
// Returns defaultVal when the parameter is absent. Returns an error with a
// user-facing message when the value is not a valid integer or falls outside
// [min, max].
func parseIntParam(q url.Values, key string, defaultVal, min, max int) (int, error) {
	v := q.Get(key)
	if v == "" {
		return defaultVal, nil
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed < min || parsed > max {
		return 0, fmt.Errorf("%s must be between %d and %d", key, min, max)
	}
	return parsed, nil
}

// parseDateParam parses a YYYY-MM-DD date query parameter. Returns nil when
// the parameter is absent. Returns an error with a user-facing message when the
// value does not match the expected format.
func parseDateParam(q url.Values, key string) (*time.Time, error) {
	v := q.Get(key)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, fmt.Errorf("%s must be in YYYY-MM-DD format", key)
	}
	return &t, nil
}

// parseFloatParam parses a floating-point query parameter. Returns nil when the
// parameter is absent. Returns an error with a user-facing message when the
// value is not a valid number.
func parseFloatParam(q url.Values, key string) (*float64, error) {
	v := q.Get(key)
	if v == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid number", key)
	}
	return &f, nil
}

// parseBoolParam parses a boolean query parameter. Returns nil when the
// parameter is absent. Returns an error with a user-facing message when the
// value is not a valid boolean (accepts 1, t, TRUE, true, 0, f, FALSE, false, etc.).
func parseBoolParam(q url.Values, key string) (*bool, error) {
	v := q.Get(key)
	if v == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil, fmt.Errorf("%s must be true or false", key)
	}
	return &b, nil
}

// parseOptionalStringParam returns a pointer to the query parameter value, or
// nil when the parameter is absent.
func parseOptionalStringParam(q url.Values, key string) *string {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	return &v
}

// parseMinLengthStringParam parses an optional string query parameter that must
// be at least minLen characters long when present. Returns nil when the
// parameter is absent. Returns an error with a user-facing message when the
// value is too short.
func parseMinLengthStringParam(q url.Values, key string, minLen int) (*string, error) {
	v := q.Get(key)
	if v == "" {
		return nil, nil
	}
	if len(v) < minLen {
		return nil, fmt.Errorf("%s must be at least %d characters", key, minLen)
	}
	return &v, nil
}

// parseEnumParam parses a string query parameter that must be one of the
// allowed values. Returns nil when the parameter is absent. Returns an error
// with a user-facing message listing the allowed values when the value is not
// recognized.
func parseEnumParam(q url.Values, key string, allowed []string) (*string, error) {
	v := q.Get(key)
	if v == "" {
		return nil, nil
	}
	for _, a := range allowed {
		if v == a {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("%s must be one of: %s", key, joinStrings(allowed))
}

// parseCSVParam collects a repeatable/comma-separated query parameter into a
// de-duplicated, trimmed slice. Accepts both ?tags=a,b and ?tags=a&tags=b.
// Returns nil when the parameter is absent or contains only empty tokens.
func parseCSVParam(q url.Values, key string) []string {
	raw := q[key]
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, v := range raw {
		for _, token := range strings.Split(v, ",") {
			t := strings.TrimSpace(token)
			if t == "" {
				continue
			}
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// joinStrings joins strings with ", " for error messages.
func joinStrings(ss []string) string {
	switch len(ss) {
	case 0:
		return ""
	case 1:
		return ss[0]
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += ", " + s
	}
	return result
}
