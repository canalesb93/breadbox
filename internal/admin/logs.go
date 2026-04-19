package admin

import (
	"net/http"

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
			params := service.SyncLogListParams{
				Page:     parsePage(r),
				PageSize: 25,
				DateFrom: parseDateParam(r, "date_from"),
				DateTo:   parseInclusiveDateParam(r, "date_to"),
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

			pageSize := 25
			showingStart := (result.Page-1)*pageSize + 1
			showingEnd := result.Page * pageSize
			if int64(showingEnd) > result.Total {
				showingEnd = int(result.Total)
			}
			pBase := paginationBase("/logs", map[string]string{
				"tab":           "syncs",
				"connection_id": filterConnID,
				"status":        filterStatus,
				"trigger":       filterTrigger,
				"date_from":     filterDateFrom,
				"date_to":       filterDateTo,
			}, "page")

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
			data["PageSize"] = pageSize
			data["ShowingStart"] = showingStart
			data["ShowingEnd"] = showingEnd
			data["PaginationBase"] = pBase
			data["Stats"] = stats
			data["SuccessCount"] = successCount
			data["ErrorCount"] = errorCount
			data["InProgressCount"] = inProgressCount
			data["WarningCount"] = stats.WarningCount
		}

		// Always fetch webhook events data.
		{
			params := service.WebhookEventListParams{
				Page:     parsePageKey(r, "wh_page"),
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

			whPageSize := 25
			whShowingStart := (result.Page-1)*whPageSize + 1
			whShowingEnd := result.Page * whPageSize
			if int64(whShowingEnd) > result.Total {
				whShowingEnd = int(result.Total)
			}
			whPBase := paginationBase("/logs", map[string]string{
				"tab":         "webhooks",
				"wh_provider": whFilterProvider,
				"wh_status":   whFilterStatus,
			}, "wh_page")

			data["WHEvents"] = result.Events
			data["WHPage"] = result.Page
			data["WHTotalPages"] = result.TotalPages
			data["WHTotal"] = result.Total
			data["WHPageSize"] = whPageSize
			data["WHShowingStart"] = whShowingStart
			data["WHShowingEnd"] = whShowingEnd
			data["WHPaginationBase"] = whPBase
			data["WHStats"] = whStats
			data["WHFilterProvider"] = whFilterProvider
			data["WHFilterStatus"] = whFilterStatus
		}

		tr.Render(w, r, "logs.html", data)
	}
}
