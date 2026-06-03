//go:build !headless && !lite

package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// devModeCtxKey carries the per-request "is developer mode on" flag from the
// middleware to BaseTemplateData, which gates the floating reporter in
// base.html. A dedicated type avoids collisions with other context keys.
type devModeCtxKey struct{}

// DevModeMiddleware reads the developer-mode flag once per request and stashes
// it in context so the base layout can decide whether to render the reporter.
func DevModeMiddleware(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			enabled := appconfig.Bool(r.Context(), queries, appconfig.KeyDevModeEnabled, false)
			ctx := context.WithValue(r.Context(), devModeCtxKey{}, enabled)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// devModeEnabledFromContext reports whether developer mode was on for this
// request. Defaults to false when the middleware didn't run.
func devModeEnabledFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(devModeCtxKey{}).(bool)
	return v
}

// devReportMaxBody bounds the JSON payload. Without a screenshot/HTML snapshot
// the body is small (title + description + metadata).
const devReportMaxBody = 256 << 10 // 256 KiB

// CreateDevReportAdminHandler handles POST /-/dev-reports — the floating
// reporter's submit. It builds a prefilled GitHub issue-draft URL from the
// report and returns it; the client opens the draft for the user to submit.
// No token, no persistence, no artifact hosting.
func CreateDevReportAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, devReportMaxBody)

		var req struct {
			Type        string         `json:"type"`
			Title       string         `json:"title"`
			Description string         `json:"description"`
			PageURL     string         `json:"page_url"`
			PagePath    string         `json:"page_path"`
			Metadata    map[string]any `json:"metadata"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		title := strings.TrimSpace(req.Title)
		if title == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "A title is required.")
			return
		}

		res, err := svc.CreateDevReport(r.Context(), service.CreateDevReportInput{
			Type:        req.Type,
			Title:       title,
			Description: req.Description,
			PageURL:     req.PageURL,
			PagePath:    req.PagePath,
			Metadata:    req.Metadata,
			CreatedBy:   sm.GetString(r.Context(), sessionKeyAccountUsername),
		})
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", strings.TrimPrefix(err.Error(), "invalid parameter: "))
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to prepare the report.")
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}
