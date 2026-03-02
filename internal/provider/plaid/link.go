package plaid

import (
	"context"
	"fmt"

	"breadbox/internal/provider"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// CreateLinkSession creates a Plaid Link token for a new bank connection.
func (p *PlaidProvider) CreateLinkSession(ctx context.Context, userID string) (provider.LinkSession, error) {
	user := plaidgo.LinkTokenCreateRequestUser{
		ClientUserId: userID,
	}

	daysRequested := int32(730)
	txOpts := plaidgo.NewLinkTokenTransactions()
	txOpts.SetDaysRequested(daysRequested)

	req := plaidgo.NewLinkTokenCreateRequest(
		"Breadbox",
		"en",
		[]plaidgo.CountryCode{plaidgo.COUNTRYCODE_US},
		user,
	)
	req.SetProducts([]plaidgo.Products{plaidgo.PRODUCTS_TRANSACTIONS})
	req.SetTransactions(*txOpts)

	if p.webhookURL != "" {
		req.SetWebhook(p.webhookURL)
	}

	resp, _, err := p.client.PlaidApi.
		LinkTokenCreate(ctx).
		LinkTokenCreateRequest(*req).
		Execute()
	if err != nil {
		return provider.LinkSession{}, fmt.Errorf("plaid link token create: %w", err)
	}

	return provider.LinkSession{
		Token:  resp.GetLinkToken(),
		Expiry: resp.GetExpiration(),
	}, nil
}
