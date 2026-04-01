package admin

import (
	"encoding/json"
	"net/http"

	"breadbox/internal/prompts"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// PromptCopyHandler serves GET /admin/agent-wizard/{type}/copy — returns the default composed prompt as plain text.
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

// PromptBuilderHandler serves GET /admin/agent-wizard/{type}.
func PromptBuilderHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentType := chi.URLParam(r, "type")

		cfg, ok := prompts.GetAgentConfig(agentType)
		if !ok {
			tr.Render(w, r, "404.html", BaseTemplateData(r, sm, "", "Not Found"))
			return
		}

		blocks, err := prompts.LoadAgentBlocks(agentType)
		if err != nil {
			http.Error(w, "Failed to load blocks", http.StatusInternalServerError)
			return
		}

		blocksJSON, _ := json.Marshal(blocks)

		data := BaseTemplateData(r, sm, "agents", cfg.Label+" — Prompt Library")
		data["AgentType"] = agentType
		data["AgentLabel"] = cfg.Label
		data["AgentDescription"] = cfg.Description
		data["AgentIcon"] = cfg.Icon
		data["AgentColor"] = cfg.Color
		data["BlocksJSON"] = string(blocksJSON)

		tr.Render(w, r, "prompt_builder.html", data)
	}
}
