//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// registerRules wires the read-only rules routes onto the authenticated subrouter.
func (h *Handler) registerRules(r chi.Router) {
	r.Get("/rules", h.rulesList)
	r.Get("/rules/{id}", h.ruleDetail)
}

// rulesList renders every transaction rule as a table.
func (h *Handler) rulesList(w http.ResponseWriter, r *http.Request) {
	result, err := h.app.Service.ListTransactionRules(r.Context(), service.TransactionRuleListParams{
		Page:     1,
		PageSize: 200,
		SortBy:   "priority",
	})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.RulesList(h.shellData(r, "Rules"), result.Rules))
}

// ruleDetail renders one rule with its conditions and actions.
func (h *Handler) ruleDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.app.Service.GetTransactionRule(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.RuleDetail(h.shellData(r, rule.Name), rule))
}
