package mcp

import (
	"fmt"
	"time"

	"breadbox/internal/service"
)

// optStr returns a pointer to s when non-empty, else nil. Used to forward
// MCP tool input strings into service-layer params that treat nil as "no
// filter" and ignore empty-string values.
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseOptionalDate parses a YYYY-MM-DD string into a time pointer. Returns
// (nil, nil) when value is empty. The field name is used in the error message
// so callers can surface it back to the MCP client without wrapping.
func parseOptionalDate(field, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", field, err)
	}
	return &t, nil
}

// parseDateRange parses optional start_date + end_date strings in one call.
// Nearly every tool that accepts a date window does so with the same
// YYYY-MM-DD pair, so handlers share the field names ("start_date", "end_date")
// and the parseOptionalDate error format. Empty inputs yield nil pointers.
func parseDateRange(start, end string) (*time.Time, *time.Time, error) {
	s, err := parseOptionalDate("start_date", start)
	if err != nil {
		return nil, nil, err
	}
	e, err := parseOptionalDate("end_date", end)
	if err != nil {
		return nil, nil, err
	}
	return s, e, nil
}

// parseSearchMode validates an MCP search_mode input and returns a pointer
// suitable for service-layer params. Empty input returns (nil, nil) —
// service layer falls back to its own default. Unknown modes return an
// error whose message mirrors the one previously duplicated across tool
// handlers so agents see a consistent hint.
func parseSearchMode(mode string) (*string, error) {
	if mode == "" {
		return nil, nil
	}
	if !service.ValidateSearchMode(mode) {
		return nil, fmt.Errorf("invalid search_mode: %s. Must be one of: contains, words, fuzzy", mode)
	}
	return &mode, nil
}
