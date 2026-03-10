package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

const maxReviewInstructionsLength = 20000

// GetReviewInstructions returns the current review instructions with template variables expanded.
func (s *Service) GetReviewInstructions(ctx context.Context) (string, error) {
	raw, _, err := s.GetReviewInstructionsRaw(ctx)
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "No custom review instructions configured. Review each transaction and approve, reject, or skip based on your judgment.", nil
	}
	return s.expandReviewVariables(ctx, raw), nil
}

// GetReviewInstructionsRaw returns the raw instructions (not expanded) and template slug for editing.
func (s *Service) GetReviewInstructionsRaw(ctx context.Context) (string, string, error) {
	var instructions, templateSlug string

	if cfg, err := s.Queries.GetAppConfig(ctx, "review_instructions"); err == nil && cfg.Value.Valid {
		instructions = cfg.Value.String
	}
	if cfg, err := s.Queries.GetAppConfig(ctx, "review_instruction_template"); err == nil && cfg.Value.Valid {
		templateSlug = cfg.Value.String
	}

	return instructions, templateSlug, nil
}

// SaveReviewInstructions saves review instructions and template slug.
func (s *Service) SaveReviewInstructions(ctx context.Context, instructions string, templateSlug string) error {
	if len(instructions) > maxReviewInstructionsLength {
		return fmt.Errorf("%w: instructions exceed maximum length of %d characters", ErrInvalidParameter, maxReviewInstructionsLength)
	}

	if err := s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "review_instructions",
		Value: pgtype.Text{String: instructions, Valid: true},
	}); err != nil {
		return fmt.Errorf("save review instructions: %w", err)
	}

	return s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "review_instruction_template",
		Value: pgtype.Text{String: templateSlug, Valid: true},
	})
}

// expandReviewVariables replaces {{variable}} tokens in review instructions with live data.
func (s *Service) expandReviewVariables(ctx context.Context, raw string) string {
	// Total pending count
	count, err := s.Queries.CountPendingReviews(ctx)
	if err != nil {
		count = 0
	}
	raw = strings.ReplaceAll(raw, "{{total_pending}}", fmt.Sprintf("%d", count))

	// Date range of pending items
	startDate, endDate := s.getPendingDateRange(ctx)
	raw = strings.ReplaceAll(raw, "{{date_range_start}}", startDate)
	raw = strings.ReplaceAll(raw, "{{date_range_end}}", endDate)

	// Family members
	users, err := s.ListUsers(ctx)
	if err == nil && len(users) > 0 {
		names := make([]string, len(users))
		for i, u := range users {
			names[i] = u.Name
		}
		raw = strings.ReplaceAll(raw, "{{family_members}}", strings.Join(names, ", "))
	} else {
		raw = strings.ReplaceAll(raw, "{{family_members}}", "")
	}

	return raw
}

// getPendingDateRange returns the earliest and latest transaction dates among pending review items.
func (s *Service) getPendingDateRange(ctx context.Context) (string, string) {
	var startDate, endDate string
	row := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(MIN(t.date)::text, ''), COALESCE(MAX(t.date)::text, '')
		FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		WHERE rq.status = 'pending' AND t.deleted_at IS NULL`)
	if err := row.Scan(&startDate, &endDate); err != nil {
		return "", ""
	}
	return startDate, endDate
}

// WebhookConfig represents the outgoing webhook configuration.
type WebhookConfig struct {
	URL              string   `json:"url"`
	Secret           string   `json:"secret,omitempty"`
	Events           []string `json:"events"`
	SecretConfigured bool     `json:"secret_configured"`
}

// TestWebhookResult represents the result of a test webhook delivery.
type TestWebhookResult struct {
	Success        bool `json:"success"`
	StatusCode     int  `json:"status_code"`
	ResponseTimeMs int  `json:"response_time_ms"`
}

// GetWebhookConfig returns the current outgoing webhook configuration.
func (s *Service) GetWebhookConfig(ctx context.Context) (*WebhookConfig, error) {
	cfg := &WebhookConfig{
		Events: []string{"review_items_added"},
	}

	if row, err := s.Queries.GetAppConfig(ctx, "review_webhook_url"); err == nil && row.Value.Valid {
		cfg.URL = row.Value.String
	}
	if row, err := s.Queries.GetAppConfig(ctx, "review_webhook_secret"); err == nil && row.Value.Valid && row.Value.String != "" {
		cfg.SecretConfigured = true
	}
	if row, err := s.Queries.GetAppConfig(ctx, "review_webhook_events"); err == nil && row.Value.Valid && row.Value.String != "" {
		// Parse JSON array of event strings
		var events []string
		if err := json.Unmarshal([]byte(row.Value.String), &events); err == nil {
			cfg.Events = events
		}
	}

	return cfg, nil
}

// SaveWebhookConfig saves outgoing webhook configuration.
func (s *Service) SaveWebhookConfig(ctx context.Context, cfg WebhookConfig) (*WebhookConfig, error) {
	// Validate URL
	if cfg.URL != "" && !strings.HasPrefix(cfg.URL, "https://") {
		return nil, fmt.Errorf("%w: webhook URL must start with https://", ErrInvalidParameter)
	}

	// Save URL
	if err := s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "review_webhook_url",
		Value: pgtype.Text{String: cfg.URL, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("save webhook url: %w", err)
	}

	// Handle secret
	secret := cfg.Secret
	if cfg.URL != "" && secret == "" {
		// Check if there's already a secret configured
		if row, err := s.Queries.GetAppConfig(ctx, "review_webhook_secret"); err != nil || !row.Value.Valid || row.Value.String == "" {
			// Auto-generate a 64-character hex secret
			secretBytes := make([]byte, 32)
			if _, err := rand.Read(secretBytes); err != nil {
				return nil, fmt.Errorf("generate webhook secret: %w", err)
			}
			secret = hex.EncodeToString(secretBytes)
		}
	}
	if secret != "" {
		if len(secret) < 32 {
			return nil, fmt.Errorf("%w: webhook secret must be at least 32 characters", ErrInvalidParameter)
		}
		if err := s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "review_webhook_secret",
			Value: pgtype.Text{String: secret, Valid: true},
		}); err != nil {
			return nil, fmt.Errorf("save webhook secret: %w", err)
		}
	}

	// Save events
	if len(cfg.Events) > 0 {
		eventsJSON, _ := json.Marshal(cfg.Events)
		if err := s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "review_webhook_events",
			Value: pgtype.Text{String: string(eventsJSON), Valid: true},
		}); err != nil {
			return nil, fmt.Errorf("save webhook events: %w", err)
		}
	}

	result := &WebhookConfig{
		URL:              cfg.URL,
		Events:           cfg.Events,
		SecretConfigured: secret != "" || cfg.URL == "",
	}
	if secret != "" {
		result.Secret = secret
	}
	return result, nil
}

// GetWebhookSecret returns the raw webhook secret for signing. Internal use only.
func (s *Service) GetWebhookSecret(ctx context.Context) (string, error) {
	if row, err := s.Queries.GetAppConfig(ctx, "review_webhook_secret"); err == nil && row.Value.Valid {
		return row.Value.String, nil
	}
	return "", nil
}

// GetWebhookURL returns the webhook URL. Internal use only.
func (s *Service) GetWebhookURL(ctx context.Context) (string, error) {
	if row, err := s.Queries.GetAppConfig(ctx, "review_webhook_url"); err == nil && row.Value.Valid {
		return row.Value.String, nil
	}
	return "", nil
}
