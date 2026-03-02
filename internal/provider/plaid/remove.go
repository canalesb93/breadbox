package plaid

import (
	"context"
	"fmt"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// RemoveConnection decrypts the connection's credentials and calls Plaid
// /item/remove to revoke access.
func (p *PlaidProvider) RemoveConnection(ctx context.Context, conn provider.Connection) error {
	accessToken, err := crypto.Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return fmt.Errorf("decrypt access token for removal: %w", err)
	}
	return p.RemoveItem(ctx, string(accessToken))
}

// RemoveItem calls Plaid /item/remove with the decrypted access token.
// If the token is already invalid (e.g., item already removed), the error
// is logged and nil is returned so the caller can proceed with local cleanup.
func (p *PlaidProvider) RemoveItem(ctx context.Context, decryptedAccessToken string) error {
	req := plaidgo.NewItemRemoveRequest(decryptedAccessToken)
	_, _, err := p.client.PlaidApi.
		ItemRemove(ctx).
		ItemRemoveRequest(*req).
		Execute()
	if err != nil {
		// If the token is already invalid, Plaid returns an error.
		// Log it and continue — the caller should still clean up locally.
		p.logger.WarnContext(ctx, "plaid item remove failed (token may already be invalid)",
			"error", err,
		)
		return nil
	}

	return nil
}
