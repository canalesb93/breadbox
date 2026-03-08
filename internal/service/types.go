package service

import "time"

type AccountResponse struct {
	ID                string   `json:"id"`
	ConnectionID      *string  `json:"connection_id"`
	UserID            *string  `json:"user_id"`
	InstitutionName   *string  `json:"institution_name"`
	Name              string   `json:"name"`
	OfficialName      *string  `json:"official_name"`
	Type              string   `json:"type"`
	Subtype           *string  `json:"subtype"`
	Mask              *string  `json:"mask"`
	BalanceCurrent    *float64 `json:"balance_current"`
	BalanceAvailable  *float64 `json:"balance_available"`
	BalanceLimit      *float64 `json:"balance_limit"`
	IsoCurrencyCode   *string  `json:"iso_currency_code"`
	LastBalanceUpdate *string  `json:"last_balance_update"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type TransactionCategoryInfo struct {
	ID                 *string `json:"id"`
	Slug               *string `json:"slug"`
	DisplayName        *string `json:"display_name"`
	PrimarySlug        *string `json:"primary_slug,omitempty"`
	PrimaryDisplayName *string `json:"primary_display_name,omitempty"`
	Icon               *string `json:"icon,omitempty"`
	Color              *string `json:"color,omitempty"`
}

type TransactionResponse struct {
	ID                 string                   `json:"id"`
	AccountID          *string                  `json:"account_id"`
	AccountName        *string                  `json:"account_name"`
	UserName           *string                  `json:"user_name"`
	Amount             float64                  `json:"amount"`
	IsoCurrencyCode    *string                  `json:"iso_currency_code"`
	Date               string                   `json:"date"`
	AuthorizedDate     *string                  `json:"authorized_date"`
	Datetime           *string                  `json:"datetime"`
	AuthorizedDatetime *string                  `json:"authorized_datetime"`
	Name               string                   `json:"name"`
	MerchantName       *string                  `json:"merchant_name"`
	Category           *TransactionCategoryInfo `json:"category"`
	CategoryOverride   bool                     `json:"category_override"`
	CategoryPrimaryRaw *string                  `json:"category_primary_raw"`
	CategoryDetailedRaw *string                 `json:"category_detailed_raw"`
	CategoryConfidence *string                  `json:"category_confidence"`
	PaymentChannel     *string                  `json:"payment_channel"`
	Pending            bool                     `json:"pending"`
	CreatedAt          string                   `json:"created_at"`
	UpdatedAt          string                   `json:"updated_at"`
}

type TransactionListResult struct {
	Transactions []TransactionResponse `json:"transactions"`
	NextCursor   string                `json:"next_cursor,omitempty"`
	HasMore      bool                  `json:"has_more"`
	Limit        int                   `json:"limit"`
}

type TransactionListParams struct {
	Cursor       string
	Limit        int
	StartDate    *time.Time
	EndDate      *time.Time
	AccountID    *string
	UserID       *string
	CategorySlug *string
	MinAmount    *float64
	MaxAmount    *float64
	Pending      *bool
	Search       *string
	SortBy       *string
	SortOrder    *string
}

type TransactionCountParams struct {
	StartDate    *time.Time
	EndDate      *time.Time
	AccountID    *string
	UserID       *string
	CategorySlug *string
	MinAmount    *float64
	MaxAmount    *float64
	Pending      *bool
	Search       *string
}

type CategoryPair struct {
	Primary  string  `json:"primary"`
	Detailed *string `json:"detailed,omitempty"`
}

type UserResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     *string `json:"email"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type ConnectionResponse struct {
	ID              string  `json:"id"`
	UserID          *string `json:"user_id"`
	UserName        *string `json:"user_name"`
	Provider        string  `json:"provider"`
	InstitutionID   *string `json:"institution_id"`
	InstitutionName *string `json:"institution_name"`
	Status          string  `json:"status"`
	ErrorCode       *string `json:"error_code"`
	ErrorMessage    *string `json:"error_message"`
	LastSyncedAt    *string `json:"last_synced_at"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type ConnectionStatusResponse struct {
	ConnectionResponse
	LastAttemptedSyncAt *string          `json:"last_attempted_sync_at"`
	LastSyncLog         *SyncLogResponse `json:"last_sync_log"`
}

type SyncLogResponse struct {
	ID            string  `json:"id"`
	ConnectionID  string  `json:"connection_id"`
	Trigger       string  `json:"trigger"`
	Status        string  `json:"status"`
	AddedCount    int32   `json:"added_count"`
	ModifiedCount int32   `json:"modified_count"`
	RemovedCount  int32   `json:"removed_count"`
	ErrorMessage  *string `json:"error_message"`
	StartedAt     *string `json:"started_at"`
	CompletedAt   *string `json:"completed_at"`
}

type APIKeyResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	KeyPrefix  string  `json:"key_prefix"`
	LastUsedAt *string `json:"last_used_at"`
	RevokedAt  *string `json:"revoked_at"`
	CreatedAt  string  `json:"created_at"`
}

type CreateAPIKeyResult struct {
	APIKeyResponse
	PlaintextKey string `json:"plaintext_key"`
}

type SyncLogListParams struct {
	Page         int
	PageSize     int
	ConnectionID *string
	Status       *string
}

type SyncLogListResult struct {
	Logs       []SyncLogRow
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}

type SyncLogRow struct {
	ID              string
	ConnectionID    string
	InstitutionName string
	Trigger         string
	Status          string
	AddedCount      int32
	ModifiedCount   int32
	RemovedCount    int32
	ErrorMessage    *string
	StartedAt       *string
	CompletedAt     *string
	Duration        *string
}

type AdminTransactionListParams struct {
	Page         int
	PageSize     int
	StartDate    *time.Time
	EndDate      *time.Time
	AccountID    *string
	UserID       *string
	ConnectionID *string
	CategorySlug *string
	MinAmount    *float64
	MaxAmount    *float64
	Pending      *bool
	Search       *string
	SortOrder    string // "desc" (default) or "asc"
}

type AdminTransactionRow struct {
	ID                  string
	AccountID           string
	AccountName         string
	InstitutionName     string
	UserName            string
	Date                string
	Name                string
	MerchantName        *string
	Amount              float64
	IsoCurrencyCode     *string
	CategoryDisplayName *string
	CategorySlug        *string
	CategoryIcon        *string
	CategoryOverride    bool
	Pending             bool
	CreatedAt           string
	UpdatedAt           string
}

type AdminTransactionListResult struct {
	Transactions []AdminTransactionRow
	Total        int64
	Page         int
	PageSize     int
	TotalPages   int
}

type AdminAccountDetail struct {
	AccountResponse
	DisplayName     *string
	Excluded        bool
	InstitutionName string
	Provider        string
	UserName        string
	ConnectionID    string
}
