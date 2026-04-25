package admin

import (
	"net/http"
	"net/url"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

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
