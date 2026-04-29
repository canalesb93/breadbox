package admin

import (
	"fmt"
	"net/http"

	"breadbox/internal/prompts"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// AgentsPageHandler serves GET /admin/agent-prompts — the Agent Prompts library.
//
// Sections, cards and block composition are read from prompts/agents.yaml
// (see internal/prompts.ListSections / ListAgents). Stats counters
// (pending reviews, active rules, account coverage) are resolved here so
// the templ stays purely presentational.
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

		stats := pages.AgentWizardStatsProps{
			PendingReviews: pendingReviews,
			TotalRules:     ruleCount,
			TotalAccounts:  totalAccounts,
		}

		sections, err := buildAgentWizardSections(stats)
		if err != nil {
			http.Error(w, "Failed to load agent catalogue: "+err.Error(), http.StatusInternalServerError)
			return
		}

		props := pages.AgentWizardProps{
			Stats:    stats,
			Sections: sections,
		}
		tr.RenderWithTempl(w, r, data, pages.AgentWizard(props))
	}
}

// buildAgentWizardSections projects prompts/agents.yaml + live stats into
// the templ's section/card shape. Sections are returned in declaration
// order, with cards filtered to those whose `section:` matches.
func buildAgentWizardSections(stats pages.AgentWizardStatsProps) ([]pages.AgentWizardSection, error) {
	sections, err := prompts.ListSections()
	if err != nil {
		return nil, err
	}
	agents, err := prompts.ListAgents()
	if err != nil {
		return nil, err
	}

	out := make([]pages.AgentWizardSection, 0, len(sections))
	for _, s := range sections {
		section := pages.AgentWizardSection{
			ID:               s.ID,
			Title:            s.Title,
			ShowPendingCount: s.ShowPendingCount,
		}
		for _, a := range agents {
			if a.Section != s.ID {
				continue
			}
			section.Cards = append(section.Cards, agentCardProps(a, stats))
		}
		out = append(out, section)
	}
	return out, nil
}

// agentCardProps resolves a single agent + stats into the templ card shape,
// pre-rendering the dynamic counter / warning text.
func agentCardProps(a prompts.Agent, stats pages.AgentWizardStatsProps) pages.AgentWizardCard {
	card := pages.AgentWizardCard{
		Slug:        a.Slug,
		Title:       a.Label,
		Body:        a.Body,
		Icon:        a.Icon,
		Color:       a.Color,
		Badge:       a.Badge,
		BadgeViolet: a.BadgeStyle == "violet",
		Layout:      a.Layout,
	}

	switch a.Counter {
	case "pending_reviews":
		if stats.PendingReviews > 0 {
			card.Counter = fmt.Sprintf("%d waiting", stats.PendingReviews)
			card.CounterMono = true
		}
	case "active_rules":
		if stats.TotalRules > 0 {
			card.Counter = fmt.Sprintf("%d active rule%s", stats.TotalRules, plural(stats.TotalRules))
		}
	case "account_coverage":
		if stats.TotalAccounts > 0 {
			card.Counter = fmt.Sprintf("Across %d account%s", stats.TotalAccounts, plural(stats.TotalAccounts))
		}
	}

	if a.WarnNoRules && stats.TotalRules == 0 {
		card.Warning = "No rules yet — run Initial Setup first"
		card.Counter = "" // warning replaces counter
	}

	return card
}

func plural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}
