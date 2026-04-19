package mcp

import (
	"fmt"
	"time"
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
