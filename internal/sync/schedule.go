//go:build !lite

package sync

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/cronspec"

	"github.com/robfig/cron/v3"
)

// jitterWindow spreads connections that share a fire time across a few minutes
// so a popular schedule (e.g. everything at 6:00am) doesn't fire every
// connection in the same 15-minute tick. The offset is deterministic per
// connection, so a given connection always lands in the same slot.
const jitterWindow = 10 * time.Minute

// backoffBaseMinutes is the base unit for failure backoff. A connection with
// consecutive failures is suppressed for backoffInterval(15, failures) minutes
// past its last error before it is allowed to retry, regardless of how
// frequently its schedule would otherwise fire.
const backoffBaseMinutes = 15

// scheduleResolver maps a connection to the cron schedules that apply to it:
// the union of all `applies_to_all` schedules plus any explicitly targeting it.
// Built once per tick from the DB so a single ListEnabledSyncSchedules +
// ListSyncScheduleConnectionPairs covers every connection.
type scheduleResolver struct {
	all     []cron.Schedule
	perConn map[[16]byte][]cron.Schedule
}

// forConnection returns every cron schedule that applies to the connection.
// The slice is freshly allocated so callers may not mutate the resolver state.
func (r *scheduleResolver) forConnection(id [16]byte) []cron.Schedule {
	if len(r.all) == 0 {
		return r.perConn[id]
	}
	out := make([]cron.Schedule, 0, len(r.all)+len(r.perConn[id]))
	out = append(out, r.all...)
	out = append(out, r.perConn[id]...)
	return out
}

// loadScheduleResolver reads the enabled schedules and their connection
// mappings and builds a resolver. Invalid cron specs are skipped with a warning
// rather than failing the whole tick. If the schedules table is empty (e.g. a
// brand-new deployment before the seed lands), a single fallback schedule is
// synthesized from the legacy interval so auto-sync never silently stops.
func (s *Scheduler) loadScheduleResolver(ctx context.Context, fallbackIntervalMinutes int) (*scheduleResolver, error) {
	rows, err := s.queries.ListEnabledSyncSchedules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled sync schedules: %w", err)
	}
	pairs, err := s.queries.ListSyncScheduleConnectionPairs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sync schedule connection pairs: %w", err)
	}

	// Cron is evaluated in the configured instance timezone (the single source
	// of truth for "what clock the cron means"); unset → server local.
	tzName := appconfig.String(ctx, s.queries, appconfig.KeyInstanceTimezone, "")

	r := &scheduleResolver{perConn: make(map[[16]byte][]cron.Schedule)}
	parsed := make(map[[16]byte]cron.Schedule, len(rows))
	appliesToAll := make(map[[16]byte]bool, len(rows))

	for _, row := range rows {
		sc, err := cronspec.Parse(row.Cron, tzName)
		if err != nil {
			s.logger.Warn("skipping sync schedule with invalid cron", "cron", row.Cron, "error", err)
			continue
		}
		parsed[row.ID.Bytes] = sc
		appliesToAll[row.ID.Bytes] = row.AppliesToAll
		if row.AppliesToAll {
			r.all = append(r.all, sc)
		}
	}

	for _, p := range pairs {
		// `applies_to_all` schedules already cover every connection; their
		// explicit mappings (if any) are redundant, so skip them here.
		if appliesToAll[p.ScheduleID.Bytes] {
			continue
		}
		if sc, ok := parsed[p.ScheduleID.Bytes]; ok {
			r.perConn[p.ConnectionID.Bytes] = append(r.perConn[p.ConnectionID.Bytes], sc)
		}
	}

	if len(rows) == 0 {
		if sc := intervalFallbackSchedule(fallbackIntervalMinutes); sc != nil {
			s.logger.Warn("no sync schedules configured, falling back to legacy interval",
				"interval_minutes", fallbackIntervalMinutes)
			r.all = append(r.all, sc)
		}
	}

	return r, nil
}

// intervalFallbackSchedule turns the legacy interval (minutes) into a cron
// schedule. Only used when the schedules table is empty.
func intervalFallbackSchedule(minutes int) cron.Schedule {
	if minutes <= 0 {
		minutes = 720
	}
	sc, err := cron.ParseStandard(fmt.Sprintf("@every %dm", minutes))
	if err != nil {
		return nil
	}
	return sc
}

// scheduleDue reports whether a connection is due to sync now given its
// applicable schedules and last successful sync. A connection with no schedules
// is never due via cron (manual/webhook still work). A never-synced connection
// (lastSynced zero) is due as long as it has at least one schedule.
//
// For each schedule we ask: has a fire time occurred between lastSynced and now
// (offset by the connection's jitter)? `Next(lastSynced)` is the first fire
// strictly after the last sync; if that moment (plus jitter) has already
// passed, the scheduled sync was missed and is now due.
func scheduleDue(schedules []cron.Schedule, lastSynced, now time.Time, jitter time.Duration) bool {
	if len(schedules) == 0 {
		return false
	}
	if lastSynced.IsZero() {
		return true
	}
	for _, sc := range schedules {
		fireAt := sc.Next(lastSynced).Add(jitter)
		if !fireAt.After(now) {
			return true
		}
	}
	return false
}

// scheduleNextRun returns the earliest upcoming fire time across the
// connection's schedules (with jitter applied), for display. Zero if none.
func scheduleNextRun(schedules []cron.Schedule, now time.Time, jitter time.Duration) time.Time {
	var earliest time.Time
	for _, sc := range schedules {
		n := sc.Next(now).Add(jitter)
		if earliest.IsZero() || n.Before(earliest) {
			earliest = n
		}
	}
	return earliest
}

// connectionJitter derives a stable per-connection offset in [0, window) from
// the connection UUID so connections sharing a fire time stagger across ticks.
func connectionJitter(id [16]byte, window time.Duration) time.Duration {
	if window <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write(id[:])
	secs := int64(window / time.Second)
	if secs <= 0 {
		return 0
	}
	return time.Duration(int64(h.Sum32())%secs) * time.Second
}

// backoffSuppressed reports whether a failing connection should be held back
// from retrying: true while now is within backoffInterval(15, failures) minutes
// of the last error. Layered on top of the schedule check so a frequently-firing
// schedule doesn't hammer a broken connection every tick.
func backoffSuppressed(consecutiveFailures int32, lastErrorAt time.Time, now time.Time) bool {
	if consecutiveFailures <= 0 || lastErrorAt.IsZero() {
		return false
	}
	delay := time.Duration(backoffInterval(backoffBaseMinutes, consecutiveFailures)) * time.Minute
	return now.Before(lastErrorAt.Add(delay))
}
