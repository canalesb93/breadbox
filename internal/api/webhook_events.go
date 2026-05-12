package api

import (
	"net/http"
	"strconv"
	"strings"

	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	bbsync "breadbox/internal/sync"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Webhook events REST surface.
//
// The admin dashboard's `/admin/logs?tab=webhooks` already pulls from
// `service.ListWebhookEventsPaginated`. This handler exposes the same data
// under `/api/v1/webhook-events` so the headless `breadbox webhooks tail`
// command can read recent events without scraping HTML.
//
// `/replay` re-enqueues an event for processing by raising a manual sync on
// the connection the event was associated with. The CLI is meant for the
// operator-on-call use case: a webhook arrived, the sync engine choked, and
// we want to re-drive it without crafting an HTTP request by hand.

// listWebhookEventsResponse mirrors the admin model but flattens to the
// `{webhook_events: [...], total, page, page_size, total_pages}` shape that
// matches our paginated-list convention.
type listWebhookEventsResponse struct {
	WebhookEvents []service.WebhookEventRow `json:"webhook_events"`
	Total         int64                     `json:"total"`
	Page          int                       `json:"page"`
	PageSize      int                       `json:"page_size"`
	TotalPages    int                       `json:"total_pages"`
}

// ListWebhookEventsHandler serves GET /api/v1/webhook-events.
func ListWebhookEventsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		params := service.WebhookEventListParams{
			Page:     parseIntQuery(q.Get("page"), 1),
			PageSize: parseIntQuery(q.Get("limit"), 25),
		}
		if params.PageSize > 200 {
			params.PageSize = 200
		}
		if prov := strings.TrimSpace(q.Get("provider")); prov != "" {
			params.Provider = &prov
		}
		if status := strings.TrimSpace(q.Get("status")); status != "" {
			params.Status = &status
		}

		res, err := svc.ListWebhookEventsPaginated(r.Context(), params)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
				"Failed to list webhook events")
			return
		}
		writeJSON(w, http.StatusOK, listWebhookEventsResponse{
			WebhookEvents: res.Events,
			Total:         res.Total,
			Page:          res.Page,
			PageSize:      res.PageSize,
			TotalPages:    res.TotalPages,
		})
	}
}

// parseIntQuery returns the parsed integer or def when missing/invalid.
func parseIntQuery(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// replayWebhookEventResponse reports whether the underlying sync was kicked.
type replayWebhookEventResponse struct {
	WebhookEventID string `json:"webhook_event_id"`
	ConnectionID   string `json:"connection_id,omitempty"`
	Triggered      bool   `json:"triggered"`
	Message        string `json:"message,omitempty"`
}

// ReplayWebhookEventHandler serves POST /api/v1/webhook-events/{id}/replay.
//
// The event row itself isn't re-delivered (we never persist the raw payload,
// only its hash); replay is implemented as a manual sync against the
// associated connection. Events without a connection (e.g., legacy rows or
// future cross-cutting events) return 200 with triggered=false + a message
// so callers can surface a clear "nothing to replay" without parsing
// status codes.
func ReplayWebhookEventHandler(svc *service.Service, syncEngine *bbsync.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "id is required")
			return
		}

		ctx := r.Context()
		// Look the row up via a tiny direct query — there's no service helper
		// for single-row read of a webhook event, and adding one for one
		// caller would be overkill.
		var connID pgtype.UUID
		row := svc.Pool.QueryRow(ctx,
			`SELECT connection_id FROM webhook_events WHERE id = $1`, id)
		err := row.Scan(&connID)
		if err != nil {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Webhook event not found")
			return
		}

		resp := replayWebhookEventResponse{WebhookEventID: id}
		if !connID.Valid {
			resp.Message = "event has no associated connection; nothing to replay"
			writeJSON(w, http.StatusOK, resp)
			return
		}
		resp.ConnectionID = pgconv.FormatUUID(connID)
		if syncEngine == nil {
			resp.Message = "sync engine not available"
			writeJSON(w, http.StatusOK, resp)
			return
		}
		if err := syncEngine.Sync(ctx, connID, db.SyncTriggerWebhook); err != nil {
			mw.WriteError(w, http.StatusBadGateway, "SYNC_FAILED",
				"Failed to trigger sync: "+err.Error())
			return
		}
		resp.Triggered = true
		writeJSON(w, http.StatusOK, resp)
	}
}

