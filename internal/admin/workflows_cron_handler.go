//go:build !headless && !lite

package admin

import (
	"net/http"

	"breadbox/internal/service"
)

// WorkflowCronPreviewAdminHandler handles GET /-/workflows/cron-preview?cron=<expr>.
// It returns {valid, description} for the configure drawer's live schedule
// preview — a human-readable rendering of a cron expression (e.g.
// "At 08:00 AM, only on Monday"). Read-only; validates with the same parser
// the scheduler uses so the preview agrees with what will actually run.
func WorkflowCronPreviewAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		valid, desc := svc.DescribeCron(r.URL.Query().Get("cron"))
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":       valid,
			"description": desc,
		})
	}
}
