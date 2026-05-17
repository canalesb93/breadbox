//go:build !lite

package teller

import (
	"context"

	"breadbox/internal/provider"
)

// CreateReauthSession returns the enrollment ID as the link token plus the
// configured application id, both needed to boot Teller Connect in
// reconnection mode. No API call is needed.
func (p *TellerProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{
		Token:         conn.ExternalID,
		ApplicationID: p.appID,
	}, nil
}
