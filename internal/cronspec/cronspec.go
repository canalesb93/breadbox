// Package cronspec defines the user-facing sync-schedule presets and helpers
// for validating and humanizing cron expressions. It is intentionally tag-free
// (pure, no DB or server deps) so both the service layer (!lite) and the admin
// templates (!headless) can share one vocabulary.
package cronspec

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// Preset is a human-friendly sync cadence backed by a standard 5-field cron
// expression. "custom" is special: it has no fixed Cron and the user supplies
// the expression directly.
type Preset struct {
	Key   string // stored in sync_schedules.preset
	Label string // shown in the UI
	Cron  string // 5-field standard cron; empty for custom
}

// CustomKey marks a schedule whose cron was entered directly rather than chosen
// from a preset.
const CustomKey = "custom"

// Presets is the ordered catalog shown in the schedule UI. Order matters — it's
// the option order in the select.
var Presets = []Preset{
	{Key: "every_15m", Label: "Every 15 minutes", Cron: "*/15 * * * *"},
	{Key: "every_30m", Label: "Every 30 minutes", Cron: "*/30 * * * *"},
	{Key: "hourly", Label: "Every hour", Cron: "0 * * * *"},
	{Key: "every_4h", Label: "Every 4 hours", Cron: "0 */4 * * *"},
	{Key: "every_8h", Label: "Every 8 hours", Cron: "0 */8 * * *"},
	{Key: "twice_daily", Label: "Twice daily (6 AM & 6 PM)", Cron: "0 6,18 * * *"},
	{Key: "morning", Label: "Every morning (6 AM)", Cron: "0 6 * * *"},
	{Key: "nightly", Label: "Nightly (3 AM)", Cron: "0 3 * * *"},
	{Key: CustomKey, Label: "Custom…", Cron: ""},
}

// PresetByKey returns the preset for a key, or false if unknown.
func PresetByKey(key string) (Preset, bool) {
	for _, p := range Presets {
		if p.Key == key {
			return p, true
		}
	}
	return Preset{}, false
}

// presetByCron returns the catalog preset whose cron exactly matches, if any.
func presetByCron(expr string) (Preset, bool) {
	for _, p := range Presets {
		if p.Cron != "" && p.Cron == expr {
			return p, true
		}
	}
	return Preset{}, false
}

// Validate reports whether expr is a parseable standard cron expression.
func Validate(expr string) error {
	if expr == "" {
		return fmt.Errorf("cron expression is empty")
	}
	if _, err := cron.ParseStandard(expr); err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return nil
}

// ResolveCron turns a (presetKey, customCron) pair from a form into the concrete
// cron expression to store, plus the preset key to persist. For a known preset
// the custom value is ignored; for custom (or an unknown key) the supplied
// expression is validated and returned with preset key "custom".
func ResolveCron(presetKey, customCron string) (expr string, preset string, err error) {
	if p, ok := PresetByKey(presetKey); ok && p.Key != CustomKey {
		return p.Cron, p.Key, nil
	}
	if err := Validate(customCron); err != nil {
		return "", "", err
	}
	return customCron, CustomKey, nil
}

// NextRun returns the earliest upcoming fire time across the given cron
// expressions (strictly after `from`), and whether any valid expression was
// found. Invalid expressions are skipped.
func NextRun(exprs []string, from time.Time) (time.Time, bool) {
	var earliest time.Time
	found := false
	for _, e := range exprs {
		sc, err := cron.ParseStandard(e)
		if err != nil {
			continue
		}
		n := sc.Next(from)
		if !found || n.Before(earliest) {
			earliest = n
			found = true
		}
	}
	return earliest, found
}

// DuePassed reports whether any of the cron expressions has a fire time in the
// half-open window (since, now] — i.e. a scheduled run was missed since `since`.
// Used to render "due now" without re-deriving the scheduler's logic.
func DuePassed(exprs []string, since, now time.Time) bool {
	for _, e := range exprs {
		sc, err := cron.ParseStandard(e)
		if err != nil {
			continue
		}
		if !sc.Next(since).After(now) {
			return true
		}
	}
	return false
}

// Humanize returns a friendly description for a stored (cron, presetKey) pair.
// A recognized preset uses its label; otherwise it falls back to matching the
// raw cron against the catalog, and finally to the literal expression.
func Humanize(expr, presetKey string) string {
	if presetKey != "" && presetKey != CustomKey {
		if p, ok := PresetByKey(presetKey); ok {
			return p.Label
		}
	}
	if p, ok := presetByCron(expr); ok {
		return p.Label
	}
	return "Custom (" + expr + ")"
}
