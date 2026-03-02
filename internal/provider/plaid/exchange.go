package plaid

import (
	"context"
	"fmt"

	"breadbox/internal/provider"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// ExchangeToken exchanges a public token for an access token, encrypts it,
// and fetches the account list from Plaid.
func (p *PlaidProvider) ExchangeToken(ctx context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	// Exchange the public token for an access token and item ID.
	exchangeReq := plaidgo.NewItemPublicTokenExchangeRequest(publicToken)
	exchangeResp, _, err := p.client.PlaidApi.
		ItemPublicTokenExchange(ctx).
		ItemPublicTokenExchangeRequest(*exchangeReq).
		Execute()
	if err != nil {
		return provider.Connection{}, nil, fmt.Errorf("plaid public token exchange: %w", err)
	}

	accessToken := exchangeResp.GetAccessToken()
	itemID := exchangeResp.GetItemId()

	// Encrypt the access token for storage.
	encrypted, err := Encrypt([]byte(accessToken), p.encryptionKey)
	if err != nil {
		return provider.Connection{}, nil, fmt.Errorf("encrypt access token: %w", err)
	}

	// Fetch accounts using the new access token.
	accounts, err := p.fetchAccounts(ctx, accessToken)
	if err != nil {
		return provider.Connection{}, nil, fmt.Errorf("fetch accounts after exchange: %w", err)
	}

	conn := provider.Connection{
		ProviderName:         "plaid",
		ExternalID:           itemID,
		EncryptedCredentials: encrypted,
		InstitutionName:      "", // Caller fills this from Link metadata.
	}

	return conn, accounts, nil
}

// fetchAccounts calls /accounts/get and maps the response to provider.Account.
func (p *PlaidProvider) fetchAccounts(ctx context.Context, accessToken string) ([]provider.Account, error) {
	req := plaidgo.NewAccountsGetRequest(accessToken)
	resp, _, err := p.client.PlaidApi.
		AccountsGet(ctx).
		AccountsGetRequest(*req).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("plaid accounts get: %w", err)
	}

	accounts := make([]provider.Account, 0, len(resp.GetAccounts()))
	for _, a := range resp.GetAccounts() {
		acct := provider.Account{
			ExternalID: a.GetAccountId(),
			Name:       a.GetName(),
			Mask:       a.GetMask(),
			Type:       string(a.GetType()),
			Subtype:    string(a.GetSubtype()),
		}
		if name, ok := a.GetOfficialNameOk(); ok && name != nil {
			acct.OfficialName = *name
		}

		bal := a.GetBalances()
		if code, ok := bal.GetIsoCurrencyCodeOk(); ok && code != nil {
			acct.ISOCurrencyCode = *code
		}

		accounts = append(accounts, acct)
	}

	return accounts, nil
}
