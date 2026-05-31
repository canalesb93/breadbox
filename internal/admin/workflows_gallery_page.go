//go:build !headless && !lite

package admin

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// buildWorkflowSpendBanner derives the gallery's spend-ceiling banner from
// the rolling-window spend status. No ceiling (unset or <= 0) → no banner.
// Shown at >= 80% of the ceiling; "Over" once spend reaches it (runs paused).
func buildWorkflowSpendBanner(s service.HouseholdSpendStatus) pages.WorkflowSpendBanner {
	if s.CeilingUSD == nil || *s.CeilingUSD <= 0 {
		return pages.WorkflowSpendBanner{}
	}
	ceiling := *s.CeilingUSD
	pct := int(s.SpentUSD / ceiling * 100)
	over := s.SpentUSD >= ceiling
	return pages.WorkflowSpendBanner{
		Show:       over || pct >= 80,
		Over:       over,
		SpentStr:   "$" + strconv.FormatFloat(s.SpentUSD, 'f', 2, 64),
		CeilingStr: "$" + strconv.FormatFloat(ceiling, 'f', 2, 64),
		PctStr:     strconv.Itoa(pct) + "%",
	}
}

// WorkflowsGalleryPageHandler renders GET /workflows — the codified preset
// gallery. Presets come from the code registry (service.ListWorkflowPresets);
// enabling one instantiates a workflow. Mirrors AgentsListPageHandler.
func WorkflowsGalleryPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var (
			presets    []service.WorkflowPresetView
			presetsErr error
			status     *service.AgentSubsystemStatus
			consented  bool
			spend      service.HouseholdSpendStatus
			wg         sync.WaitGroup
		)
		wg.Add(4)
		go func() { defer wg.Done(); presets, presetsErr = svc.ListWorkflowPresets(ctx) }()
		go func() { defer wg.Done(); status = svc.GetAgentSubsystemStatus(ctx) }()
		go func() { defer wg.Done(); consented = svc.WorkflowsConsentAcknowledged(ctx) }()
		go func() { defer wg.Done(); spend, _ = svc.HouseholdSpendStatus(ctx) }()
		wg.Wait()

		if presetsErr != nil {
			http.Error(w, "failed to load workflow presets", http.StatusInternalServerError)
			return
		}

		props := pages.WorkflowsGalleryProps{
			Categories:          groupWorkflowPresets(presets),
			Status:              buildAgentsListStatus(status),
			CSRFToken:           GetCSRFToken(r),
			ConsentAcknowledged: consented,
			Spend:               buildWorkflowSpendBanner(spend),
			IsAdmin:             IsAdmin(sm, r),
		}

		data := BaseTemplateData(r, sm, "workflows", "Workflows")
		tr.RenderWithTempl(w, r, data, pages.WorkflowsGallery(props))
	}
}

// groupWorkflowPresets buckets presets into category sections, preserving the
// registry order for both categories and presets within them.
func groupWorkflowPresets(views []service.WorkflowPresetView) []pages.WorkflowCategoryProps {
	order := make([]string, 0)
	byCat := make(map[string][]pages.WorkflowPresetCardProps)
	for _, v := range views {
		if _, seen := byCat[v.Category]; !seen {
			order = append(order, v.Category)
		}
		card := pages.WorkflowPresetCardProps{
			Slug:             v.Slug,
			Name:             v.Name,
			Description:      v.Description,
			Icon:             v.Icon,
			TriggerLabel:     presetTriggerLabel(v),
			ToolScope:        v.ToolScope,
			ScheduleCron:     v.ScheduleCron,
			TriggerOnSync:    v.TriggerOnSyncComplete,
			EstCostPerRunUSD: v.EstCostPerRunUSD,
			Enabled:          v.Enabled,
		}
		if v.WorkflowSlug != nil {
			card.WorkflowSlug = *v.WorkflowSlug
		}
		if v.WorkflowEnabled != nil {
			card.WorkflowEnabled = *v.WorkflowEnabled
		}
		for _, opt := range v.Options {
			cardOpt := pages.WorkflowPresetOptionProps{
				Key:     opt.Key,
				Label:   opt.Label,
				Help:    opt.Help,
				Default: opt.Default,
			}
			for _, ch := range opt.Choices {
				cardOpt.Choices = append(cardOpt.Choices, pages.WorkflowPresetChoiceProps{Value: ch.Value, Label: ch.Label})
			}
			card.Options = append(card.Options, cardOpt)
		}
		byCat[v.Category] = append(byCat[v.Category], card)
	}
	out := make([]pages.WorkflowCategoryProps, 0, len(order))
	for _, cat := range order {
		out = append(out, pages.WorkflowCategoryProps{
			Name:    cat,
			Icon:    workflowCategoryIcon(cat),
			Presets: byCat[cat],
		})
	}
	return out
}

func workflowCategoryIcon(category string) string {
	switch category {
	case "Categorization & Review":
		return "sparkles"
	case "Insights & Reports":
		return "bar-chart-3"
	case "Hygiene & Maintenance":
		return "wrench"
	case "Alerts & Anomalies":
		return "bell"
	default:
		return "workflow"
	}
}

// presetTriggerLabel renders a short human-readable trigger summary for a card.
func presetTriggerLabel(v service.WorkflowPresetView) string {
	if v.TriggerOnSyncComplete {
		return "After each sync"
	}
	switch v.ScheduleCron {
	case "":
		return "Manual"
	case "0 7 * * 1":
		return "Weekly"
	case "0 8 1 * *":
		return "Monthly"
	}
	// Daily-ish heuristics for other crons; fall back to a generic label.
	if strings.HasPrefix(v.ScheduleCron, "0 ") && strings.HasSuffix(v.ScheduleCron, " * * *") {
		return "Daily"
	}
	return "Scheduled"
}
