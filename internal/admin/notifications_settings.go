//go:build !headless && !lite

package admin

import (
	"net/http"
	"strings"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// NotificationsSettingsHandler serves GET /settings/notifications — the
// dedicated Notifications tab. It owns the outbound notification sink
// (webhook URL + wire format + public base URL) that workflow runs push
// reports to. Split out of the Workflows settings tab so notification
// delivery has room to grow its own surface.
func NotificationsSettingsHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.GetNotificationSettings(r.Context())
		if err != nil {
			http.Error(w, "Failed to load notification settings: "+err.Error(), http.StatusInternalServerError)
			return
		}

		flash := GetFlash(r.Context(), sm)
		var formError, formSuccess string
		if flash != nil {
			switch flash.Type {
			case "error":
				formError = flash.Message
			case "success":
				formSuccess = flash.Message
			}
		}

		props := pages.NotificationsSettingsProps{
			Form: pages.NotificationsSettingsFormFields{
				WebhookURL:    settings.WebhookURL,
				Format:        settings.Format,
				PublicBaseURL: settings.PublicBaseURL,
			},
			FieldErrors: map[string]string{},
			FormError:   formError,
			FormSuccess: formSuccess,
			CSRFToken:   GetCSRFToken(r),
		}

		data := BaseTemplateData(r, sm, "notifications-settings", "Notifications")
		data["CSRFToken"] = props.CSRFToken
		data["Flash"] = nil
		renderSettingsTab(tr, w, r, data, pages.SettingsTabNotifications, pages.NotificationsSettings(props))
	}
}

// NotificationsSettingsPostHandler handles POST /settings/notifications —
// the multi-input sink form. Saves webhook URL, format, and public base
// URL together, then redirects back to the tab with a flash. The service
// validates URLs (http(s)) and the format enum.
func NotificationsSettingsPostHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Could not read the submitted form.", "/settings/notifications")
			return
		}

		var params service.UpdateNotificationSettingsParams
		if r.Form.Has("webhook_url") {
			v := strings.TrimSpace(r.FormValue("webhook_url"))
			params.WebhookURL = &v // empty clears; service validates http(s)
		}
		if r.Form.Has("format") {
			v := strings.TrimSpace(r.FormValue("format"))
			params.Format = &v
		}
		if r.Form.Has("public_base_url") {
			v := strings.TrimSpace(r.FormValue("public_base_url"))
			params.PublicBaseURL = &v // empty clears; service validates http(s)
		}

		if _, err := svc.UpdateNotificationSettings(r.Context(), params); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to save notification settings: "+err.Error(), "/settings/notifications")
			return
		}
		SetFlash(r.Context(), sm, "success", "Notification settings saved.")
		http.Redirect(w, r, "/settings/notifications", http.StatusSeeOther)
	}
}
