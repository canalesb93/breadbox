package admin

import (
	"net/http"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// AgentWizardStats holds live stats shown on the agent wizard cards.
type AgentWizardStats struct {
	PendingReviews int64
	TotalRules     int64
	TotalAccounts  int64
}

// AgentWizardHandler serves GET /admin/agent-wizard.
func AgentWizardHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		data := BaseTemplateData(r, sm, "agent-wizard", "Agent Wizard")

		var stats AgentWizardStats

		// Fetch pending review count (transactions tagged needs-review).
		if n, err := pendingReviewsCount(ctx, svc); err == nil {
			stats.PendingReviews = n
		}

		// Fetch total active rules.
		enabled := true
		if result, err := svc.ListTransactionRules(ctx, service.TransactionRuleListParams{
			Enabled: &enabled,
			Limit:   1, // We only need the total count.
		}); err == nil {
			stats.TotalRules = result.Total
		}

		// Fetch account count.
		if accounts, err := svc.ListAccounts(ctx, nil); err == nil {
			stats.TotalAccounts = int64(len(accounts))
		}

		data["Stats"] = stats

		tr.Render(w, r, "agent_wizard.html", data)
	}
}
