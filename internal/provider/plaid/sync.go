package plaid

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"breadbox/internal/provider"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
	"github.com/shopspring/decimal"
)

// SyncTransactions fetches incremental transaction changes from Plaid using the
// /transactions/sync endpoint. The cursor tracks pagination state; pass an
// empty string on the first call.
func (p *PlaidProvider) SyncTransactions(ctx context.Context, conn provider.Connection, cursor string) (provider.SyncResult, error) {
	accessToken, err := Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return provider.SyncResult{}, fmt.Errorf("decrypt access token: %w", err)
	}

	req := plaidgo.NewTransactionsSyncRequest(string(accessToken))
	if cursor != "" {
		req.SetCursor(cursor)
	}
	req.SetCount(500)

	resp, httpResp, err := p.syncWithRetry(ctx, req)
	if err != nil {
		return provider.SyncResult{}, err
	}
	if httpResp != nil {
		httpResp.Body.Close()
	}

	result := provider.SyncResult{
		Cursor:  resp.GetNextCursor(),
		HasMore: resp.GetHasMore(),
	}

	for _, txn := range resp.GetAdded() {
		result.Added = append(result.Added, mapTransaction(txn))
	}
	for _, txn := range resp.GetModified() {
		result.Modified = append(result.Modified, mapTransaction(txn))
	}
	for _, removed := range resp.GetRemoved() {
		result.Removed = append(result.Removed, removed.GetTransactionId())
	}

	return result, nil
}

// syncWithRetry calls TransactionsSync with exponential backoff on rate limits.
func (p *PlaidProvider) syncWithRetry(ctx context.Context, req *plaidgo.TransactionsSyncRequest) (plaidgo.TransactionsSyncResponse, *http.Response, error) {
	const maxRetries = 5
	delay := 2 * time.Second

	var zero plaidgo.TransactionsSyncResponse

	for attempt := 0; ; attempt++ {
		resp, httpResp, err := p.client.PlaidApi.
			TransactionsSync(ctx).
			TransactionsSyncRequest(*req).
			Execute()
		if err == nil {
			return resp, httpResp, nil
		}

		// Check for Plaid-specific errors.
		if plaidErr := extractPlaidError(err); plaidErr != nil {
			switch plaidErr.GetErrorCode() {
			case "TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION":
				return zero, nil, ErrMutationDuringPagination
			case "ITEM_LOGIN_REQUIRED", "INVALID_CREDENTIALS", "ITEM_LOCKED":
				return zero, nil, ErrItemReauthRequired
			}
		}

		// Retry on rate limits (HTTP 429).
		if httpResp != nil && httpResp.StatusCode == http.StatusTooManyRequests && attempt <= maxRetries {
			httpResp.Body.Close()
			p.logger.WarnContext(ctx, "plaid rate limited, retrying",
				"attempt", attempt+1,
				"delay", delay,
			)
			select {
			case <-ctx.Done():
				return zero, nil, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			if delay > 60*time.Second {
				delay = 60 * time.Second
			}
			continue
		}

		if httpResp != nil {
			httpResp.Body.Close()
		}
		return zero, nil, fmt.Errorf("plaid transactions sync: %w", err)
	}
}

// extractPlaidError attempts to extract a PlaidError from a Plaid API error.
func extractPlaidError(err error) *plaidgo.PlaidError {
	genErr, ok := err.(*plaidgo.GenericOpenAPIError)
	if !ok {
		return nil
	}
	if pErr, ok := genErr.Model().(plaidgo.PlaidError); ok {
		return &pErr
	}
	return nil
}

// mapTransaction converts a Plaid SDK Transaction to our provider.Transaction.
func mapTransaction(txn plaidgo.Transaction) provider.Transaction {
	t := provider.Transaction{
		ExternalID:        txn.GetTransactionId(),
		AccountExternalID: txn.GetAccountId(),
		Amount:            decimal.NewFromFloat(txn.GetAmount()),
		Name:              txn.GetName(),
		PaymentChannel:    txn.GetPaymentChannel(),
		Pending:           txn.GetPending(),
	}

	// Date (required, string "2006-01-02").
	if parsed, err := time.Parse("2006-01-02", txn.GetDate()); err == nil {
		t.Date = parsed
	}

	// Authorized date (optional).
	if dateStr, ok := txn.GetAuthorizedDateOk(); ok && dateStr != nil && *dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", *dateStr); err == nil {
			t.AuthorizedDate = &parsed
		}
	}

	// Datetime (optional, full ISO 8601).
	if dt, ok := txn.GetDatetimeOk(); ok && dt != nil {
		v := *dt
		t.Datetime = &v
	}

	// Authorized datetime (optional, full ISO 8601).
	if dt, ok := txn.GetAuthorizedDatetimeOk(); ok && dt != nil {
		v := *dt
		t.AuthorizedDatetime = &v
	}

	// Pending transaction ID (links posted to pending).
	if id, ok := txn.GetPendingTransactionIdOk(); ok && id != nil && *id != "" {
		t.PendingExternalID = id
	}

	// Merchant name.
	if name, ok := txn.GetMerchantNameOk(); ok && name != nil && *name != "" {
		t.MerchantName = name
	}

	// ISO currency code.
	if code, ok := txn.GetIsoCurrencyCodeOk(); ok && code != nil {
		t.ISOCurrencyCode = *code
	}

	// Personal finance category.
	if pfc, ok := txn.GetPersonalFinanceCategoryOk(); ok && pfc != nil {
		primary := pfc.GetPrimary()
		t.CategoryPrimary = &primary
		detailed := pfc.GetDetailed()
		t.CategoryDetailed = &detailed
		if conf, ok := pfc.GetConfidenceLevelOk(); ok && conf != nil {
			t.CategoryConfidence = conf
		}
	}

	return t
}
