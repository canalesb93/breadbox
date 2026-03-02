package plaid

import (
	"context"
	"fmt"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// RemoveConnection is the interface method. Since the Provider interface
// receives only a connectionID string, the caller must load the connection
// from the DB, decrypt the access token, and call RemoveItem directly.
func (p *PlaidProvider) RemoveConnection(_ context.Context, _ string) error {
	return fmt.Errorf("RemoveConnection requires connection lookup — use RemoveItem helper")
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
