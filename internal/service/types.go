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
	ConnectionStatus  *string  `json:"connection_status,omitempty"`
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
	ID                  string                   `json:"id"`
	AccountID           *string                  `json:"account_id"`
	AccountName         *string                  `json:"account_name"`
	UserName            *string                  `json:"user_name"`
	AttributedUserID    *string                  `json:"attributed_user_id,omitempty"`
	AttributedUserName  *string                  `json:"attributed_user_name,omitempty"`
	Amount              float64                  `json:"amount"`
	IsoCurrencyCode     *string                  `json:"iso_currency_code"`
	Date                string                   `json:"date"`
	AuthorizedDate      *string                  `json:"authorized_date"`
	Datetime            *string                  `json:"datetime"`
	AuthorizedDatetime  *string                  `json:"authorized_datetime"`
	Name                string                   `json:"name"`
	MerchantName        *string                  `json:"merchant_name"`
	Category            *TransactionCategoryInfo `json:"category"`
	CategoryOverride    bool                     `json:"category_override"`
	CategoryPrimaryRaw  *string                  `json:"category_primary_raw"`
	CategoryDetailedRaw *string                  `json:"category_detailed_raw"`
	CategoryConfidence  *string                  `json:"category_confidence"`
	PaymentChannel      *string                  `json:"payment_channel"`
	Pending             bool                     `json:"pending"`
	CreatedAt           string                   `json:"created_at"`
	UpdatedAt           string                   `json:"updated_at"`
}

type TransactionListResult struct {
	Transactions []TransactionResponse `json:"transactions"`
	NextCursor   string                `json:"next_cursor,omitempty"`
	HasMore      bool                  `json:"has_more"`
	Limit        int                   `json:"limit"`
}

type TransactionListParams struct {
	Cursor           string
	Limit            int
	StartDate        *time.Time
	EndDate          *time.Time
	AccountID        *string
	UserID           *string
	CategorySlug     *string
	MinAmount        *float64
	MaxAmount        *float64
	Pending          *bool
	Search           *string
	SearchMode       *string // contains (default), words, fuzzy
	ExcludeSearch    *string
	SortBy           *string
	SortOrder        *string
	IncludeDependent bool
}

type TransactionCountParams struct {
	StartDate        *time.Time
	EndDate          *time.Time
	AccountID        *string
	UserID           *string
	CategorySlug     *string
	MinAmount        *float64
	MaxAmount        *float64
	Pending          *bool
	Search           *string
	SearchMode       *string
	ExcludeSearch    *string
	IncludeDependent bool
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
	DurationMs    *int32  `json:"duration_ms,omitempty"`
	Duration      *string `json:"duration,omitempty"`
}

type APIKeyResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	KeyPrefix  string  `json:"key_prefix"`
	Scope      string  `json:"scope"`
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
	ID                   string
	ConnectionID         string
	InstitutionName      string
	Trigger              string
	Status               string
	AddedCount           int32
	ModifiedCount        int32
	RemovedCount         int32
	ErrorMessage         *string // raw technical error for debugging
	FriendlyErrorMessage *string // human-friendly error for display
	WarningMessage       *string // non-fatal warning (e.g., balance fetch issues)
	StartedAt            *string
	CompletedAt          *string
	Duration             *string
	DurationMs           *int32
	AccountsAffected     int64 // number of accounts with activity in this sync
}

// SyncLogAccountRow represents a per-account breakdown within a sync log.
type SyncLogAccountRow struct {
	ID            string
	SyncLogID     string
	AccountID     *string
	AccountName   string
	AddedCount    int32
	ModifiedCount int32
	RemovedCount  int32
}

// SyncLogStats contains aggregate statistics about sync logs.
type SyncLogStats struct {
	TotalSyncs    int64
	SuccessCount  int64
	ErrorCount    int64
	WarningCount  int64   // syncs that succeeded with warnings
	SuccessRate   float64 // 0-100 percentage
	AvgDurationMs float64 // average duration in milliseconds
	TotalAdded    int64
	TotalModified int64
	TotalRemoved  int64
}

// SyncHealthSummary contains a dashboard-oriented overview of sync health.
type SyncHealthSummary struct {
	LastSyncTime      *string // relative time of most recent completed sync (any status)
	LastSyncStatus    string  // status of the most recent sync: "success", "error", or ""
	RecentSyncCount   int64   // total syncs in the last 24h
	RecentSuccessRate float64 // 0-100 success rate over last 24h
	RecentErrorCount  int64   // error syncs in the last 24h
	ConnectionErrors  int64   // count of connections currently in error/pending_reauth status
	NextSyncTime      string  // human-readable time until next scheduled sync
	OverallHealth     string  // "healthy", "degraded", or "unhealthy"
}

type AdminTransactionListParams struct {
	Page          int
	PageSize      int
	StartDate     *time.Time
	EndDate       *time.Time
	AccountID     *string
	UserID        *string
	ConnectionID  *string
	CategorySlug  *string
	MinAmount     *float64
	MaxAmount     *float64
	Pending       *bool
	Search        *string
	SearchMode    *string
	ExcludeSearch *string
	SortOrder     string // "desc" (default) or "asc"
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
	CategoryID          *string
	CategoryDisplayName *string
	CategorySlug        *string
	CategoryIcon        *string
	CategoryColor       *string
	CategoryOverride    bool
	Pending             bool
	AgentReviewed       bool
	HasPendingReview    bool
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

// Comment types

type CreateCommentParams struct {
	TransactionID string
	Content       string
	Actor         Actor
}

type UpdateCommentParams struct {
	Content string
	Actor   Actor
}

type CommentResponse struct {
	ID            string  `json:"id"`
	TransactionID string  `json:"transaction_id"`
	AuthorType    string  `json:"author_type"`
	AuthorID      *string `json:"author_id"`
	AuthorName    string  `json:"author_name"`
	Content       string  `json:"content"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// Review queue types

type ReviewResponse struct {
	ID                  string               `json:"id"`
	TransactionID       string               `json:"transaction_id"`
	ReviewType          string               `json:"review_type"`
	Status              string               `json:"status"`
	Provider            *string              `json:"provider,omitempty"`
	SuggestedCategoryID *string              `json:"suggested_category_id,omitempty"`
	SuggestedCategory   *string              `json:"suggested_category_slug,omitempty"`
	ConfidenceScore     *float64             `json:"confidence_score,omitempty"`
	ReviewerType        *string              `json:"reviewer_type,omitempty"`
	ReviewerID          *string              `json:"reviewer_id,omitempty"`
	ReviewerName        *string              `json:"reviewer_name,omitempty"`
	ReviewNote          *string              `json:"review_note,omitempty"`
	ResolvedCategoryID  *string              `json:"resolved_category_id,omitempty"`
	ResolvedCategory    *string              `json:"resolved_category_slug,omitempty"`
	CreatedAt           string               `json:"created_at"`
	ReviewedAt          *string              `json:"reviewed_at,omitempty"`
	Transaction         *TransactionResponse `json:"transaction,omitempty"`
}

type ReviewListParams struct {
	Status             *string
	ReviewType         *string
	AccountID          *string
	UserID             *string
	CategoryPrimaryRaw *string
	Limit              int
	Cursor             string
}

type ReviewListResult struct {
	Reviews    []ReviewResponse `json:"reviews"`
	NextCursor string           `json:"next_cursor,omitempty"`
	HasMore    bool             `json:"has_more"`
	Total      int64            `json:"total"`
}

type SubmitReviewParams struct {
	ReviewID   string
	Decision   string
	CategoryID *string
	Note       *string
	Actor      Actor
}

type BulkSubmitReviewParams struct {
	Reviews []BulkReviewItem
	Actor   Actor
}

type BulkReviewItem struct {
	ReviewID   string  `json:"review_id"`
	Decision   string  `json:"decision"`
	CategoryID *string `json:"category_id,omitempty"`
	Note       *string `json:"note,omitempty"`
}

type BulkReviewResult struct {
	Succeeded int              `json:"succeeded"`
	Failed    []BulkReviewError `json:"failed,omitempty"`
}

type BulkReviewError struct {
	ReviewID string `json:"review_id"`
	Error    string `json:"error"`
}

type ReviewCountsResponse struct {
	Pending       int64 `json:"pending"`
	ApprovedToday int64 `json:"approved_today"`
	RejectedToday int64 `json:"rejected_today"`
	SkippedToday  int64 `json:"skipped_today"`
}

// Transaction rule types

type Condition struct {
	Field string      `json:"field,omitempty"`
	Op    string      `json:"op,omitempty"`
	Value interface{} `json:"value,omitempty"`

	And []Condition `json:"and,omitempty"`
	Or  []Condition `json:"or,omitempty"`
	Not *Condition  `json:"not,omitempty"`
}

type TransactionContext struct {
	Name             string
	MerchantName     string
	Amount           float64
	CategoryPrimary  string
	CategoryDetailed string
	Pending          bool
	Provider         string
	AccountID        string
	UserID           string
	UserName         string
}

type TransactionRuleResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Conditions    Condition `json:"conditions"`
	CategoryID    *string   `json:"category_id,omitempty"`
	CategorySlug  *string   `json:"category_slug,omitempty"`
	CategoryName  *string   `json:"category_display_name,omitempty"`
	Priority      int       `json:"priority"`
	Enabled       bool      `json:"enabled"`
	ExpiresAt     *string   `json:"expires_at,omitempty"`
	CreatedByType string    `json:"created_by_type"`
	CreatedByID   *string   `json:"created_by_id,omitempty"`
	CreatedByName string    `json:"created_by_name"`
	HitCount      int       `json:"hit_count"`
	LastHitAt     *string   `json:"last_hit_at,omitempty"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

type TransactionRuleListParams struct {
	CategorySlug *string
	Enabled      *bool
	Search       *string
	SearchMode   *string
	Limit        int
	Cursor       string
}

type TransactionRuleListResult struct {
	Rules      []TransactionRuleResponse `json:"rules"`
	NextCursor string                    `json:"next_cursor,omitempty"`
	HasMore    bool                      `json:"has_more"`
	Total      int64                     `json:"total"`
}

type CreateTransactionRuleParams struct {
	Name         string
	Conditions   Condition
	CategorySlug string
	Priority     int
	ExpiresIn    string // e.g., "30d", "24h"
	Actor        Actor
}

type UpdateTransactionRuleParams struct {
	Name         *string
	Conditions   *Condition
	CategorySlug *string
	Priority     *int
	Enabled      *bool
	ExpiresAt    *string // ISO timestamp or empty to clear
}

// Batch categorize types

type BatchCategorizeItem struct {
	TransactionID string `json:"transaction_id"`
	CategorySlug  string `json:"category_slug"`
}

type BatchCategorizeResult struct {
	Succeeded int                    `json:"succeeded"`
	Failed    []BatchCategorizeError `json:"failed,omitempty"`
}

type BatchCategorizeError struct {
	TransactionID string `json:"transaction_id"`
	Error         string `json:"error"`
}

// Bulk recategorize types

type BulkRecategorizeParams struct {
	StartDate          *time.Time
	EndDate            *time.Time
	AccountID          *string
	UserID             *string
	CategorySlug       *string // current category filter
	MinAmount          *float64
	MaxAmount          *float64
	Pending            *bool
	Search             *string
	NameContains       *string
	TargetCategorySlug string
}

type BulkRecategorizeResult struct {
	MatchedCount int64 `json:"matched_count"`
	UpdatedCount int64 `json:"updated_count"`
}

// Merchant summary types

type MerchantSummaryParams struct {
	StartDate     *time.Time
	EndDate       *time.Time
	AccountID     *string
	UserID        *string
	CategorySlug  *string
	MinAmount     *float64
	MaxAmount     *float64
	Search        *string
	SearchMode    *string
	ExcludeSearch *string
	MinCount      int  // minimum transaction count to include (default 1)
	SpendingOnly  bool // only positive amounts
}

type MerchantSummaryRow struct {
	Merchant         string  `json:"merchant"`
	TransactionCount int64   `json:"transaction_count"`
	TotalAmount      float64 `json:"total_amount"`
	AvgAmount        float64 `json:"avg_amount"`
	FirstDate        string  `json:"first_date"`
	LastDate         string  `json:"last_date"`
	IsoCurrencyCode  string  `json:"iso_currency_code"`
}

type MerchantSummaryResult struct {
	Merchants []MerchantSummaryRow  `json:"merchants"`
	Totals    MerchantSummaryTotals `json:"totals"`
	Filters   MerchantSummaryFilters `json:"filters"`
}

type MerchantSummaryTotals struct {
	MerchantCount    int64    `json:"merchant_count"`
	TransactionCount int64    `json:"transaction_count"`
	TotalAmount      *float64 `json:"total_amount,omitempty"`
	Note             string   `json:"note,omitempty"`
}

type MerchantSummaryFilters struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	MinCount  int    `json:"min_count"`
}
