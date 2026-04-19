package admin

import (
	"net/http"
	"net/url"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// SyncLogsHandler serves GET /admin/sync-logs.
func SyncLogsHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse query params.
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

		// Fetch paginated sync logs.
		result, err := svc.ListSyncLogsPaginated(ctx, params)
		if err != nil {
			a.Logger.Error("list sync logs", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Fetch connections for the filter dropdown.
		connections, err := a.Queries.ListBankConnections(ctx)
		if err != nil {
			a.Logger.Error("list bank connections for sync log filters", "error", err)
		}

		filterConnID := stringOrEmpty(params.ConnectionID)
		filterStatus := stringOrEmpty(params.Status)
		filterTrigger := stringOrEmpty(params.Trigger)
		filterDateFrom := r.URL.Query().Get("date_from")
		filterDateTo := r.URL.Query().Get("date_to")

		// Build base params for stats/counts (non-status filters apply, status does not).
		baseParams := service.SyncLogListParams{
			ConnectionID: params.ConnectionID,
			Trigger:      params.Trigger,
			DateFrom:     params.DateFrom,
			DateTo:       params.DateTo,
		}

		stats, err := svc.SyncLogStats(ctx, baseParams)
		if err != nil {
			a.Logger.Error("sync log stats", "error", err)
			stats = &service.SyncLogStats{} // graceful fallback
		}

		// Count per-status for the tab badges (include trigger+date but not status).
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

		// Determine whether the advanced filter panel has active filters.
		hasAdvancedFilters := filterConnID != "" || filterTrigger != "" || filterDateFrom != "" || filterDateTo != ""

		pageSize := 25
		showingStart := (result.Page-1)*pageSize + 1
		showingEnd := result.Page * pageSize
		if int64(showingEnd) > result.Total {
			showingEnd = int(result.Total)
		}

		pBase := paginationBase("/sync-logs", map[string]string{
			"connection_id": filterConnID,
			"status":        filterStatus,
			"trigger":       filterTrigger,
			"date_from":     filterDateFrom,
			"date_to":       filterDateTo,
		}, "page")

		data := map[string]any{
			"PageTitle":          "Sync Logs",
			"CurrentPage":        "sync-logs",
			"CSRFToken":          GetCSRFToken(r),
			"Flash":              GetFlash(ctx, sm),
			"Logs":               result.Logs,
			"Connections":        connections,
			"FilterConnID":       filterConnID,
			"FilterStatus":       filterStatus,
			"FilterTrigger":      filterTrigger,
			"FilterDateFrom":     filterDateFrom,
			"FilterDateTo":       filterDateTo,
			"HasAdvancedFilters": hasAdvancedFilters,
			"Page":               result.Page,
			"TotalPages":         result.TotalPages,
			"Total":              result.Total,
			"PageSize":           pageSize,
			"ShowingStart":       showingStart,
			"ShowingEnd":         showingEnd,
			"PaginationBase":     pBase,
			"Stats":              stats,
			"SuccessCount":       successCount,
			"ErrorCount":         errorCount,
			"InProgressCount":    inProgressCount,
			"WarningCount":       stats.WarningCount,
		}
		tr.Render(w, r, "sync_logs.html", data)
	}
}

// SyncLogDetailHandler serves GET /admin/sync-logs/{id}.
func SyncLogDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		syncLogID := chi.URLParam(r, "id")
		if syncLogID == "" {
			http.Error(w, "Missing sync log ID", http.StatusBadRequest)
			return
		}

		// Fetch the sync log details.
		syncLog, err := svc.GetSyncLog(ctx, syncLogID)
		if err != nil {
			a.Logger.Error("get sync log", "id", syncLogID, "error", err)
			http.Error(w, "Sync log not found", http.StatusNotFound)
			return
		}

		// Fetch per-account breakdown.
		accounts, err := svc.ListSyncLogAccounts(ctx, syncLogID)
		if err != nil {
			a.Logger.Error("list sync log accounts", "id", syncLogID, "error", err)
			accounts = nil // graceful fallback
		}

		data := map[string]any{
			"PageTitle":   "Sync Log Detail",
			"CurrentPage": "logs",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
			"Log":         syncLog,
			"Accounts":    accounts,
			"Breadcrumbs": []Breadcrumb{
				{Label: "Logs", Href: "/logs?tab=syncs"},
				{Label: syncLog.InstitutionName},
			},
		}
		tr.Render(w, r, "sync_log_detail.html", data)
	}
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// paginationBase builds a URL query string prefix for pagination links.
// It preserves all current filter params and ends with "&page=" (or "?page=")
// so the template can append the page number directly.
func paginationBase(path string, params map[string]string, pageParam string) string {
	q := url.Values{}
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	q.Del(pageParam) // remove page param so we can append it fresh
	encoded := q.Encode()
	if encoded == "" {
		return path + "?" + pageParam + "="
	}
	return path + "?" + encoded + "&" + pageParam + "="
}
