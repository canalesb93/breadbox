//go:build !lite

package plaid

import (
	"context"
	"log/slog"
	"sync"

	"breadbox/internal/provider"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// Compile-time check that PlaidProvider implements provider.Provider.
var _ provider.Provider = (*PlaidProvider)(nil)

// PlaidProvider implements provider.Provider using the Plaid API.
type PlaidProvider struct {
	client        *plaidgo.APIClient
	encryptionKey []byte
	webhookURL    string
	logger        *slog.Logger
	jwkCache      sync.Map // kid -> *ecdsa.PublicKey
}

// NewProvider creates a new PlaidProvider.
func NewProvider(client *plaidgo.APIClient, encryptionKey []byte, webhookURL string, logger *slog.Logger) *PlaidProvider {
	return &PlaidProvider{
		client:        client,
		encryptionKey: encryptionKey,
		webhookURL:    webhookURL,
		logger:        logger,
	}
}

// HandleWebhook verifies and parses an inbound Plaid webhook.
func (p *PlaidProvider) HandleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	return p.handleWebhook(ctx, payload)
}

// ReconcilesPendingByPolling reports false: Plaid uses cursor-based sync and
// links pending→posted via pending_transaction_id, so the engine must not
// soft-delete pending rows that weren't re-returned.
func (p *PlaidProvider) ReconcilesPendingByPolling() bool { return false }
