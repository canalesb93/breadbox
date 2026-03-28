package admin

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
)

// AgentWizardHandler serves GET /admin/agent-wizard.
func AgentWizardHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := BaseTemplateData(r, sm, "agent-wizard", "Agent Wizard")
		tr.Render(w, r, "agent_wizard.html", data)
	}
}
