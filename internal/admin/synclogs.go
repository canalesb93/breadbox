package admin

import (
	"net/http"
	"strconv"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
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

		// Compute summary stats (unfiltered by status for overview).
		statsParams := service.SyncLogListParams{}
		if params.ConnectionID != nil {
			statsParams.ConnectionID = params.ConnectionID
		}
		stats, err := svc.SyncLogStats(ctx, statsParams)
		if err != nil {
			a.Logger.Error("sync log stats", "error", err)
			stats = &service.SyncLogStats{} // graceful fallback
		}

		// Also count per-status for the tab badges.
		successStatus := "success"
		errorStatus := "error"
		inProgressStatus := "in_progress"
		successParams := service.SyncLogListParams{ConnectionID: params.ConnectionID, Status: &successStatus}
		errorParams := service.SyncLogListParams{ConnectionID: params.ConnectionID, Status: &errorStatus}
		inProgressParams := service.SyncLogListParams{ConnectionID: params.ConnectionID, Status: &inProgressStatus}

		successCount, _ := svc.CountSyncLogsFiltered(ctx, successParams)
		errorCount, _ := svc.CountSyncLogsFiltered(ctx, errorParams)
		inProgressCount, _ := svc.CountSyncLogsFiltered(ctx, inProgressParams)

		data := map[string]any{
			"PageTitle":        "Sync Logs",
			"CurrentPage":      "sync-logs",
			"CSRFToken":        GetCSRFToken(r),
			"Flash":            GetFlash(ctx, sm),
			"Logs":             result.Logs,
			"Connections":      connections,
			"FilterConnID":     filterConnID,
			"FilterStatus":     filterStatus,
			"Page":             result.Page,
			"TotalPages":       result.TotalPages,
			"Total":            result.Total,
			"Stats":            stats,
			"SuccessCount":     successCount,
			"ErrorCount":       errorCount,
			"InProgressCount":  inProgressCount,
		}
		tr.Render(w, r, "sync_logs.html", data)
	}
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
