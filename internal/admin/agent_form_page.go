//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// AgentFormPageHandler serves GET /agents/new and GET /agents/{slug}/edit.
// Same handler distinguishes by presence of the {slug} URL param — empty
// renders an empty form (new mode), non-empty loads an existing agent and
// pre-fills the form (edit mode). 404 on missing agent.
//
// New mode supports a ?prompt=<urlencoded> query param: when present it
// pre-fills the Prompt textarea. This is the "Save as agent from prompt
// library" hand-off from /agent-prompts — clicking a prompt card deep-links
// here with the rendered prompt body so users can convert ad-hoc prompts
// into scheduled agents.
//
// The POST submit handlers (/-/agents, /-/agents/{slug}/update,
// /-/agents/{slug}/run, /-/agents/{slug}/delete) are wired in a later PR;
// this handler renders the form only.
func AgentFormPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		props := pages.AgentFormProps{
			Mode:         "new",
			Title:        "New agent",
			Form:         defaultAgentFormFields(),
			ModelOptions: pages.DefaultAgentModelOptions(),
			CSRFToken:    GetCSRFToken(r),
		}

		// Edit mode — load the existing agent and overwrite Form with its
		// fields. Slug presence on the URL is the discriminator.
		if slug := chi.URLParam(r, "slug"); slug != "" {
			def, err := svc.GetAgentDefinition(ctx, slug)
			if err != nil {
				if errors.Is(err, service.ErrNotFound) {
					tr.RenderNotFound(w, r)
					return
				}
				tr.RenderError(w, r)
				return
			}
			props.Mode = "edit"
			props.Slug = def.Slug
			props.Title = "Edit agent " + def.Name
			props.Form = agentFormFieldsFromResponse(def)
		} else if pre := r.URL.Query().Get("prompt"); pre != "" {
			// New-mode prompt-library hand-off: ?prompt=<urlencoded> drops
			// the body into the Prompt textarea. Net.URL already decoded
			// the percent-encoding for us; just trim surrounding whitespace
			// so a trailing newline from a copy/paste link doesn't add a
			// blank first line.
			props.Form.Prompt = strings.TrimSpace(pre)
		}

		pageTitle := props.Title
		data := BaseTemplateData(r, sm, "agents", pageTitle)
		data["Breadcrumbs"] = pages.AgentFormBreadcrumbs(props)
		tr.RenderWithTempl(w, r, data, pages.AgentForm(props))
	}
}

// defaultAgentFormFields seeds a fresh new-agent form. Mirrors the
// agent_definitions migration defaults so the form previews what the row
// would look like before the user touches anything: enabled off, model =
// DefaultAgentModel, tool scope = read_write, max_turns = 15,
// max_budget = 1.00 USD.
func defaultAgentFormFields() pages.AgentFormFields {
	return pages.AgentFormFields{
		ToolScope:    "read_write",
		Model:        service.DefaultAgentModel,
		MaxTurns:     15,
		MaxBudgetUSD: service.DefaultAgentMaxBudgetUSD,
		Enabled:      false,
	}
}

// agentFormFieldsFromResponse maps a service.AgentDefinitionResponse into
// the flat form shape. Pointer fields collapse to empty string when nil
// so the input round-trips cleanly.
func agentFormFieldsFromResponse(def *service.AgentDefinitionResponse) pages.AgentFormFields {
	f := pages.AgentFormFields{
		Name:                  def.Name,
		Slug:                  def.Slug,
		Prompt:                def.Prompt,
		ToolScope:             def.ToolScope,
		Model:                 def.Model,
		MaxTurns:              def.MaxTurns,
		Enabled:               def.Enabled,
		TriggerOnSyncComplete: def.TriggerOnSyncComplete,
		AllowedTools:          strings.Join(def.AllowedTools, ", "),
	}
	if def.SystemPrompt != nil {
		f.SystemPrompt = *def.SystemPrompt
	}
	if def.ScheduleCron != nil {
		f.ScheduleCron = *def.ScheduleCron
	}
	if def.QuietHoursStart != nil {
		f.QuietHoursStart = *def.QuietHoursStart
	}
	if def.QuietHoursEnd != nil {
		f.QuietHoursEnd = *def.QuietHoursEnd
	}
	if def.MaxBudgetUSD != nil {
		f.MaxBudgetUSD = *def.MaxBudgetUSD
	}
	return f
}
