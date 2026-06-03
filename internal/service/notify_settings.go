//go:build !lite

package service

import (
	"context"
	"fmt"
	"strings"

	"breadbox/internal/appconfig"
)

// NotificationSettingsResponse is the read shape for the Settings →
// Notifications page. Unlike credential settings, nothing here is secret —
// the webhook URL, format, and public base URL all surface in plaintext.
type NotificationSettingsResponse struct {
	WebhookURL string `json:"webhook_url"`
	Format     string `json:"format"` // auto | ntfy | slack | discord | json
	// PublicBaseURL is the manual override (empty = auto-detect).
	PublicBaseURL string `json:"public_base_url"`
	// DetectedBaseURL is the origin auto-captured from the admin's last
	// visit to the settings page (read-only; surfaced so the UI can show
	// the effective deep-link origin).
	DetectedBaseURL string `json:"detected_base_url"`
	MinPriority     string `json:"min_priority"` // info | warning | critical
}

// UpdateNotificationSettingsParams holds the writable notification settings.
// Nil = leave untouched; an empty string clears the value (webhook/base URL)
// or resets to the default (format → auto, min_priority → info).
type UpdateNotificationSettingsParams struct {
	WebhookURL    *string
	Format        *string
	PublicBaseURL *string
	MinPriority   *string
}

// GetNotificationSettings reads the notify.* keys from app_config.
func (s *Service) GetNotificationSettings(ctx context.Context) (*NotificationSettingsResponse, error) {
	return &NotificationSettingsResponse{
		WebhookURL:      appconfig.String(ctx, s.Queries, appconfig.KeyNotifyWebhookURL, ""),
		Format:          notifyFormatOrDefault(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyFormat, appconfig.NotifyFormatAuto)),
		PublicBaseURL:   appconfig.String(ctx, s.Queries, appconfig.KeyNotifyPublicBaseURL, ""),
		DetectedBaseURL: appconfig.String(ctx, s.Queries, appconfig.KeyNotifyDetectedBaseURL, ""),
		MinPriority:     notifyMinPriorityOrDefault(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyMinPriority, appconfig.NotifyMinPriorityInfo)),
	}, nil
}

// ResolveNotifyBaseURL returns the effective origin prepended to report
// deep links at send time: the manual override (KeyNotifyPublicBaseURL)
// when set, otherwise the origin auto-detected from the admin's last visit
// to the settings page (KeyNotifyDetectedBaseURL). Empty when neither is
// known — deep links then stay relative. The result is normalized (no
// trailing slash).
func (s *Service) ResolveNotifyBaseURL(ctx context.Context) string {
	if override := normalizeBaseURL(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyPublicBaseURL, "")); override != "" {
		return override
	}
	return normalizeBaseURL(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyDetectedBaseURL, ""))
}

// SetDetectedNotifyBaseURL records the origin Breadbox is currently being
// browsed from so background notification sends can build absolute deep
// links. It is a best-effort write-through: invalid origins are ignored, and
// the write is skipped when the stored value already matches (so a normal
// page load doesn't churn app_config on every request).
func (s *Service) SetDetectedNotifyBaseURL(ctx context.Context, origin string) error {
	normalized := normalizeBaseURL(origin)
	if normalized == "" || validateNotifyURL(normalized) != nil {
		return nil
	}
	// A loopback origin (localhost/127.0.0.1) is only reachable from the server
	// itself — persisting it would bake dead deep links into every background
	// notification. So a dev/tunnel visit to the settings page doesn't clobber
	// a real public (or LAN) origin captured from a normal visit.
	if isLoopbackOrigin(normalized) {
		return nil
	}
	if normalized == normalizeBaseURL(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyDetectedBaseURL, "")) {
		return nil
	}
	if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyNotifyDetectedBaseURL, normalized)); err != nil {
		return fmt.Errorf("set notify_detected_base_url: %w", err)
	}
	return nil
}

// UpdateNotificationSettings validates and writes the non-nil fields, then
// returns the new state. URLs must be http(s); the format must be one of the
// canonical values.
func (s *Service) UpdateNotificationSettings(ctx context.Context, p UpdateNotificationSettingsParams) (*NotificationSettingsResponse, error) {
	if p.WebhookURL != nil {
		trimmed := strings.TrimSpace(*p.WebhookURL)
		if trimmed != "" {
			if err := validateNotifyURL(trimmed); err != nil {
				return nil, err
			}
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyNotifyWebhookURL, trimmed)); err != nil {
			return nil, fmt.Errorf("set notify_webhook_url: %w", err)
		}
	}
	if p.Format != nil {
		format := strings.TrimSpace(*p.Format)
		if format == "" {
			format = appconfig.NotifyFormatAuto
		}
		if !validNotifyFormat(format) {
			return nil, fmt.Errorf("%w: notification format must be auto, ntfy, slack, discord, googlechat, or json", ErrInvalidParameter)
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyNotifyFormat, format)); err != nil {
			return nil, fmt.Errorf("set notify_format: %w", err)
		}
	}
	if p.MinPriority != nil {
		mp := strings.TrimSpace(*p.MinPriority)
		if mp == "" {
			mp = appconfig.NotifyMinPriorityInfo
		}
		if !validNotifyMinPriority(mp) {
			return nil, fmt.Errorf("%w: minimum priority must be info, warning, or critical", ErrInvalidParameter)
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyNotifyMinPriority, mp)); err != nil {
			return nil, fmt.Errorf("set notify_min_priority: %w", err)
		}
	}
	if p.PublicBaseURL != nil {
		trimmed := normalizeBaseURL(*p.PublicBaseURL)
		if trimmed != "" {
			if err := validateNotifyURL(trimmed); err != nil {
				return nil, fmt.Errorf("%w: public base URL must be a valid http(s) URL", ErrInvalidParameter)
			}
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyNotifyPublicBaseURL, trimmed)); err != nil {
			return nil, fmt.Errorf("set notify_public_base_url: %w", err)
		}
	}
	return s.GetNotificationSettings(ctx)
}

// validNotifyFormat reports whether v is one of the canonical format values.
func validNotifyFormat(v string) bool {
	switch v {
	case appconfig.NotifyFormatAuto, appconfig.NotifyFormatNtfy,
		appconfig.NotifyFormatSlack, appconfig.NotifyFormatDiscord,
		appconfig.NotifyFormatGoogleChat, appconfig.NotifyFormatJSON:
		return true
	default:
		return false
	}
}

// notifyFormatOrDefault normalizes a stored format, coercing empty or
// unrecognized values back to "auto" so the UI always shows a valid choice.
func notifyFormatOrDefault(v string) string {
	if validNotifyFormat(v) {
		return v
	}
	return appconfig.NotifyFormatAuto
}

// validNotifyMinPriority reports whether v is a canonical priority floor.
func validNotifyMinPriority(v string) bool {
	switch v {
	case appconfig.NotifyMinPriorityInfo, appconfig.NotifyMinPriorityWarning, appconfig.NotifyMinPriorityCritical:
		return true
	default:
		return false
	}
}

// notifyMinPriorityOrDefault coerces an unrecognized floor back to "info".
func notifyMinPriorityOrDefault(v string) string {
	if validNotifyMinPriority(v) {
		return v
	}
	return appconfig.NotifyMinPriorityInfo
}
