//go:build !lite

package api

import (
	"net/http"
	"strings"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListSeriesHandler returns recurring series (subscriptions), optionally
// filtered by ?status=active|candidate|paused|cancelled.
// GET /api/v1/series — mirrors the list_series MCP tool.
func ListSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var status *string
		if s := strings.TrimSpace(r.URL.Query().Get("status")); s != "" {
			status = &s
		}
		series, err := svc.ListSeries(r.Context(), status)
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

// assignSeriesRequest is the body for POST /api/v1/series — create a series
// detection missed, or assign transactions to one. Provide series_id to assign
// to an existing series, or merchant_key + create_if_missing to mint one.
type assignSeriesRequest struct {
	SeriesID        *string  `json:"series_id,omitempty"`
	MerchantKey     string   `json:"merchant_key,omitempty"`
	CreateIfMissing bool     `json:"create_if_missing,omitempty"`
	Name            string   `json:"name,omitempty"`
	Cadence         string   `json:"cadence,omitempty"`
	ExpectedAmount  *float64 `json:"expected_amount,omitempty"`
	Currency        *string  `json:"currency,omitempty"`
	CategoryID      *string  `json:"category_id,omitempty"`
	UserID          *string  `json:"user_id,omitempty"`
	TransactionIDs  []string `json:"transaction_ids,omitempty"`
	Confirm         bool     `json:"confirm,omitempty"`
}

// linkSeriesRequest is the body for POST /api/v1/series/{id}/transactions.
type linkSeriesRequest struct {
	TransactionIDs []string `json:"transaction_ids"`
	Confirm        bool     `json:"confirm,omitempty"`
}

// AssignSeriesHandler creates a series (create_if_missing) or assigns
// transactions to one. POST /api/v1/series — mirrors the assign_series MCP
// tool. Requires full_access scope (write).
func AssignSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body assignSeriesRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		actor := service.ActorFromContext(r.Context())
		s, err := svc.AssignSeries(r.Context(), service.AssignSeriesInput{
			SeriesID:        body.SeriesID,
			MerchantKey:     body.MerchantKey,
			CreateIfMissing: body.CreateIfMissing,
			Name:            body.Name,
			Cadence:         body.Cadence,
			ExpectedAmount:  body.ExpectedAmount,
			Currency:        body.Currency,
			CategoryID:      body.CategoryID,
			UserID:          body.UserID,
			TransactionIDs:  body.TransactionIDs,
			Confirm:         body.Confirm,
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
			Confirm:        body.Confirm,
		}, actor)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to link transactions")
			return
		}
		writeData(w, s)
	}
}

// reviewSeriesRequest is the body for PATCH /api/v1/series/{id}.
type reviewSeriesRequest struct {
	Verdict string `json:"verdict"` // confirm | reject | pause | cancel
}

// ReviewSeriesHandler applies a verdict to a series (the agent's + user's
// verdict tool). PATCH /api/v1/series/{id} — mirrors the review_series MCP tool.
// Requires full_access scope (write).
func ReviewSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body reviewSeriesRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		verdict := service.SeriesVerdict(strings.TrimSpace(body.Verdict))
		switch verdict {
		case service.VerdictConfirm, service.VerdictReject, service.VerdictPause, service.VerdictCancel:
		default:
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"verdict must be one of: confirm, reject, pause, cancel")
			return
		}
		actor := service.ActorFromContext(r.Context())
		s, err := svc.ReviewSeries(r.Context(), id, verdict, actor)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to review series")
			return
		}
		writeData(w, s)
	}
}
