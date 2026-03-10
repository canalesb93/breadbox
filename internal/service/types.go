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
	CategoryID          *string
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

// Audit log types

type AuditLogEntry struct {
	EntityType string
	EntityID   string
	Action     string // "create", "update", "delete"
	Field      *string
	OldValue   *string
	NewValue   *string
	Actor      Actor
	Metadata   map[string]string
}

type AuditLogListParams struct {
	EntityType string
	EntityID   string
	Limit      int
	Cursor     string
}

type AuditLogGlobalParams struct {
	EntityType *string
	ActorType  *string
	Limit      int
	Cursor     string
}

type AuditLogListResult struct {
	Entries    []AuditLogResponse `json:"entries"`
	NextCursor string             `json:"next_cursor,omitempty"`
	HasMore    bool               `json:"has_more"`
}

type AuditLogResponse struct {
	ID         string            `json:"id"`
	EntityType string            `json:"entity_type"`
	EntityID   string            `json:"entity_id"`
	Action     string            `json:"action"`
	Field      *string           `json:"field,omitempty"`
	OldValue   *string           `json:"old_value,omitempty"`
	NewValue   *string           `json:"new_value,omitempty"`
	ActorType  string            `json:"actor_type"`
	ActorID    *string           `json:"actor_id,omitempty"`
	ActorName  string            `json:"actor_name"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  string            `json:"created_at"`
}

// Review queue types

type ReviewResponse struct {
	ID                  string               `json:"id"`
	TransactionID       string               `json:"transaction_id"`
	ReviewType          string               `json:"review_type"`
	Status              string               `json:"status"`
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
	Status     *string
	ReviewType *string
	AccountID  *string
	UserID     *string
	Limit      int
	Cursor     string
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

// Phase 25: Pending review types for external agent polling

type PendingReviewParams struct {
	Limit               int
	Cursor              string
	AccountID           *string
	UserID              *string
	CategorySlug        *string
	Since               *time.Time
	IncludeInstructions bool
}

type PendingReviewResult struct {
	Items              []PendingReviewItem `json:"items"`
	ReviewInstructions *string             `json:"review_instructions,omitempty"`
	NextCursor         string              `json:"next_cursor,omitempty"`
	HasMore            bool                `json:"has_more"`
	TotalPending       int64               `json:"total_pending"`
}

type PendingReviewItem struct {
	ReviewID              string              `json:"review_id"`
	TransactionID         string              `json:"transaction_id"`
	Transaction           TransactionResponse `json:"transaction"`
	ReviewType            string              `json:"review_type"`
	SuggestedCategoryID   *string             `json:"suggested_category_id,omitempty"`
	SuggestedCategorySlug *string             `json:"suggested_category_slug,omitempty"`
	CreatedAt             string              `json:"created_at"`
}

type ReviewDecision struct {
	ReviewID             string  `json:"review_id"`
	Decision             string  `json:"decision"`
	OverrideCategorySlug *string `json:"override_category_slug,omitempty"`
	Comment              *string `json:"comment,omitempty"`
}

type SubmitReviewsResult struct {
	Submitted int                    `json:"submitted"`
	Results   []ReviewDecisionResult `json:"results"`
}

type ReviewDecisionResult struct {
	ReviewID string `json:"review_id"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}
