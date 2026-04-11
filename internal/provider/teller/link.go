package teller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"
)

// tellerExchangePayload is the JSON structure received from Teller Connect's
// onSuccess callback, forwarded by the client as the publicToken.
type tellerExchangePayload struct {
	AccessToken     string `json:"access_token"`
	EnrollmentID    string `json:"enrollment_id"`
	InstitutionName string `json:"institution_name"`
}

// tellerAccount represents an account in the Teller API response.
type tellerAccount struct {
	ID           string `json:"id"`
	EnrollmentID string `json:"enrollment_id"`
	Type         string `json:"type"`
	Subtype      string `json:"subtype"`
	Status       string `json:"status"`
	Currency     string `json:"currency"`
	LastFour     string `json:"last_four"`
	Name         string `json:"name"`
	Institution  struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"institution"`
}

// CreateLinkSession returns the app ID as the link token. Teller Connect
// is initialized client-side with just the application ID.
func (p *TellerProvider) CreateLinkSession(ctx context.Context, userID string) (provider.LinkSession, error) {
	return provider.LinkSession{
		Token: p.appID,
	}, nil
}

// ExchangeToken parses the Teller Connect onSuccess payload, encrypts the
// access token, and discovers accounts.
func (p *TellerProvider) ExchangeToken(ctx context.Context, publicToken string) (provider.Connection, []provider.Account, error) {
	var payload tellerExchangePayload
	if err := json.Unmarshal([]byte(publicToken), &payload); err != nil {
		return provider.Connection{}, nil, fmt.Errorf("teller: parse exchange payload: %w", err)
	}

	if payload.AccessToken == "" || payload.EnrollmentID == "" {
		return provider.Connection{}, nil, fmt.Errorf("teller: exchange payload missing access_token or enrollment_id")
	}

	encrypted, err := crypto.Encrypt([]byte(payload.AccessToken), p.encryptionKey)
	if err != nil {
		return provider.Connection{}, nil, fmt.Errorf("teller: encrypt access token: %w", err)
	}

	accounts, err := p.fetchAccounts(ctx, payload.AccessToken)
	if err != nil {
		return provider.Connection{}, nil, fmt.Errorf("teller: fetch accounts after exchange: %w", err)
	}

	conn := provider.Connection{
		ProviderName:         "teller",
		ExternalID:           payload.EnrollmentID,
		EncryptedCredentials: encrypted,
		InstitutionName:      payload.InstitutionName,
	}

	return conn, accounts, nil
}

// fetchAccounts calls GET /accounts and maps the response to provider.Account.
func (p *TellerProvider) fetchAccounts(ctx context.Context, accessToken string) ([]provider.Account, error) {
	resp, err := p.client.doWithRetry(ctx, http.MethodGet, "/accounts", accessToken, "")
	if err != nil {
		return nil, fmt.Errorf("teller accounts get: %w", err)
	}
	defer resp.Body.Close()

	if isReauthResponse(resp) {
		return nil, ErrReauthRequired
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, classifyHTTPError("accounts get", resp.StatusCode, body)
	}

	var tellerAccounts []tellerAccount
	if err := json.NewDecoder(resp.Body).Decode(&tellerAccounts); err != nil {
		return nil, fmt.Errorf("teller accounts decode: %w", err)
	}

	accounts := make([]provider.Account, 0, len(tellerAccounts))
	for _, a := range tellerAccounts {
		accounts = append(accounts, provider.Account{
			ExternalID:      a.ID,
			Name:            a.Name,
			Type:            a.Type,
			Subtype:         a.Subtype,
			Mask:            a.LastFour,
			ISOCurrencyCode: a.Currency,
		})
	}

	return accounts, nil
}
