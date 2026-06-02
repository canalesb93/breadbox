//go:build !lite

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/shortid"
)

// NotificationChannel is one outbound sink in the multi-channel model. A
// workflow notification fans out to every enabled channel, each rendered in
// its own format and gated by its own priority floor. Channels are stored as
// a JSON array under app_config[notify.channels].
type NotificationChannel struct {
	ID          string `json:"id"`           // 8-char short id
	Name        string `json:"name"`         // operator label ("Family ntfy", "#alerts")
	URL         string `json:"url"`          // http(s) sink
	Format      string `json:"format"`       // auto | ntfy | slack | discord | json
	MinPriority string `json:"min_priority"` // info | warning | critical
	NtfyToken   string `json:"ntfy_token,omitempty"`
	Enabled     bool   `json:"enabled"`
	// LastStatus records the most recent delivery attempt to this channel.
	LastStatus *NotificationDeliveryStatus `json:"last_status,omitempty"`
}

// NotificationDeliveryStatus is the per-channel delivery observability record.
type NotificationDeliveryStatus struct {
	OK     bool   `json:"ok"`
	At     string `json:"at"`     // RFC3339
	Format string `json:"format"` // resolved wire format used
	Detail string `json:"detail"` // "delivered" or the error string
}

// AddNotificationChannelParams is the input for creating a channel.
type AddNotificationChannelParams struct {
	Name        string
	URL         string
	Format      string
	MinPriority string
	NtfyToken   string
}

// loadNotificationChannels returns the configured channels. When none are
// stored, it synthesizes a single "Default" channel from the legacy single-
// webhook keys so configs created before the multi-channel model keep
// delivering. The synthesized channel is not persisted until first edited.
func (s *Service) loadNotificationChannels(ctx context.Context) []NotificationChannel {
	raw := strings.TrimSpace(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyChannels, ""))
	if raw != "" {
		var chans []NotificationChannel
		if err := json.Unmarshal([]byte(raw), &chans); err == nil {
			return chans
		}
		// Corrupt JSON: fall through to legacy/empty rather than erroring a send.
	}
	legacyURL := strings.TrimSpace(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyWebhookURL, ""))
	if legacyURL == "" {
		return nil
	}
	return []NotificationChannel{{
		ID:          "legacy",
		Name:        "Default",
		URL:         legacyURL,
		Format:      notifyFormatOrDefault(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyFormat, appconfig.NotifyFormatAuto)),
		MinPriority: notifyMinPriorityOrDefault(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyMinPriority, appconfig.NotifyMinPriorityInfo)),
		Enabled:     true,
	}}
}

// persistNotificationChannels writes the channel array back to app_config.
func (s *Service) persistNotificationChannels(ctx context.Context, chans []NotificationChannel) error {
	if chans == nil {
		chans = []NotificationChannel{}
	}
	b, err := json.Marshal(chans)
	if err != nil {
		return fmt.Errorf("marshal notification channels: %w", err)
	}
	if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyNotifyChannels, string(b))); err != nil {
		return fmt.Errorf("set notify_channels: %w", err)
	}
	return nil
}

// GetNotificationChannels returns the configured channels for display. The
// returned slice always reflects the persisted (or synthesized-legacy) state.
func (s *Service) GetNotificationChannels(ctx context.Context) ([]NotificationChannel, error) {
	return s.loadNotificationChannels(ctx), nil
}

// AddNotificationChannel validates and appends a new enabled channel,
// migrating any synthesized legacy channel into the persisted array so the
// existing sink is preserved alongside the new one.
func (s *Service) AddNotificationChannel(ctx context.Context, p AddNotificationChannelParams) (*NotificationChannel, error) {
	url := strings.TrimSpace(p.URL)
	if err := validateNotifyURL(url); err != nil {
		return nil, err
	}
	format := strings.TrimSpace(p.Format)
	if format == "" {
		format = appconfig.NotifyFormatAuto
	}
	if !validNotifyFormat(format) {
		return nil, fmt.Errorf("%w: notification format must be auto, ntfy, slack, discord, or json", ErrInvalidParameter)
	}
	minPriority := strings.TrimSpace(p.MinPriority)
	if minPriority == "" {
		minPriority = appconfig.NotifyMinPriorityInfo
	}
	if !validNotifyMinPriority(minPriority) {
		return nil, fmt.Errorf("%w: minimum priority must be info, warning, or critical", ErrInvalidParameter)
	}
	id, err := shortid.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate channel id: %w", err)
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = defaultChannelName(url)
	}
	ch := NotificationChannel{
		ID:          id,
		Name:        name,
		URL:         url,
		Format:      format,
		MinPriority: minPriority,
		NtfyToken:   strings.TrimSpace(p.NtfyToken),
		Enabled:     true,
	}
	chans := s.persistedOrMigratedChannels(ctx)
	chans = append(chans, ch)
	if err := s.persistNotificationChannels(ctx, chans); err != nil {
		return nil, err
	}
	return &ch, nil
}

// SetNotificationChannelEnabled flips a channel's enabled flag.
func (s *Service) SetNotificationChannelEnabled(ctx context.Context, id string, enabled bool) error {
	chans := s.persistedOrMigratedChannels(ctx)
	found := false
	for i := range chans {
		if chans[i].ID == id {
			chans[i].Enabled = enabled
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: notification channel not found", ErrNotFound)
	}
	return s.persistNotificationChannels(ctx, chans)
}

// DeleteNotificationChannel removes a channel by id.
func (s *Service) DeleteNotificationChannel(ctx context.Context, id string) error {
	chans := s.persistedOrMigratedChannels(ctx)
	out := chans[:0]
	found := false
	for _, c := range chans {
		if c.ID == id {
			found = true
			continue
		}
		out = append(out, c)
	}
	if !found {
		return fmt.Errorf("%w: notification channel not found", ErrNotFound)
	}
	return s.persistNotificationChannels(ctx, out)
}

// SendTestToChannel delivers a sample notification to a single channel and
// records its status — backs the per-channel "Send test" button.
func (s *Service) SendTestToChannel(ctx context.Context, id string) error {
	chans := s.persistedOrMigratedChannels(ctx)
	idx := -1
	for i := range chans {
		if chans[i].ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("%w: notification channel not found", ErrNotFound)
	}
	baseURL := normalizeBaseURL(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyPublicBaseURL, ""))
	p := testNotificationPayload()
	err := s.sendToChannel(ctx, &chans[idx], p, baseURL)
	// Best-effort persist of the recorded status.
	_ = s.persistNotificationChannels(ctx, chans)
	return err
}

// persistedOrMigratedChannels returns the persisted channel array, first
// migrating a synthesized legacy channel into storage if that's all there is.
// This guarantees a mutation (add/toggle/delete) operates on a real array.
func (s *Service) persistedOrMigratedChannels(ctx context.Context) []NotificationChannel {
	raw := strings.TrimSpace(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyChannels, ""))
	if raw != "" {
		var chans []NotificationChannel
		if err := json.Unmarshal([]byte(raw), &chans); err == nil {
			return chans
		}
	}
	return s.loadNotificationChannels(ctx) // legacy-synth (or nil)
}

// defaultChannelName derives a friendly label from a URL host when the
// operator didn't name the channel.
func defaultChannelName(rawURL string) string {
	if h := urlHost(rawURL); h != "" {
		return h
	}
	return "Channel"
}

// testNotificationPayload is the shared sample payload for test sends.
func testNotificationPayload() NotificationPayload {
	return NotificationPayload{
		Event:    "test",
		Title:    "Breadbox notification test",
		Body:     "If you can see this, this channel is wired up correctly.",
		Priority: "info",
	}
}

// nowRFC3339 is the timestamp helper for delivery records.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
