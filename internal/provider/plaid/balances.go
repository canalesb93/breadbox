package plaid

import (
	"context"
	"fmt"

	"breadbox/internal/provider"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
	"github.com/shopspring/decimal"
)

// GetBalances fetches current account balances from Plaid using /accounts/get.
func (p *PlaidProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	accessToken, err := Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt access token: %w", err)
	}

	req := plaidgo.NewAccountsGetRequest(string(accessToken))
	resp, _, err := p.client.PlaidApi.
		AccountsGet(ctx).
		AccountsGetRequest(*req).
		Execute()
	if err != nil {
		if plaidErr := extractPlaidError(err); plaidErr != nil {
			switch plaidErr.GetErrorCode() {
			case "ITEM_LOGIN_REQUIRED", "INVALID_CREDENTIALS", "ITEM_LOCKED":
				return nil, ErrItemReauthRequired
			}
		}
		return nil, fmt.Errorf("plaid accounts get: %w", err)
	}

	balances := make([]provider.AccountBalance, 0, len(resp.GetAccounts()))
	for _, acct := range resp.GetAccounts() {
		bal := acct.GetBalances()

		ab := provider.AccountBalance{
			AccountExternalID: acct.GetAccountId(),
			Current:           decimal.NewFromFloat(bal.GetCurrent()),
		}

		if avail, ok := bal.GetAvailableOk(); ok && avail != nil {
			d := decimal.NewFromFloat(*avail)
			ab.Available = &d
		}

		if limit, ok := bal.GetLimitOk(); ok && limit != nil {
			d := decimal.NewFromFloat(*limit)
			ab.Limit = &d
		}

		if code, ok := bal.GetIsoCurrencyCodeOk(); ok && code != nil {
			ab.ISOCurrencyCode = *code
		}

		balances = append(balances, ab)
	}

	return balances, nil
}
