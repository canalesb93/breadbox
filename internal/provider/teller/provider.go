//go:build !lite

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

// ReconcilesPendingByPolling reports that Teller re-returns its full
// transaction window on every sync (date-range polling), so the sync engine
// should soft-delete pending rows no longer present in the window.
func (p *TellerProvider) ReconcilesPendingByPolling() bool { return true }

