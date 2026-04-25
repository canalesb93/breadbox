// Package pgconv provides shared conversion helpers for pgtype values.
package pgconv

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// FormatUUID converts a pgtype.UUID to its standard string representation
// (e.g., "12345678-9abc-def0-1234-56789abcdef0"). Returns an empty string
// if the UUID is not valid.
func FormatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ParseUUID parses a UUID string into a pgtype.UUID. Accepts any format
// pgx's UUID.Scan accepts (canonical 36-char with dashes, or 32-char without).
func ParseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	err := u.Scan(s)
	return u, err
}

// NumericToFloat returns the float64 value of a pgtype.Numeric. Returns
// (0, false) when the numeric is NULL, NaN, or fails to convert (e.g. overflow).
// Callers that need to distinguish those cases should use Float64Value directly.
func NumericToFloat(n pgtype.Numeric) (float64, bool) {
	if !n.Valid || n.NaN {
		return 0, false
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0, false
	}
	return f.Float64, true
}

// TextPtr converts a pgtype.Text to *string for JSON serialization. Returns
// nil when the text is NULL; otherwise returns a pointer to the underlying
// string value.
func TextPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

// TextOr returns the underlying string value of a pgtype.Text, or fallback
// when the text is NULL. Use this to collapse `if t.Valid { x = t.String }`
// boilerplate at display/serialization sites.
func TextOr(t pgtype.Text, fallback string) string {
	if !t.Valid {
		return fallback
	}
	return t.String
}

// Text wraps a string as a non-NULL pgtype.Text. Empty strings are preserved
// as valid (non-NULL) empty values — use TextFromPtr when an empty/missing
// input should map to NULL.
func Text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// TextFromPtr wraps *string as pgtype.Text: nil → NULL, non-nil → valid
// (including the empty string). Mirrors TextPtr in reverse.
func TextFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// TextIfNotEmpty wraps a string as pgtype.Text, treating "" as NULL. Use this
// to collapse `pgtype.Text{String: x, Valid: x != ""}` boilerplate at insert
// sites where the empty string and NULL should be the same thing.
func TextIfNotEmpty(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// TimestampStr renders a pgtype.Timestamptz as an RFC3339 UTC string. Returns
// an empty string when the timestamp is NULL. Use for NOT NULL columns where
// an empty response field is acceptable for the rare invalid case.
func TimestampStr(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

// TimestampStrPtr renders a pgtype.Timestamptz as an RFC3339 UTC string wrapped
// in *string. Returns nil when the timestamp is NULL. Use for nullable columns
// where JSON should serialize absent timestamps as null (and be omitted via
// omitempty).
func TimestampStrPtr(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}

// DateStrPtr renders a pgtype.Date as a "2006-01-02" string wrapped in
// *string. Returns nil when the date is NULL.
func DateStrPtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}
