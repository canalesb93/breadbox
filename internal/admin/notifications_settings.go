//go:build !headless && !lite

package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

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
			Channels:      notificationChannelViews(channels),
			PublicBaseURL: settings.PublicBaseURL,
			FieldErrors:   map[string]string{},
			FormError:     formError,
			FormSuccess:   formSuccess,
			CSRFToken:     GetCSRFToken(r),
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

// NotificationsToggleChannelHandler handles
// POST /settings/notifications/channels/{id}/toggle.
func NotificationsToggleChannelHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		id := chi.URLParam(r, "id")
		enabled := r.FormValue("enabled") == "true"
		if err := svc.SetNotificationChannelEnabled(r.Context(), id, enabled); err != nil {
			FlashRedirect(w, r, sm, "error", "Couldn't update channel: "+err.Error(), "/settings/notifications")
			return
		}
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
			ToggleURL:   "/settings/notifications/channels/" + c.ID + "/toggle",
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
	when := relTimeRFC3339(s.At)
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

// relTimeRFC3339 turns an RFC3339 timestamp into a coarse relative string.
func relTimeRFC3339(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return t.Local().Format("Jan 2")
	}
}
