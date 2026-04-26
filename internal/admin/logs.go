package admin

import (
	"net/http"
	"net/url"

	"breadbox/internal/app"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
	"breadbox/internal/timefmt"

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
		}

		props := pages.LogsProps{Tab: tab}

		// Always fetch sync logs data.
		{
			q := r.URL.Query()
			params := service.SyncLogListParams{
				Page:         parsePage(r),
				PageSize:     25,
				ConnectionID: optStrQuery(q, "connection_id"),
				Status:       optStrQuery(q, "status"),
				Trigger:      optStrQuery(q, "trigger"),
				DateFrom:     parseDateParam(r, "date_from"),
				DateTo:       parseInclusiveDateParam(r, "date_to"),
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
			pVals := pickValues(r, []string{
				"connection_id", "status", "trigger", "date_from", "date_to",
			})
			pVals.Set("tab", "syncs")
			pBase := paginationBase("/logs", pVals, "page")

			// Connection filter options.
			connOpts := make([]pages.LogsConnectionOption, 0, len(connections))
			for _, c := range connections {
				id := pgconv.FormatUUID(c.ID)
				connOpts = append(connOpts, pages.LogsConnectionOption{
					ID:       id,
					Name:     c.InstitutionName.String,
					Selected: id == filterConnID,
				})
			}

			// Project sync log rows into the templ view-model with
			// pre-rendered relative timestamps.
			rows := make([]pages.LogsSyncRow, 0, len(result.Logs))
			for _, l := range result.Logs {
				rows = append(rows, pages.LogsSyncRow{
					ID:                   l.ID,
					InstitutionName:      l.InstitutionName,
					Trigger:              l.Trigger,
					Status:               l.Status,
					StartedAtRelative:    timefmt.RelativeRFC3339Ptr(l.StartedAt),
					Duration:             l.Duration,
					AccountsAffected:     l.AccountsAffected,
					FriendlyErrorMessage: l.FriendlyErrorMessage,
					ErrorMessage:         l.ErrorMessage,
					WarningMessage:       l.WarningMessage,
					AddedCount:           l.AddedCount,
					ModifiedCount:        l.ModifiedCount,
					RemovedCount:         l.RemovedCount,
					UnchangedCount:       l.UnchangedCount,
				})
			}

			props.Stats = stats
			props.HasAdvancedFilters = hasAdvancedFilters
			props.Connections = connOpts
			props.FilterConnID = filterConnID
			props.FilterStatus = filterStatus
			props.FilterTrigger = filterTrigger
			props.FilterDateFrom = filterDateFrom
			props.FilterDateTo = filterDateTo
			props.Logs = rows
			props.Total = result.Total
			props.Page = result.Page
			props.TotalPages = result.TotalPages
			props.ShowingStart = showingStart
			props.ShowingEnd = showingEnd
			props.PaginationBase = pBase
			props.SuccessCount = successCount
			props.ErrorCount = errorCount
			props.InProgressCount = inProgressCount

			// Pre-encode the status-tab query strings (mirrors the
			// `syncLogFilterQuery` funcMap helper).
			props.StatusQueryAll = encodeSyncStatusQuery("", filterConnID, filterTrigger, filterDateFrom, filterDateTo)
			props.StatusQuerySuccess = encodeSyncStatusQuery("success", filterConnID, filterTrigger, filterDateFrom, filterDateTo)
			props.StatusQueryError = encodeSyncStatusQuery("error", filterConnID, filterTrigger, filterDateFrom, filterDateTo)
			props.StatusQueryInProgress = encodeSyncStatusQuery("in_progress", filterConnID, filterTrigger, filterDateFrom, filterDateTo)
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
			whVals := pickValues(r, []string{"wh_provider", "wh_status"})
			whVals.Set("tab", "webhooks")
			whPBase := paginationBase("/logs", whVals, "wh_page")

			// Project webhook events into the templ view-model with
			// pre-rendered relative + full timestamps.
			whRows := make([]pages.LogsWebhookRow, 0, len(result.Events))
			for _, e := range result.Events {
				whRows = append(whRows, pages.LogsWebhookRow{
					ID:                e.ID,
					Provider:          e.Provider,
					EventType:         e.EventType,
					Status:            e.Status,
					ConnectionID:      e.ConnectionID,
					InstitutionName:   e.InstitutionName,
					PayloadHash:       e.PayloadHash,
					ErrorMessage:      e.ErrorMessage,
					CreatedAtRelative: timefmt.RelativeRFC3339Ptr(e.CreatedAt),
					CreatedAtFull:     timefmt.FormatRFC3339Ptr(e.CreatedAt, timefmt.LayoutDateTime),
				})
			}

			props.WHEvents = whRows
			props.WHTotal = result.Total
			props.WHPage = result.Page
			props.WHTotalPages = result.TotalPages
			props.WHShowingStart = whShowingStart
			props.WHShowingEnd = whShowingEnd
			props.WHPaginationBase = whPBase
			props.WHStats = whStats
			props.WHFilterProvider = whFilterProvider
			props.WHFilterStatus = whFilterStatus
		}

		renderLogs(w, r, tr, data, props)
	}
}

// renderLogs mirrors the renderSettings / renderPromptBuilder pattern:
// it hands the typed LogsProps to the templ component and uses
// RenderWithTempl to host it inside base.html.
func renderLogs(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.LogsProps) {
	tr.RenderWithTempl(w, r, data, pages.Logs(props))
}

// encodeSyncStatusQuery mirrors the `syncLogFilterQuery` funcMap helper:
// builds a url.Values-encoded query string (no leading "?") from the
// non-empty filter values.
func encodeSyncStatusQuery(status, connID, trigger, dateFrom, dateTo string) string {
	v := url.Values{}
	if status != "" {
		v.Set("status", status)
	}
	if connID != "" {
		v.Set("connection_id", connID)
	}
	if trigger != "" {
		v.Set("trigger", trigger)
	}
	if dateFrom != "" {
		v.Set("date_from", dateFrom)
	}
	if dateTo != "" {
		v.Set("date_to", dateTo)
	}
	return v.Encode()
}

