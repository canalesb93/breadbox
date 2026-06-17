//go:build !lite

package service

import (
	"encoding/json"
	"time"
)

type AccountResponse struct {
	ID                string   `json:"id"`
	ShortID           string   `json:"short_id"`
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
	IsDependentLinked bool     `json:"is_dependent_linked"`
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
	ID                         string                   `json:"id"`
	ShortID                    string                   `json:"short_id"`
	AccountID                  *string                  `json:"account_id"`
	AccountName                *string                  `json:"account_name"`
	UserName                   *string                  `json:"user_name"`
	AttributedUserID           *string                  `json:"attributed_user_id,omitempty"`
	AttributedUserName         *string                  `json:"attributed_user_name,omitempty"`
	EffectiveUserID            *string                  `json:"effective_user_id,omitempty"`
	Amount                     float64                  `json:"amount"`
	IsoCurrencyCode            *string                  `json:"iso_currency_code"`
	Date                       string                   `json:"date"`
	AuthorizedDate             *string                  `json:"authorized_date"`
	Datetime                   *string                  `json:"datetime"`
	AuthorizedDatetime         *string                  `json:"authorized_datetime"`
	ProviderName               string                   `json:"provider_name"`
	ProviderMerchantName       *string                  `json:"provider_merchant_name"`
	Category                   *TransactionCategoryInfo `json:"category"`
	CategoryOverride           string                   `json:"category_override"`
	ProviderCategoryPrimary    *string                  `json:"provider_category_primary"`
	ProviderCategoryDetailed   *string                  `json:"provider_category_detailed"`
	ProviderCategoryConfidence *string                  `json:"provider_category_confidence"`
	ProviderPaymentChannel     *string                  `json:"provider_payment_channel"`
	Pending                    bool                     `json:"pending"`
	CreatedAt                  string                   `json:"created_at"`
	UpdatedAt                  string                   `json:"updated_at"`

	// Tags attached to this transaction (slug list). Empty slice when none are
	// attached. Populated by ListTransactions / GetTransaction.
	Tags []string `json:"tags,omitempty"`

	// Metadata is the free-form JSONB enrichment store on this transaction. Always
	// present as a JSON object (the empty object {} when nothing has been written).
	// Written via the scoped metadata ops; never holds first-class fields.
	Metadata json.RawMessage `json:"metadata"`

	// FlaggedAt is set when the transaction is flagged for human attention
	// (null when not flagged). The reason lives in a comment annotation.
	FlaggedAt *string `json:"flagged_at,omitempty"`
}

type TransactionListResult struct {
	Transactions []TransactionResponse `json:"transactions"`
	NextCursor   string                `json:"next_cursor,omitempty"`
	HasMore      bool                  `json:"has_more"`
	Limit        int                   `json:"limit"`
}

type TransactionListParams struct {
	Cursor string
	// Offset enables random-access pagination (LIMIT/OFFSET) — used by the
	// admin page-numbered pagination. Cursor pagination remains the default
	// for external REST clients; when Offset > 0 the service ignores Cursor.
	Offset       int
	Limit        int
	StartDate    *time.Time
	EndDate      *time.Time
	AccountID    *string
	UserID       *string
	CategorySlug *string
	// Multi-select variants. When non-empty they take precedence over the
	// singular `AccountID` / `CategorySlug` fields above and produce an OR
	// match across every value in the list (parent categories still include
	// their children).
	AccountIDs       []string
	CategorySlugs    []string
	MinAmount        *float64
	MaxAmount        *float64
	Pending          *bool
	Flagged          *bool
	Search           *string
	SearchMode       *string // contains (default), words, fuzzy
	ExcludeSearch    *string
	SortBy           *string
	SortOrder        *string
	IncludeDependent bool
	// Tags (AND) — transaction must have EVERY tag in this list.
	Tags []string
	// AnyTag (OR) — transaction must have AT LEAST ONE tag in this list.
	AnyTag []string
}

type TransactionCountParams struct {
	StartDate    *time.Time
	EndDate      *time.Time
	AccountID    *string
	UserID       *string
	CategorySlug *string
	// Multi-select variants — see TransactionListParams.
	AccountIDs       []string
	CategorySlugs    []string
	MinAmount        *float64
	MaxAmount        *float64
	Pending          *bool
	Flagged          *bool
	Search           *string
	SearchMode       *string
	ExcludeSearch    *string
	IncludeDependent bool
	Tags             []string
	AnyTag           []string
}

type CategoryPair struct {
	Primary  string  `json:"primary"`
	Detailed *string `json:"detailed,omitempty"`
}

type UserResponse struct {
	ID        string  `json:"id"`
	ShortID   string  `json:"short_id"`
	Name      string  `json:"name"`
	Email     *string `json:"email"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type ConnectionResponse struct {
	ID              string  `json:"id"`
	ShortID         string  `json:"short_id"`
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

// ConnectionDetailResponse is the full per-connection detail returned by
// GET /api/v1/connections/{id}. It extends ConnectionResponse with fields
// that are useful for management operations (paused flag, per-conn sync
// interval override, account count).
type ConnectionDetailResponse struct {
	ConnectionResponse
	Paused                      bool   `json:"paused"`
	SyncIntervalOverrideMinutes *int32 `json:"sync_interval_override_minutes"`
	ConsecutiveFailures         int32  `json:"consecutive_failures"`
	AccountCount                int    `json:"account_count"`
}

type SyncLogResponse struct {
	ID            string  `json:"id"`
	ShortID       string  `json:"short_id"`
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
	ActorType  string  `json:"actor_type"`
	ActorName  *string `json:"actor_name,omitempty"`
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
	Trigger      *string
	DateFrom     *time.Time
	DateTo       *time.Time
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
	Provider             string
	Trigger              string
	Status               string
	AddedCount           int32
	ModifiedCount        int32
	RemovedCount         int32
	UnchangedCount       int32
	ErrorMessage         *string // raw technical error for debugging
	FriendlyErrorMessage *string // human-friendly error for display
	WarningMessage       *string // non-fatal warning (e.g., balance fetch issues)
	StartedAt            *string
	CompletedAt          *string
	Duration             *string
	DurationMs           *int32
	AccountsAffected     int64          // number of accounts with activity in this sync
	RuleHits             []RuleHitEntry // per-rule hit counts from this sync run
	TotalRuleHits        int            // sum of all rule hits
}

// RuleHitEntry represents a single rule's hit count within a sync run.
type RuleHitEntry struct {
	RuleID     string
	RuleName   string
	Count      int
	Conditions *Condition
}

// SyncLogAccountRow represents a per-account breakdown within a sync log.
type SyncLogAccountRow struct {
	ID             string
	SyncLogID      string
	AccountID      *string
	AccountName    string
	AddedCount     int32
	ModifiedCount  int32
	RemovedCount   int32
	UnchangedCount int32
}

// SyncLogStats contains aggregate statistics about sync logs.
type SyncLogStats struct {
	TotalSyncs     int64
	SuccessCount   int64
	ErrorCount     int64
	WarningCount   int64   // syncs that succeeded with warnings
	SuccessRate    float64 // 0-100 percentage
	AvgDurationMs  float64 // average duration in milliseconds
	TotalAdded     int64
	TotalModified  int64
	TotalRemoved   int64
	TotalUnchanged int64
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
	Flagged       *bool
	Search        *string
	SearchMode    *string
	SearchField   *string // "all" (default), "name", "merchant"
	ExcludeSearch *string
	SortOrder     string // "desc" (default) or "asc"
	// Tags filter — AND semantics (must have every slug). Empty = no constraint.
	Tags []string
	// AnyTag filter — OR semantics (must have at least one slug). Empty = no constraint.
	AnyTag []string
}

type AdminTransactionRow struct {
	ID                  string
	AccountID           string
	AccountName         string
	InstitutionName     string
	UserName            string
	EffectiveUserID     *string
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
	CategoryOverride    string
	Pending             bool
	CommentCount        int
	HasPendingReview    bool
	FlaggedAt           *string
	CreatedAt           string
	UpdatedAt           string
	// Tags attached to this transaction. Populated as a separate batched
	// lookup keyed by transaction id so list pages can render chips.
	Tags []AdminTransactionTag
}

// AdminTransactionTag is a compact tag descriptor for list-row rendering.
type AdminTransactionTag struct {
	Slug        string
	DisplayName string
	Color       *string
	Icon        *string
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

// AccountDetailResponse is the public REST detail payload returned from
// GET /api/v1/accounts/{id}/detail. It composes the standard
// AccountResponse with a few admin-only fields and the most recent
// transactions for the account. Per-currency balances are returned on
// the embedded AccountResponse fields (BalanceCurrent / IsoCurrencyCode);
// `Balances` is the array form for forward compatibility with multi-currency
// accounts (always single-element today).
type AccountDetailResponse struct {
	AccountResponse
	DisplayName        *string               `json:"display_name"`
	Excluded           bool                  `json:"excluded"`
	Provider           string                `json:"provider,omitempty"`
	UserName           string                `json:"connection_user_name,omitempty"`
	ConnectionShortID  string                `json:"connection_short_id,omitempty"`
	Balances           []AccountBalance      `json:"balances"`
	RecentTransactions []TransactionResponse `json:"recent_transactions"`
}

// AccountBalance represents a balance in a single currency. Today every
// Breadbox account has a single balance; the slice shape exists so the
// payload stays stable if multi-currency accounts ever land.
type AccountBalance struct {
	IsoCurrencyCode  *string  `json:"iso_currency_code"`
	BalanceCurrent   *float64 `json:"balance_current"`
	BalanceAvailable *float64 `json:"balance_available"`
	BalanceLimit     *float64 `json:"balance_limit"`
}

// Comment types

type CreateCommentParams struct {
	TransactionID string
	Content       string
	Actor         Actor
	ReviewID      string // optional: links comment to a review resolution
}

type UpdateCommentParams struct {
	Content string
	Actor   Actor
}

type CommentResponse struct {
	ID            string  `json:"id"`
	ShortID       string  `json:"short_id"`
	TransactionID string  `json:"transaction_id"`
	AuthorType    string  `json:"author_type"`
	AuthorID      *string `json:"author_id"`
	AuthorName    string  `json:"author_name"`
	Content       string  `json:"content"`
	ReviewID      *string `json:"review_id,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// Transaction rule types

// RuleAction is the typed action shape for transaction rules.
//
// Type values:
//   - "set_category": requires CategorySlug.
//   - "add_tag":      requires TagSlug (validated slug format).
//   - "add_comment":  requires Content.
type RuleAction struct {
	Type         string `json:"type"`
	CategorySlug string `json:"category_slug,omitempty"`
	TagSlug      string `json:"tag_slug,omitempty"`
	Content      string `json:"content,omitempty"`
	// assign_series fields: assign matching transactions to an existing series
	// (SeriesShortID) or mint one keyed on MerchantKey (CreateIfMissing).
	SeriesShortID   string `json:"series_short_id,omitempty"`
	MerchantKey     string `json:"merchant_key,omitempty"`
	CreateIfMissing bool   `json:"create_if_missing,omitempty"`
	// set_metadata / remove_metadata fields. MetadataKey is the metadata blob
	// key (≤128 chars). MetadataValue is the JSON value to write for
	// set_metadata (any JSON-serializable value); unused by remove_metadata.
	MetadataKey   string `json:"metadata_key,omitempty"`
	MetadataValue any    `json:"metadata_value,omitempty"`
}

// ActivityEntry represents a single event in a transaction's activity timeline.
type ActivityEntry struct {
	Type      string `json:"type"`      // "review", "comment", "rule", "tag", "category"
	Timestamp string `json:"timestamp"` // RFC3339

	ActorName string  `json:"actor_name"`
	ActorType string  `json:"actor_type"`         // "user", "agent", "system"
	ActorID   *string `json:"actor_id,omitempty"` // user UUID when available (for avatar rendering)

	// ActorAvatarVersion is a unix-timestamp string from the actor user's
	// users.updated_at, used as the `?v=<ts>` cache buster on the rendered
	// avatar URL. Mirrors the pattern in users.html so that avatar uploads
	// invalidate the timeline image immediately rather than living in the
	// browser cache for the avatar handler's 24h max-age window. Empty for
	// non-user actors and for user actors whose row has been deleted.
	ActorAvatarVersion string `json:"-"`

	Summary string `json:"summary"`          // Short: "Approved as Food & Drink"
	Detail  string `json:"detail,omitempty"` // Longer text (review note or comment body)

	// Type-specific
	ReviewStatus string `json:"review_status,omitempty"` // approved, rejected, skipped, pending
	CategoryName string `json:"category_name,omitempty"` // display name
	// CategoryColor and CategoryIcon drive the icon-tile rendering on
	// category_set timeline rows so each event reads at a glance with the
	// same color cue used elsewhere in the app. Empty/nil when the category
	// is no longer registered (deleted categories fall back to a neutral
	// folder icon).
	CategoryColor *string `json:"category_color,omitempty"`
	CategoryIcon  *string `json:"category_icon,omitempty"`
	RuleName      string  `json:"rule_name,omitempty"`
	RuleID        string  `json:"rule_id,omitempty"`
	// RuleShortID is the rule's 8-char short_id used to build the
	// /rules/<short_id> link target on rule_applied timeline rows. The
	// rule detail route accepts either UUID or short_id, but the
	// activity timeline prefers short_id so the rendered href matches
	// the canonical handle agents and humans use everywhere else.
	RuleShortID string `json:"rule_short_id,omitempty"`
	// ActionField identifies what a rule_applied row targeted ("tag",
	// "category", or "comment"). Drives the chip-vs-text branch in the
	// templ — when ActionField=="tag" the row renders the tag chip
	// already populated in TagSlug/TagDisplayName/TagColor; when
	// "category" it renders the category chip from CategoryName +
	// CategoryColor + CategoryIcon.
	ActionField string `json:"action_field,omitempty"`
	CommentID   string `json:"comment_id,omitempty"`
	TagSlug     string `json:"tag_slug,omitempty"` // for tag_added / tag_removed entries

	// TagDisplayName, TagColor and TagIcon drive the rendered tag-chip on
	// tag_added / tag_removed timeline rows. Empty/nil when the tag no
	// longer exists in the registry (deleted tags still show their slug).
	TagDisplayName string  `json:"tag_display_name,omitempty"`
	TagColor       *string `json:"tag_color,omitempty"`
	TagIcon        *string `json:"tag_icon,omitempty"`
	// TagAction distinguishes "added" vs "removed" so the template can
	// render a plus / minus overlay on tag rows.
	TagAction string `json:"tag_action,omitempty"` // "added" | "removed"

	// Origin describes how a rule-sourced entry was applied ("during sync",
	// "retroactively"). Rule-applied rows previously overloaded ActorName
	// with this phrase; Origin keeps the actor slot empty for system rows
	// and lets the template render origin as a subordinate meta pill.
	Origin string `json:"origin,omitempty"`

	// IsDeleted flags a tombstoned comment. The activity timeline keeps the
	// row to preserve audit value — actor + timestamp survive — but renders
	// it as a muted single-line "<Actor> deleted a comment" sentence
	// instead of the chat bubble.
	IsDeleted bool `json:"is_deleted,omitempty"`
}

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
	CategoryPrimary  string // raw provider primary category
	CategoryDetailed string // raw provider detailed category
	// Category is the transaction's assigned category slug (distinct from
	// CategoryPrimary's raw provider value).
	Category    string
	Pending     bool
	Provider    string
	AccountID   string
	AccountName string
	UserID      string
	UserName    string
	// Tags is populated from transaction_tags so tag-based conditions
	// (field: "tags") can match against the transaction's current tags.
	Tags []string
	// SeriesShortID is the short_id of the recurring series this transaction
	// belongs to (field: "series"), empty when unassigned. Populated from the
	// recurring_series JOIN in the retroactive / preview context query.
	SeriesShortID string
	// InSeries reports whether the transaction is linked to any recurring
	// series (field: "in_series").
	InSeries bool
	// Metadata holds the transaction's free-form metadata blob so conditions on
	// dotted fields (field: "metadata.<key>") can read arbitrary enrichment
	// values. Updated mid-resolver as earlier-stage set_metadata / remove_metadata
	// actions apply, so later-stage rules observe the running blob.
	Metadata map[string]any
}

type TransactionRuleResponse struct {
	ID      string `json:"id"`
	ShortID string `json:"short_id"`
	Name    string `json:"name"`
	// Conditions may be a zero-value Condition{} to mean "match all transactions"
	// (stored as NULL in the DB).
	Conditions Condition    `json:"conditions"`
	Actions    []RuleAction `json:"actions"`
	Trigger    string       `json:"trigger"`
	// CategorySlug/CategoryName/CategoryIcon/CategoryColor are derived from the
	// first set_category action in Actions (kept for admin UI convenience).
	// Category info is no longer a denormalized column on transaction_rules —
	// these are populated at response time. Categories are referenced by slug
	// (canonical stable handle), so there's no separate CategoryID field.
	CategorySlug  *string `json:"category_slug,omitempty"`
	CategoryName  *string `json:"category_display_name,omitempty"`
	CategoryIcon  *string `json:"category_icon,omitempty"`
	CategoryColor *string `json:"category_color,omitempty"`
	Priority      int     `json:"priority"`
	Enabled       bool    `json:"enabled"`
	ExpiresAt     *string `json:"expires_at,omitempty"`
	CreatedByType string  `json:"created_by_type"`
	CreatedByID   *string `json:"created_by_id,omitempty"`
	CreatedByName string  `json:"created_by_name"`
	HitCount      int     `json:"hit_count"`
	LastHitAt     *string `json:"last_hit_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type TransactionRuleListParams struct {
	CategorySlug *string
	Enabled      *bool
	Search       *string
	SearchMode   *string
	// CreatorType filters by the rule's creator. Accepted values:
	// "user", "agent", "system". Other values (or nil) leave the
	// dimension unfiltered. Used by the admin list page's "Type"
	// filter and the query_transaction_rules MCP tool.
	CreatorType *string
	// Trigger filters by the rule's firing trigger. Accepted values:
	// "on_create", "on_change" (alias "on_update"), "always". nil leaves
	// the dimension unfiltered.
	Trigger *string
	// MinHitCount filters to rules whose hit_count is >= this value. Use to
	// surface high-impact rules. nil leaves the dimension unfiltered.
	MinHitCount *int
	// OnlyUnused filters to rules that have never fired (hit_count = 0). Use
	// to surface dead/over-specific rules worth pruning. Ignored when false.
	OnlyUnused bool
	// SortBy drives the ORDER BY clause. Accepted values: "priority" (default),
	// "created_at", "hit_count", "last_hit_at", "name". Honored by BOTH the
	// offset-paginated path (admin UI) and the cursor-paginated path (API/MCP).
	// Cursor pagination is only emitted for the default ordering — see
	// ListTransactionRules; an explicit non-default SortBy returns a single
	// top-N page with no next_cursor.
	SortBy string
	// SortDir is "asc" or "desc". Empty → per-column default (desc for most, asc for name/priority).
	SortDir string
	Limit   int
	Cursor  string
	// Offset-based pagination (used by admin UI). When Page > 0, cursor is ignored.
	Page     int
	PageSize int
}

type TransactionRuleListResult struct {
	Rules      []TransactionRuleResponse `json:"rules"`
	NextCursor string                    `json:"next_cursor,omitempty"`
	HasMore    bool                      `json:"has_more"`
	Total      int64                     `json:"total"`
	// Offset-based pagination fields (populated when Page > 0)
	Page       int `json:"page,omitempty"`
	PageSize   int `json:"page_size,omitempty"`
	TotalPages int `json:"total_pages,omitempty"`
}

type CreateTransactionRuleParams struct {
	Name string
	// Conditions: a zero-value Condition{} (no Field/And/Or/Not) means "match all".
	Conditions   Condition
	Actions      []RuleAction // if set, takes precedence over CategorySlug
	CategorySlug string       // sugar for actions: [{"type":"set_category","category_slug":slug}]
	Trigger      string       // "on_create" (default), "on_change", or "always" ("on_update" accepted as alias)
	Priority     int
	// Stage is a semantic alias for Priority: "baseline" (0), "standard" (10),
	// "refinement" (50), or "override" (100). If both Stage and Priority are
	// supplied, Priority wins. Unknown values return a validation error.
	Stage     string
	ExpiresIn string // e.g., "30d", "24h"
	Actor     Actor
}

type UpdateTransactionRuleParams struct {
	Name *string
	// Conditions: nil means "don't change"; non-nil zero-value Condition means match-all.
	Conditions   *Condition
	Actions      *[]RuleAction // if set, replaces actions entirely
	CategorySlug *string       // sugar: replaces set_category action, keeps others
	Trigger      *string       // optional trigger change
	Priority     *int
	// Stage is the semantic alias for Priority. When Priority is also supplied,
	// Priority wins. An empty string means "don't change".
	Stage     *string
	Enabled   *bool
	ExpiresAt *string // ISO timestamp or empty to clear
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
	Merchants []MerchantSummaryRow   `json:"merchants"`
	Totals    MerchantSummaryTotals  `json:"totals"`
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

// ProviderHealthSummary contains per-provider health info for the providers page.
type ProviderHealthSummary struct {
	Provider        string  // "plaid", "teller", "csv"
	ConnectionCount int64   // active (non-disconnected) connections using this provider
	AccountCount    int64   // accounts across those connections
	LastSyncStatus  string  // "success", "error", "in_progress", or ""
	LastSyncTime    *string // relative time string, nil if never synced
	LastSyncError   *string // error message from last sync, nil if success
}
