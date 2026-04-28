package admin

import (
	"net/http"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// AgentsPageHandler serves GET /admin/agent-prompts — the Agent Prompts library.
//
// Historically this route was a 4-tab composite (guide / wizard / settings /
// activity). The guide tab was removed, MCP settings moved into the unified
// Settings shell at /settings/mcp, and the activity tab moved to
// /logs?tab=sessions. What remains is a single page rendering the prompt
// wizard cards via the AgentWizard templ component.
func AgentsPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		data := BaseTemplateData(r, sm, "agent-prompts", "Agent Prompts")

		var pendingReviews, ruleCount, totalAccounts int64
		if n, err := pendingReviewsCount(ctx, svc); err == nil {
			pendingReviews = n
		}
		enabled := true
		if result, err := svc.ListTransactionRules(ctx, service.TransactionRuleListParams{Enabled: &enabled, Limit: 1}); err == nil {
			ruleCount = int64(result.Total)
		}
		if accounts, err := svc.ListAccounts(ctx, nil); err == nil {
			totalAccounts = int64(len(accounts))
		}

		props := pages.AgentWizardProps{
			Stats: pages.AgentWizardStatsProps{
				PendingReviews: pendingReviews,
				TotalRules:     ruleCount,
				TotalAccounts:  totalAccounts,
			},
		}
		tr.RenderWithTempl(w, r, data, pages.AgentWizard(props))
	}
}
