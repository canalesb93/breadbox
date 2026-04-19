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

// optSearchMode validates an MCP search_mode input and returns a pointer to
// the value, or nil when the input is empty. Callers surface the error back
// to the MCP client unchanged.
func optSearchMode(value string) (*string, error) {
	if value == "" {
		return nil, nil
	}
	if !service.ValidateSearchMode(value) {
		return nil, fmt.Errorf("invalid search_mode: %s. Must be one of: contains, words, fuzzy", value)
	}
	return &value, nil
}
