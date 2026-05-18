//go:build !lite

package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"breadbox/internal/agent"
	"breadbox/internal/appconfig"
	"breadbox/internal/db"
)

// appconfigParam builds a db.SetAppConfigParams with the value wrapped
// in pgtype.Text. Used by every settings write below.
func appconfigParam(key, value string) db.SetAppConfigParams {
	return db.SetAppConfigParams{
		Key:   key,
		Value: pgtype.Text{String: value, Valid: true},
	}
}

// AgentSettingsResponse is what GET /api/v1/agents/settings returns.
// Token fields are masked when present; full plaintext never leaves the server.
type AgentSettingsResponse struct {
	AuthMode           string   `json:"auth_mode"`
	SubscriptionToken  *string  `json:"subscription_token,omitempty"`
	AnthropicAPIKey    *string  `json:"anthropic_api_key,omitempty"`
	MaxConcurrent      int      `json:"max_concurrent"`
	GlobalMaxBudgetUSD *float64 `json:"global_max_budget_usd,omitempty"`
	RuntimePath        string   `json:"runtime_path"`
	TranscriptDir      string   `json:"transcript_dir"`
}

// UpdateAgentSettingsParams holds writable settings. Nil = don't touch;
// empty-string for token fields clears the stored value.
type UpdateAgentSettingsParams struct {
	AuthMode           *string
	SubscriptionToken  *string
	AnthropicAPIKey    *string
	MaxConcurrent      *int
	GlobalMaxBudgetUSD *float64
	RuntimePath        *string
	TranscriptDir      *string
}

// GetAgentSettings reads agent.* keys from app_config, decrypts tokens,
// returns masked values.
func (s *Service) GetAgentSettings(ctx context.Context, encKey []byte) (*AgentSettingsResponse, error) {
	authMode := appconfig.String(ctx, s.Queries, appconfig.KeyAgentAuthMode, appconfig.AuthModeSubscription)

	subToken, _, err := appconfig.ReadEncrypted(ctx, s.Queries, appconfig.KeyAgentSubscriptionToken, encKey)
	if err != nil {
		return nil, fmt.Errorf("read subscription token: %w", err)
	}
	apiToken, _, err := appconfig.ReadEncrypted(ctx, s.Queries, appconfig.KeyAgentAnthropicAPIKey, encKey)
	if err != nil {
		return nil, fmt.Errorf("read anthropic api key: %w", err)
	}

	maxConcurrent := appconfig.Int(ctx, s.Queries, appconfig.KeyAgentMaxConcurrent, 3)
	runtimePath := appconfig.String(ctx, s.Queries, appconfig.KeyAgentRuntimePath, "")
	// Default matches serve.go fallback — keeps the Settings → Agents
	// form showing the active path on a fresh install rather than blank.
	transcriptDir := appconfig.String(ctx, s.Queries, appconfig.KeyAgentTranscriptDir, "transcripts/agents")
	globalBudget := readOptionalFloat(ctx, s.Queries, appconfig.KeyAgentGlobalMaxBudgetUSD)

	return &AgentSettingsResponse{
		AuthMode:           authMode,
		SubscriptionToken:  maskToken(subToken),
		AnthropicAPIKey:    maskToken(apiToken),
		MaxConcurrent:      maxConcurrent,
		GlobalMaxBudgetUSD: globalBudget,
		RuntimePath:        runtimePath,
		TranscriptDir:      transcriptDir,
	}, nil
}

// UpdateAgentSettings writes the non-nil fields and returns the new masked state.
func (s *Service) UpdateAgentSettings(ctx context.Context, p UpdateAgentSettingsParams, encKey []byte) (*AgentSettingsResponse, error) {
	if p.AuthMode != nil {
		if *p.AuthMode != appconfig.AuthModeSubscription && *p.AuthMode != appconfig.AuthModeAPIKey {
			return nil, fmt.Errorf("%w: auth_mode must be subscription or api_key", ErrInvalidParameter)
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyAgentAuthMode, *p.AuthMode)); err != nil {
			return nil, fmt.Errorf("set auth_mode: %w", err)
		}
	}
	if p.SubscriptionToken != nil {
		if err := appconfig.WriteEncrypted(ctx, s.Queries, appconfig.KeyAgentSubscriptionToken, *p.SubscriptionToken, encKey); err != nil {
			return nil, fmt.Errorf("write subscription token: %w", err)
		}
	}
	if p.AnthropicAPIKey != nil {
		if err := appconfig.WriteEncrypted(ctx, s.Queries, appconfig.KeyAgentAnthropicAPIKey, *p.AnthropicAPIKey, encKey); err != nil {
			return nil, fmt.Errorf("write anthropic api key: %w", err)
		}
	}
	if p.MaxConcurrent != nil {
		if *p.MaxConcurrent < 1 || *p.MaxConcurrent > 50 {
			return nil, fmt.Errorf("%w: max_concurrent must be 1-50", ErrInvalidParameter)
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyAgentMaxConcurrent, strconv.Itoa(*p.MaxConcurrent))); err != nil {
			return nil, fmt.Errorf("set max_concurrent: %w", err)
		}
	}
	if p.GlobalMaxBudgetUSD != nil {
		if *p.GlobalMaxBudgetUSD < 0 || *p.GlobalMaxBudgetUSD > 1000 {
			return nil, fmt.Errorf("%w: global_max_budget_usd must be 0-1000", ErrInvalidParameter)
		}
		v := strconv.FormatFloat(*p.GlobalMaxBudgetUSD, 'f', 4, 64)
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyAgentGlobalMaxBudgetUSD, v)); err != nil {
			return nil, fmt.Errorf("set global_max_budget_usd: %w", err)
		}
	}
	if p.RuntimePath != nil {
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyAgentRuntimePath, *p.RuntimePath)); err != nil {
			return nil, fmt.Errorf("set runtime_path: %w", err)
		}
	}
	if p.TranscriptDir != nil {
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyAgentTranscriptDir, *p.TranscriptDir)); err != nil {
			return nil, fmt.Errorf("set transcript_dir: %w", err)
		}
	}
	return s.GetAgentSettings(ctx, encKey)
}

// AgentSubsystemStatus is a cheap, side-effect-free readiness report for
// the agent subsystem — used by the v2 SPA list page to surface
// onboarding hints, and (later) by the smoke-test endpoints to short-
// circuit before charging a real API call.
type AgentSubsystemStatus struct {
	AuthMode        string `json:"auth_mode"`
	AuthConfigured  bool   `json:"auth_configured"`
	BinaryPresent   bool   `json:"binary_present"`
	BinaryPath      string `json:"binary_path,omitempty"`
	Ready           bool   `json:"ready"` // AuthConfigured && BinaryPresent
}

// GetAgentSubsystemStatus inspects app_config + the filesystem to report
// whether the agent subsystem is ready to fire a run. Mirrors the
// `breadbox doctor` check; the only difference is the wire shape.
func (s *Service) GetAgentSubsystemStatus(ctx context.Context) *AgentSubsystemStatus {
	authMode := appconfig.String(ctx, s.Queries, appconfig.KeyAgentAuthMode, appconfig.AuthModeSubscription)
	tokenKey := appconfig.KeyAgentSubscriptionToken
	if authMode == appconfig.AuthModeAPIKey {
		tokenKey = appconfig.KeyAgentAnthropicAPIKey
	}
	stored, _ := appconfig.Read(ctx, s.Queries, tokenKey)
	authConfigured := stored != ""

	binaryPath := appconfig.String(ctx, s.Queries, appconfig.KeyAgentRuntimePath, "")
	resolved, binErr := agent.LocateBinary(binaryPath)
	binaryPresent := binErr == nil
	if !binaryPresent {
		resolved = ""
	}
	return &AgentSubsystemStatus{
		AuthMode:       authMode,
		AuthConfigured: authConfigured,
		BinaryPresent:  binaryPresent,
		BinaryPath:     resolved,
		Ready:          authConfigured && binaryPresent,
	}
}

// maskToken returns a display string for a secret. nil for empty input.
// Format: first 16 chars + "••••" + last 4. Shorter tokens render with bullets only.
func maskToken(token string) *string {
	if token == "" {
		return nil
	}
	if len(token) <= 20 {
		m := "••••" + lastN(token, 4)
		return &m
	}
	m := token[:16] + "••••" + lastN(token, 4)
	return &m
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func readOptionalFloat(ctx context.Context, r appconfig.Reader, key string) *float64 {
	raw, ok := appconfig.Read(ctx, r, key)
	if !ok || raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}
