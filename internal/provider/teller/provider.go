package teller

import (
	"context"
	"fmt"
	"log/slog"

	"breadbox/internal/provider"
)

// Compile-time check that TellerProvider implements provider.Provider.
var _ provider.Provider = (*TellerProvider)(nil)

// TellerProvider implements provider.Provider using the Teller API.
type TellerProvider struct {
	client        *Client
	appID         string
	env           string
	webhookSecret string
	encryptionKey []byte
	logger        *slog.Logger
}

// NewProvider creates a new TellerProvider.
func NewProvider(client *Client, appID, env, webhookSecret string, encryptionKey []byte, logger *slog.Logger) *TellerProvider {
	return &TellerProvider{
		client:        client,
		appID:         appID,
		env:           env,
		webhookSecret: webhookSecret,
		encryptionKey: encryptionKey,
		logger:        logger,
	}
}

// CreateLinkSession returns the app ID as the link token. Teller Connect
// is initialized client-side with just the application ID.
func (p *TellerProvider) CreateLinkSession(ctx context.Context, userID string) (provider.LinkSession, error) {
	return provider.LinkSession{}, fmt.Errorf("teller: not implemented")
}

// ExchangeToken parses the Teller Connect onSuccess payload, encrypts the
// access token, and discovers accounts.
func (p *TellerProvider) ExchangeToken(ctx context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	return provider.Connection{}, nil, fmt.Errorf("teller: not implemented")
}

// SyncTransactions fetches transactions using date-range polling.
func (p *TellerProvider) SyncTransactions(ctx context.Context, conn provider.Connection, cursor string) (provider.SyncResult, error) {
	return provider.SyncResult{}, fmt.Errorf("teller: not implemented")
}

// GetBalances fetches current balances for all accounts in the connection.
func (p *TellerProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	return nil, fmt.Errorf("teller: not implemented")
}

// HandleWebhook verifies and parses an inbound Teller webhook.
func (p *TellerProvider) HandleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, fmt.Errorf("teller: not implemented")
}

// CreateReauthSession returns the enrollment ID as the link token for
// Teller Connect reconnection mode.
func (p *TellerProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{}, fmt.Errorf("teller: not implemented")
}

// RemoveConnection revokes Teller's access to the enrollment.
func (p *TellerProvider) RemoveConnection(ctx context.Context, conn provider.Connection) error {
	return fmt.Errorf("teller: not implemented")
}
