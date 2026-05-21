//go:build !headless && !lite

package webapp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/webapp/pages"
)

// providerOrder is the deterministic display order, mirroring the REST
// providers registry (internal/api/providers.go).
var providerOrder = []string{"plaid", "teller", "csv"}

// registerProviders wires the providers read surface onto the authenticated
// /app subrouter.
func (h *Handler) registerProviders(r chi.Router) {
	r.Get("/providers", h.providersList)
}

// providersList renders one card per provider with its configured state and a
// health summary. Configured state comes from the live provider registry
// (CSV is always available); counts + last-sync come from the service layer.
func (h *Handler) providersList(w http.ResponseWriter, r *http.Request) {
	health, err := h.app.Service.GetProviderHealthSummaries(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	cards := make([]pages.ProviderCard, 0, len(providerOrder))
	for _, name := range providerOrder {
		card := pages.ProviderCard{
			Name:       name,
			Configured: h.providerConfigured(name),
		}
		if s := health[name]; s != nil {
			card.ConnectionCount = s.ConnectionCount
			card.AccountCount = s.AccountCount
			card.LastSyncStatus = s.LastSyncStatus
			card.LastSyncTime = s.LastSyncTime
			card.LastSyncError = s.LastSyncError
		}
		cards = append(cards, card)
	}

	render(w, r, http.StatusOK, pages.ProvidersList(h.shellData(r, "Providers"), cards))
}

// providerConfigured mirrors isProviderConfigured in internal/api/providers.go:
// CSV is always available (it's an import path, no credentials), and the
// external providers are configured when a live instance is registered.
func (h *Handler) providerConfigured(name string) bool {
	if name == "csv" {
		return true
	}
	_, ok := h.app.Providers[name]
	return ok
}
