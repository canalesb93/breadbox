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
	Type            string   `json:"type,omitempty"`
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
			Type:            body.Type,
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

// addSeriesTagRequest is the body for POST /api/v1/series/{id}/tags.
type addSeriesTagRequest struct {
	TagSlug string `json:"tag_slug"`
}

// AddSeriesTagHandler attaches a tag to a series; the tag is materialized onto
// the series' linked transactions. POST /api/v1/series/{id}/tags (write).
func AddSeriesTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body addSeriesTagRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.TagSlug) == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "tag_slug is required")
			return
		}
		actor := service.ActorFromContext(r.Context())
		if err := svc.AddSeriesTag(r.Context(), id, body.TagSlug, actor); err != nil {
			writeServiceError(w, err, "Series or tag not found", "Failed to add series tag")
			return
		}
		s, err := svc.GetSeries(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to load series")
			return
		}
		writeData(w, s)
	}
}

// RemoveSeriesTagHandler detaches a tag from a series and strips the
// series-inherited copies from its members. DELETE /api/v1/series/{id}/tags/{slug} (write).
func RemoveSeriesTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		slug := chi.URLParam(r, "slug")
		if err := svc.RemoveSeriesTag(r.Context(), id, slug); err != nil {
			writeServiceError(w, err, "Series or tag not found", "Failed to remove series tag")
			return
		}
		s, err := svc.GetSeries(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to load series")
			return
		}
		writeData(w, s)
	}
}

// rekeySeriesRequest is the body for POST /api/v1/series/{id}/rekey.
type rekeySeriesRequest struct {
	NewMerchantKey string `json:"new_merchant_key"`
}

// RekeySeriesHandler corrects a series' merchant_key (and repoints its members).
// POST /api/v1/series/{id}/rekey — mirrors the rekey_series MCP tool (write).
func RekeySeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body rekeySeriesRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.NewMerchantKey) == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "new_merchant_key is required")
			return
		}
		actor := service.ActorFromContext(r.Context())
		s, err := svc.RekeySeries(r.Context(), id, body.NewMerchantKey, actor)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to re-key series")
			return
		}
		writeData(w, s)
	}
}

// splitSeriesRequest is the body for POST /api/v1/series/{id}/split.
type splitSeriesRequest struct {
	NewMerchantKey string   `json:"new_merchant_key"`
	Name           string   `json:"name,omitempty"`
	TransactionIDs []string `json:"transaction_ids"`
}

// SplitSeriesHandler moves a subset of a series' members into a new series.
// POST /api/v1/series/{id}/split — mirrors the split_series MCP tool (write).
func SplitSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body splitSeriesRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.NewMerchantKey) == "" || len(body.TransactionIDs) == 0 {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "new_merchant_key and transaction_ids are required")
			return
		}
		actor := service.ActorFromContext(r.Context())
		s, err := svc.SplitSeries(r.Context(), id, body.TransactionIDs, body.NewMerchantKey, body.Name, actor)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to split series")
			return
		}
		writeData(w, s)
	}
}

// setSeriesTypeRequest is the body for POST /api/v1/series/{id}/type.
type setSeriesTypeRequest struct {
	Type string `json:"type"` // subscription | bill | loan | other
}

// SetSeriesTypeHandler overrides a series' type (sticky correction).
// POST /api/v1/series/{id}/type — mirrors the set_series_type MCP tool (write).
func SetSeriesTypeHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body setSeriesTypeRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		actor := service.ActorFromContext(r.Context())
		s, err := svc.SetSeriesType(r.Context(), id, body.Type, actor)
		if err != nil {
			writeServiceError(w, err, "Series not found", "Failed to set series type")
			return
		}
		writeData(w, s)
	}
}

// patchSeriesRequest is the body for PATCH /api/v1/series/{id} — a partial
// update. `verdict` (optional) adjudicates the lifecycle (confirm|reject|pause|
// cancel); the remaining fields edit the series' user-owned attributes. Any
// subset may be present; edits apply first, then the verdict, so a caller can
// rename-and-confirm in one request. An empty body is rejected.
type patchSeriesRequest struct {
	Verdict         string   `json:"verdict,omitempty"`
	Name            *string  `json:"name,omitempty"`
	ExpectedAmount  *float64 `json:"expected_amount,omitempty"`
	AmountTolerance *float64 `json:"amount_tolerance,omitempty"`
	Currency        *string  `json:"currency,omitempty"`
	Cadence         *string  `json:"cadence,omitempty"`
	ExpectedDay     *int32   `json:"expected_day,omitempty"`
	CategoryID      *string  `json:"category_id,omitempty"`
	UserID          *string  `json:"user_id,omitempty"`
}

func (b patchSeriesRequest) hasEdit() bool {
	return b.Name != nil || b.ExpectedAmount != nil || b.AmountTolerance != nil ||
		b.Currency != nil || b.Cadence != nil || b.ExpectedDay != nil ||
		b.CategoryID != nil || b.UserID != nil
}

// PatchSeriesHandler partially updates a series — edit its user-owned attributes
// and/or apply a lifecycle verdict in one call. PATCH /api/v1/series/{id} —
// mirrors the update_series (edit) + review_series (verdict) MCP tools.
// Requires full_access scope (write).
func PatchSeriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body patchSeriesRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		verdict := service.SeriesVerdict(strings.TrimSpace(body.Verdict))
		hasVerdict := strings.TrimSpace(body.Verdict) != ""
		if hasVerdict {
			switch verdict {
			case service.VerdictConfirm, service.VerdictReject, service.VerdictPause, service.VerdictCancel:
			default:
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
					"verdict must be one of: confirm, reject, pause, cancel")
				return
			}
		}
		if !hasVerdict && !body.hasEdit() {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"provide a verdict and/or at least one editable field")
			return
		}

		actor := service.ActorFromContext(r.Context())

		// Apply edit + verdict atomically in one transaction (PatchSeries), so a
		// combined request like {name, verdict:confirm} can never leave the edit
		// committed while reporting failure.
		var edit *service.EditSeriesInput
		if body.hasEdit() {
			edit = &service.EditSeriesInput{
				Name:            body.Name,
				ExpectedAmount:  body.ExpectedAmount,
				AmountTolerance: body.AmountTolerance,
				Currency:        body.Currency,
				Cadence:         body.Cadence,
				ExpectedDay:     body.ExpectedDay,
				CategoryID:      body.CategoryID,
				UserID:          body.UserID,
			}
		}
		var verdictPtr *service.SeriesVerdict
		if hasVerdict {
			verdictPtr = &verdict
		}
		s, err := svc.PatchSeries(r.Context(), id, edit, verdictPtr, actor)
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
