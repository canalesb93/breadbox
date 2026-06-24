package client

import (
	"context"
	"net/http"
	"net/url"
)

// Account mirrors service.AccountResponse — the JSON shape returned by
// GET /api/v1/accounts and friends. Defined here so the CLI doesn't
// depend on the `internal/service` package (which is server-only under
// the `-tags=lite` build).
type Account struct {
	ID                string   `json:"id"`
	ShortID           string   `json:"short_id"`
	ConnectionID      *string  `json:"connection_id,omitempty"`
	UserID            *string  `json:"user_id,omitempty"`
	InstitutionName   *string  `json:"institution_name,omitempty"`
	Name              string   `json:"name"`
	OfficialName      *string  `json:"official_name,omitempty"`
	Type              string   `json:"type"`
	Subtype           *string  `json:"subtype,omitempty"`
	Mask              *string  `json:"mask,omitempty"`
	BalanceCurrent    *float64 `json:"balance_current,omitempty"`
	BalanceAvailable  *float64 `json:"balance_available,omitempty"`
	BalanceLimit      *float64 `json:"balance_limit,omitempty"`
	IsoCurrencyCode   *string  `json:"iso_currency_code,omitempty"`
	LastBalanceUpdate *string  `json:"last_balance_update,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
	ConnectionStatus  *string  `json:"connection_status,omitempty"`
	IsDependentLinked bool     `json:"is_dependent_linked"`
	OwnerUserID       *string  `json:"owner_user_id,omitempty"`
	OwnerUserName     *string  `json:"owner_user_name,omitempty"`
}

// AccountBalance is a single-currency balance block.
type AccountBalance struct {
	IsoCurrencyCode  *string  `json:"iso_currency_code"`
	BalanceCurrent   *float64 `json:"balance_current"`
	BalanceAvailable *float64 `json:"balance_available"`
	BalanceLimit     *float64 `json:"balance_limit"`
}

// AccountDetail mirrors service.AccountDetailResponse — the
// GET /api/v1/accounts/{id}/detail payload.
type AccountDetail struct {
	Account
	DisplayName        *string       `json:"display_name,omitempty"`
	Excluded           bool          `json:"excluded"`
	Provider           string        `json:"provider,omitempty"`
	ConnectionUserName string        `json:"connection_user_name,omitempty"`
	ConnectionShortID  string        `json:"connection_short_id,omitempty"`
	Balances           []AccountBalance `json:"balances"`
	RecentTransactions []Transaction    `json:"recent_transactions"`
}

// AccountPatch carries the optional fields accepted by
// PATCH /api/v1/accounts/{id}.
type AccountPatch struct {
	DisplayName       *string `json:"display_name,omitempty"`
	IsExcluded        *bool   `json:"is_excluded,omitempty"`
	IsDependentLinked *bool   `json:"is_dependent_linked,omitempty"`
	OwnerUserID       *string `json:"owner_user_id,omitempty"`
}

// AccountLink mirrors service.AccountLinkResponse — the
// GET /api/v1/account-links payload row.
type AccountLink struct {
	ID                      string `json:"id"`
	ShortID                 string `json:"short_id"`
	PrimaryAccountID        string `json:"primary_account_id"`
	PrimaryAccountName      string `json:"primary_account_name"`
	PrimaryUserName         string `json:"primary_user_name"`
	DependentAccountID      string `json:"dependent_account_id"`
	DependentAccountName    string `json:"dependent_account_name"`
	DependentUserName       string `json:"dependent_user_name"`
	MatchStrategy           string `json:"match_strategy"`
	MatchToleranceDays      int    `json:"match_tolerance_days"`
	Enabled                 bool   `json:"enabled"`
	MatchCount              int64  `json:"match_count"`
	UnmatchedDependentCount int64  `json:"unmatched_dependent_count"`
	CreatedAt               string `json:"created_at"`
	UpdatedAt               string `json:"updated_at"`
}

// CreateAccountLinkParams matches the POST /api/v1/account-links body.
type CreateAccountLinkParams struct {
	PrimaryAccountID   string `json:"primary_account_id"`
	DependentAccountID string `json:"dependent_account_id"`
	MatchStrategy      string `json:"match_strategy,omitempty"`
	MatchToleranceDays int    `json:"match_tolerance_days,omitempty"`
}

// ListAccounts fetches the household's accounts. `fields` and `userID`
// are passed through as query params (each empty when not set).
func (c *Client) ListAccounts(ctx context.Context, fields, userID string) ([]Account, error) {
	path := "/api/v1/accounts"
	q := url.Values{}
	if fields != "" {
		q.Set("fields", fields)
	}
	if userID != "" {
		q.Set("user_id", userID)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []Account
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetAccount fetches a single account by short_id or uuid.
func (c *Client) GetAccount(ctx context.Context, id, fields string) (*Account, error) {
	path := "/api/v1/accounts/" + url.PathEscape(id)
	if fields != "" {
		path += "?fields=" + url.QueryEscape(fields)
	}
	var out Account
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAccountDetail fetches the full detail payload (balances + recent txns).
func (c *Client) GetAccountDetail(ctx context.Context, id string) (*AccountDetail, error) {
	path := "/api/v1/accounts/" + url.PathEscape(id) + "/detail"
	var out AccountDetail
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateAccount patches the named fields. Fields left nil are not sent.
func (c *Client) UpdateAccount(ctx context.Context, id string, patch AccountPatch) (*Account, error) {
	path := "/api/v1/accounts/" + url.PathEscape(id)
	var out Account
	if err := c.Do(ctx, http.MethodPatch, path, patch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAccountLinks returns every account-link row. The server doesn't yet
// filter by `account_id`, so the CLI filters client-side when scoping to
// one account.
func (c *Client) ListAccountLinks(ctx context.Context) ([]AccountLink, error) {
	var out []AccountLink
	if err := c.Do(ctx, http.MethodGet, "/api/v1/account-links", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateAccountLink mints a new primary/dependent account-link.
func (c *Client) CreateAccountLink(ctx context.Context, params CreateAccountLinkParams) (*AccountLink, error) {
	var out AccountLink
	if err := c.Do(ctx, http.MethodPost, "/api/v1/account-links", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAccountLink drops the link by id.
func (c *Client) DeleteAccountLink(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/account-links/"+url.PathEscape(id), nil, nil)
}
