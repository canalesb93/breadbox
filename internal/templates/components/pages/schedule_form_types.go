//go:build !headless && !lite

package pages

import (
	"breadbox/internal/cronspec"
	"breadbox/internal/service"

	"github.com/a-h/templ"
)

// scheduleEditURL / scheduleDeleteURL build the per-schedule action URLs.
// Defined here (a .go file) rather than inline in the .templ so the
// routes-drift scanner — which only reads .html/.templ and would mis-read the
// concatenated literal as a static href — leaves them alone.
func scheduleEditURL(shortID string) templ.SafeURL {
	return templ.SafeURL("/settings/sync/schedules/" + shortID + "/edit")
}

func scheduleDeleteURL(shortID string) templ.SafeURL {
	return templ.SafeURL("/settings/sync/schedules/" + shortID + "/delete")
}

// ScheduleFormProps drives the create/edit sync-schedule form page.
type ScheduleFormProps struct {
	CSRFToken string
	// IsEdit toggles the form between create (POST /settings/sync/schedules)
	// and update (POST /settings/sync/schedules/{shortID}) modes.
	IsEdit  bool
	ShortID string
	Error   string

	// Current field values (also used to repopulate on validation error).
	Name         string
	PresetKey    string
	Cron         string
	AppliesToAll bool
	Enabled      bool

	Presets     []cronspec.Preset
	Connections []service.ConnectionResponse
	// SelectedConns marks which connection short IDs are targeted.
	SelectedConns map[string]bool
}

// FormAction returns the POST target for the form.
func (p ScheduleFormProps) FormAction() string {
	if p.IsEdit {
		return "/settings/sync/schedules/" + p.ShortID
	}
	return "/settings/sync/schedules"
}

// DrawerID is the $store.drawers key for this form's drawer: "schedule-new" for
// create, "schedule-<shortID>" for edit. Buttons open the matching drawer.
func (p ScheduleFormProps) DrawerID() string {
	if p.IsEdit {
		return "schedule-" + p.ShortID
	}
	return "schedule-new"
}

// connectionLabel renders a connection's display name for the target picker.
func connectionLabel(c service.ConnectionResponse) string {
	name := ""
	if c.InstitutionName != nil && *c.InstitutionName != "" {
		name = *c.InstitutionName
	} else {
		name = c.Provider
	}
	if c.UserName != nil && *c.UserName != "" {
		name += " · " + *c.UserName
	}
	return name
}
