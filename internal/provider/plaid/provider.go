package plaid

import (
	"context"
	"errors"
	"log/slog"

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

// SyncTransactions is not yet implemented (Phase 3).
func (p *PlaidProvider) SyncTransactions(_ context.Context, _ provider.Connection, _ string) (provider.SyncResult, error) {
	return provider.SyncResult{}, errors.New("SyncTransactions not implemented")
}

// GetBalances is not yet implemented (Phase 3).
func (p *PlaidProvider) GetBalances(_ context.Context, _ provider.Connection) ([]provider.AccountBalance, error) {
	return nil, errors.New("GetBalances not implemented")
}

// HandleWebhook is not yet implemented (Phase 3).
func (p *PlaidProvider) HandleWebhook(_ context.Context, _ provider.WebhookPayload) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, errors.New("HandleWebhook not implemented")
}
