package service

import (
	"fmt"

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
	return pgconv.TimestampStrPtr(ts)
}

func dateStr(d pgtype.Date) *string {
	return pgconv.DateStrPtr(d)
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

// SyncLogDurationMs returns the duration of a sync log in milliseconds, preferring
// the persisted duration_ms column and falling back to (completed_at - started_at)
// when duration_ms is NULL. Returns (0, false) when neither source is available.
//
// Use this anywhere a handler or service needs the duration of a sync log row —
// it consolidates the if-DurationMs.Valid / else-if-both-timestamps pattern that
// otherwise gets duplicated at every display/serialization site.
func SyncLogDurationMs(durationMs pgtype.Int4, startedAt, completedAt pgtype.Timestamptz) (int32, bool) {
	if durationMs.Valid {
		return durationMs.Int32, true
	}
	if startedAt.Valid && completedAt.Valid {
		return int32(completedAt.Time.Sub(startedAt.Time).Milliseconds()), true
	}
	return 0, false
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
