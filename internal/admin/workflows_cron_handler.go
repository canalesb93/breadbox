//go:build !headless && !lite

package admin

import (
	"net/http"

	"breadbox/internal/service"
)

// WorkflowCronPreviewAdminHandler handles
// GET /-/workflows/cron-preview?cron=<expr>&tz=<IANA>.
// It returns {valid, description} for the configure drawer's live schedule
// preview — a human-readable rendering of a cron expression (e.g.
// "At 08:00 AM, only on Monday"). When a `tz` (the viewer's IANA timezone)
// is supplied, the times are localized to it, since the scheduler fires cron
// in the server's timezone. Read-only; validates with the same parser the
// scheduler uses so the preview agrees with what will actually run.
func WorkflowCronPreviewAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		valid, desc := svc.DescribeCronInTZ(q.Get("cron"), q.Get("tz"))
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":       valid,
			"description": desc,
		})
	}
}
