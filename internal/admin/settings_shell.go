package admin

import (
	"net/http"

	"breadbox/internal/templates/components/pages"

	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
)

// renderSettingsTab wraps the given tab body in the unified Settings shell
// and hands the composed component off to TemplateRenderer.RenderWithTempl.
// Pages migrated to the shell call this instead of RenderWithTempl directly,
// keeping the rail role-gated from one place — IsAdmin/IsEditor are derived
// from the live session, not from the data map (handlers that don't go
// through BaseTemplateData would otherwise miss those keys).
func renderSettingsTab(
	tr *TemplateRenderer,
	w http.ResponseWriter,
	r *http.Request,
	sm *scs.SessionManager,
	data map[string]any,
	tab string,
	body templ.Component,
) {
	role := SessionRole(sm, r)
	layoutProps := pages.SettingsLayoutProps{
		CurrentTab: tab,
		IsAdmin:    role == RoleAdmin,
		IsEditor:   role == RoleAdmin || role == RoleEditor,
	}
	tr.RenderWithTempl(w, r, data, pages.SettingsLayout(layoutProps, body))
}
