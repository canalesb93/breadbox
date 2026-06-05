//go:build !lite

package simplefin

import (
	"log/slog"

	"breadbox/internal/provider"
)

// Compile-time check that SimpleFINProvider implements provider.Provider.
var _ provider.Provider = (*SimpleFINProvider)(nil)

// SimpleFINProvider implements provider.Provider using the SimpleFIN protocol
// (https://www.simplefin.org/protocol.html). SimpleFIN is a token-paste,
// poll-only protocol: there is no OAuth handshake and no webhooks. The user
// pastes a one-time setup token which is claimed for a long-lived access URL
// (HTTP Basic credentials embedded), and transactions are fetched by polling
// GET {accessURL}/accounts over a date range.
type SimpleFINProvider struct {
	client        *Client
	encryptionKey []byte
	logger        *slog.Logger
}

// NewProvider creates a new SimpleFINProvider.
func NewProvider(client *Client, encryptionKey []byte, logger *slog.Logger) *SimpleFINProvider {
	return &SimpleFINProvider{
		client:        client,
		encryptionKey: encryptionKey,
		logger:        logger,
	}
}

// ReconcilesPendingByPolling reports that SimpleFIN re-returns its full
// transaction window on every sync (date-range polling), so the sync engine
// should soft-delete pending rows no longer present in the window.
func (p *SimpleFINProvider) ReconcilesPendingByPolling() bool { return true }
