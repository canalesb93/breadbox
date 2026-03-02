package plaid

import (
	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// NewPlaidClient creates a configured Plaid API client from credentials and environment.
func NewPlaidClient(clientID, secret, env string) *plaidgo.APIClient {
	cfg := plaidgo.NewConfiguration()
	cfg.AddDefaultHeader("PLAID-CLIENT-ID", clientID)
	cfg.AddDefaultHeader("PLAID-SECRET", secret)

	switch env {
	case "production":
		cfg.UseEnvironment(plaidgo.Production)
	default:
		// The Plaid Go SDK v29 only exposes Sandbox and Production environments.
		// "development" and "sandbox" both route to the Sandbox base URL.
		cfg.UseEnvironment(plaidgo.Sandbox)
	}

	return plaidgo.NewAPIClient(cfg)
}
