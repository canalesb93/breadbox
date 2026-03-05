package teller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"

	"github.com/shopspring/decimal"
)

// tellerTransaction represents a transaction in the Teller API response.
type tellerTransaction struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	Amount      string `json:"amount"`
	Date        string `json:"date"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	Details     struct {
		Category     string `json:"category"`
		Counterparty struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"counterparty"`
	} `json:"details"`
}

const tellerPageSize = 250

// SyncTransactions fetches transactions using date-range polling.
func (p *TellerProvider) SyncTransactions(ctx context.Context, conn provider.Connection, cursor string) (provider.SyncResult, error) {
	accessToken, err := crypto.Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return provider.SyncResult{}, fmt.Errorf("teller: decrypt access token: %w", err)
	}
	token := string(accessToken)

	// Compute date range.
	now := time.Now()
	var fromDate time.Time
	if cursor == "" {
		// Initial sync: fetch 2 years of history.
		fromDate = now.AddDate(-2, 0, 0)
	} else {
		parsed, err := time.Parse(time.RFC3339, cursor)
		if err != nil {
			return provider.SyncResult{}, fmt.Errorf("teller: parse cursor: %w", err)
		}
		// Overlap by 10 days to catch any late-posting transactions.
		fromDate = parsed.AddDate(0, 0, -10)
	}
	fromDateStr := fromDate.Format("2006-01-02")
	toDateStr := now.Format("2006-01-02")

	// Get account list to iterate over.
	accounts, err := p.fetchAccounts(ctx, token)
	if err != nil {
		return provider.SyncResult{}, fmt.Errorf("teller: fetch accounts for sync: %w", err)
	}

	// Build a currency map from accounts.
	currencyMap := make(map[string]string, len(accounts))
	for _, acct := range accounts {
		currencyMap[acct.ExternalID] = acct.ISOCurrencyCode
	}

	var allTxns []provider.Transaction

	for _, acct := range accounts {
		txns, err := p.fetchTransactionsForAccount(ctx, token, acct.ExternalID, fromDateStr, toDateStr, currencyMap)
		if err != nil {
			return provider.SyncResult{}, err
		}
		allTxns = append(allTxns, txns...)
	}

	return provider.SyncResult{
		Added:   allTxns,
		HasMore: false,
		Cursor:  now.Format(time.RFC3339),
	}, nil
}

// fetchTransactionsForAccount fetches all transactions for a single account
// using date-range pagination.
func (p *TellerProvider) fetchTransactionsForAccount(
	ctx context.Context,
	accessToken, accountID, fromDate, toDate string,
	currencyMap map[string]string,
) ([]provider.Transaction, error) {
	var allTxns []provider.Transaction
	var fromID string

	for {
		path := fmt.Sprintf("/accounts/%s/transactions?from_date=%s&to_date=%s&count=%d",
			accountID, fromDate, toDate, tellerPageSize)
		if fromID != "" {
			path += "&from_id=" + fromID
		}

		resp, err := p.client.doWithRetry(ctx, http.MethodGet, path, accessToken, "")
		if err != nil {
			return nil, fmt.Errorf("teller transactions get: %w", err)
		}

		if resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			return nil, ErrReauthRequired
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("teller transactions get: status %d: %s", resp.StatusCode, body)
		}

		var page []tellerTransaction
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("teller transactions decode: %w", err)
		}
		resp.Body.Close()

		for _, txn := range page {
			mapped, err := mapTellerTransaction(txn, currencyMap)
			if err != nil {
				p.logger.WarnContext(ctx, "teller: skipping transaction with parse error",
					"transaction_id", txn.ID,
					"error", err,
				)
				continue
			}
			allTxns = append(allTxns, mapped)
		}

		// If we got a full page, paginate from the last transaction ID.
		if len(page) >= tellerPageSize {
			fromID = page[len(page)-1].ID
			continue
		}
		break
	}

	return allTxns, nil
}

// mapTellerTransaction converts a Teller API transaction to provider.Transaction.
func mapTellerTransaction(txn tellerTransaction, currencyMap map[string]string) (provider.Transaction, error) {
	// Parse amount and negate (Teller: negative=debit, Breadbox: positive=debit).
	amount, err := decimal.NewFromString(txn.Amount)
	if err != nil {
		return provider.Transaction{}, fmt.Errorf("parse amount %q: %w", txn.Amount, err)
	}
	amount = amount.Neg()

	// Parse date.
	date, err := time.Parse("2006-01-02", txn.Date)
	if err != nil {
		return provider.Transaction{}, fmt.Errorf("parse date %q: %w", txn.Date, err)
	}

	t := provider.Transaction{
		ExternalID:        txn.ID,
		AccountExternalID: txn.AccountID,
		Amount:            amount,
		Date:              date,
		Name:              txn.Description,
		Pending:           txn.Status == "pending",
		PaymentChannel:    mapPaymentChannel(txn.Type),
		ISOCurrencyCode:   currencyMap[txn.AccountID],
	}

	// Category.
	if txn.Details.Category != "" {
		primary, detailed := mapCategory(txn.Details.Category)
		t.CategoryPrimary = &primary
		t.CategoryDetailed = detailed
	}

	// Merchant name (counterparty).
	if txn.Details.Counterparty.Name != "" {
		name := txn.Details.Counterparty.Name
		t.MerchantName = &name
	}

	return t, nil
}

// mapPaymentChannel converts a Teller transaction type to a Breadbox payment channel.
func mapPaymentChannel(tellerType string) string {
	switch tellerType {
	case "card_payment":
		return "in store"
	default:
		return "other"
	}
}
