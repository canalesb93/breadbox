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
