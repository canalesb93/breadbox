package service

import (
	"fmt"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

func formatUUID(u pgtype.UUID) string {
	return pgconv.FormatUUID(u)
}

func uuidPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := formatUUID(u)
	return &s
}

func textPtr(t pgtype.Text) *string {
	return pgconv.TextPtr(t)
}

func numericFloat(n pgtype.Numeric) *float64 {
	if n.Int == nil {
		return nil
	}
	f, ok := pgconv.NumericToFloat(n)
	if !ok {
		return nil
	}
	return &f
}

func timestampStr(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func nullConnStatusPtr(s db.NullConnectionStatus) *string {
	if !s.Valid {
		return nil
	}
	str := string(s.ConnectionStatus)
	return &str
}

func connStatusPtr(s db.ConnectionStatus) *string {
	str := string(s)
	return &str
}

// FormatDurationMs converts milliseconds to a human-readable duration string.
// Examples: 42ms, 1.2s, 2m 15s
func FormatDurationMs(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	mins := ms / 60000
	secs := (ms % 60000) / 1000
	if secs == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm %ds", mins, secs)
}
