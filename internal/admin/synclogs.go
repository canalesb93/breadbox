package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"
	"breadbox/internal/timefmt"

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
			"CurrentPage": "activity",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}

		breadcrumbs := []components.Breadcrumb{
			{Label: "Activity", Href: "/activity?tab=syncs"},
			{Label: syncLog.InstitutionName},
		}

		props := buildSyncLogDetailProps(syncLog, accounts, breadcrumbs)
		renderSyncLogDetail(w, r, tr, data, props)
	}
}

// renderSyncLogDetail mirrors renderLogs / renderSettings: hands the
// typed SyncLogDetailProps to the templ component and uses
// RenderWithTempl to host it inside base.html.
func renderSyncLogDetail(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.SyncLogDetailProps) {
	tr.RenderWithTempl(w, r, data, pages.SyncLogDetail(props))
}

// buildSyncLogDetailProps projects service-layer types into the flat
// view-model the templ renders. Pre-renders relative time and rule
// condition summaries so the templ stays free of funcMap helpers.
func buildSyncLogDetailProps(log *service.SyncLogRow, accounts []service.SyncLogAccountRow, breadcrumbs []components.Breadcrumb) pages.SyncLogDetailProps {
	out := pages.SyncLogDetailProps{
		Breadcrumbs: breadcrumbs,
	}
	if log != nil {
		out.Log = pages.SyncLogDetailLog{
			ID:                log.ID,
			ConnectionID:      log.ConnectionID,
			InstitutionName:   log.InstitutionName,
			Provider:          log.Provider,
			Trigger:           log.Trigger,
			Status:            log.Status,
			AddedCount:        log.AddedCount,
			ModifiedCount:     log.ModifiedCount,
			RemovedCount:      log.RemovedCount,
			UnchangedCount:    log.UnchangedCount,
			ErrorMessage:      stringOrEmpty(log.ErrorMessage),
			WarningMessage:    stringOrEmpty(log.WarningMessage),
			StartedAtRelative: timefmt.RelativeRFC3339Ptr(log.StartedAt),
			Duration:          stringOrEmpty(log.Duration),
			AccountsAffected:  log.AccountsAffected,
			TotalRuleHits:     log.TotalRuleHits,
		}
		if len(log.RuleHits) > 0 {
			out.Log.RuleHits = make([]pages.SyncLogDetailRuleHit, 0, len(log.RuleHits))
			for _, h := range log.RuleHits {
				summary := ""
				if h.Conditions != nil {
					summary = service.ConditionSummary(*h.Conditions)
				}
				out.Log.RuleHits = append(out.Log.RuleHits, pages.SyncLogDetailRuleHit{
					RuleID:           h.RuleID,
					RuleName:         h.RuleName,
					Count:            h.Count,
					ConditionSummary: summary,
				})
			}
		}
	}
	if len(accounts) > 0 {
		out.Accounts = make([]pages.SyncLogDetailAccount, 0, len(accounts))
		for _, a := range accounts {
			out.Accounts = append(out.Accounts, pages.SyncLogDetailAccount{
				AccountName:    a.AccountName,
				AddedCount:     a.AddedCount,
				ModifiedCount:  a.ModifiedCount,
				RemovedCount:   a.RemovedCount,
				UnchangedCount: a.UnchangedCount,
			})
		}
	}
	return out
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

