//go:build !headless && !lite

package admin

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// devReportMaxBody bounds the JSON payload (a redacted screenshot + HTML
// snapshot can be a few MB together).
const devReportMaxBody = 24 << 20 // 24 MiB

// CreateDevReportAdminHandler handles POST /-/dev-reports — the floating
// reporter's submit. It hosts the (redacted) screenshot + HTML snapshot on the
// artifact store, builds a prefilled GitHub issue-draft URL with them, and
// returns it; the client opens the draft for the user to submit. No token, no
// DB persistence.
func CreateDevReportAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
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

		imgData, _ := decodeDataURL(req.Screenshot)

		res, err := svc.CreateDevReport(r.Context(), service.CreateDevReportInput{
			Type:           req.Type,
			Title:          title,
			Description:    req.Description,
			PageURL:        req.PageURL,
			PagePath:       req.PagePath,
			ScreenshotData: imgData,
			HTMLSnapshot:   req.HTML,
			Metadata:       req.Metadata,
			CreatedBy:      sm.GetString(r.Context(), sessionKeyAccountUsername),
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

// decodeDataURL splits a "data:<ct>;base64,<payload>" URL into bytes + content
// type. A bare base64 string is treated as JPEG. Any decode failure yields nil
// bytes — the report is filed without a screenshot rather than rejected.
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
