//go:build !lite

package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListCounterpartiesHandler returns all live counterparties (the canonical,
// cross-provider "other side" of a charge). GET /api/v1/counterparties — mirrors
// the list_counterparties MCP tool.
func ListCounterpartiesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cps, err := svc.ListCounterparties(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list counterparties")
			return
		}
		writeData(w, map[string]any{"counterparties": cps})
	}
}

// GetCounterpartyHandler returns a single counterparty by short_id or uuid.
// GET /api/v1/counterparties/{id} — mirrors the get_counterparty MCP tool.
func GetCounterpartyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		cp, err := svc.GetCounterparty(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "Counterparty not found", "Failed to get counterparty")
			return
		}
		writeData(w, cp)
	}
}

// createCounterpartyRequest is the body for POST /api/v1/counterparties — create
// a counterparty by name with optional enrichment fields.
type createCounterpartyRequest struct {
	Name       string  `json:"name"`
	WebsiteURL *string `json:"website_url,omitempty"`
	LogoURL    *string `json:"logo_url,omitempty"`
	CategoryID *string `json:"category_id,omitempty"`
	MCC        *string `json:"mcc,omitempty"`
}

// CreateCounterpartyHandler creates a counterparty (strict create — a duplicate
// live name is rejected). POST /api/v1/counterparties. Requires full_access scope.
func CreateCounterpartyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createCounterpartyRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Name == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "name is required")
			return
		}
		actor := service.ActorFromContext(r.Context())
		cp, err := svc.AssignCounterparty(r.Context(), service.AssignCounterpartyInput{
			Name:            body.Name,
			CreateIfMissing: true,
			FailIfExists:    true,
		}, actor)
		if err != nil {
			writeServiceError(w, err, "Counterparty not found", "Failed to create counterparty")
			return
		}
		// Apply any enrichment supplied at create time.
		if body.WebsiteURL != nil || body.LogoURL != nil || body.CategoryID != nil || body.MCC != nil {
			enriched, eerr := svc.UpdateCounterparty(r.Context(), cp.ShortID, service.EditCounterpartyInput{
				WebsiteURL: body.WebsiteURL,
				LogoURL:    body.LogoURL,
				CategoryID: body.CategoryID,
				MCC:        body.MCC,
			}, actor)
			if eerr != nil {
				writeServiceError(w, eerr, "Counterparty not found", "Failed to enrich counterparty")
				return
			}
			cp = enriched
		}
		writeData(w, cp)
	}
}

// patchCounterpartyRequest is the body for PATCH /api/v1/counterparties/{id} —
// partial enrichment update. An empty body is rejected by the service.
type patchCounterpartyRequest struct {
	Name       *string `json:"name,omitempty"`
	WebsiteURL *string `json:"website_url,omitempty"`
	LogoURL    *string `json:"logo_url,omitempty"`
	CategoryID *string `json:"category_id,omitempty"`
	MCC        *string `json:"mcc,omitempty"`
}

// PatchCounterpartyHandler enriches a counterparty (name + website/logo/category/
// mcc). PATCH /api/v1/counterparties/{id} — mirrors update_counterparty. Requires
// full_access scope.
func PatchCounterpartyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body patchCounterpartyRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		actor := service.ActorFromContext(r.Context())
		cp, err := svc.UpdateCounterparty(r.Context(), id, service.EditCounterpartyInput{
			Name:       body.Name,
			WebsiteURL: body.WebsiteURL,
			LogoURL:    body.LogoURL,
			CategoryID: body.CategoryID,
			MCC:        body.MCC,
		}, actor)
		if err != nil {
			writeServiceError(w, err, "Counterparty not found", "Failed to update counterparty")
			return
		}
		writeData(w, cp)
	}
}

// assignCounterpartyRequest is the body for POST /api/v1/counterparties/{id}/
// transactions — link transactions to an existing counterparty.
type assignCounterpartyRequest struct {
	TransactionIDs []string `json:"transaction_ids"`
}

// AssignCounterpartyTransactionsHandler links transactions to a counterparty.
// POST /api/v1/counterparties/{id}/transactions. Requires full_access scope.
func AssignCounterpartyTransactionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body assignCounterpartyRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		actor := service.ActorFromContext(r.Context())
		cp, err := svc.AssignCounterparty(r.Context(), service.AssignCounterpartyInput{
			CounterpartyShortID: &id,
			TransactionIDs:      body.TransactionIDs,
		}, actor)
		if err != nil {
			writeServiceError(w, err, "Counterparty not found", "Failed to link transactions")
			return
		}
		writeData(w, cp)
	}
}

// UnlinkCounterpartyTransactionHandler detaches a single transaction from a
// counterparty. DELETE /api/v1/counterparties/{id}/transactions/{txid} — mirrors
// the unlink_counterparty_transactions MCP tool. Requires full_access scope.
func UnlinkCounterpartyTransactionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		txID := chi.URLParam(r, "txid")
		actor := service.ActorFromContext(r.Context())
		cp, err := svc.UnlinkCounterpartyTransactions(r.Context(), id, []string{txID}, actor)
		if err != nil {
			writeServiceError(w, err, "Counterparty or transaction not found", "Failed to unlink transaction")
			return
		}
		writeData(w, cp)
	}
}
