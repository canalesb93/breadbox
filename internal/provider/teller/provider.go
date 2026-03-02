package teller

import (
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

