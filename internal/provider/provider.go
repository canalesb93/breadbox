package provider

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Provider is the abstraction over bank data providers (Plaid, Teller, CSV).
// All methods take a context for cancellation and timeout propagation.
type Provider interface {
	// CreateLinkSession starts a new account connection flow.
	CreateLinkSession(ctx context.Context, userID string) (LinkSession, error)

	// ExchangeToken completes the connection flow after the user authenticates.
	ExchangeToken(ctx context.Context, publicToken string) (Connection, []Account, error)

	// SyncTransactions fetches incremental transaction changes for a connection.
	SyncTransactions(ctx context.Context, conn Connection, cursor string) (SyncResult, error)

	// GetBalances fetches current account balances for a connection.
	GetBalances(ctx context.Context, conn Connection) ([]AccountBalance, error)

	// HandleWebhook parses and validates an inbound webhook payload.
	HandleWebhook(ctx context.Context, payload WebhookPayload) (WebhookEvent, error)

	// CreateReauthSession starts a re-authentication flow for a broken connection.
	CreateReauthSession(ctx context.Context, connectionID string) (LinkSession, error)

	// RemoveConnection revokes the provider's access and marks the connection removed.
	RemoveConnection(ctx context.Context, connectionID string) error
}

type LinkSession struct {
	Token  string
	Expiry time.Time
}

type Connection struct {
	ProviderName         string
	ExternalID           string
	EncryptedCredentials []byte
	InstitutionName      string
}

type Account struct {
	ExternalID      string
	Name            string
	OfficialName    string
	Type            string // depository, credit, loan, investment
	Subtype         string // checking, savings, credit card, etc.
	Mask            string // last 4 digits
	ISOCurrencyCode string
}

type AccountBalance struct {
	AccountExternalID string
	Current           decimal.Decimal
	Available         *decimal.Decimal // nil if not provided
	Limit             *decimal.Decimal // nil if not applicable
	ISOCurrencyCode   string
}

type SyncResult struct {
	Added    []Transaction
	Modified []Transaction
	Removed  []string // external transaction IDs
	Cursor   string
	HasMore  bool
}

type Transaction struct {
	ExternalID         string
	PendingExternalID  *string // set when this transaction posts a pending one
	AccountExternalID  string
	Amount             decimal.Decimal // positive = debit, negative = credit
	Date               time.Time
	AuthorizedDate     *time.Time
	Datetime           *time.Time
	AuthorizedDatetime *time.Time
	Name               string
	MerchantName       *string
	CategoryPrimary    *string
	CategoryDetailed   *string
	CategoryConfidence *string
	PaymentChannel     string // online, in store, other
	Pending            bool
	ISOCurrencyCode    string
}

type WebhookPayload struct {
	RawBody []byte
	Headers map[string]string
}

type WebhookEvent struct {
	Type                  string // "sync_available", "connection_error", "pending_expiration", "new_accounts", "unknown"
	ConnectionID          string
	ErrorCode             *string
	ErrorMessage          *string
	ConsentExpirationTime *string
}
