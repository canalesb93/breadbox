package plaid

import (
	"context"
	"fmt"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// CreateReauthSession creates a Plaid Link token in update mode for
// re-authenticating an existing connection. It decrypts the connection's
// credentials and delegates to CreateReauthLinkToken.
func (p *PlaidProvider) CreateReauthSession(ctx context.Context, conn provider.Connection) (provider.LinkSession, error) {
	accessToken, err := crypto.Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return provider.LinkSession{}, fmt.Errorf("decrypt access token for reauth: %w", err)
	}
	return p.CreateReauthLinkToken(ctx, string(accessToken), conn.UserID)
}

// CreateReauthLinkToken creates a Plaid Link token in update mode for
// re-authenticating an existing connection. The caller provides the
// decrypted access token and userID.
func (p *PlaidProvider) CreateReauthLinkToken(ctx context.Context, decryptedAccessToken, userID string) (provider.LinkSession, error) {
	user := plaidgo.LinkTokenCreateRequestUser{
		ClientUserId: userID,
	}

	req := plaidgo.NewLinkTokenCreateRequest(
		"Breadbox",
		"en",
		[]plaidgo.CountryCode{plaidgo.COUNTRYCODE_US},
		user,
	)
	// Setting access_token activates Link update mode.
	// Do NOT set products or transactions options in update mode.
	req.SetAccessToken(decryptedAccessToken)

	if p.webhookURL != "" {
		req.SetWebhook(p.webhookURL)
	}

	resp, _, err := p.client.PlaidApi.
		LinkTokenCreate(ctx).
		LinkTokenCreateRequest(*req).
		Execute()
	if err != nil {
		return provider.LinkSession{}, fmt.Errorf("plaid link token create (update mode): %w", err)
	}

	return provider.LinkSession{
		Token:  resp.GetLinkToken(),
		Expiry: resp.GetExpiration(),
	}, nil
}
