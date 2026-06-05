//go:build !lite

package simplefin

import (
	"context"
	"fmt"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"

	"github.com/shopspring/decimal"
)

// GetBalances fetches current balances for every account reachable through the
// connection's access URL (balances only — no transactions). SimpleFIN exposes
// no credit limit, so AccountBalance.Limit is always nil.
func (p *SimpleFINProvider) GetBalances(ctx context.Context, conn provider.Connection) ([]provider.AccountBalance, error) {
	accessURLBytes, err := crypto.Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("simplefin: decrypt access URL: %w", err)
	}

	set, err := p.fetchAccountSet(ctx, string(accessURLBytes), "balances-only=1")
	if err != nil {
		return nil, err
	}

	balances := make([]provider.AccountBalance, 0, len(set.Accounts))
	for _, acct := range set.Accounts {
		current, err := decimal.NewFromString(acct.Balance)
		if err != nil {
			p.logger.WarnContext(ctx, "simplefin: skipping balance with parse error",
				"account_id", acct.ID, "balance", acct.Balance, "error", err)
			continue
		}

		ab := provider.AccountBalance{
			AccountExternalID: acct.ID,
			Current:           current,
			ISOCurrencyCode:   acct.Currency,
		}

		if acct.AvailableBalance != "" {
			if avail, err := decimal.NewFromString(acct.AvailableBalance); err == nil {
				ab.Available = &avail
			}
		}

		balances = append(balances, ab)
	}

	return balances, nil
}
