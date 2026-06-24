//go:build !lite

package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListSeriesHandler returns recurring series (thin, rule-maintained).
// GET /api/v1/series — mirrors the list_series MCP tool.
func ListSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		series, err := svc.ListSeries(r.Context(), nil)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list series")
			return
		}
		writeData(w, map[string]any{"series": series})
	}
}

// GetSeriesHandler returns a single series by short_id or uuid.
// GET /api/v1/series/{id} — mirrors the get_series MCP tool.
func GetSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		s, err := svc.GetSeries(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to get series")
			return
		}
		writeData(w, s)
	}
}

// assignSeriesRequest is the body for POST /api/v1/series — link transactions to
// a series, creating it if needed. Provide series_id to assign to an existing
// series, or series_name + create_if_missing to mint/resolve one by name.
type assignSeriesRequest struct {
	SeriesID        *string  `json:"series_id,omitempty"`
	SeriesName      string   `json:"series_name,omitempty"`
	CreateIfMissing bool     `json:"create_if_missing,omitempty"`
	Type            string   `json:"type,omitempty"`
	TransactionIDs  []string `json:"transaction_ids,omitempty"`
}

// linkSeriesRequest is the body for POST /api/v1/series/{id}/transactions.
type linkSeriesRequest struct {
	TransactionIDs []string `json:"transaction_ids"`
}

// AssignSeriesHandler links transactions to a series (or mints one).
// POST /api/v1/series — mirrors the assign_series MCP tool. Requires
// full_access scope (write).
func AssignSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body assignSeriesRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		actor := service.ActorFromContext(r.Context())
		s, err := svc.AssignSeries(r.Context(), service.AssignSeriesInput{
			SeriesID:        body.SeriesID,
			Name:            body.SeriesName,
			CreateIfMissing: body.CreateIfMissing,
			Type:            body.Type,
			TransactionIDs:  body.TransactionIDs,
		}, actor)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to assign series")
			return
		}
		writeData(w, s)
	}
}

// LinkSeriesTransactionsHandler links transactions to an existing series.
// POST /api/v1/series/{id}/transactions. Requires full_access scope (write).
func LinkSeriesTransactionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body linkSeriesRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		actor := service.ActorFromContext(r.Context())
		s, err := svc.AssignSeries(r.Context(), service.AssignSeriesInput{
			SeriesID:       &id,
			TransactionIDs: body.TransactionIDs,
		}, actor)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to link transactions")
			return
		}
		writeData(w, s)
	}
}

// patchSeriesRequest is the body for PATCH /api/v1/series/{id} — edit a thin
// series' name and/or type. An empty body is rejected.
type patchSeriesRequest struct {
	Name *string `json:"name,omitempty"`
	Type *string `json:"type,omitempty"`
}

// PatchSeriesHandler edits a series' name and/or type. PATCH /api/v1/series/{id}
// — mirrors the update_series MCP tool. Requires full_access scope (write).
func PatchSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body patchSeriesRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Name == nil && body.Type == nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"provide at least one of name or type")
			return
		}
		actor := service.ActorFromContext(r.Context())
		s, err := svc.UpdateSeries(r.Context(), id, service.EditSeriesInput{
			Name: body.Name,
			Type: body.Type,
		}, actor)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to update series")
			return
		}
		writeData(w, s)
	}
}

// UnlinkSeriesTransactionHandler detaches a single transaction from a series.
// DELETE /api/v1/series/{id}/transactions/{txid} — mirrors the
// unlink_series_transactions MCP tool. Requires full_access scope (write).
func UnlinkSeriesTransactionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		txID := chi.URLParam(r, "txid")
		actor := service.ActorFromContext(r.Context())
		s, err := svc.UnlinkSeriesTransactions(r.Context(), id, []string{txID}, actor)
		if err != nil {
			writeServiceError(w, err, "Series or transaction not found", "Failed to unlink transaction")
			return
		}
		writeData(w, s)
	}
}
