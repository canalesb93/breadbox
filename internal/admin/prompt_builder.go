package admin

import (
	"net/http"

	"breadbox/internal/prompts"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// PromptCopyHandler serves GET /admin/agent-prompts/builder/{type}/copy — returns the default composed prompt as plain text.
func PromptCopyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentType := chi.URLParam(r, "type")

		_, ok := prompts.GetAgentConfig(agentType)
		if !ok {
			http.Error(w, "unknown agent type", http.StatusNotFound)
			return
		}

		blocks, err := prompts.LoadAgentBlocks(agentType)
		if err != nil {
			http.Error(w, "failed to load blocks", http.StatusInternalServerError)
			return
		}

		var composed string
		for _, b := range blocks {
			if !b.Enabled {
				continue
			}
			if composed != "" {
				composed += "\n\n"
			}
			composed += b.Content
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(composed))
	}
}

// PromptBuilderHandler serves GET /admin/agent-prompts/builder/{type}.
func PromptBuilderHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentType := chi.URLParam(r, "type")

		cfg, ok := prompts.GetAgentConfig(agentType)
		if !ok {
			tr.RenderNotFound(w, r)
			return
		}

		blocks, err := prompts.LoadAgentBlocks(agentType)
		if err != nil {
			http.Error(w, "Failed to load blocks", http.StatusInternalServerError)
			return
		}

		data := BaseTemplateData(r, sm, "agent-prompts", cfg.Label+" — Prompt Library")
		data["AgentType"] = agentType
		data["AgentLabel"] = cfg.Label
		data["AgentDescription"] = cfg.Description
		data["AgentIcon"] = cfg.Icon
		data["AgentColor"] = cfg.Color

		renderPromptBuilder(w, r, tr, data, pages.PromptBuilderProps{
			AgentType:        agentType,
			AgentLabel:       cfg.Label,
			AgentDescription: cfg.Description,
			AgentIcon:        cfg.Icon,
			AgentColor:       cfg.Color,
			Blocks:           blocks,
		})
	}
}

// renderPromptBuilder mirrors the renderSettings / renderTransactions pattern:
// it hands the typed PromptBuilderProps to the templ component and uses
// RenderWithTempl to host it inside base.html.
func renderPromptBuilder(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.PromptBuilderProps) {
	tr.RenderWithTempl(w, r, data, pages.PromptBuilder(props))
}
