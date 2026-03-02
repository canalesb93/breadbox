package plaid

import (
	"context"
	"fmt"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
)

// ValidateCredentials verifies Plaid API credentials by making a lightweight
// /institutions/get call with count=1. Returns nil if credentials are valid.
func ValidateCredentials(ctx context.Context, clientID, secret, env string) error {
	client := NewPlaidClient(clientID, secret, env)

	req := client.PlaidApi.InstitutionsGet(ctx)
	req = req.InstitutionsGetRequest(plaidgo.InstitutionsGetRequest{
		Count:        1,
		Offset:       0,
		CountryCodes: []plaidgo.CountryCode{plaidgo.COUNTRYCODE_US},
	})

	_, _, err := req.Execute()
	if err != nil {
		var plaidErr *plaidgo.PlaidError
		if genErr, ok := err.(*plaidgo.GenericOpenAPIError); ok {
			if pErr, ok := genErr.Model().(plaidgo.PlaidError); ok {
				plaidErr = &pErr
			}
		}
		if plaidErr != nil {
			return fmt.Errorf("Plaid error: %s", plaidErr.GetErrorMessage())
		}
		return fmt.Errorf("could not connect to Plaid: %v", err)
	}

	return nil
}
