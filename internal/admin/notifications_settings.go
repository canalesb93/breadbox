//go:build !headless && !lite

package admin

import (
	"net/http"
	"net/url"
	"strings"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
	"breadbox/internal/timefmt"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// NotificationsSettingsHandler serves GET /settings/notifications — the
// Notifications tab. It manages the list of outbound notification channels
// (the multi-sink model) plus the global public base URL for deep links.
func NotificationsSettingsHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channels, err := svc.GetNotificationChannels(r.Context())
		if err != nil {
			http.Error(w, "Failed to load notification channels: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Capture the origin the admin is reaching Breadbox from so background
		// notification sends can build absolute deep links without the admin
		// typing one. Best-effort: a write failure must not block the page.
		_ = svc.SetDetectedNotifyBaseURL(r.Context(), requestOrigin(r))
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
			Channels:        notificationChannelViews(channels),
			PublicBaseURL:   settings.PublicBaseURL,
			DetectedBaseURL: settings.DetectedBaseURL,
			FieldErrors:     map[string]string{},
			FormError:       formError,
			FormSuccess:     formSuccess,
			CSRFToken:       GetCSRFToken(r),
		}

		data := BaseTemplateData(r, sm, "notifications-settings", "Notifications")
		data["CSRFToken"] = props.CSRFToken
		data["Flash"] = nil
		renderSettingsTab(tr, w, r, data, pages.SettingsTabNotifications, pages.NotificationsSettings(props))
	}
}

// NotificationsSettingsPostHandler handles POST /settings/notifications — the
// global (cross-channel) settings, currently just the public base URL.
func NotificationsSettingsPostHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Could not read the submitted form.", "/settings/notifications")
			return
		}
		var params service.UpdateNotificationSettingsParams
		if r.Form.Has("public_base_url") {
			v := strings.TrimSpace(r.FormValue("public_base_url"))
			params.PublicBaseURL = &v
		}
		if _, err := svc.UpdateNotificationSettings(r.Context(), params); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to save: "+err.Error(), "/settings/notifications")
			return
		}
		SetFlash(r.Context(), sm, "success", "Notification settings saved.")
		http.Redirect(w, r, "/settings/notifications", http.StatusSeeOther)
	}
}

// NotificationsAddChannelHandler handles POST /settings/notifications/channels.
func NotificationsAddChannelHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Could not read the submitted form.", "/settings/notifications")
			return
		}
		_, err := svc.AddNotificationChannel(r.Context(), service.AddNotificationChannelParams{
			Name:        strings.TrimSpace(r.FormValue("name")),
			URL:         strings.TrimSpace(r.FormValue("url")),
			Format:      strings.TrimSpace(r.FormValue("format")),
			MinPriority: strings.TrimSpace(r.FormValue("min_priority")),
			NtfyToken:   strings.TrimSpace(r.FormValue("ntfy_token")),
		})
		if err != nil {
			FlashRedirect(w, r, sm, "error", "Couldn't add channel: "+err.Error(), "/settings/notifications")
			return
		}
		SetFlash(r.Context(), sm, "success", "Channel added.")
		http.Redirect(w, r, "/settings/notifications", http.StatusSeeOther)
	}
}

// NotificationsUpdateChannelHandler handles
// POST /settings/notifications/channels/{id} — the edit-channel drawer. The
// URL and ntfy token are kept when their fields are submitted blank (the form
// never echoes secrets back), so only non-empty values replace them.
func NotificationsUpdateChannelHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Could not read the submitted form.", "/settings/notifications")
			return
		}
		id := chi.URLParam(r, "id")
		params := service.UpdateNotificationChannelParams{
			Name:        strings.TrimSpace(r.FormValue("name")),
			Format:      strings.TrimSpace(r.FormValue("format")),
			MinPriority: strings.TrimSpace(r.FormValue("min_priority")),
			Enabled:     r.FormValue("enabled") == "true",
		}
		if v := strings.TrimSpace(r.FormValue("url")); v != "" {
			params.URL = &v
		}
		if v := strings.TrimSpace(r.FormValue("ntfy_token")); v != "" {
			params.NtfyToken = &v
		}
		if _, err := svc.UpdateNotificationChannel(r.Context(), id, params); err != nil {
			FlashRedirect(w, r, sm, "error", "Couldn't update channel: "+err.Error(), "/settings/notifications")
			return
		}
		SetFlash(r.Context(), sm, "success", "Channel updated.")
		http.Redirect(w, r, "/settings/notifications", http.StatusSeeOther)
	}
}

// NotificationsDeleteChannelHandler handles
// POST /settings/notifications/channels/{id}/delete.
func NotificationsDeleteChannelHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteNotificationChannel(r.Context(), id); err != nil {
			FlashRedirect(w, r, sm, "error", "Couldn't remove channel: "+err.Error(), "/settings/notifications")
			return
		}
		SetFlash(r.Context(), sm, "success", "Channel removed.")
		http.Redirect(w, r, "/settings/notifications", http.StatusSeeOther)
	}
}

// NotificationsTestChannelHandler handles POST /-/notifications/channels/{id}/test —
// a JSON endpoint that fires a sample notification to one channel.
func NotificationsTestChannelHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.SendTestToChannel(r.Context(), id); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// requestOrigin derives the public origin (scheme://host, no path) from the
// incoming request, honoring X-Forwarded-Proto / X-Forwarded-Host when set by
// a reverse proxy. Mirrors mcpServerURL so deep-link origins match the MCP
// endpoint, OAuth issuer, and setup links — all derived the same way.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

// notificationChannelViews maps service channels to the display shape.
func notificationChannelViews(channels []service.NotificationChannel) []pages.NotificationChannelView {
	out := make([]pages.NotificationChannelView, 0, len(channels))
	for _, c := range channels {
		v := pages.NotificationChannelView{
			ID:          c.ID,
			Name:        c.Name,
			Format:      c.Format,
			FormatLabel: channelFormatLabel(c.Format, c.URL),
			URLMasked:   maskNotifyURL(c.URL),
			MinPriority: c.MinPriority,
			Enabled:     c.Enabled,
			EditURL:     "/settings/notifications/channels/" + c.ID,
			DeleteURL:   "/settings/notifications/channels/" + c.ID + "/delete",
		}
		if c.LastStatus != nil {
			v.HasStatus = true
			v.StatusOK = c.LastStatus.OK
			v.StatusText = notifyStatusText(c.LastStatus)
		}
		out = append(out, v)
	}
	return out
}

// notifyFormatLabel renders a human label for a stored format value.
func notifyFormatLabel(format string) string {
	switch format {
	case "ntfy":
		return "ntfy"
	case "slack":
		return "Slack"
	case "discord":
		return "Discord"
	case "googlechat":
		return "Google Chat"
	case "json":
		return "Generic JSON"
	default:
		return "Auto-detect"
	}
}

// channelFormatLabel labels a channel's format for the list. For an
// "auto" channel it shows what the URL actually resolves to (e.g.
// "Auto · ntfy") so the operator sees the effective provider; for a
// generic URL it stays "Auto-detect".
func channelFormatLabel(format, rawURL string) string {
	if format != "auto" && format != "" {
		return notifyFormatLabel(format)
	}
	resolved := service.ResolveNotifyFormat("auto", rawURL)
	if resolved == "json" {
		return "Auto-detect"
	}
	return "Auto · " + notifyFormatLabel(resolved)
}

// maskNotifyURL renders a webhook URL with its secret path obscured —
// "https://host/…abcd" — so the channel list doesn't print full secret URLs.
func maskNotifyURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if len(raw) > 12 {
			return raw[:8] + "…" + raw[len(raw)-4:]
		}
		return raw
	}
	base := u.Scheme + "://" + u.Host
	if u.Path == "" || u.Path == "/" {
		return base
	}
	tail := raw
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	return base + "/…" + tail
}

// notifyStatusText renders a per-channel delivery status line.
func notifyStatusText(s *service.NotificationDeliveryStatus) string {
	when := timefmt.RelativeRFC3339(s.At)
	if s.OK {
		if when != "" {
			return "Delivered · " + when
		}
		return "Delivered"
	}
	detail := s.Detail
	if detail == "" {
		detail = "failed"
	}
	if when != "" {
		return "Failed (" + when + "): " + detail
	}
	return "Failed: " + detail
}
