package client

import (
	"context"
	"net/http"
	"net/url"
)

// ProviderInfo mirrors api.providerInfo — the entry returned by
// GET /api/v1/providers and friends. Defined here so the CLI doesn't
// depend on `internal/api`.
type ProviderInfo struct {
	Name              string         `json:"name"`
	Configured        bool           `json:"configured"`
	NeedsLinkSession  bool           `json:"needs_link_session"`
	Capabilities      []string       `json:"capabilities"`
	CredentialsSchema map[string]any `json:"credentials_schema"`
}

// ProviderConfigView is the redacted view returned by GET /settings/providers.
type ProviderConfigView struct {
	Plaid  PlaidProviderView  `json:"plaid"`
	Teller TellerProviderView `json:"teller"`
}

// PlaidProviderView holds the redacted Plaid block.
type PlaidProviderView struct {
	Configured  bool   `json:"configured"`
	FromEnv     bool   `json:"from_env"`
	ClientID    string `json:"client_id,omitempty"`
	Environment string `json:"environment,omitempty"`
	WebhookURL  string `json:"webhook_url,omitempty"`
	SecretSet   bool   `json:"secret_set"`
}

// TellerProviderView holds the redacted Teller block.
type TellerProviderView struct {
	Configured       bool   `json:"configured"`
	FromEnv          bool   `json:"from_env"`
	ApplicationID    string `json:"application_id,omitempty"`
	Environment      string `json:"environment,omitempty"`
	CertificateSet   bool   `json:"certificate_set"`
	WebhookSecretSet bool   `json:"webhook_secret_set"`
}

// UpdatePlaidParams is the body shape for PUT /settings/providers/plaid.
type UpdatePlaidParams struct {
	ClientID    string  `json:"client_id"`
	Secret      *string `json:"secret,omitempty"`
	Environment string  `json:"environment,omitempty"`
	WebhookURL  string  `json:"webhook_url,omitempty"`
}

// UpdateTellerParams is the body shape for PUT /settings/providers/teller.
type UpdateTellerParams struct {
	ApplicationID string  `json:"application_id"`
	Environment   string  `json:"environment,omitempty"`
	Certificate   *string `json:"certificate,omitempty"`
	PrivateKey    *string `json:"private_key,omitempty"`
	WebhookSecret *string `json:"webhook_secret,omitempty"`
}

// ProviderTestResult mirrors api.providerTestResult.
type ProviderTestResult struct {
	Provider string `json:"provider"`
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
}

// ListProviders fetches the provider registry.
func (c *Client) ListProviders(ctx context.Context) ([]ProviderInfo, error) {
	var out []ProviderInfo
	if err := c.Do(ctx, http.MethodGet, "/api/v1/providers", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetProviderConfig fetches the redacted provider-credentials view.
func (c *Client) GetProviderConfig(ctx context.Context) (*ProviderConfigView, error) {
	var out ProviderConfigView
	if err := c.Do(ctx, http.MethodGet, "/api/v1/settings/providers", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePlaidConfig writes a new Plaid credentials set.
func (c *Client) UpdatePlaidConfig(ctx context.Context, p UpdatePlaidParams) (*ProviderConfigView, error) {
	var out ProviderConfigView
	if err := c.Do(ctx, http.MethodPut, "/api/v1/settings/providers/plaid", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTellerConfig writes a new Teller credentials set.
func (c *Client) UpdateTellerConfig(ctx context.Context, p UpdateTellerParams) (*ProviderConfigView, error) {
	var out ProviderConfigView
	if err := c.Do(ctx, http.MethodPut, "/api/v1/settings/providers/teller", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TestProvider runs a server-side credentials check against the given provider.
func (c *Client) TestProvider(ctx context.Context, name string) (*ProviderTestResult, error) {
	path := "/api/v1/providers/" + url.PathEscape(name) + "/test"
	var out ProviderTestResult
	if err := c.Do(ctx, http.MethodPost, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DisableProvider clears credentials + drops the provider from the live map.
func (c *Client) DisableProvider(ctx context.Context, name string) (*ProviderTestResult, error) {
	path := "/api/v1/providers/" + url.PathEscape(name)
	var out ProviderTestResult
	if err := c.Do(ctx, http.MethodDelete, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
