package teller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"

	"github.com/shopspring/decimal"
)

// tellerBalance represents a balance response from the Teller API.
type tellerBalance struct {
	AccountID string `json:"account_id"`
	Ledger    string `json:"ledger"`
	Available string `json:"available"`
}

// GetBalances fetches current balances for all accounts in the connection.
func (p *TellerProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	accessToken, err := crypto.Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("teller: decrypt access token: %w", err)
	}
	token := string(accessToken)

	// Get accounts to iterate and to get currency codes.
	accounts, err := p.fetchAccounts(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("teller: fetch accounts for balances: %w", err)
	}

	currencyMap := make(map[string]string, len(accounts))
	for _, acct := range accounts {
		currencyMap[acct.ExternalID] = acct.ISOCurrencyCode
	}

	balances := make([]provider.AccountBalance, 0, len(accounts))
	for _, acct := range accounts {
		path := fmt.Sprintf("/accounts/%s/balances", acct.ExternalID)
		resp, err := p.client.doWithRetry(ctx, http.MethodGet, path, token, "")
		if err != nil {
			return nil, fmt.Errorf("teller balance get for %s: %w", acct.ExternalID, err)
		}

		if resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			return nil, ErrReauthRequired
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("teller balance get: status %d: %s", resp.StatusCode, body)
		}

		var bal tellerBalance
		if err := json.NewDecoder(resp.Body).Decode(&bal); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("teller balance decode for %s: %w", acct.ExternalID, err)
		}
		resp.Body.Close()

		current, err := decimal.NewFromString(bal.Ledger)
		if err != nil {
			return nil, fmt.Errorf("teller: parse ledger balance %q for %s: %w", bal.Ledger, acct.ExternalID, err)
		}

		ab := provider.AccountBalance{
			AccountExternalID: acct.ExternalID,
			Current:           current,
			ISOCurrencyCode:   currencyMap[acct.ExternalID],
		}

		if bal.Available != "" {
			avail, err := decimal.NewFromString(bal.Available)
			if err != nil {
				return nil, fmt.Errorf("teller: parse available balance %q for %s: %w", bal.Available, acct.ExternalID, err)
			}
			ab.Available = &avail
		}

		balances = append(balances, ab)
	}

	return balances, nil
}
