//go:build !headless && !lite

package components

import (
	"encoding/json"

	"breadbox/internal/cronspec"
)

// CronFieldProps configures the shared cron-input component: a row of preset
// shortcut chips, an always-visible custom expression input, and a one-line
// live preview (English description of when it next fires).
//
// The active chip is derived from the cron VALUE (not a stored preset key), so
// an expression with no matching preset simply lights no chip and shows the raw
// cron in the input — never a misleading preset label.
type CronFieldProps struct {
	// Name is the form field the resolved cron expression is submitted as
	// (required). A hidden input bound to the live value carries it.
	Name string
	// PresetName, when set, submits the active preset key as a second hidden
	// field (for storing which preset was chosen, "custom" otherwise).
	PresetName string
	// Value is the initial cron expression.
	Value string
	// Presets is the catalog of preset chips (cronspec.Presets for sync
	// schedules; cronspec.WorkflowSchedulePresets for workflows). Chips with an
	// empty Cron (e.g. the legacy "Custom…" entry) are not rendered — the input
	// is the custom path.
	Presets []cronspec.Preset
	// PreviewURL is the GET endpoint for the live preview. Defaults to
	// /-/cron/preview when empty. The component always appends &tz=<viewer IANA>;
	// endpoints that localize (the workflow one) honor it, others ignore it.
	PreviewURL string
	// LocalizePresets treats each preset's Cron as a VIEWER-local intent (e.g.
	// "0 9 * * *" = 9 AM their time) and converts it to the server-local cron the
	// scheduler fires, using ServerUTCOffsetMin. Off (the default) applies preset
	// crons literally — the sync-schedule behavior.
	LocalizePresets bool
	// ServerUTCOffsetMin is the server's current UTC offset in minutes (east of
	// UTC). Only consulted when LocalizePresets is set; drives the viewer-local →
	// server-local chip conversion. 0 means no shift (server tz == viewer tz, the
	// common self-hosted case, or an unset value).
	ServerUTCOffsetMin int
	// ModelExpr, when set, two-way binds the component's internal cron value to a
	// caller Alpine expression via x-modelable (e.g. "reconfigure.scheduleCron").
	// Use it when the field's value is hydrated by the parent after render (the
	// workflow reconfigure drawer); leave empty for server-seeded forms.
	ModelExpr string
}

func (p CronFieldProps) previewURL() string {
	if p.PreviewURL != "" {
		return p.PreviewURL
	}
	return "/-/cron/preview"
}

// cronFieldConfigJSON serializes the config the Alpine factory reads from the
// component root's data-config attribute.
func cronFieldConfigJSON(p CronFieldProps) string {
	type preset struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Cron  string `json:"cron"`
	}
	ps := make([]preset, 0, len(p.Presets))
	for _, x := range p.Presets {
		ps = append(ps, preset{Key: x.Key, Label: x.Label, Cron: x.Cron})
	}
	b, _ := json.Marshal(map[string]any{
		"value":              p.Value,
		"presets":            ps,
		"previewUrl":         p.previewURL(),
		"customKey":          cronspec.CustomKey,
		"localizePresets":    p.LocalizePresets,
		"serverUtcOffsetMin": p.ServerUTCOffsetMin,
	})
	return string(b)
}
