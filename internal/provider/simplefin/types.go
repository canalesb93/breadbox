//go:build !lite

package simplefin

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"breadbox/internal/provider"

	"github.com/shopspring/decimal"
)

// decodeAccountSet decodes a SimpleFIN /accounts response body.
func decodeAccountSet(r io.Reader) (accountSet, error) {
	var set accountSet
	if err := json.NewDecoder(r).Decode(&set); err != nil {
		return accountSet{}, fmt.Errorf("simplefin: decode accounts response: %w", err)
	}
	return set, nil
}

// accountSet is the top-level SimpleFIN /accounts response. The error list is
// captured under both "errors" (what bridge.simplefin.org actually returns) and
// "errlist" (the name used in the protocol spec) for robustness.
type accountSet struct {
	Errors   []json.RawMessage `json:"errors"`
	Errlist  []json.RawMessage `json:"errlist"`
	Accounts []sfinAccount     `json:"accounts"`
}

// errorStrings returns the combined error/errlist entries as display strings.
func (a accountSet) errorStrings() []string {
	var out []string
	for _, raw := range append(append([]json.RawMessage{}, a.Errors...), a.Errlist...) {
		out = append(out, rawErrorString(raw))
	}
	return out
}

// rawErrorString renders a SimpleFIN error entry, which may be a bare string or
// an object ({code,msg,...}), as a human-readable string.
func rawErrorString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		switch {
		case obj.Msg != "" && obj.Code != "":
			return fmt.Sprintf("%s: %s", obj.Code, obj.Msg)
		case obj.Msg != "":
			return obj.Msg
		case obj.Code != "":
			return obj.Code
		}
	}
	return string(raw)
}

// sfinOrg is the institution an account belongs to. A single access URL can
// span multiple orgs (SimpleFIN is a multi-bank aggregator).
type sfinOrg struct {
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	SFINURL string `json:"sfin-url"`
	URL     string `json:"url"`
	ID      string `json:"id"`
}

// displayName returns the best human label for the org.
func (o sfinOrg) displayName() string {
	if o.Name != "" {
		return o.Name
	}
	return o.Domain
}

// sfinAccount is a single account in the /accounts response.
type sfinAccount struct {
	Org              sfinOrg           `json:"org"`
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Currency         string            `json:"currency"`
	Balance          string            `json:"balance"`
	AvailableBalance string            `json:"available-balance"`
	BalanceDate      int64             `json:"balance-date"`
	Transactions     []sfinTransaction `json:"transactions"`
}

// sfinTransaction is a single transaction nested under an account.
type sfinTransaction struct {
	ID           string `json:"id"`
	Posted       int64  `json:"posted"`
	Amount       string `json:"amount"`
	Description  string `json:"description"`
	Payee        string `json:"payee"`
	Memo         string `json:"memo"`
	TransactedAt int64  `json:"transacted_at"`
	Pending      bool   `json:"pending"`
}

// toAccount maps a SimpleFIN account to a provider.Account. SimpleFIN does not
// expose an account type/subtype/mask, so type defaults to "depository" and the
// owning org's name is carried in OfficialName for institution context.
func (a sfinAccount) toAccount() provider.Account {
	return provider.Account{
		ExternalID:      a.ID,
		Name:            a.Name,
		OfficialName:    a.Org.displayName(),
		Type:            "depository",
		ISOCurrencyCode: a.Currency,
	}
}

// toTransaction maps a SimpleFIN transaction to a provider.Transaction.
//
// Sign convention: SimpleFIN amounts are positive for money IN (deposits);
// Breadbox uses positive for money OUT (debits), so we negate uniformly.
func (t sfinTransaction) toTransaction(accountID, currency string) (provider.Transaction, error) {
	amount, err := decimal.NewFromString(t.Amount)
	if err != nil {
		return provider.Transaction{}, fmt.Errorf("parse amount %q: %w", t.Amount, err)
	}
	amount = amount.Neg()

	pending := t.Pending || t.Posted == 0
	date := t.date()

	out := provider.Transaction{
		ExternalID:        t.ID,
		AccountExternalID: accountID,
		Amount:            amount,
		Date:              date,
		Name:              t.Description,
		Pending:           pending,
		PaymentChannel:    "other",
		ISOCurrencyCode:   currency,
	}

	if t.Payee != "" {
		payee := t.Payee
		out.MerchantName = &payee
	}

	if raw, err := json.Marshal(t); err == nil {
		out.Raw = raw
	}

	return out, nil
}

// date resolves the transaction's effective date, preferring the settlement
// timestamp (posted) and falling back to transacted_at, then now.
func (t sfinTransaction) date() time.Time {
	switch {
	case t.Posted > 0:
		return time.Unix(t.Posted, 0).UTC()
	case t.TransactedAt > 0:
		return time.Unix(t.TransactedAt, 0).UTC()
	default:
		return time.Now().UTC()
	}
}
