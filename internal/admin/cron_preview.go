//go:build !headless && !lite

package admin

import (
	"net/http"

	"breadbox/internal/service"
)

// CronPreviewAdminHandler serves GET /-/cron/preview?cron=<expr> — the shared
// live-preview backend for the cron-field component (sync schedules, workflows).
// Returns the validity, English description, and next fire times, all in the
// instance timezone. Read-only; no CSRF needed (GET).
func CronPreviewAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res := svc.CronPreview(r.Context(), r.URL.Query().Get("cron"), 3)
		writeJSON(w, http.StatusOK, res)
	}
}
