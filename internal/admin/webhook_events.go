package admin

import (
	"net/http"
	"strconv"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// WebhookEventsHandler serves GET /admin/webhook-events.
func WebhookEventsHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		params := service.WebhookEventListParams{
			Page:     page,
			PageSize: 25,
		}

		if prov := r.URL.Query().Get("provider"); prov != "" {
			params.Provider = &prov
		}
		if status := r.URL.Query().Get("status"); status != "" {
			params.Status = &status
		}

		result, err := svc.ListWebhookEventsPaginated(ctx, params)
		if err != nil {
			a.Logger.Error("list webhook events", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		stats, err := svc.WebhookEventCounts(ctx)
		if err != nil {
			a.Logger.Error("webhook event counts", "error", err)
			stats = &service.WebhookEventStats{}
		}

		filterProvider := ""
		if params.Provider != nil {
			filterProvider = *params.Provider
		}
		filterStatus := ""
		if params.Status != nil {
			filterStatus = *params.Status
		}

		data := map[string]any{
			"PageTitle":      "Webhook Events",
			"CurrentPage":    "webhook-events",
			"CSRFToken":      GetCSRFToken(r),
			"Flash":          GetFlash(ctx, sm),
			"Events":         result.Events,
			"Page":           result.Page,
			"TotalPages":     result.TotalPages,
			"Total":          result.Total,
			"Stats":          stats,
			"FilterProvider": filterProvider,
			"FilterStatus":   filterStatus,
		}
		tr.Render(w, r, "webhook_events.html", data)
	}
}
