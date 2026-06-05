//go:build !headless && !lite

package components

import (
	"encoding/json"

	"breadbox/internal/cronspec"
)

// CronFieldProps configures the shared cron-input component: timezone-aware
// preset chips, a custom expression input, and a live preview (English
// description + next fire times) rendered against the instance timezone.
//
// The component derives the active preset from the cron VALUE (not a stored
// preset key), so an expression with no matching preset shows "Custom" with the
// raw cron — never a misleading preset label.
type CronFieldProps struct {
	// Name is the form field the resolved cron expression is submitted as
	// (required). A hidden input bound to the live value carries it.
	Name string
	// PresetName, when set, submits the active preset key as a second hidden
	// field (for storing which preset was chosen, "custom" otherwise).
	PresetName string
	// Value is the initial cron expression.
	Value string
	// Presets is the catalog of preset chips (cronspec.Presets).
	Presets []cronspec.Preset
	// PreviewURL is the GET endpoint for the live preview. Defaults to
	// /-/cron/preview when empty.
	PreviewURL string
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
		"value":      p.Value,
		"presets":    ps,
		"previewUrl": p.previewURL(),
		"customKey":  cronspec.CustomKey,
	})
	return string(b)
}
