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
	WebhookURL    string `json:"webhook_url"`
	Format        string `json:"format"` // auto | ntfy | json
	PublicBaseURL string `json:"public_base_url"`
}

// UpdateNotificationSettingsParams holds the writable notification settings.
// Nil = leave untouched; an empty string clears the value (webhook/base URL)
// or resets to the default (format → auto).
type UpdateNotificationSettingsParams struct {
	WebhookURL    *string
	Format        *string
	PublicBaseURL *string
}

// GetNotificationSettings reads the notify.* keys from app_config.
func (s *Service) GetNotificationSettings(ctx context.Context) (*NotificationSettingsResponse, error) {
	return &NotificationSettingsResponse{
		WebhookURL:    appconfig.String(ctx, s.Queries, appconfig.KeyNotifyWebhookURL, ""),
		Format:        notifyFormatOrDefault(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyFormat, appconfig.NotifyFormatAuto)),
		PublicBaseURL: appconfig.String(ctx, s.Queries, appconfig.KeyNotifyPublicBaseURL, ""),
	}, nil
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
			return nil, fmt.Errorf("%w: notification format must be auto, ntfy, or json", ErrInvalidParameter)
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyNotifyFormat, format)); err != nil {
			return nil, fmt.Errorf("set notify_format: %w", err)
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
	case appconfig.NotifyFormatAuto, appconfig.NotifyFormatNtfy, appconfig.NotifyFormatJSON:
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
