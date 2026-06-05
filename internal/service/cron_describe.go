//go:build !lite

package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/cronspec"

	lcron "github.com/lnquy/cron"
	rcron "github.com/robfig/cron/v3"
)

// InstanceTimezone returns the configured instance IANA timezone (e.g.
// "America/Los_Angeles"), or "" when unset or invalid. Callers treat "" as the
// server process's local zone. This is the single clock every cron schedule is
// evaluated and previewed in.
func (s *Service) InstanceTimezone(ctx context.Context) string {
	tz := strings.TrimSpace(appconfig.String(ctx, s.Queries, appconfig.KeyInstanceTimezone, ""))
	if tz == "" {
		return ""
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return ""
	}
	return tz
}

// InstanceTimezoneLabel is a human display label for the instance zone — the
// IANA name when configured, else the server zone's abbreviation (e.g. "UTC").
func (s *Service) InstanceTimezoneLabel(ctx context.Context) string {
	if tz := s.InstanceTimezone(ctx); tz != "" {
		return tz
	}
	z, _ := time.Now().Zone()
	return z
}

// CronPreviewResult is the JSON shape returned to the cron-field component: the
// validity, the English description, the next N fire times (formatted in the
// instance timezone), and the zone label they're shown in.
type CronPreviewResult struct {
	Valid       bool     `json:"valid"`
	Description string   `json:"description"`
	NextRuns    []string `json:"next_runs"`
	TZLabel     string   `json:"tz_label"`
}

// CronPreview validates + describes a cron expression and computes its next n
// fire times, all in the instance timezone. Powers the shared cron-field
// live preview for sync schedules and workflows alike.
func (s *Service) CronPreview(ctx context.Context, expr string, n int) CronPreviewResult {
	tz := s.InstanceTimezone(ctx)
	valid, desc := s.DescribeCron(expr)
	res := CronPreviewResult{Valid: valid, Description: desc, TZLabel: s.InstanceTimezoneLabel(ctx)}
	if !valid {
		return res
	}
	for _, f := range cronspec.NextRuns(strings.TrimSpace(expr), tz, time.Now(), n) {
		res.NextRuns = append(res.NextRuns, f.Format("Mon Jan 2 · 3:04 PM"))
	}
	return res
}

// cronDescriptor renders a cron expression as an English sentence. Built
// once at package init (stateless + concurrency-safe). English-only keeps
// the embedded locale table small.
var cronDescriptor = func() *lcron.ExpressionDescriptor {
	d, err := lcron.NewDescriptor(
		lcron.SetLocales(lcron.Locale_en),
		lcron.Use24HourTimeFormat(false),
	)
	if err != nil {
		return nil
	}
	return d
}()

// DescribeCron validates a standard 5-field cron expression and returns a
// human-readable description of when it fires (e.g. "At 07:00 PM, only on
// Tuesday and Thursday"). Returns valid=false with a short reason when the
// expression doesn't parse. Powers the workflow configure drawer's live
// schedule preview.
//
// Validation uses the same parser the scheduler registers with
// (robfig/cron ParseStandard) so the preview never disagrees with what
// actually runs; the English rendering comes from lnquy/cron.
func (s *Service) DescribeCron(expr string) (valid bool, description string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false, "Enter a schedule"
	}
	if _, err := rcron.ParseStandard(expr); err != nil {
		return false, "Not a valid cron expression"
	}
	if cronDescriptor == nil {
		return true, "Custom schedule"
	}
	desc, err := cronDescriptor.ToDescription(expr, lcron.Locale_en)
	if err != nil || strings.TrimSpace(desc) == "" {
		// Parsed fine but couldn't render — still valid, just generic.
		return true, "Custom schedule"
	}
	return true, desc
}

// DescribeCronInTZ is DescribeCron localized to a viewer's IANA timezone
// (e.g. "America/Los_Angeles"). The scheduler fires cron in the server's
// local time, so the raw expression's times are server-local; this shifts
// them into the viewer's zone before rendering, so the preview matches the
// clock the viewer is actually reading.
//
//   - tzName empty or unknown → falls back to DescribeCron (server-local).
//   - viewer tz == server tz (the common dev case) → identical to
//     DescribeCron, no suffix.
//   - shift representable (single hour:minute, no day-of-month constraint a
//     midnight-wrap would invalidate) → times shifted, " (your time)" suffix.
//   - shift not representable (lists/ranges/steps in the time fields, or a
//     monthly schedule that would wrap past midnight) → described as-is with
//     a " (server time)" suffix so the frame is never ambiguous.
func (s *Service) DescribeCronInTZ(expr, tzName string) (valid bool, description string) {
	valid, description = s.DescribeCron(expr)
	if !valid {
		return valid, description
	}
	tzName = strings.TrimSpace(tzName)
	if tzName == "" {
		return valid, description
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return valid, description // unknown tz → server-local, unlabeled
	}

	// Offset delta (viewer − server) at now. Using the current instant keeps
	// this DST-correct for the near-term schedule the user is editing.
	now := time.Now()
	_, serverOff := now.Zone()
	_, viewerOff := now.In(loc).Zone()
	deltaMin := (viewerOff - serverOff) / 60
	if deltaMin == 0 {
		return valid, description // same wall clock — nothing to convert
	}

	if shifted, ok := shiftCronTimeFields(strings.TrimSpace(expr), deltaMin); ok {
		if d, derr := cronDescriptor.ToDescription(shifted, lcron.Locale_en); derr == nil && strings.TrimSpace(d) != "" {
			return true, d + " (your time)"
		}
	}
	// Couldn't safely convert — keep the server-time wording but label it.
	return true, description + " (server time)"
}

// shiftCronTimeFields shifts a standard 5-field cron's time-of-day by
// deltaMin minutes, carrying any midnight wrap into the day-of-week set.
// It returns ok=true only for the representable case: minute and hour are
// each a single integer (every built-in preset plus the common custom
// schedule). A wrap is only applied when the schedule is day-of-week based
// (dom and month are "*"); a day-of-month constrained schedule that would
// wrap returns ok=false so the caller can fall back rather than silently
// move, say, a "1st of the month" run onto a different day.
func shiftCronTimeFields(expr string, deltaMin int) (string, bool) {
	f := strings.Fields(expr)
	if len(f) != 5 {
		return "", false
	}
	minute, okM := singleCronInt(f[0])
	hour, okH := singleCronInt(f[1])
	if !okM || !okH {
		return "", false
	}
	total := hour*60 + minute + deltaMin
	dayDelta := 0
	for total < 0 {
		total += 1440
		dayDelta--
	}
	for total >= 1440 {
		total -= 1440
		dayDelta++
	}
	f[0] = strconv.Itoa(total % 60)
	f[1] = strconv.Itoa(total / 60)

	if dayDelta != 0 {
		dom, month, dow := f[2], f[3], f[4]
		if dom != "*" || month != "*" {
			return "", false // monthly/dom-constrained + wrap → too risky
		}
		if dow != "*" { // "*" is daily — a wrap leaves it daily
			shifted, ok := shiftCronDow(dow, dayDelta)
			if !ok {
				return "", false
			}
			f[4] = shifted
		}
	}
	return strings.Join(f, " "), true
}

// singleCronInt parses a cron field that is a single non-negative integer
// (no "*", list, range, or step). Returns ok=false for anything else.
func singleCronInt(field string) (int, bool) {
	n, err := strconv.Atoi(field)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// shiftCronDow shifts a day-of-week field (a single value or comma list of
// integers, 0/7 = Sunday) by dayDelta days, wrapping within the week. Only
// numeric values are handled; named days (MON, TUE…), ranges, and steps
// return ok=false so the caller falls back to a labeled server-time render.
func shiftCronDow(field string, dayDelta int) (string, bool) {
	parts := strings.Split(field, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 || n > 7 {
			return "", false
		}
		if n == 7 {
			n = 0 // normalize Sunday
		}
		shifted := ((n+dayDelta)%7 + 7) % 7
		out = append(out, strconv.Itoa(shifted))
	}
	return strings.Join(out, ","), true
}
