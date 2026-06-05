//go:build !lite

package simplefin

import (
	"context"

	"breadbox/internal/provider"
)

// HandleWebhook is unsupported: SimpleFIN is poll-only and never pushes events.
func (p *SimpleFINProvider) HandleWebhook(ctx context.Context, payload provider.WebhookPayload) (provider.WebhookEvent, error) {
	return provider.WebhookEvent{}, provider.ErrNotSupported
}

// CreateReauthSession is unsupported: SimpleFIN reauth is a fresh token paste,
// not a server-mintable session. The admin relink flow re-runs the token-paste
// exchange against the existing connection and rotates its access URL in place.
func (p *SimpleFINProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	return provider.LinkSession{}, provider.ErrNotSupported
}

// RemoveConnection is a no-op. SimpleFIN has no documented revoke endpoint; the
// user disables the access token at their bridge. Local cleanup proceeds.
func (p *SimpleFINProvider) RemoveConnection(ctx context.Context, conn provider.Connection) error {
	return nil
}
