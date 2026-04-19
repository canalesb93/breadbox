// Package pgconv provides shared conversion helpers for pgtype values.
package pgconv

import (
	"fmt"

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
