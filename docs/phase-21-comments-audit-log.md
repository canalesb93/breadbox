# Phase 21: Transaction Comments & Audit Log

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## 1. Overview

Phase 21 adds two new tables — `transaction_comments` and `audit_log` — that bring annotation and traceability to Breadbox. Comments allow users and AI agents to attach notes to transactions (e.g., "This is the annual insurance payment" or "Recategorized from Shopping to Insurance"). The audit log captures every mutation to tracked entities, giving both humans and agents a queryable history of what changed, when, and why. Together, these features make Breadbox a collaborative workspace where decisions are visible and reversible.

---

## 2. Goals

1. **Annotation**: Users and AI agents can leave comments on transactions, supporting markdown formatting.
2. **Accountability**: Every mutation to a transaction (and later, other entities) is recorded with old/new values and actor identity.
3. **Agent learning**: AI agents can query the audit log via MCP to understand past decisions (e.g., "what categories has this family overridden?"), enabling smarter recommendations.
4. **Unified timeline**: The dashboard transaction detail page shows comments and audit log entries in a single chronological timeline.

---

## 3. Data Model

### 3.1 `transaction_comments` Table

```sql
-- +goose Up
CREATE TABLE transaction_comments (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id  UUID          NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
    author_type     TEXT          NOT NULL CHECK (author_type IN ('user', 'agent', 'system')),
    author_id       TEXT          NULL,
    author_name     TEXT          NOT NULL,
    content         TEXT          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX transaction_comments_transaction_id_idx ON transaction_comments (transaction_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS transaction_comments_transaction_id_idx;
DROP TABLE IF EXISTS transaction_comments;
```

**Column notes:**

| Column | Description |
|---|---|
| `transaction_id` | FK to `transactions.id`. CASCADE delete — if a transaction is hard-deleted, its comments go too. Soft-deleted transactions retain their comments (soft delete only sets `deleted_at`). |
| `author_type` | `user` = admin dashboard user, `agent` = AI agent via MCP/API, `system` = automated (e.g., sync engine). CHECK constraint, not an enum, to avoid migration complexity. |
| `author_id` | Opaque identifier for the author. For `user`: admin_account UUID. For `agent`: API key ID (UUID). For `system`: NULL. Nullable because system comments have no actor. |
| `author_name` | Display name at time of comment creation. Denormalized to survive author deletion. For `user`: admin username. For `agent`: API key name. For `system`: `"Breadbox"`. |
| `content` | Markdown-formatted text. No length limit at DB level; service layer enforces max 10,000 characters. |

### 3.2 `audit_log` Table

```sql
-- +goose Up
CREATE TABLE audit_log (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type     TEXT          NOT NULL,
    entity_id       UUID          NOT NULL,
    action          TEXT          NOT NULL CHECK (action IN ('create', 'update', 'delete')),
    field           TEXT          NULL,
    old_value       TEXT          NULL,
    new_value       TEXT          NULL,
    actor_type      TEXT          NOT NULL CHECK (actor_type IN ('user', 'agent', 'system')),
    actor_id        TEXT          NULL,
    actor_name      TEXT          NOT NULL,
    metadata        JSONB         NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_log_created_at_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_type, actor_id);

-- +goose Down
DROP INDEX IF EXISTS audit_log_actor_idx;
DROP INDEX IF EXISTS audit_log_created_at_idx;
DROP INDEX IF EXISTS audit_log_entity_idx;
DROP TABLE IF EXISTS audit_log;
```

**Column notes:**

| Column | Description |
|---|---|
| `entity_type` | String identifier for the entity being modified. Phase 21 values: `transaction`, `transaction_comment`. Future: `account`, `connection`, `user`, `category`. |
| `entity_id` | UUID of the entity. Not a FK — the entity may be deleted. |
| `action` | `create`, `update`, or `delete`. CHECK constraint. |
| `field` | The specific field that changed (e.g., `category_primary`, `merchant_name`, `display_name`). NULL for `create`/`delete` actions that affect the whole entity. |
| `old_value` | Previous value as a string. NULL for `create` actions. |
| `new_value` | New value as a string. NULL for `delete` actions. |
| `actor_type` | Same semantics as `transaction_comments.author_type`. |
| `actor_id` | Same semantics as `transaction_comments.author_id`. |
| `actor_name` | Denormalized display name. Same semantics as `transaction_comments.author_name`. |
| `metadata` | Optional JSONB for extra context (e.g., `{"trigger": "sync", "provider": "plaid"}` for system-initiated changes). |
| `created_at` | Immutable — audit log entries are append-only. No `updated_at`. |

### 3.3 Migrations

Two new migration files following the existing `NNNNN_description.sql` naming scheme:

- `00017_transaction_comments.sql`
- `00018_audit_log.sql`

### 3.4 FK and Deletion Policy

| Relationship | Policy | Rationale |
|---|---|---|
| `transaction_comments.transaction_id → transactions.id` | CASCADE | Comments are meaningless without their transaction. |
| `audit_log.entity_id` | No FK | Audit log must survive entity deletion. The log *is* the record that deletion happened. |

---

## 4. Service Layer

All service functions live in `internal/service/`. New files: `comments.go`, `audit.go`.

### 4.1 Actor Context

A new type to thread actor identity through service calls:

```go
// internal/service/types.go

type Actor struct {
    Type string // "user", "agent", "system"
    ID   string // admin_account ID, API key ID, or ""
    Name string // display name
}
```

### 4.2 Comment Operations (`internal/service/comments.go`)

```go
// CreateComment adds a comment to a transaction.
func (s *Service) CreateComment(ctx context.Context, params CreateCommentParams) (*CommentResponse, error)

// ListComments returns all comments for a transaction, ordered by created_at ASC.
func (s *Service) ListComments(ctx context.Context, transactionID string) ([]CommentResponse, error)

// UpdateComment updates the content of an existing comment. Only the original author can edit.
func (s *Service) UpdateComment(ctx context.Context, id string, params UpdateCommentParams) (*CommentResponse, error)

// DeleteComment soft-deletes a comment. Only the original author or a user (admin) can delete.
func (s *Service) DeleteComment(ctx context.Context, id string, actor Actor) error
```

**Types:**

```go
type CreateCommentParams struct {
    TransactionID string
    Content       string // max 10,000 chars
    Actor         Actor
}

type UpdateCommentParams struct {
    Content string
    Actor   Actor
}

type CommentResponse struct {
    ID            string `json:"id"`
    TransactionID string `json:"transaction_id"`
    AuthorType    string `json:"author_type"`
    AuthorID      *string `json:"author_id"`
    AuthorName    string `json:"author_name"`
    Content       string `json:"content"`
    CreatedAt     string `json:"created_at"`
    UpdatedAt     string `json:"updated_at"`
}
```

**Business rules:**

- `Content` must be 1–10,000 characters after trimming whitespace.
- `TransactionID` must reference an existing, non-soft-deleted transaction.
- Update/delete: actor must match `author_type`+`author_id`, OR actor must be `author_type=user` (admins can moderate).
- Creating a comment writes an `audit_log` entry with `entity_type=transaction`, `action=update`, `field=comment_added`.
- Deleting a comment writes an audit log entry.

### 4.3 Audit Log Operations (`internal/service/audit.go`)

```go
// WriteAuditLog writes one or more audit log entries. Used internally by other service methods.
func (s *Service) WriteAuditLog(ctx context.Context, entries []AuditLogEntry) error

// ListAuditLog returns audit log entries for a specific entity.
func (s *Service) ListAuditLog(ctx context.Context, params AuditLogListParams) (*AuditLogListResult, error)

// ListAuditLogGlobal returns audit log entries across all entities (admin/agent view).
func (s *Service) ListAuditLogGlobal(ctx context.Context, params AuditLogGlobalParams) (*AuditLogListResult, error)
```

**Types:**

```go
type AuditLogEntry struct {
    EntityType string
    EntityID   string
    Action     string // "create", "update", "delete"
    Field      *string
    OldValue   *string
    NewValue   *string
    Actor      Actor
    Metadata   map[string]string // optional, stored as JSONB
}

type AuditLogListParams struct {
    EntityType string
    EntityID   string
    Limit      int // default 50, max 200
    Cursor     string
}

type AuditLogGlobalParams struct {
    EntityType *string  // optional filter
    ActorType  *string  // optional filter
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
```

**Business rules:**

- Audit log entries are append-only. No update or delete operations.
- `WriteAuditLog` is a batch insert (single `INSERT ... VALUES` with multiple rows) for efficiency during sync.
- Cursor pagination uses `(created_at DESC, id DESC)` ordering, same pattern as transaction list.
- `ListAuditLog` always filters on `entity_type` + `entity_id`.

### 4.4 Audit Log Integration Points

The following existing mutations must write audit log entries. The `Actor` is derived from context (admin session, API key, or `system`).

| Operation | Entity Type | Action | Field(s) Logged |
|---|---|---|---|
| Category override (future Phase 20) | `transaction` | `update` | `category_id`, `category_override` |
| Account display name change | `account` | `update` | `display_name` |
| Account excluded toggle | `account` | `update` | `excluded` |
| Connection pause toggle | `connection` | `update` | `paused` |
| Connection sync interval change | `connection` | `update` | `sync_interval_override_minutes` |
| Connection delete | `connection` | `delete` | — |
| User create | `user` | `create` | — |
| User update | `user` | `update` | `name`, `email` |
| Comment create | `transaction` | `update` | `comment_added` (new_value = comment content preview) |
| Comment delete | `transaction` | `update` | `comment_deleted` |

**Actor resolution:**

- Admin dashboard actions: `Actor{Type: "user", ID: adminAccountID, Name: adminUsername}`. Resolved from session.
- REST API actions: `Actor{Type: "agent", ID: apiKeyID, Name: apiKeyName}`. Resolved from API key auth middleware (already sets key info in context).
- Sync engine actions: `Actor{Type: "system", ID: "", Name: "Breadbox"}`.

To thread the actor through, add a helper to extract it from the request context:

```go
// internal/service/actor.go

func ActorFromContext(ctx context.Context) Actor {
    // Check for API key context (set by APIKeyAuth middleware)
    // Check for admin session context
    // Default to system actor
}
```

The API key auth middleware (`internal/middleware/apikey.go`) already validates the key. Extend it to store the API key ID and name in the request context. Similarly, the admin session already has the admin account — add a middleware or helper to populate the `Actor`.

---

## 5. REST API

### 5.1 Comment Endpoints

All under `/api/v1/transactions/{transaction_id}/comments`. API key authenticated.

#### `GET /api/v1/transactions/{transaction_id}/comments`

List all comments for a transaction.

**Response (200):**
```json
{
    "comments": [
        {
            "id": "uuid",
            "transaction_id": "uuid",
            "author_type": "agent",
            "author_id": "uuid",
            "author_name": "Budget Bot",
            "content": "Recategorized from Shopping to Insurance — this is the annual home insurance premium.",
            "created_at": "2026-03-08T12:00:00Z",
            "updated_at": "2026-03-08T12:00:00Z"
        }
    ]
}
```

**Errors:**
- `404 NOT_FOUND` — transaction does not exist

#### `POST /api/v1/transactions/{transaction_id}/comments`

Create a new comment.

**Request:**
```json
{
    "content": "This is the quarterly HOA payment."
}
```

**Response (201):**
```json
{
    "id": "uuid",
    "transaction_id": "uuid",
    "author_type": "agent",
    "author_id": "uuid",
    "author_name": "Budget Bot",
    "content": "This is the quarterly HOA payment.",
    "created_at": "2026-03-08T12:00:00Z",
    "updated_at": "2026-03-08T12:00:00Z"
}
```

**Errors:**
- `400 INVALID_PARAMETER` — content empty or exceeds 10,000 characters
- `404 NOT_FOUND` — transaction does not exist

#### `PUT /api/v1/transactions/{transaction_id}/comments/{id}`

Update a comment's content.

**Request:**
```json
{
    "content": "Updated: This is the quarterly HOA payment, not monthly."
}
```

**Response (200):** Same shape as create response.

**Errors:**
- `400 INVALID_PARAMETER` — content empty or too long
- `403 FORBIDDEN` — actor is not the comment author
- `404 NOT_FOUND` — comment or transaction not found

#### `DELETE /api/v1/transactions/{transaction_id}/comments/{id}`

Delete a comment.

**Response (204):** No body.

**Errors:**
- `403 FORBIDDEN` — actor is not the comment author (and not an admin)
- `404 NOT_FOUND` — comment not found

### 5.2 Audit Log Endpoints

#### `GET /api/v1/audit-log`

Query the global audit log. API key authenticated.

**Query parameters:**

| Param | Type | Description |
|---|---|---|
| `entity_type` | string | Filter by entity type (e.g., `transaction`, `account`) |
| `entity_id` | string | Filter by entity ID (requires `entity_type`) |
| `actor_type` | string | Filter by actor type (`user`, `agent`, `system`) |
| `limit` | int | Max results (default 50, max 200) |
| `cursor` | string | Pagination cursor |

**Response (200):**
```json
{
    "entries": [
        {
            "id": "uuid",
            "entity_type": "transaction",
            "entity_id": "uuid",
            "action": "update",
            "field": "category_primary",
            "old_value": "SHOPPING",
            "new_value": "INSURANCE",
            "actor_type": "agent",
            "actor_id": "uuid",
            "actor_name": "Budget Bot",
            "metadata": {"reason": "Annual premium payment"},
            "created_at": "2026-03-08T12:00:00Z"
        }
    ],
    "next_cursor": "...",
    "has_more": true
}
```

#### `GET /api/v1/transactions/{id}/audit-log`

Get audit log entries for a specific transaction. Convenience endpoint (equivalent to global audit log with `entity_type=transaction&entity_id={id}`).

**Query parameters:** `limit`, `cursor` only.

**Response (200):** Same shape as global audit log response.

### 5.3 Handler Files

- `internal/api/comments.go` — `ListCommentsHandler`, `CreateCommentHandler`, `UpdateCommentHandler`, `DeleteCommentHandler`
- `internal/api/audit.go` — `ListAuditLogHandler`, `ListTransactionAuditLogHandler`

### 5.4 Router Changes (`internal/api/router.go`)

```go
// Inside /api/v1 route group:
r.Get("/transactions/{transaction_id}/comments", ListCommentsHandler(svc))
r.Post("/transactions/{transaction_id}/comments", CreateCommentHandler(svc))
r.Put("/transactions/{transaction_id}/comments/{id}", UpdateCommentHandler(svc))
r.Delete("/transactions/{transaction_id}/comments/{id}", DeleteCommentHandler(svc))

r.Get("/audit-log", ListAuditLogHandler(svc))
r.Get("/transactions/{id}/audit-log", ListTransactionAuditLogHandler(svc))
```

---

## 6. MCP Tools

### 6.1 New Tools

Add to `internal/mcp/tools.go`:

#### `add_transaction_comment`

```go
type addTransactionCommentInput struct {
    TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction to comment on"`
    Content       string `json:"content" jsonschema:"required,Comment text (markdown supported, max 10000 chars). Use comments to explain categorization decisions or flag transactions for review."`
}
```

**Tool description:** `"Add a comment to a transaction. Use this to explain categorization decisions, flag unusual transactions, or leave notes for the family. Comments are visible on the transaction detail page and to other agents. Supports markdown formatting."`

Returns the created comment as JSON.

#### `list_transaction_comments`

```go
type listTransactionCommentsInput struct {
    TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction"`
}
```

**Tool description:** `"List all comments on a transaction, ordered chronologically. Check comments before making changes to understand prior context and decisions by other agents or family members."`

Returns array of comments as JSON.

#### `get_transaction_history`

```go
type getTransactionHistoryInput struct {
    TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction"`
    Limit         int    `json:"limit,omitempty" jsonschema:"Max entries to return (default 50, max 200)"`
}
```

**Tool description:** `"Get the change history (audit log) for a specific transaction. Shows all modifications including category changes, comment additions, and sync updates. Use this to understand how a transaction's categorization evolved over time and learn from past decisions."`

Returns `AuditLogListResult` as JSON.

#### `query_audit_log`

```go
type queryAuditLogInput struct {
    EntityType string `json:"entity_type,omitempty" jsonschema:"Filter by entity type: transaction, account, connection, user"`
    ActorType  string `json:"actor_type,omitempty" jsonschema:"Filter by who made the change: user, agent, system"`
    Limit      int    `json:"limit,omitempty" jsonschema:"Max entries to return (default 50, max 200)"`
    Cursor     string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
}
```

**Tool description:** `"Query the global audit log to see all changes across the system. Use entity_type to focus on specific data types. Filter by actor_type='agent' to see what other AI agents have done, or actor_type='user' to see manual changes by the family. Useful for learning patterns: e.g., query all category overrides to understand the family's preferences."`

Returns `AuditLogListResult` as JSON.

### 6.2 MCP Server Instructions Update

Add to the instructions string in `internal/mcp/server.go`:

```
COMMENTS & AUDIT LOG:
- Use add_transaction_comment to explain your reasoning when recategorizing transactions
- Check list_transaction_comments before modifying a transaction to see prior context
- Use get_transaction_history to understand how a transaction has been modified over time
- Use query_audit_log with actor_type='user' to learn the family's categorization preferences
```

---

## 7. Dashboard UI

### 7.1 Transaction Detail Page (New)

**File:** `internal/templates/pages/transaction_detail.html`

Currently there is no transaction detail page — the transactions list is the only view. Phase 21 adds a detail page at `/admin/transactions/{id}`.

**Layout:**

```
┌──────────────────────────────────────────────┐
│ Transaction Detail                            │
│                                               │
│ ┌─────────────────────────────────────────┐   │
│ │ Transaction Info Card                    │   │
│ │ Name: Target                             │   │
│ │ Amount: $42.50 USD                       │   │
│ │ Date: 2026-03-01                         │   │
│ │ Account: Chase Checking (Ricardo)        │   │
│ │ Category: SHOPPING                       │   │
│ │ Status: Posted                           │   │
│ └─────────────────────────────────────────┘   │
│                                               │
│ ┌─────────────────────────────────────────┐   │
│ │ Activity Timeline                        │   │
│ │                                          │   │
│ │ 🔵 Mar 8 12:00 — Budget Bot (agent)     │   │
│ │    Recategorized from Shopping to...     │   │
│ │                                          │   │
│ │ 💬 Mar 8 11:30 — Budget Bot (agent)     │   │
│ │    "This is the annual insurance..."     │   │
│ │                                          │   │
│ │ 🔵 Mar 1 09:00 — Breadbox (system)      │   │
│ │    Transaction synced from Plaid         │   │
│ └─────────────────────────────────────────┘   │
│                                               │
│ ┌─────────────────────────────────────────┐   │
│ │ Add Comment                              │   │
│ │ ┌─────────────────────────────────────┐ │   │
│ │ │ Textarea (markdown)                  │ │   │
│ │ └─────────────────────────────────────┘ │   │
│ │ [Post Comment]                          │   │
│ └─────────────────────────────────────────┘   │
└──────────────────────────────────────────────┘
```

**Timeline merging:** Comments and audit log entries are merged into a single chronological list (newest first), distinguished by type:

- **Audit entries** render as compact status lines with a dot icon. Fields show `old_value → new_value`.
- **Comments** render as message bubbles with author name, type badge (`user`/`agent`/`system`), and markdown-rendered content.

The merge happens in the handler, not the template. The handler builds a `[]TimelineEntry` slice:

```go
type TimelineEntry struct {
    Type      string // "comment" or "audit"
    Timestamp time.Time
    // Comment fields (only if Type == "comment")
    Comment   *CommentResponse
    // Audit fields (only if Type == "audit")
    Audit     *AuditLogResponse
}
```

### 7.2 Transaction Detail Handler

**File:** `internal/admin/transactions.go`

```go
func TransactionDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc
```

**Route:** `r.Get("/transactions/{id}", TransactionDetailHandler(a, sm, tr, svc))` (add to admin router)

**Admin comment API routes:**

```go
// In admin API section of router:
r.Post("/transactions/{id}/comments", CreateTransactionCommentHandler(a, sm, svc))
r.Delete("/transactions/{id}/comments/{comment_id}", DeleteTransactionCommentHandler(a, sm, svc))
```

### 7.3 Transaction List Link

Add a clickable link from each transaction row in the transactions list page (`transactions.html`) to `/admin/transactions/{id}`. The transaction name column becomes a link.

### 7.4 Comment Form

The comment form uses a standard HTML form POST (no Alpine.js needed for MVP). After successful creation, redirect back to the transaction detail page with a flash message. Delete uses a small form with a confirm modal (DaisyUI `modal` pattern, already used elsewhere).

---

## 8. Migration Strategy

### 8.1 New Tables

Both tables are new — no data migration needed. The migrations are purely additive.

### 8.2 Backfill Decision

**No backfill of existing audit log entries.** Rationale:

- Existing mutations were not instrumented to capture old/new values at the time they happened.
- Retroactively creating audit entries with `created_at = NOW()` would be misleading (they didn't happen "now").
- The transaction `updated_at` timestamps already provide some history.
- The cost of a complex backfill outweighs the benefit for a system that's still in early use.

The audit log starts recording from the moment the migration runs. The first entries for existing entities will be the next time those entities are modified.

### 8.3 sqlc Queries

New query file: `internal/db/queries/comments.sql`

```sql
-- name: CreateComment :one
INSERT INTO transaction_comments (transaction_id, author_type, author_id, author_name, content)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListCommentsByTransaction :many
SELECT * FROM transaction_comments
WHERE transaction_id = $1
ORDER BY created_at ASC;

-- name: GetComment :one
SELECT * FROM transaction_comments WHERE id = $1;

-- name: UpdateComment :one
UPDATE transaction_comments
SET content = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteComment :exec
DELETE FROM transaction_comments WHERE id = $1;

-- name: CountCommentsByTransaction :one
SELECT COUNT(*) FROM transaction_comments WHERE transaction_id = $1;
```

Audit log queries use dynamic SQL in the service layer (same pattern as `ListTransactions`) since they need composable filters and cursor pagination. No sqlc queries for audit log reads.

Audit log writes use a single sqlc query for batch insert:

New query file: `internal/db/queries/audit_log.sql`

```sql
-- name: InsertAuditLogEntry :exec
INSERT INTO audit_log (entity_type, entity_id, action, field, old_value, new_value, actor_type, actor_id, actor_name, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
```

For batch inserts, the service layer builds a multi-row `INSERT` dynamically (same pattern as other dynamic queries in the codebase) rather than calling `InsertAuditLogEntry` in a loop.

---

## 9. Implementation Tasks

Ordered by dependency. Each task should be a separate, reviewable unit of work.

### Task 1: Database Migrations

- Create `internal/db/migrations/00017_transaction_comments.sql`
- Create `internal/db/migrations/00018_audit_log.sql`
- Run migrations, verify tables exist

### Task 2: sqlc Queries

- Create `internal/db/queries/comments.sql` with queries from Section 8.3
- Create `internal/db/queries/audit_log.sql` with single-row insert
- Run `make sqlc` to generate Go code

### Task 3: Actor Type and Context Helpers

- Add `Actor` struct to `internal/service/types.go`
- Create `internal/service/actor.go` with `ActorFromContext()` helper
- Extend API key auth middleware (`internal/middleware/apikey.go`) to store API key ID + name in context
- Add helper to extract admin session actor in `internal/admin/` package

### Task 4: Audit Log Service Layer

- Create `internal/service/audit.go`
- Implement `WriteAuditLog` (batch insert with dynamic SQL)
- Implement `ListAuditLog` (entity-scoped, cursor pagination)
- Implement `ListAuditLogGlobal` (cross-entity, cursor pagination)
- Add `AuditLogEntry`, `AuditLogListParams`, `AuditLogGlobalParams`, `AuditLogListResult`, `AuditLogResponse` to `internal/service/types.go`

### Task 5: Comment Service Layer

- Create `internal/service/comments.go`
- Implement `CreateComment`, `ListComments`, `UpdateComment`, `DeleteComment`
- Add `CreateCommentParams`, `UpdateCommentParams`, `CommentResponse` to `internal/service/types.go`
- Add `ErrForbidden` to `internal/service/errors.go`
- CreateComment and DeleteComment call `WriteAuditLog`

### Task 6: Comment REST API Handlers

- Create `internal/api/comments.go`
- Implement `ListCommentsHandler`, `CreateCommentHandler`, `UpdateCommentHandler`, `DeleteCommentHandler`
- Register routes in `internal/api/router.go`

### Task 7: Audit Log REST API Handlers

- Create `internal/api/audit.go`
- Implement `ListAuditLogHandler`, `ListTransactionAuditLogHandler`
- Register routes in `internal/api/router.go`

### Task 8: MCP Tools

- Add `add_transaction_comment`, `list_transaction_comments`, `get_transaction_history`, `query_audit_log` tools to `internal/mcp/tools.go`
- Add input structs for each tool
- Update MCP server instructions in `internal/mcp/server.go`

### Task 9: Audit Log Integration in Existing Mutations

- Instrument `UpdateAccountExcludedHandler` (`internal/admin/connections.go`)
- Instrument `UpdateAccountDisplayNameHandler` (`internal/admin/connections.go`)
- Instrument `UpdateConnectionPausedHandler` (`internal/admin/connections.go`)
- Instrument `UpdateConnectionSyncIntervalHandler` (`internal/admin/connections.go`)
- Instrument `DeleteConnectionHandler` (`internal/admin/connections.go`)
- Instrument `CreateUserHandler`, `UpdateUserHandler` (`internal/admin/users.go`)
- Each instrumented handler: resolve actor from session, call `svc.WriteAuditLog` after successful mutation

### Task 10: Transaction Detail Dashboard Page

- Create `internal/templates/pages/transaction_detail.html`
- Add `TransactionDetailHandler` to `internal/admin/transactions.go`
- Add admin API handlers for comment creation/deletion to `internal/admin/transactions.go`
- Register routes in `internal/admin/router.go`:
  - `r.Get("/transactions/{id}", TransactionDetailHandler(...))`
  - `r.Post("/api/transactions/{id}/comments", ...)`
  - `r.Delete("/api/transactions/{id}/comments/{comment_id}", ...)`
- Build `TimelineEntry` merge logic in the handler
- Template: transaction info card, activity timeline, comment form

### Task 11: Transaction List Linkification

- Update `internal/templates/pages/transactions.html` — make transaction name a link to `/admin/transactions/{id}`
- Update `internal/templates/pages/account_detail.html` — same change for the embedded transaction list

### Task 12: CSS and Polish

- Run `make css` to compile any new styles
- Verify dark mode compatibility of new components
- Test timeline rendering with mixed comment/audit entries

---

## 10. Dependencies

### Depends On

- **Phase 1–19** (existing schema, service layer, API, MCP, admin dashboard)
- The `transactions` table must exist with its current schema.
- API key auth middleware must be functional (already is).
- Admin session management must be functional (already is).

### Depended On By

- **Phase 20 (Category System)**: Category overrides will write audit log entries using the infrastructure built here. The `WriteAuditLog` service method and `Actor` type are prerequisites for instrumented category changes.
- **Future phases** that add entity mutations (e.g., transaction splitting, rule-based categorization) will use the audit log.

### No Breaking Changes

- All new tables, no schema changes to existing tables.
- All new API endpoints, no changes to existing endpoint contracts.
- New MCP tools are additive — existing tool definitions unchanged.
- Dashboard adds a new page and links; existing pages unchanged except for adding clickable links on transaction names.
