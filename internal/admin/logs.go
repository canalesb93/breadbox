package admin

import (
	"net/http"
	"strconv"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// LogsPageHandler serves GET /admin/logs — combined sync logs + webhook events.
func LogsPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tab := r.URL.Query().Get("tab")
		if tab != "webhooks" {
			tab = "syncs"
		}

		data := map[string]any{
			"PageTitle":   "Logs",
			"CurrentPage": "logs",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
			"Tab":         tab,
		}

		// Always fetch sync logs data.
		{
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}

			params := service.SyncLogListParams{
				Page:     page,
				PageSize: 25,
			}

			if connID := r.URL.Query().Get("connection_id"); connID != "" {
				params.ConnectionID = &connID
			}
			if status := r.URL.Query().Get("status"); status != "" {
				params.Status = &status
			}
			if trigger := r.URL.Query().Get("trigger"); trigger != "" {
				params.Trigger = &trigger
			}
			if v := r.URL.Query().Get("date_from"); v != "" {
				if t, err := time.Parse("2006-01-02", v); err == nil {
					params.DateFrom = &t
				}
			}
			if v := r.URL.Query().Get("date_to"); v != "" {
				if t, err := time.Parse("2006-01-02", v); err == nil {
					t = t.AddDate(0, 0, 1)
					params.DateTo = &t
				}
			}

			result, err := svc.ListSyncLogsPaginated(ctx, params)
			if err != nil {
				a.Logger.Error("list sync logs", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			connections, err := a.Queries.ListBankConnections(ctx)
			if err != nil {
				a.Logger.Error("list bank connections for sync log filters", "error", err)
			}

			filterConnID := stringOrEmpty(params.ConnectionID)
			filterStatus := stringOrEmpty(params.Status)
			filterTrigger := stringOrEmpty(params.Trigger)
			filterDateFrom := r.URL.Query().Get("date_from")
			filterDateTo := r.URL.Query().Get("date_to")

			baseParams := service.SyncLogListParams{
				ConnectionID: params.ConnectionID,
				Trigger:      params.Trigger,
				DateFrom:     params.DateFrom,
				DateTo:       params.DateTo,
			}

			stats, err := svc.SyncLogStats(ctx, baseParams)
			if err != nil {
				a.Logger.Error("sync log stats", "error", err)
				stats = &service.SyncLogStats{}
			}

			successStatus := "success"
			errorStatus := "error"
			inProgressStatus := "in_progress"
			successParams := baseParams
			successParams.Status = &successStatus
			errorParams := baseParams
			errorParams.Status = &errorStatus
			inProgressParams := baseParams
			inProgressParams.Status = &inProgressStatus

			successCount, _ := svc.CountSyncLogsFiltered(ctx, successParams)
			errorCount, _ := svc.CountSyncLogsFiltered(ctx, errorParams)
			inProgressCount, _ := svc.CountSyncLogsFiltered(ctx, inProgressParams)

			hasAdvancedFilters := filterConnID != "" || filterTrigger != "" || filterDateFrom != "" || filterDateTo != ""

			data["Logs"] = result.Logs
			data["Connections"] = connections
			data["FilterConnID"] = filterConnID
			data["FilterStatus"] = filterStatus
			data["FilterTrigger"] = filterTrigger
			data["FilterDateFrom"] = filterDateFrom
			data["FilterDateTo"] = filterDateTo
			data["HasAdvancedFilters"] = hasAdvancedFilters
			data["Page"] = result.Page
			data["TotalPages"] = result.TotalPages
			data["Total"] = result.Total
			data["Stats"] = stats
			data["SuccessCount"] = successCount
			data["ErrorCount"] = errorCount
			data["InProgressCount"] = inProgressCount
			data["WarningCount"] = stats.WarningCount
		}

		// Always fetch webhook events data.
		{
			page, _ := strconv.Atoi(r.URL.Query().Get("wh_page"))
			if page < 1 {
				page = 1
			}

			params := service.WebhookEventListParams{
				Page:     page,
				PageSize: 25,
			}

			if prov := r.URL.Query().Get("wh_provider"); prov != "" {
				params.Provider = &prov
			}
			if status := r.URL.Query().Get("wh_status"); status != "" {
				params.Status = &status
			}

			result, err := svc.ListWebhookEventsPaginated(ctx, params)
			if err != nil {
				a.Logger.Error("list webhook events", "error", err)
				result = &service.WebhookEventListResult{}
			}

			whStats, err := svc.WebhookEventCounts(ctx)
			if err != nil {
				a.Logger.Error("webhook event counts", "error", err)
				whStats = &service.WebhookEventStats{}
			}

			whFilterProvider := ""
			if params.Provider != nil {
				whFilterProvider = *params.Provider
			}
			whFilterStatus := ""
			if params.Status != nil {
				whFilterStatus = *params.Status
			}

			data["WHEvents"] = result.Events
			data["WHPage"] = result.Page
			data["WHTotalPages"] = result.TotalPages
			data["WHTotal"] = result.Total
			data["WHStats"] = whStats
			data["WHFilterProvider"] = whFilterProvider
			data["WHFilterStatus"] = whFilterStatus
		}

		tr.Render(w, r, "logs.html", data)
	}
}
