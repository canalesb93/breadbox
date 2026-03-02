package teller

import (
	"context"

	"breadbox/internal/provider"
)

// CreateReauthSession returns the enrollment ID as the link token for
// Teller Connect reconnection mode. No API call is needed.
func (p *TellerProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{
		Token: conn.ExternalID,
	}, nil
}
