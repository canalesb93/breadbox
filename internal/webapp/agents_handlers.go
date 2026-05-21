//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// registerAgents wires the read-only agents + runs routes onto the authenticated
// subrouter. The static /agents/runs (all-runs) route is registered before the
// dynamic /agents/{slug}/runs so chi resolves it unambiguously.
func (h *Handler) registerAgents(r chi.Router) {
	r.Get("/agents", h.agentsList)
	r.Get("/agents/runs", h.allAgentRuns)
	r.Get("/agents/{slug}/runs", h.agentRuns)
}

// agentsList renders all agent definitions as cards.
func (h *Handler) agentsList(w http.ResponseWriter, r *http.Request) {
	agents, err := h.app.Service.ListAgentDefinitions(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AgentsList(h.shellData(r, "Agents"), agents))
}

// agentRuns renders the run history for one agent definition.
func (h *Handler) agentRuns(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	agent, err := h.app.Service.GetAgentDefinition(r.Context(), slug)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	result, err := h.app.Service.ListAgentRuns(r.Context(), slug, service.AgentRunListParams{Limit: 100})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AgentRuns(h.shellData(r, agent.Name+" runs"), agent, result.Runs))
}

// allAgentRuns renders the cross-agent run history.
func (h *Handler) allAgentRuns(w http.ResponseWriter, r *http.Request) {
	result, err := h.app.Service.ListAllAgentRuns(r.Context(), service.AllAgentRunListParams{Limit: 100})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AllAgentRuns(h.shellData(r, "All runs"), result.Runs))
}
