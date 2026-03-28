package admin

import (
	"encoding/json"
	"net/http"

	"breadbox/internal/prompts"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

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

		data := BaseTemplateData(r, sm, "agent-wizard", cfg.Label+" — Agent Wizard")
		data["AgentType"] = agentType
		data["AgentLabel"] = cfg.Label
		data["AgentDescription"] = cfg.Description
		data["AgentIcon"] = cfg.Icon
		data["AgentColor"] = cfg.Color
		data["BlocksJSON"] = string(blocksJSON)

		tr.Render(w, r, "prompt_builder.html", data)
	}
}
