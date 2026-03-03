package csv

import (
	"context"

	"breadbox/internal/provider"
)

// Compile-time check that CSVProvider implements the Provider interface.
var _ provider.Provider = (*CSVProvider)(nil)

// CSVProvider is a stub implementation of the Provider interface for CSV imports.
// CSV import bypasses the provider interface (uses the service layer directly).
// This stub exists so the provider registry has a "csv" entry.
type CSVProvider struct{}

// NewProvider creates a new CSVProvider.
func NewProvider() *CSVProvider {
	return &CSVProvider{}
}

func (p *CSVProvider) CreateLinkSession(ctx context.Context, userID string) (provider.LinkSession, error) {
	return provider.LinkSession{}, provider.ErrNotSupported
}

func (p *CSVProvider) ExchangeToken(ctx context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	return provider.Connection{}, nil, provider.ErrNotSupported
}

func (p *CSVProvider) SyncTransactions(ctx context.Context, conn provider.Connection, cursor string) (provider.SyncResult, error) {
	return provider.SyncResult{}, provider.ErrNotSupported
}

func (p *CSVProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	return nil, provider.ErrNotSupported
}

func (p *CSVProvider) HandleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, provider.ErrNotSupported
}

func (p *CSVProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{}, provider.ErrNotSupported
}

func (p *CSVProvider) RemoveConnection(ctx context.Context, conn provider.Connection) error {
	return nil
}
