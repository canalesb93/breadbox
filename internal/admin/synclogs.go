package admin

import (
	"net/http"
	"strconv"
	"time"

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
				// Add one day so the end date is inclusive.
				t = t.AddDate(0, 0, 1)
				params.DateTo = &t
			}
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
			"CurrentPage": "sync-logs",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
			"Log":         syncLog,
			"Accounts":    accounts,
			"Breadcrumbs": []Breadcrumb{
				{Label: "Sync Logs", Href: "/sync-logs"},
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
