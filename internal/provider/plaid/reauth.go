package plaid

import (
	"context"
	"fmt"

	"breadbox/internal/provider"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// CreateReauthSession is the interface method. Since the Provider interface
// receives only a connectionID string, the caller must load the connection
// from the DB, decrypt the access token, and call CreateReauthLinkToken
// directly. This method returns an error directing callers to the helper.
func (p *PlaidProvider) CreateReauthSession(_ context.Context, _ string) (provider.LinkSession, error) {
	return provider.LinkSession{}, fmt.Errorf("CreateReauthSession requires connection lookup — use CreateReauthLinkToken helper")
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
