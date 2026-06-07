//go:build !headless && !lite

package admin

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// buildAgentsListStatus maps the service readiness struct to the props
// shape. Nil-safe: a nil status (shouldn't happen in practice — the
// service method always returns a value) renders as "not ready".
func buildAgentsListStatus(s *service.AgentSubsystemStatus) pages.AgentSubsystemStatusProps {
	if s == nil {
		return pages.AgentSubsystemStatusProps{}
	}
	return pages.AgentSubsystemStatusProps{
		Ready:          s.Ready,
		AuthConfigured: s.AuthConfigured,
		BinaryPresent:  s.BinaryPresent,
		BinaryPath:     s.BinaryPath,
	}
}

// buildCustomWorkflowCards maps the household's hand-authored workflows
// (source_template IS NULL) into gallery cards for the Custom section.
// Preset-instantiated definitions are skipped — they render in the preset
// gallery above.
func buildCustomWorkflowCards(defs []service.AgentDefinitionResponse) []pages.WorkflowCustomCardProps {
	out := make([]pages.WorkflowCustomCardProps, 0, len(defs))
	for _, d := range defs {
		if d.SourceTemplate != nil {
			continue // preset-backed → handled by the preset gallery
		}
		card := pages.WorkflowCustomCardProps{
			Slug:        d.Slug,
			Name:        d.Name,
			Description: firstPromptLine(d.Prompt, 120),
			Enabled:     d.Enabled,
		}
		if d.AvatarSeed != nil {
			card.AvatarSeed = *d.AvatarSeed
		}
		if d.LastRun != nil && d.LastRun.Status == "error" {
			card.LastRunError = true
		}
		out = append(out, card)
	}
	return out
}

// firstPromptLine returns the first non-empty line of a prompt, truncated to
// max runes with an ellipsis. Drives the custom card's one-line description.
func firstPromptLine(s string, max int) string {
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > max {
			return string(r[:max]) + "…"
		}
		return line
	}
	return ""
}

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

// buildWorkflowLastRun maps a service run summary (string RFC3339 timestamps)
// into the gallery card's last-run view. Prefers CompletedAt; falls back to
// StartedAt for an in-progress run so the relative time still renders. A
// timestamp that won't parse leaves FinishedAt zero — workflowsRelativeTime
// then renders an empty string, which is fine (the status pill still shows).
func buildWorkflowLastRun(run *service.AgentRunSummary) *pages.WorkflowLastRunProps {
	if run == nil {
		return nil
	}
	out := &pages.WorkflowLastRunProps{
		ShortID: run.ShortID,
		Status:  run.Status,
	}
	when := run.StartedAt
	if run.CompletedAt != nil && *run.CompletedAt != "" {
		when = *run.CompletedAt
	}
	if t, err := time.Parse(time.RFC3339, when); err == nil {
		out.FinishedAt = t
	}
	return out
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
			lastRuns   map[string]*service.AgentRunSummary
			defs       []service.AgentDefinitionResponse
			wg         sync.WaitGroup
		)
		wg.Add(6)
		go func() { defer wg.Done(); presets, presetsErr = svc.ListWorkflowPresets(ctx) }()
		go func() { defer wg.Done(); status = svc.GetAgentSubsystemStatus(ctx) }()
		go func() { defer wg.Done(); consented = svc.WorkflowsConsentAcknowledged(ctx) }()
		go func() { defer wg.Done(); spend, _ = svc.HouseholdSpendStatus(ctx) }()
		// Last-run summaries are a soft enrichment: a query hiccup drops the
		// per-card "Last run" line but doesn't fail the gallery.
		go func() { defer wg.Done(); lastRuns, _ = svc.GetEnabledWorkflowLastRuns(ctx) }()
		// Definitions back the Custom section (source_template IS NULL). A
		// failure just drops the section — the preset gallery still renders.
		go func() { defer wg.Done(); defs, _ = svc.ListAgentDefinitions(ctx) }()
		wg.Wait()

		if presetsErr != nil {
			http.Error(w, "failed to load workflow presets", http.StatusInternalServerError)
			return
		}

		props := pages.WorkflowsGalleryProps{
			Categories:          groupWorkflowPresets(presets, lastRuns),
			Status:              buildAgentsListStatus(status),
			CSRFToken:           GetCSRFToken(r),
			ConsentAcknowledged: consented,
			Spend:               buildWorkflowSpendBanner(spend),
			IsAdmin:             IsAdmin(sm, r),
			Custom:              buildCustomWorkflowCards(defs),
		}

		data := BaseTemplateData(r, sm, "workflows", "Workflows")
		tr.RenderWithTempl(w, r, data, pages.WorkflowsGallery(props))
	}
}

// groupWorkflowPresets buckets presets into category sections, preserving the
// registry order for both categories and presets within them. lastRuns maps an
// instantiated workflow's slug → its most-recent run summary (nil/absent when a
// preset isn't enabled or has never run); it drives the per-card "Last run"
// line.
func groupWorkflowPresets(views []service.WorkflowPresetView, lastRuns map[string]*service.AgentRunSummary) []pages.WorkflowCategoryProps {
	order := make([]string, 0)
	byCat := make(map[string][]pages.WorkflowPresetCardProps)
	for _, v := range views {
		if _, seen := byCat[v.Category]; !seen {
			order = append(order, v.Category)
		}
		model := v.Model
		if model == "" {
			model = service.DefaultAgentModel
		}
		maxTurns := v.MaxTurns
		if maxTurns == 0 {
			maxTurns = service.DefaultAgentMaxTurns
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
			Model:            model,
			MaxTurns:         maxTurns,
			MaxBudgetUSD:     v.MaxBudgetUSD,
			OneOff:           v.OneOff,
			Enabled:          v.Enabled,
		}
		if v.WorkflowSlug != nil {
			card.WorkflowSlug = *v.WorkflowSlug
			if run := lastRuns[*v.WorkflowSlug]; run != nil {
				card.LastRun = buildWorkflowLastRun(run)
			}
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
	case "Setup & Bulk":
		return "rocket"
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
	if v.OneOff {
		return "On demand"
	}
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
