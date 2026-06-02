//go:build !headless && !lite

package admin

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// devModeCtxKey carries the per-request "is developer mode on" flag from the
// middleware to BaseTemplateData, which gates the floating reporter in
// base.html. A dedicated type avoids collisions with other context keys.
type devModeCtxKey struct{}

// DevModeMiddleware reads the developer-mode flag once per request and stashes
// it in context so the base layout can decide whether to render the reporter.
// One cheap app_config read; mirrors NavBadgesMiddleware's shape.
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

// devReportMaxBody bounds the JSON payload (screenshot + HTML snapshot can be
// a few MB together).
const devReportMaxBody = 24 << 20 // 24 MiB

// CreateDevReportAdminHandler handles POST /-/dev-reports — the floating
// reporter's submit. It decodes the screenshot data URL + HTML snapshot,
// persists the report, and files a labelled GitHub issue. A GitHub failure is
// reported in the JSON result (status="failed") rather than as an HTTP error —
// the report is still saved locally.
func CreateDevReportAdminHandler(svc *service.Service, sm *scs.SessionManager, encKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, devReportMaxBody)

		var req struct {
			Type        string         `json:"type"`
			Title       string         `json:"title"`
			Description string         `json:"description"`
			PageURL     string         `json:"page_url"`
			PagePath    string         `json:"page_path"`
			Screenshot  string         `json:"screenshot"` // data URL (optional)
			HTML        string         `json:"html"`       // outerHTML (optional)
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

		imgData, imgCT := decodeDataURL(req.Screenshot)

		in := service.CreateDevReportInput{
			Type:                  req.Type,
			Title:                 title,
			Description:           req.Description,
			PageURL:               req.PageURL,
			PagePath:              req.PagePath,
			ScreenshotData:        imgData,
			ScreenshotContentType: imgCT,
			HTMLSnapshot:          req.HTML,
			Metadata:              req.Metadata,
			CreatedBy:             sm.GetString(r.Context(), sessionKeyAccountUsername),
		}

		res, err := svc.CreateDevReport(r.Context(), in, encKey)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", strings.TrimPrefix(err.Error(), "invalid parameter: "))
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to file the report.")
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// DevReportScreenshotHandler serves GET /-/dev-reports/{shortId}/screenshot —
// the durable screenshot stored alongside the report.
func DevReportScreenshotHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, ct, err := svc.GetDevReportArtifact(r.Context(), chi.URLParam(r, "shortId"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.Write(data)
	}
}

// DevReportSnapshotHandler serves GET /-/dev-reports/{shortId}/snapshot.html —
// the raw HTML capture. Served under a `sandbox` CSP so the captured DOM
// (which includes the app's own scripts) renders for inspection without
// executing in the admin origin.
func DevReportSnapshotHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		html, err := svc.GetDevReportSnapshot(r.Context(), chi.URLParam(r, "shortId"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "sandbox")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.Write([]byte(html))
	}
}

// decodeDataURL splits a "data:<ct>;base64,<payload>" URL into bytes + content
// type. A bare base64 string (no data: prefix) is treated as JPEG. Any decode
// failure yields nil bytes — the report is filed without a screenshot rather
// than rejected.
func decodeDataURL(s string) ([]byte, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ""
	}
	ct := "image/jpeg"
	if strings.HasPrefix(s, "data:") {
		comma := strings.IndexByte(s, ',')
		if comma < 0 {
			return nil, ""
		}
		meta := s[len("data:"):comma]
		if semi := strings.IndexByte(meta, ';'); semi >= 0 {
			meta = meta[:semi]
		}
		if meta != "" {
			ct = meta
		}
		s = s[comma+1:]
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, ""
	}
	return data, ct
}
