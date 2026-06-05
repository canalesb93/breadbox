//go:build !lite

package simplefin

import (
	"context"
	"fmt"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"
	"breadbox/internal/shortid"
)

// CreateLinkSession is unsupported: SimpleFIN has no server-side token mint. The
// user obtains a setup token from their SimpleFIN bridge's /create page and
// pastes it; the admin connect flow passes it straight to ExchangeToken.
func (p *SimpleFINProvider) CreateLinkSession(ctx context.Context, userID string) (provider.LinkSession, error) {
	return provider.LinkSession{}, provider.ErrNotSupported
}

// ExchangeToken claims a pasted SimpleFIN setup token for a long-lived access
// URL, encrypts the access URL as the connection credential, and discovers the
// accounts reachable through it.
//
// SimpleFIN exposes no stable upstream connection identifier (a single access
// URL spans every bank the user linked, and re-claiming yields a brand-new
// URL), so we mint our own opaque external_id. Reauth keeps the same row and
// only rotates the encrypted credential (see the admin relink path).
func (p *SimpleFINProvider) ExchangeToken(ctx context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	claimURL, err := decodeSetupToken(publicToken)
	if err != nil {
		return provider.Connection{}, nil, err
	}

	accessURL, err := p.client.claim(ctx, claimURL)
	if err != nil {
		return provider.Connection{}, nil, err
	}

	encrypted, err := crypto.Encrypt([]byte(accessURL), p.encryptionKey)
	if err != nil {
		return provider.Connection{}, nil, fmt.Errorf("simplefin: encrypt access URL: %w", err)
	}

	externalID, err := shortid.Generate()
	if err != nil {
		return provider.Connection{}, nil, fmt.Errorf("simplefin: mint external id: %w", err)
	}

	accounts, err := p.discoverAccounts(ctx, accessURL)
	if err != nil {
		return provider.Connection{}, nil, fmt.Errorf("simplefin: discover accounts after claim: %w", err)
	}

	conn := provider.Connection{
		ProviderName:         "simplefin",
		ExternalID:           externalID,
		EncryptedCredentials: encrypted,
		InstitutionName:      institutionName(accounts),
	}

	return conn, accounts, nil
}

// discoverAccounts fetches the account list (balances only — no transactions)
// for initial connection setup.
func (p *SimpleFINProvider) discoverAccounts(ctx context.Context, accessURL string) ([]provider.Account, error) {
	set, err := p.fetchAccountSet(ctx, accessURL, "balances-only=1")
	if err != nil {
		return nil, err
	}

	accounts := make([]provider.Account, 0, len(set.Accounts))
	for _, a := range set.Accounts {
		accounts = append(accounts, a.toAccount())
	}

	if len(accounts) == 0 {
		if errs := set.errorStrings(); len(errs) > 0 {
			return nil, fmt.Errorf("simplefin: no accounts returned; server reported: %v", errs)
		}
		return nil, fmt.Errorf("simplefin: no accounts returned for this access URL")
	}

	return accounts, nil
}

// institutionName summarizes the connection's institution label. A SimpleFIN
// access URL can span multiple banks, so we use the single org name when there's
// only one, and a generic label otherwise.
func institutionName(accounts []provider.Account) string {
	seen := map[string]struct{}{}
	var only string
	for _, a := range accounts {
		if a.OfficialName == "" {
			continue
		}
		if _, ok := seen[a.OfficialName]; !ok {
			seen[a.OfficialName] = struct{}{}
			only = a.OfficialName
		}
	}
	if len(seen) == 1 {
		return only
	}
	return "SimpleFIN"
}
