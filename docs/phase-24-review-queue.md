# Phase 24: Review Queue & Review API

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## 1. Overview

Phase 24 introduces a transaction review system that allows both humans (via the admin dashboard) and AI agents (via REST API and MCP tools) to review transactions through a unified queue. Transactions are automatically enqueued during sync when they are new, uncategorized, or have low-confidence category mappings. Reviewers can approve, override the category, reject, or skip each item. All review decisions are recorded in the Phase 21 audit log, creating a full trail of categorization decisions that agents can learn from.

---

## 2. Goals

1. **Unified review workflow.** A single `review_queue` table serves both dashboard and API/MCP reviewers, preventing duplicate work.
2. **Auto-enqueue on sync.** New transactions, uncategorized transactions, and low-confidence category mappings are automatically queued for review — no manual queue management.
3. **Category improvement loop.** Review decisions that override categories feed back into the audit log, enabling agents to learn family preferences over time.
4. **Bulk operations.** Agents and admins can review multiple transactions in a single request for efficiency.
5. **Permission-aware.** MCP review tools respect the Phase 23 permission model (read-only keys can list but not submit reviews).

---

## 3. Data Model

### 3.1 `review_queue` Table

```sql
-- Migration: 00019_review_queue.sql
-- +goose Up
CREATE TABLE review_queue (
    id                    UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id        UUID          NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    review_type           TEXT          NOT NULL CHECK (review_type IN ('new_transaction', 'uncategorized', 'low_confidence', 'manual')),
    status                TEXT          NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'skipped')),
    suggested_category_id UUID          NULL REFERENCES categories (id) ON DELETE SET NULL,
    confidence_score      NUMERIC(5,4) NULL,
    reviewer_type         TEXT          NULL CHECK (reviewer_type IN ('user', 'agent')),
    reviewer_id           TEXT          NULL,
    reviewer_name         TEXT          NULL,
    review_note           TEXT          NULL,
    resolved_category_id  UUID          NULL REFERENCES categories (id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    reviewed_at           TIMESTAMPTZ   NULL,

    CONSTRAINT review_queue_reviewer_complete CHECK (
        (status = 'pending' AND reviewer_type IS NULL AND reviewer_id IS NULL AND reviewed_at IS NULL)
        OR (status != 'pending' AND reviewer_type IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);

CREATE INDEX review_queue_status_idx ON review_queue (status, created_at ASC) WHERE status = 'pending';
CREATE INDEX review_queue_transaction_id_idx ON review_queue (transaction_id);
CREATE INDEX review_queue_reviewed_at_idx ON review_queue (reviewed_at DESC) WHERE status != 'pending';

-- Prevent duplicate pending reviews for the same transaction.
CREATE UNIQUE INDEX review_queue_pending_unique_idx ON review_queue (transaction_id) WHERE status = 'pending';

-- +goose Down
DROP INDEX IF EXISTS review_queue_pending_unique_idx;
DROP INDEX IF EXISTS review_queue_reviewed_at_idx;
DROP INDEX IF EXISTS review_queue_transaction_id_idx;
DROP INDEX IF EXISTS review_queue_status_idx;
DROP TABLE IF EXISTS review_queue;
```

**Column notes:**

| Column | Description |
|---|---|
| `transaction_id` | FK to `transactions.id`. CASCADE delete — if a transaction is hard-deleted, its review items go too. Soft-deleted transactions retain their review items (soft delete only sets `deleted_at`). |
| `review_type` | Why this transaction was enqueued. `new_transaction` = first time synced. `uncategorized` = synced with no category mapping resolution. `low_confidence` = category mapping resolved but with confidence below threshold. `manual` = manually added by user or agent. |
| `status` | `pending` = awaiting review. `approved` = reviewer accepted the suggested category (or existing category). `rejected` = reviewer declined and optionally overrode the category. `skipped` = reviewer deferred to a later time. |
| `suggested_category_id` | The category resolved by the category mapping system during sync (if any). NULL for `uncategorized` items. FK with SET NULL — if the suggested category is deleted, the suggestion is cleared but the review item remains. |
| `confidence_score` | Provider-reported or mapping-system confidence (0.0000–1.0000). NULL when no confidence is available (e.g., CSV imports). |
| `reviewer_type` | `user` = admin dashboard user, `agent` = AI agent via MCP/API. NULL while pending. Same semantics as Phase 21 `actor_type` but excludes `system` — reviews are always by humans or agents. |
| `reviewer_id` | Opaque identifier for the reviewer. For `user`: admin_account UUID. For `agent`: API key ID (UUID). NULL while pending. |
| `reviewer_name` | Denormalized display name at time of review. Same rationale as Phase 21 `actor_name`. NULL while pending. |
| `review_note` | Optional free-text note explaining the review decision (e.g., "Recategorized to Insurance — this is the annual home insurance premium"). Max 2,000 characters, enforced at service layer. |
| `resolved_category_id` | The final category chosen by the reviewer. NULL if no category change was made (pure approval of existing). FK with SET NULL. |
| `reviewed_at` | Timestamp of review completion. NULL while pending. |

**Constraint notes:**

- The `review_queue_reviewer_complete` CHECK constraint ensures data integrity: pending items have no reviewer info, and resolved items always have reviewer info and a timestamp.
- The `review_queue_pending_unique_idx` partial unique index prevents a transaction from having multiple pending review items simultaneously. Once a review is resolved, the transaction can be re-enqueued (e.g., if a subsequent sync changes its category confidence).

### 3.2 Migration Numbering

The migration file is `00019_review_queue.sql`, following the existing sequence (last migration is `00016_remove_legacy_config.sql`, Phase 21 uses `00017`–`00018`, Phase 23 uses a separate migration for API key scope).

**Note:** The exact migration number may shift depending on whether Phases 20B, 21, 22, and 23 are implemented first. The number should be the next available integer at implementation time. The file is listed here as `00019` assuming Phases 21 and 23 add migrations `00017` and `00018`.

### 3.3 FK and Deletion Policy

| Relationship | Policy | Rationale |
|---|---|---|
| `review_queue.transaction_id → transactions.id` | CASCADE | Review items are meaningless without their transaction. |
| `review_queue.suggested_category_id → categories.id` | SET NULL | If the suggested category is deleted, the review item remains valid — reviewer can still assign a different category. |
| `review_queue.resolved_category_id → categories.id` | SET NULL | Historical review decisions survive category deletion. The audit log captures the category slug/name at decision time. |

---

## 4. Auto-Enqueue Logic

### 4.1 Hook Point

Auto-enqueue runs inside the sync engine's write transaction, after all transaction upserts are complete but before the transaction commits. This ensures atomicity: if the sync transaction rolls back, no orphaned review items exist.

**File:** `internal/sync/engine.go` — inside `runSync()`, after the upsert loops for `pendingAdded` and `pendingModified`, and before `tx.Commit()`.

### 4.2 Enqueue Conditions

For each upserted transaction (both added and modified), the sync engine evaluates:

| Condition | Review Type | Description |
|---|---|---|
| Transaction is newly added (INSERT, not UPDATE via ON CONFLICT) | `new_transaction` | First-time sync of this transaction. |
| Transaction has NULL `category_primary` AND NULL `category_id` (once Phase 20B `category_id` column exists) | `uncategorized` | Provider returned no category, and no mapping resolved. |
| Transaction has `category_confidence` value below configurable threshold | `low_confidence` | Category was resolved but with low provider confidence. |

**Priority:** If multiple conditions match, use the first matching type in this order: `uncategorized` > `low_confidence` > `new_transaction`. Only one review item is created per transaction per sync.

### 4.3 Confidence Threshold

A new `app_config` key controls the threshold:

| Key | Default | Description |
|---|---|---|
| `review_confidence_threshold` | `0.5` | Transactions with `category_confidence` below this value are enqueued as `low_confidence`. Set to `0` to disable confidence-based enqueue. Set to `1` to enqueue all categorized transactions. |

### 4.4 Enqueue Implementation

```go
// internal/sync/review.go (new file)

// EnqueueForReview evaluates a transaction and creates a review_queue entry if needed.
// Called within the sync write transaction (txQueries).
func (e *Engine) enqueueForReview(ctx context.Context, txQueries *db.Queries, txnResult db.Transaction, isNew bool, confidenceThreshold float64) error {
    // Skip if transaction has category_override = true (user already manually categorized)
    if txnResult.CategoryOverride.Bool {
        return nil
    }

    reviewType := ""

    // Determine review type (priority order)
    hasCategory := txnResult.CategoryPrimary.Valid || txnResult.CategoryID.Valid
    if !hasCategory {
        reviewType = "uncategorized"
    } else if confidenceThreshold > 0 && txnResult.CategoryConfidence.Valid {
        conf, _ := txnResult.CategoryConfidence.Float64Value()
        if conf.Float64 < confidenceThreshold {
            reviewType = "low_confidence"
        }
    }

    // For new transactions without another review type, enqueue as new_transaction
    if reviewType == "" && isNew {
        reviewType = "new_transaction"
    }

    if reviewType == "" {
        return nil // No review needed
    }

    // Use ON CONFLICT DO NOTHING to respect the pending_unique_idx
    return txQueries.EnqueueReview(ctx, db.EnqueueReviewParams{
        TransactionID:       txnResult.ID,
        ReviewType:          reviewType,
        SuggestedCategoryID: txnResult.CategoryID, // may be NULL
        ConfidenceScore:     parseConfidenceScore(txnResult.CategoryConfidence),
    })
}
```

### 4.5 Detecting New vs. Modified

The `UpsertTransaction` query uses `ON CONFLICT ... DO UPDATE` and returns `*`. To distinguish inserts from updates, compare the returned `created_at` with `updated_at` — if they are equal (within a small tolerance), the row was just inserted:

```go
isNew := txnResult.CreatedAt.Time.Equal(txnResult.UpdatedAt.Time)
```

Alternatively, add `xmax = 0` to the RETURNING clause (PostgreSQL-specific: `xmax = 0` means INSERT, `xmax != 0` means UPDATE). The simpler timestamp comparison is preferred to avoid modifying the sqlc query.

### 4.6 Configurable Auto-Enqueue

A boolean `app_config` key controls whether auto-enqueue is active:

| Key | Default | Description |
|---|---|---|
| `review_auto_enqueue` | `true` | When `false`, the sync engine skips all auto-enqueue logic. Useful for families who don't want a review workflow. |

---

## 5. Service Layer

All service functions live in `internal/service/`. New file: `reviews.go`.

### 5.1 Types (`internal/service/types.go`)

```go
// ReviewResponse represents a review queue item with denormalized transaction context.
type ReviewResponse struct {
    ID                  string   `json:"id"`
    TransactionID       string   `json:"transaction_id"`
    ReviewType          string   `json:"review_type"`
    Status              string   `json:"status"`
    SuggestedCategoryID *string  `json:"suggested_category_id,omitempty"`
    SuggestedCategory   *string  `json:"suggested_category_slug,omitempty"`
    ConfidenceScore     *float64 `json:"confidence_score,omitempty"`
    ReviewerType        *string  `json:"reviewer_type,omitempty"`
    ReviewerID          *string  `json:"reviewer_id,omitempty"`
    ReviewerName        *string  `json:"reviewer_name,omitempty"`
    ReviewNote          *string  `json:"review_note,omitempty"`
    ResolvedCategoryID  *string  `json:"resolved_category_id,omitempty"`
    ResolvedCategory    *string  `json:"resolved_category_slug,omitempty"`
    CreatedAt           string   `json:"created_at"`
    ReviewedAt          *string  `json:"reviewed_at,omitempty"`

    // Denormalized transaction context
    Transaction *TransactionResponse `json:"transaction,omitempty"`
}

// ReviewListParams controls filtering and pagination for review list queries.
type ReviewListParams struct {
    Status     *string // filter by status (default: "pending")
    ReviewType *string // filter by review_type
    AccountID  *string // filter by transaction's account
    UserID     *string // filter by transaction's user (family member)
    Limit      int     // default 50, max 200
    Cursor     string  // pagination cursor (created_at ASC, id ASC for pending)
}

// ReviewListResult wraps paginated review results.
type ReviewListResult struct {
    Reviews    []ReviewResponse `json:"reviews"`
    NextCursor string           `json:"next_cursor,omitempty"`
    HasMore    bool             `json:"has_more"`
    Total      int64            `json:"total"`
}

// SubmitReviewParams contains the reviewer's decision.
type SubmitReviewParams struct {
    ReviewID   string
    Decision   string  // "approved", "rejected", "skipped"
    CategoryID *string // override category (optional, only meaningful for approved/rejected)
    Note       *string // optional review note
    Actor      Actor   // reviewer identity (from Phase 21)
}

// BulkSubmitReviewParams allows batch review decisions.
type BulkSubmitReviewParams struct {
    Reviews []BulkReviewItem
    Actor   Actor
}

// BulkReviewItem is a single decision within a bulk review.
type BulkReviewItem struct {
    ReviewID   string  `json:"review_id"`
    Decision   string  `json:"decision"`
    CategoryID *string `json:"category_id,omitempty"`
    Note       *string `json:"note,omitempty"`
}

// BulkReviewResult reports the outcome of a bulk review.
type BulkReviewResult struct {
    Succeeded int              `json:"succeeded"`
    Failed    []BulkReviewError `json:"failed,omitempty"`
}

// BulkReviewError reports a single failure in a bulk review.
type BulkReviewError struct {
    ReviewID string `json:"review_id"`
    Error    string `json:"error"`
}

// ReviewCountsResponse reports review queue statistics.
type ReviewCountsResponse struct {
    Pending       int64 `json:"pending"`
    ApprovedToday int64 `json:"approved_today"`
    RejectedToday int64 `json:"rejected_today"`
    SkippedToday  int64 `json:"skipped_today"`
}
```

### 5.2 Service Methods (`internal/service/reviews.go`)

```go
// ListReviews returns review queue items with filters and pagination.
// Includes denormalized transaction context for each review item.
func (s *Service) ListReviews(ctx context.Context, params ReviewListParams) (*ReviewListResult, error)

// GetReview returns a single review item with full transaction context.
func (s *Service) GetReview(ctx context.Context, id string) (*ReviewResponse, error)

// SubmitReview processes a single review decision.
// Writes audit log entry. Optionally updates the transaction's category.
func (s *Service) SubmitReview(ctx context.Context, params SubmitReviewParams) (*ReviewResponse, error)

// BulkSubmitReviews processes multiple review decisions in a single DB transaction.
// Partial failures are collected and returned — the batch does not abort on individual errors.
func (s *Service) BulkSubmitReviews(ctx context.Context, params BulkSubmitReviewParams) (*BulkReviewResult, error)

// EnqueueManualReview adds a transaction to the review queue manually.
func (s *Service) EnqueueManualReview(ctx context.Context, transactionID string, actor Actor) (*ReviewResponse, error)

// GetReviewCounts returns aggregate counts for the review queue (dashboard widget).
func (s *Service) GetReviewCounts(ctx context.Context) (*ReviewCountsResponse, error)

// DismissReview removes a pending review item (admin-only, different from "skip").
func (s *Service) DismissReview(ctx context.Context, id string, actor Actor) error
```

### 5.3 Business Rules

**ListReviews:**
- Default filter: `status = 'pending'`.
- Pending reviews are ordered `created_at ASC, id ASC` (oldest first — FIFO queue).
- Resolved reviews are ordered `reviewed_at DESC, id DESC` (most recent first).
- Each review response includes the full `TransactionResponse` (via JOIN, same pattern as `ListTransactions`), plus the suggested and resolved category slugs.
- Cursor pagination follows the same `EncodeCursor`/`DecodeCursor` pattern used by `ListTransactions`.

**SubmitReview:**
- The review must be in `pending` status. Attempting to review an already-resolved item returns `ErrReviewAlreadyResolved`.
- Valid decisions: `approved`, `rejected`, `skipped`.
- If `CategoryID` is provided and `Decision` is `approved` or `rejected`:
  - Validate the category exists in the `categories` table.
  - Update the transaction's `category_id` and set `category_override = true`.
  - Write an audit log entry with `entity_type=transaction`, `action=update`, `field=category_id`, recording old and new values.
- If `Decision` is `approved` and no `CategoryID` is provided:
  - If `suggested_category_id` is not NULL, apply it to the transaction (same as above).
  - If `suggested_category_id` is NULL, just mark the review as approved with no category change.
- If `Decision` is `skipped`, the review is marked as skipped. The item can be re-found later by filtering `status=skipped`.
- If `Note` is provided, validate length (max 2,000 chars) and store it. Also create a `transaction_comment` with the note content (linking review decisions to the comment timeline from Phase 21).
- Write an audit log entry with `entity_type=review_queue`, `action=update`, `field=status`, `old_value=pending`, `new_value={decision}`.
- Actor is resolved from the request context (Phase 21 `ActorFromContext`).

**BulkSubmitReviews:**
- Wraps all decisions in a single DB transaction for atomicity.
- Maximum 100 items per bulk request (enforced at service layer).
- Individual review failures do not abort the batch — errors are collected and returned in `BulkReviewResult.Failed`.
- Each successful review triggers the same audit log + category update logic as single review.

**EnqueueManualReview:**
- Validates the transaction exists and is not soft-deleted.
- Uses `review_type = 'manual'`.
- Respects the `pending_unique_idx` — returns `ErrReviewAlreadyPending` if a pending review already exists for this transaction.

**DismissReview:**
- Hard-deletes the review queue row (not a status change).
- Only allowed for `pending` items.
- Writes an audit log entry with `entity_type=review_queue`, `action=delete`.

### 5.4 New Error Sentinels (`internal/service/errors.go`)

```go
var (
    ErrReviewAlreadyResolved = errors.New("review has already been resolved")
    ErrReviewAlreadyPending  = errors.New("a pending review already exists for this transaction")
    ErrInvalidDecision       = errors.New("invalid review decision")
)
```

---

## 6. REST API

All endpoints under `/api/v1/reviews`. API key authenticated via existing `APIKeyAuth` middleware.

### 6.1 `GET /api/v1/reviews`

List review queue items with filters. Read tool — accessible to all API key scopes.

**Query parameters:**

| Param | Type | Default | Description |
|---|---|---|---|
| `status` | string | `pending` | Filter by status: `pending`, `approved`, `rejected`, `skipped`, or `all` |
| `review_type` | string | — | Filter by review type: `new_transaction`, `uncategorized`, `low_confidence`, `manual` |
| `account_id` | UUID | — | Filter by transaction's account ID |
| `user_id` | UUID | — | Filter by transaction's user (family member) ID |
| `limit` | int | 50 | Max results (max 200) |
| `cursor` | string | — | Pagination cursor from previous result |

**Response (200):**

```json
{
    "reviews": [
        {
            "id": "uuid",
            "transaction_id": "uuid",
            "review_type": "low_confidence",
            "status": "pending",
            "suggested_category_id": "uuid",
            "suggested_category_slug": "food_and_drink_groceries",
            "confidence_score": 0.35,
            "created_at": "2026-03-08T10:00:00Z",
            "transaction": {
                "id": "uuid",
                "account_id": "uuid",
                "account_name": "Chase Checking",
                "user_name": "Ricardo",
                "amount": 42.50,
                "iso_currency_code": "USD",
                "date": "2026-03-07",
                "name": "WHOLEFDS MKT 10234",
                "merchant_name": "Whole Foods Market",
                "category_primary": "FOOD_AND_DRINK",
                "category_detailed": "FOOD_AND_DRINK_GROCERIES",
                "pending": false,
                "created_at": "2026-03-08T09:00:00Z",
                "updated_at": "2026-03-08T09:00:00Z"
            }
        }
    ],
    "next_cursor": "...",
    "has_more": true,
    "total": 23
}
```

### 6.2 `GET /api/v1/reviews/{id}`

Get a single review item with full transaction context.

**Response (200):** Same shape as a single item from the list response.

**Errors:**
- `404 NOT_FOUND` — review item does not exist

### 6.3 `POST /api/v1/reviews/{id}/submit`

Submit a review decision. Write operation — requires `full_access` API key scope (Phase 23).

**Request:**

```json
{
    "decision": "approved",
    "category_id": "uuid",
    "note": "Confirmed as groceries."
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `decision` | string | yes | `approved`, `rejected`, or `skipped` |
| `category_id` | UUID | no | Override category. If omitted for `approved`, the suggested category is applied. |
| `note` | string | no | Free-text review note (max 2,000 chars). Saved as review_note and as a transaction comment. |

**Response (200):**

```json
{
    "id": "uuid",
    "transaction_id": "uuid",
    "review_type": "low_confidence",
    "status": "approved",
    "suggested_category_id": "uuid",
    "suggested_category_slug": "food_and_drink_groceries",
    "confidence_score": 0.35,
    "reviewer_type": "agent",
    "reviewer_id": "uuid",
    "reviewer_name": "Budget Bot",
    "review_note": "Confirmed as groceries.",
    "resolved_category_id": "uuid",
    "resolved_category_slug": "food_and_drink_groceries",
    "created_at": "2026-03-08T10:00:00Z",
    "reviewed_at": "2026-03-08T12:00:00Z"
}
```

**Errors:**
- `400 INVALID_PARAMETER` — invalid decision value, note too long, or invalid category_id
- `404 NOT_FOUND` — review item does not exist
- `409 REVIEW_ALREADY_RESOLVED` — review has already been resolved

### 6.4 `POST /api/v1/reviews/bulk`

Submit multiple review decisions. Write operation — requires `full_access` API key scope.

**Request:**

```json
{
    "reviews": [
        {
            "review_id": "uuid",
            "decision": "approved",
            "category_id": "uuid"
        },
        {
            "review_id": "uuid",
            "decision": "rejected",
            "category_id": "uuid",
            "note": "This is insurance, not shopping."
        },
        {
            "review_id": "uuid",
            "decision": "skipped"
        }
    ]
}
```

**Response (200):**

```json
{
    "succeeded": 2,
    "failed": [
        {
            "review_id": "uuid",
            "error": "review has already been resolved"
        }
    ]
}
```

**Errors:**
- `400 INVALID_PARAMETER` — empty reviews array or exceeds 100 items

### 6.5 `POST /api/v1/reviews/enqueue`

Manually enqueue a transaction for review. Write operation.

**Request:**

```json
{
    "transaction_id": "uuid"
}
```

**Response (201):** The created review item.

**Errors:**
- `404 NOT_FOUND` — transaction does not exist
- `409 REVIEW_ALREADY_PENDING` — a pending review already exists for this transaction

### 6.6 `GET /api/v1/reviews/counts`

Get review queue statistics. Read operation.

**Response (200):**

```json
{
    "pending": 23,
    "approved_today": 15,
    "rejected_today": 3,
    "skipped_today": 5
}
```

### 6.7 Handler File

**File:** `internal/api/reviews.go`

Handlers: `ListReviewsHandler`, `GetReviewHandler`, `SubmitReviewHandler`, `BulkSubmitReviewsHandler`, `EnqueueReviewHandler`, `ReviewCountsHandler`.

### 6.8 Router Changes (`internal/api/router.go`)

```go
// Inside /api/v1 route group:

// Review endpoints — read
r.Get("/reviews", ListReviewsHandler(svc))
r.Get("/reviews/counts", ReviewCountsHandler(svc))
r.Get("/reviews/{id}", GetReviewHandler(svc))

// Review endpoints — write (full_access scope required, Phase 23)
r.Group(func(r chi.Router) {
    r.Use(mw.RequireWriteScope())
    r.Post("/reviews/{id}/submit", SubmitReviewHandler(svc))
    r.Post("/reviews/bulk", BulkSubmitReviewsHandler(svc))
    r.Post("/reviews/enqueue", EnqueueReviewHandler(svc))
})
```

---

## 7. MCP Tools

### 7.1 New Tools

Add to `internal/mcp/tools.go`. Both tools are classified for Phase 23 permissions.

#### `list_pending_reviews` (read)

```go
type listPendingReviewsInput struct {
    ReviewType string `json:"review_type,omitempty" jsonschema:"Filter by why the transaction was queued: new_transaction, uncategorized, low_confidence, manual"`
    AccountID  string `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
    UserID     string `json:"user_id,omitempty" jsonschema:"Filter by user (family member) ID"`
    Limit      int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default 20, max 100). Start with a small batch for efficient review."`
    Cursor     string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
}
```

**Tool description:** `"List transactions pending human or agent review. Each item includes the full transaction context (amount, merchant, date, account, current category) and why it was queued (new_transaction, uncategorized, low_confidence, manual). Review items are returned oldest-first (FIFO). Start with a small limit (10-20) and review each batch before fetching more. Use review_type filter to focus on specific categories of items (e.g., 'uncategorized' for transactions needing categorization)."`

Returns `ReviewListResult` as JSON.

**Handler:**

```go
func (s *MCPServer) handleListPendingReviews(_ context.Context, _ *mcpsdk.CallToolRequest, input listPendingReviewsInput) (*mcpsdk.CallToolResult, any, error) {
    ctx := context.Background()
    status := "pending"
    params := service.ReviewListParams{
        Status: &status,
        Limit:  input.Limit,
        Cursor: input.Cursor,
    }
    if input.ReviewType != "" {
        params.ReviewType = &input.ReviewType
    }
    if input.AccountID != "" {
        params.AccountID = &input.AccountID
    }
    if input.UserID != "" {
        params.UserID = &input.UserID
    }
    result, err := s.svc.ListReviews(ctx, params)
    if err != nil {
        return errorResult(err), nil, nil
    }
    return jsonResult(result)
}
```

#### `submit_review` (write)

```go
type submitReviewInput struct {
    ReviewID   string  `json:"review_id" jsonschema:"required,UUID of the review queue item to review"`
    Decision   string  `json:"decision" jsonschema:"required,Review decision: approved (accept current/suggested category), rejected (override with a different category), or skipped (defer to later)"`
    CategoryID string  `json:"category_id,omitempty" jsonschema:"UUID of the category to assign. Required for rejected decisions. For approved decisions, omit to accept the suggested category or provide to override."`
    Note       string  `json:"note,omitempty" jsonschema:"Optional note explaining the review decision (max 2000 chars). Saved as a transaction comment visible to other reviewers and agents. Use this to explain non-obvious categorization choices."`
}
```

**Tool description:** `"Submit a review decision for a pending transaction. Use 'approved' to accept the suggested category (or current category if none suggested). Use 'rejected' with a category_id to override the category. Use 'skipped' to defer the review. Always provide a note when overriding categories to help other agents and family members understand the decision. Review decisions are recorded in the audit log and category overrides are permanent until manually changed."`

**Handler:**

```go
func (s *MCPServer) handleSubmitReview(ctx context.Context, _ *mcpsdk.CallToolRequest, input submitReviewInput) (*mcpsdk.CallToolResult, any, error) {
    // Permission guard (Phase 23 belt-and-suspenders)
    if err := s.checkWritePermission(ctx); err != nil {
        return errorResult(err), nil, nil
    }

    bgCtx := context.Background()
    actor := service.ActorFromContext(ctx) // Phase 21 actor resolution

    params := service.SubmitReviewParams{
        ReviewID: input.ReviewID,
        Decision: input.Decision,
        Actor:    actor,
    }
    if input.CategoryID != "" {
        params.CategoryID = &input.CategoryID
    }
    if input.Note != "" {
        params.Note = &input.Note
    }

    result, err := s.svc.SubmitReview(bgCtx, params)
    if err != nil {
        return errorResult(err), nil, nil
    }
    return jsonResult(result)
}
```

### 7.2 Tool Classification (Phase 23)

| Tool Name | Classification |
|---|---|
| `list_pending_reviews` | read |
| `submit_review` | write |

### 7.3 MCP Server Instructions Update

Add to the instructions string in `internal/mcp/server.go`:

```
REVIEW QUEUE:
- Use list_pending_reviews to see transactions awaiting review (oldest first)
- Review items include full transaction context: amount, merchant, date, account, category
- Use submit_review to approve, reject (with category override), or skip items
- Always include a note when rejecting/overriding to explain your reasoning
- Query the audit log to learn from past review decisions before making new ones
- Review types: new_transaction (first sync), uncategorized (no category), low_confidence (uncertain mapping), manual (human-requested)
```

---

## 8. Dashboard UI

### 8.1 Review Page

**Route:** `GET /admin/reviews`
**File:** `internal/templates/pages/reviews.html`
**Handler:** `ReviewsPageHandler` in `internal/admin/reviews.go`
**Nav:** New top-level nav item "Reviews" with `clipboard-check` Lucide icon, placed after "Transactions" in the nav.

The review page shows a card-based queue where admins process pending transactions one at a time or in batches.

### 8.2 Page Layout

```
┌──────────────────────────────────────────────────────────────┐
│ Reviews                                          [23 pending]│
│                                                              │
│ ┌─ Filter Bar ─────────────────────────────────────────────┐ │
│ │ Type: [All ▼]  Account: [All ▼]  User: [All ▼]          │ │
│ │ Status: [Pending ▼]                                      │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ ┌─ Review Card ─────────────────────────────────────────┐    │
│ │ ┌─ Transaction Info ─────────────────────────────────┐│    │
│ │ │ 🏷️ low_confidence                    Mar 7, 2026  ││    │
│ │ │                                                    ││    │
│ │ │ WHOLEFDS MKT 10234              $42.50 USD         ││    │
│ │ │ Whole Foods Market                                 ││    │
│ │ │ Chase Checking · Ricardo                           ││    │
│ │ │                                                    ││    │
│ │ │ Suggested: Groceries (35% confidence)              ││    │
│ │ └────────────────────────────────────────────────────┘│    │
│ │                                                       │    │
│ │ ┌─ Action Bar ───────────────────────────────────────┐│    │
│ │ │ Category: [Groceries           ▼]                  ││    │
│ │ │ Note:     [________________________________]       ││    │
│ │ │                                                    ││    │
│ │ │ [✓ Approve]  [✗ Reject]  [→ Skip]                 ││    │
│ │ └────────────────────────────────────────────────────┘│    │
│ └───────────────────────────────────────────────────────┘    │
│                                                              │
│ ┌─ Review Card ─────────────────────────────────────────┐    │
│ │ (next pending item...)                                │    │
│ └───────────────────────────────────────────────────────┘    │
│                                                              │
│ ┌─ Pagination ──────────────────────────────────────────┐    │
│ │ Showing 1-10 of 23          [← Previous] [Next →]     │    │
│ └───────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
```

### 8.3 Card Components

Each review card contains:

1. **Review type badge** — DaisyUI `badge` with type-specific color:
   - `new_transaction` → `badge-info`
   - `uncategorized` → `badge-warning`
   - `low_confidence` → `badge-error`
   - `manual` → `badge-neutral`

2. **Transaction context** — merchant/name, amount (with currency), date, account name, user name. Links to transaction detail page (Phase 21).

3. **Current/suggested category** — Shows the suggested category with confidence score if available. If uncategorized, shows "No category".

4. **Action bar:**
   - Category selector dropdown populated from the `categories` table (Phase 20B). Pre-selected to the suggested category if one exists.
   - Note text input (single line, expands on focus).
   - Three action buttons:
     - **Approve** (green, `btn-success`): Accept the selected category. POSTs to `/admin/api/reviews/{id}/submit` with `decision=approved`.
     - **Reject** (red, `btn-error`): Override with a different category. POSTs with `decision=rejected` and the selected `category_id`.
     - **Skip** (gray, `btn-ghost`): Defer. POSTs with `decision=skipped`.

### 8.4 Interaction Pattern

- Actions use Alpine.js `x-on:click` to submit via `fetch()` and remove the card from the DOM with a CSS transition (no full page reload).
- After an action, a DaisyUI `toast` notification confirms the decision.
- When all visible cards are processed, the page auto-loads the next batch or shows an empty state ("All caught up!").

### 8.5 Dashboard Widget

The main dashboard (`/admin/`) gets a "Review Queue" stat card showing the pending count. If pending > 0, the card links to `/admin/reviews`. Uses the same `stat` DaisyUI component as existing metric cards.

**File:** `internal/admin/dashboard.go` — add `ReviewPending` to the dashboard data, fetched via `svc.GetReviewCounts()`.

### 8.6 Admin API Routes

```go
// In admin API section of router (internal/admin/router.go):
r.Post("/reviews/{id}/submit", SubmitReviewAdminHandler(a, sm, svc))
r.Post("/reviews/{id}/dismiss", DismissReviewAdminHandler(a, sm, svc))
r.Post("/reviews/enqueue", EnqueueReviewAdminHandler(a, sm, svc))
```

### 8.7 Admin Route Registration

```go
// In authenticated admin routes (internal/admin/router.go):
r.Get("/reviews", ReviewsPageHandler(a, sm, tr, svc))
```

---

## 9. Audit Log Integration

All review actions write to the Phase 21 `audit_log` table via `svc.WriteAuditLog()`.

### 9.1 Audit Log Entries

| Operation | Entity Type | Action | Field | Old Value | New Value | Metadata |
|---|---|---|---|---|---|---|
| Review submitted | `review_queue` | `update` | `status` | `pending` | `approved`/`rejected`/`skipped` | `{"review_type": "low_confidence", "transaction_id": "uuid"}` |
| Category override via review | `transaction` | `update` | `category_id` | old category slug or NULL | new category slug | `{"trigger": "review", "review_id": "uuid"}` |
| Category override via review | `transaction` | `update` | `category_override` | `false` | `true` | `{"trigger": "review", "review_id": "uuid"}` |
| Manual enqueue | `review_queue` | `create` | — | — | — | `{"transaction_id": "uuid"}` |
| Review dismissed | `review_queue` | `delete` | — | — | — | `{"transaction_id": "uuid", "review_type": "..."}` |
| Auto-enqueue (sync) | `review_queue` | `create` | — | — | — | `{"transaction_id": "uuid", "trigger": "sync", "review_type": "..."}` |

### 9.2 Review Note as Comment

When a reviewer provides a `note` with their decision, the service layer also creates a `transaction_comment` (Phase 21) with:

- `author_type` = reviewer's actor type (`user` or `agent`)
- `author_id` = reviewer's actor ID
- `author_name` = reviewer's actor name
- `content` = `"[Review: {decision}] {note}"` — prefixed with the decision for context in the comment timeline.

This ensures review notes appear in the Phase 21 transaction detail page timeline alongside other comments and audit log entries.

---

## 10. sqlc Queries

New query file: `internal/db/queries/reviews.sql`

```sql
-- name: EnqueueReview :one
INSERT INTO review_queue (transaction_id, review_type, suggested_category_id, confidence_score)
VALUES ($1, $2, $3, $4)
ON CONFLICT (transaction_id) WHERE status = 'pending' DO NOTHING
RETURNING *;

-- name: GetReviewByID :one
SELECT * FROM review_queue WHERE id = $1;

-- name: GetPendingReviewByTransactionID :one
SELECT * FROM review_queue WHERE transaction_id = $1 AND status = 'pending';

-- name: UpdateReviewDecision :one
UPDATE review_queue
SET status = $2,
    reviewer_type = $3,
    reviewer_id = $4,
    reviewer_name = $5,
    review_note = $6,
    resolved_category_id = $7,
    reviewed_at = NOW()
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: DeleteReview :exec
DELETE FROM review_queue WHERE id = $1 AND status = 'pending';

-- name: CountPendingReviews :one
SELECT COUNT(*) FROM review_queue WHERE status = 'pending';

-- name: CountReviewsByStatusToday :many
SELECT status, COUNT(*) as count
FROM review_queue
WHERE reviewed_at >= CURRENT_DATE
GROUP BY status;
```

The `ListReviews` query uses dynamic SQL in the service layer (same pattern as `ListTransactions`) since it needs composable filters, JOINs for transaction context, and cursor pagination.

---

## 11. Implementation Tasks

Ordered by dependency. Each task should be a separate, reviewable unit of work.

### Task 1: Database Migration

- Create `internal/db/migrations/00019_review_queue.sql` with the schema from Section 3.1.
- Add `review_confidence_threshold` and `review_auto_enqueue` keys to the `app_config` seed migration (or as a separate migration if the seed migration is not idempotent for new keys).
- Run migrations, verify table and indexes exist.

### Task 2: sqlc Queries

- Create `internal/db/queries/reviews.sql` with queries from Section 10.
- Run `make sqlc` to generate Go code.

### Task 3: Service Layer — Review Types and Errors

- Add `ReviewResponse`, `ReviewListParams`, `ReviewListResult`, `SubmitReviewParams`, `BulkSubmitReviewParams`, `BulkReviewItem`, `BulkReviewResult`, `BulkReviewError`, `ReviewCountsResponse` to `internal/service/types.go`.
- Add `ErrReviewAlreadyResolved`, `ErrReviewAlreadyPending`, `ErrInvalidDecision` to `internal/service/errors.go`.

### Task 4: Service Layer — Review Operations

- Create `internal/service/reviews.go`.
- Implement `ListReviews` with dynamic SQL, transaction JOINs, cursor pagination.
- Implement `GetReview`.
- Implement `SubmitReview` with category update, audit log, and comment creation.
- Implement `BulkSubmitReviews` with DB transaction wrapping.
- Implement `EnqueueManualReview`.
- Implement `GetReviewCounts`.
- Implement `DismissReview`.

### Task 5: Sync Engine — Auto-Enqueue Hook

- Create `internal/sync/review.go` with `enqueueForReview` function.
- Modify `internal/sync/engine.go` `runSync()` to call `enqueueForReview` after each successful `upsertTransaction` call (for both added and modified transactions).
- Read `review_auto_enqueue` and `review_confidence_threshold` from `app_config` at the start of each sync (or pass them through the Engine).
- Ensure the enqueue happens within the same DB transaction as the upserts.

### Task 6: REST API Handlers

- Create `internal/api/reviews.go` with handlers: `ListReviewsHandler`, `GetReviewHandler`, `SubmitReviewHandler`, `BulkSubmitReviewsHandler`, `EnqueueReviewHandler`, `ReviewCountsHandler`.
- Register routes in `internal/api/router.go` (read endpoints accessible to all keys, write endpoints behind `RequireWriteScope`).

### Task 7: MCP Tools

- Add `list_pending_reviews` and `submit_review` tools to `internal/mcp/tools.go`.
- Add input structs `listPendingReviewsInput` and `submitReviewInput`.
- Classify `list_pending_reviews` as `read` and `submit_review` as `write` in the Phase 23 tool registry.
- Add `submit_review` permission guard (belt-and-suspenders, Phase 23 pattern).
- Update MCP server instructions in `internal/mcp/server.go` with review queue section.

### Task 8: Admin Handlers — Review Page

- Create `internal/admin/reviews.go` with `ReviewsPageHandler`, `SubmitReviewAdminHandler`, `DismissReviewAdminHandler`, `EnqueueReviewAdminHandler`.
- Register routes in `internal/admin/router.go`.

### Task 9: Admin Templates

- Create `internal/templates/pages/reviews.html` with the card-based layout from Section 8.2.
- Register in `internal/admin/templates.go` `basePages` slice.
- Add "Reviews" nav item to `internal/templates/partials/nav.html` with `clipboard-check` icon.
- Alpine.js interactions for inline approve/reject/skip without page reload.

### Task 10: Dashboard Widget

- Update `internal/admin/dashboard.go` to call `svc.GetReviewCounts()`.
- Add a review queue stat card to the dashboard template showing pending count with link to `/admin/reviews`.

### Task 11: CSS and Polish

- Run `make css` to compile any new styles.
- Verify dark mode compatibility of review cards, badges, and action buttons.
- Test card removal animation on review submission.

### Task 12: Configuration UI

- Add `review_auto_enqueue` and `review_confidence_threshold` settings to the admin settings page (`/admin/settings`) or the review page as a settings section.
- Allow admins to toggle auto-enqueue and adjust the confidence threshold.

---

## 11. Dependencies

### Depends On

- **Phase 20B (Category System):** The `categories` table and `category_id` FK on `transactions` must exist for category-aware review decisions. The `suggested_category_id` and `resolved_category_id` columns reference `categories.id`.
- **Phase 21 (Comments & Audit Log):** The `audit_log` table, `transaction_comments` table, `Actor` type, `ActorFromContext()` helper, and `WriteAuditLog()` service method are all prerequisites. Review notes create transaction comments.
- **Phase 23 (MCP Permissions):** The tool classification system (`read`/`write`), `RequireWriteScope` middleware, and `checkWritePermission` guard are used to protect write review tools and endpoints.

### Depended On By

- **Phase 25 (External Agent Support):** Webhook notifications fire when new items enter the review queue. The `GET /api/v1/reviews` polling endpoint is the primary way external agents discover pending reviews.
- **Phase 26 (Built-in Agent):** The built-in review agent consumes `list_pending_reviews` and `submit_review` MCP tools to autonomously process the queue.

### No Breaking Changes

- All new tables — no schema changes to existing tables (beyond the `category_id` column added in Phase 20B which is a prerequisite).
- All new API endpoints — no changes to existing endpoint contracts.
- New MCP tools are additive — existing tool definitions unchanged.
- Sync engine modification is internal (adds a post-upsert hook) — no change to provider interface or sync API.
- Dashboard adds a new page and a dashboard widget. Existing pages unchanged except nav gets a new item.
