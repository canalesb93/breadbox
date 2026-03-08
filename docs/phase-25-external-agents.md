# Phase 25: Agentic Review — External Agent Support

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## 1. Overview

Phase 25 extends the review queue (Phase 24) so that users' own AI agents can autonomously review transactions via MCP or REST API. It adds outgoing webhook notifications when new items enter the review queue, a polling endpoint for pull-based agents, configurable review instructions (the context/prompt agents see), and dedicated MCP tools for fetching and submitting reviews. Together these features let a household connect any LLM-based agent to Breadbox and have it categorize, approve, or flag transactions without human intervention.

---

## 2. Goals

1. **Push-based notification.** When transactions enter the review queue (via sync auto-enqueue or manual submission), fire a webhook to a user-configured URL so external agents can respond immediately.
2. **Pull-based polling.** Agents that cannot receive webhooks can poll `GET /api/v1/reviews/pending` on a schedule.
3. **Configurable review context.** Admins write markdown instructions that are included in every review payload and MCP tool response, giving the agent household-specific context (e.g., "We consider Costco runs under $200 as groceries, not shopping").
4. **MCP-native review workflow.** The `review_transactions` tool returns pending items with instructions baked in. The `submit_review` tool accepts decisions. Both respect Phase 23 MCP permissions.
5. **Auditable.** Every review decision writes to the Phase 21 audit log with full actor attribution (API key identity).

---

## 3. Webhook System

### 3.1 Configuration

Outgoing webhook configuration is stored in the `app_config` table using these keys:

| Key | Type | Default | Description |
|---|---|---|---|
| `review_webhook_url` | Text (URL) | `""` (disabled) | HTTPS URL to POST review notifications to. Empty = disabled. |
| `review_webhook_secret` | Text | `""` | Shared secret for HMAC-SHA256 signing. Generated on first save if empty. |
| `review_webhook_events` | JSON string array | `["review_items_added"]` | Which events trigger webhooks. Phase 25 defines one event; future phases may add more. |

**Validation rules:**

- URL must start with `https://` (reject `http://` — webhook payloads contain financial data).
- Secret must be at least 32 characters. If the admin saves a URL without providing a secret, the server generates a 64-character hex random secret automatically.
- Events must be a subset of known event types.

### 3.2 Event Types

| Event | Trigger | Description |
|---|---|---|
| `review_items_added` | After sync auto-enqueue or manual enqueue | Fired once per batch of new review queue items, not per item. |

### 3.3 Payload Format

```json
{
    "event": "review_items_added",
    "timestamp": "2026-03-08T14:30:00Z",
    "data": {
        "count": 12,
        "review_ids": ["uuid1", "uuid2", "..."],
        "source": "sync",
        "connection_id": "uuid",
        "institution_name": "Chase"
    }
}
```

**Field notes:**

| Field | Description |
|---|---|
| `event` | Event type string. |
| `timestamp` | ISO 8601 UTC timestamp of when the event occurred. |
| `data.count` | Number of new review items in this batch. |
| `data.review_ids` | Array of review queue item UUIDs (max 100; if more, truncated with `has_more: true`). |
| `data.source` | What caused the enqueue: `sync`, `manual`, or `rule`. |
| `data.connection_id` | Connection UUID if source is `sync`; null otherwise. |
| `data.institution_name` | Human-readable institution name if available; null otherwise. |

### 3.4 HMAC Signing

Every outgoing webhook request includes an HMAC-SHA256 signature for verification by the receiving agent. The signing scheme follows the same pattern used by Teller's inbound webhooks (`internal/provider/teller/webhook.go`).

**Request headers:**

| Header | Value |
|---|---|
| `Content-Type` | `application/json` |
| `User-Agent` | `Breadbox/{version}` |
| `X-Breadbox-Signature` | `t={unix_timestamp},v1={hex_signature}` |
| `X-Breadbox-Event` | Event type string (e.g., `review_items_added`) |
| `X-Breadbox-Delivery-ID` | UUID for idempotency / dedup on the receiver side |

**Signature computation:**

```
signed_payload = "{unix_timestamp}.{json_body}"
signature = HMAC-SHA256(signed_payload, review_webhook_secret)
header = "t={unix_timestamp},v1={hex(signature)}"
```

This is the same `t=...,v1=...` format that Teller uses, so agents using standard webhook verification libraries will work out of the box.

### 3.5 Delivery and Retry Logic

Webhook delivery is handled by a new `internal/webhook/outgoing.go` module (distinct from the existing `internal/webhook/handler.go` which handles *inbound* provider webhooks).

**Delivery behavior:**

- Requests use a 10-second timeout.
- Success: HTTP 2xx response.
- Failure: HTTP 4xx/5xx or network error.
- Retry schedule: 3 attempts with exponential backoff — 30s, 5m, 30m.
- After all retries exhausted, the delivery is marked `failed` and no further attempts are made.
- Concurrent deliveries are serialized per webhook URL (no concurrent requests to the same endpoint).

**Implementation:**

- Deliveries run in a background goroutine managed by the `App` struct (similar to how sync runs in the background).
- A `webhook_deliveries` table logs every attempt for observability (see Section 3.6).
- On server restart, any `pending` deliveries are retried.

### 3.6 Delivery Log Table

```sql
-- Migration: 000XX_webhook_deliveries.sql (number assigned at implementation time)
-- +goose Up
CREATE TABLE webhook_deliveries (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    event           TEXT          NOT NULL,
    url             TEXT          NOT NULL,
    payload         JSONB         NOT NULL,
    delivery_id     UUID          NOT NULL UNIQUE,
    status          TEXT          NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
    attempts        INTEGER       NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ   NULL,
    next_retry_at   TIMESTAMPTZ   NULL,
    response_status INTEGER       NULL,
    response_body   TEXT          NULL,
    error_message   TEXT          NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX webhook_deliveries_status_idx ON webhook_deliveries (status, next_retry_at)
    WHERE status = 'pending';
CREATE INDEX webhook_deliveries_created_at_idx ON webhook_deliveries (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS webhook_deliveries;
```

**Column notes:**

| Column | Description |
|---|---|
| `event` | Event type string (e.g., `review_items_added`). |
| `url` | The webhook URL at the time of delivery (snapshot, not a reference to current config). |
| `payload` | Full JSON payload as JSONB for debugging. |
| `delivery_id` | Unique ID sent in `X-Breadbox-Delivery-ID` header. Used for receiver-side dedup. |
| `status` | `pending` (queued/retrying), `success` (2xx received), `failed` (all retries exhausted). |
| `attempts` | Number of delivery attempts made so far. |
| `response_status` | HTTP status code from the last attempt. NULL if no attempt yet. |
| `response_body` | First 1000 characters of the response body from the last attempt. For debugging. |
| `error_message` | Error string if the last attempt failed at the network level (timeout, DNS, etc.). |

**Retention:** Deliveries older than 7 days are automatically cleaned up by a periodic job (runs hourly via the existing cron scheduler).

---

## 4. Polling Endpoint

### 4.1 `GET /api/v1/reviews/pending`

Returns pending review queue items. API key authenticated.

**Query parameters:**

| Param | Type | Default | Description |
|---|---|---|---|
| `limit` | int | 50 | Max results (max 200). |
| `cursor` | string | — | Pagination cursor from previous response. |
| `account_id` | string | — | Filter by account UUID. |
| `user_id` | string | — | Filter by user (family member) UUID. |
| `category_slug` | string | — | Filter by suggested category slug. |
| `since` | string | — | ISO 8601 timestamp. Only return items created after this time. Useful for incremental polling. |
| `include_instructions` | bool | false | If true, include `review_instructions` field in the response (the markdown instructions configured by the admin). |

**Response (200):**

```json
{
    "items": [
        {
            "review_id": "uuid",
            "transaction_id": "uuid",
            "transaction": {
                "id": "uuid",
                "account_id": "uuid",
                "account_name": "Chase Checking",
                "user_name": "Ricardo",
                "amount": 42.50,
                "iso_currency_code": "USD",
                "date": "2026-03-01",
                "name": "TARGET",
                "merchant_name": "Target",
                "category_primary": "SHOPPING",
                "category_detailed": "SHOPPING_GENERAL",
                "pending": false
            },
            "review_type": "new_transaction",
            "suggested_category_id": "uuid",
            "suggested_category_slug": "shopping_general",
            "created_at": "2026-03-08T14:30:00Z"
        }
    ],
    "review_instructions": "...(only if include_instructions=true)...",
    "next_cursor": "...",
    "has_more": true,
    "total_pending": 47
}
```

**Field notes:**

- `transaction` is a denormalized snapshot of the transaction at the time of the response (same fields as `TransactionResponse` from the REST API, plus `account_name` and `user_name`).
- `review_instructions` is only present when `include_instructions=true`. This avoids sending potentially large markdown text on every poll.
- `total_pending` is the total count of pending items (not just those returned), so agents can decide whether to paginate.

**Errors:**

- `401 UNAUTHORIZED` — missing or invalid API key.
- `403 INSUFFICIENT_SCOPE` — read-only API key (review submission is a write operation, but *reading* pending reviews is read-only; this endpoint is accessible to read-only keys).

### 4.2 Long-Polling (Future Consideration)

Phase 25 uses standard short polling. Long-polling (hold connection open until new items arrive) is deferred. Agents should poll on a reasonable interval (e.g., every 60 seconds) or rely on webhooks for real-time notification.

---

## 5. Review Instructions

### 5.1 Storage

Review instructions are stored as plain markdown text in the `app_config` table:

| Key | Type | Default | Description |
|---|---|---|---|
| `review_instructions` | Text (markdown) | `""` (empty) | Instructions/context given to reviewing agents. |
| `review_instruction_template` | Text (slug) | `""` | Which built-in template is active (UI state tracking only). |

Maximum length: 20,000 characters (validated in the service layer).

### 5.2 Template Variables

Review instructions support simple template variables that are expanded at response time. Variables use `{{variable}}` syntax.

| Variable | Expands To | Example |
|---|---|---|
| `{{total_pending}}` | Number of pending review items | `47` |
| `{{date_range_start}}` | Earliest transaction date in pending items | `2026-03-01` |
| `{{date_range_end}}` | Latest transaction date in pending items | `2026-03-08` |
| `{{family_members}}` | Comma-separated list of user names | `Ricardo, Maria` |

Template variables are expanded by the service layer when instructions are returned via MCP or REST API (not stored expanded). Unrecognized `{{...}}` tokens are left as-is to avoid breaking markdown.

### 5.3 Injection into MCP and REST Responses

Review instructions are included in two places:

1. **MCP `review_transactions` tool response** — always included as a top-level `instructions` field.
2. **REST `GET /api/v1/reviews/pending`** — only when `include_instructions=true` query param is set.

### 5.4 Built-in Templates

Templates are hardcoded Go constants (same pattern as Phase 23 MCP instruction templates in `internal/mcp/templates.go`).

```go
// internal/service/review_templates.go (new file)

type ReviewInstructionTemplate struct {
    Slug        string `json:"slug"`
    Name        string `json:"name"`
    Description string `json:"description"`
    Content     string `json:"content"`
}

var ReviewInstructionTemplates = []ReviewInstructionTemplate{
    {
        Slug:        "categorization",
        Name:        "Transaction Categorization",
        Description: "Guide agents to categorize transactions based on merchant and amount patterns.",
        Content: `You are reviewing transactions for a family household.

TASK: For each pending transaction, determine the correct category.

DECISION FRAMEWORK:
1. Look at the merchant name and transaction description.
2. Check the suggested category — it was auto-assigned by the bank provider.
3. If the suggestion looks correct, approve it.
4. If incorrect, reject it and provide the correct category_slug.
5. For ambiguous transactions (e.g., Amazon could be groceries or shopping), consider:
   - Amount: grocery runs are typically $50-200
   - Look at past decisions for the same merchant using get_transaction_history

RESPONSE FORMAT for each review:
- decision: "approve" if the suggested category is correct
- decision: "reject" with override_category_slug if incorrect
- decision: "skip" if you cannot determine the category
- Always include a brief comment explaining your reasoning

AMOUNT CONVENTION:
- Positive = money out (purchases, payments)
- Negative = money in (refunds, deposits)

Family members: {{family_members}}
Pending items: {{total_pending}} ({{date_range_start}} to {{date_range_end}})`,
    },
    {
        Slug:        "anomaly_detection",
        Name:        "Anomaly Detection",
        Description: "Focus agents on flagging unusual or suspicious transactions.",
        Content: `You are reviewing transactions for potential anomalies or fraud.

TASK: Review each pending transaction and flag anything unusual.

FLAG CRITERIA:
- Transactions significantly larger than typical for that merchant/category
- Transactions from unfamiliar merchants
- Duplicate or near-duplicate transactions (same merchant + similar amount within 24h)
- Transactions in unexpected locations or currencies
- Recurring charges that have changed amount

DECISION FRAMEWORK:
- "approve" — transaction looks normal
- "reject" — transaction looks suspicious (add a detailed comment explaining why)
- "skip" — not enough context to judge

Always add a comment, even for approved transactions, noting any observations.

Family members: {{family_members}}
Pending items: {{total_pending}} ({{date_range_start}} to {{date_range_end}})`,
    },
    {
        Slug:        "budget_review",
        Name:        "Budget Review",
        Description: "Guide agents to review transactions against household budget expectations.",
        Content: `You are a budget-conscious reviewer for a family household.

TASK: Review each pending transaction and evaluate it against reasonable household spending.

REVIEW APPROACH:
1. Categorize the transaction correctly (approve or override the suggested category).
2. In your comment, note whether this seems like a planned/expected expense or discretionary spending.
3. Flag any transactions that seem unusually high for their category.

GUIDELINES:
- Be factual, not judgmental. Note "this is above typical for groceries" not "you spent too much."
- Group related transactions in your reasoning (e.g., "3 restaurant charges totaling $X this week").
- Use the audit log to understand past patterns for this merchant.

Family members: {{family_members}}
Pending items: {{total_pending}} ({{date_range_start}} to {{date_range_end}})`,
    },
}
```

---

## 6. MCP Tools

Phase 25 adds two new MCP tools to `internal/mcp/tools.go`. Both are classified as **write** tools (they interact with the review system, which is a mutating workflow) and are subject to Phase 23 MCP permission checks.

### 6.1 `review_transactions`

Fetches pending review items with review instructions included. Classified as **read** (it only reads the queue).

```go
type reviewTransactionsInput struct {
    Limit         int    `json:"limit,omitempty" jsonschema:"Max items to return (default 20, max 100). Start small to avoid overwhelming context."`
    AccountID     string `json:"account_id,omitempty" jsonschema:"Filter by account UUID."`
    UserID        string `json:"user_id,omitempty" jsonschema:"Filter by user (family member) UUID."`
    CategorySlug  string `json:"category_slug,omitempty" jsonschema:"Filter by suggested category slug."`
}
```

**Tool description:** `"Get pending transactions awaiting review, with instructions on how to review them. Each item includes the transaction details, suggested category, and review context. Use this as your starting point for reviewing transactions. After reviewing, submit decisions with submit_review. Amount convention: positive = money out (debits), negative = money in (credits)."`

**Response structure:**

```json
{
    "instructions": "...(expanded review instructions markdown)...",
    "items": [
        {
            "review_id": "uuid",
            "transaction": { "...same as REST response..." },
            "review_type": "new_transaction",
            "suggested_category_slug": "shopping_general",
            "created_at": "2026-03-08T14:30:00Z"
        }
    ],
    "total_pending": 47,
    "has_more": true
}
```

**Behavior:**

- Always includes expanded `instructions` (unlike REST where it's opt-in).
- Default limit is 20 (lower than REST default of 50) to keep MCP context manageable.
- If no review instructions are configured, `instructions` is a default string: `"No custom review instructions configured. Review each transaction and approve, reject, or skip based on your judgment."`

### 6.2 `submit_review`

Submits a review decision for one or more pending items. Classified as **write**.

```go
type submitReviewInput struct {
    Reviews []reviewDecision `json:"reviews" jsonschema:"required,Array of review decisions. Submit one or more at a time."`
}

type reviewDecision struct {
    ReviewID             string  `json:"review_id" jsonschema:"required,UUID of the review queue item."`
    Decision             string  `json:"decision" jsonschema:"required,One of: approve, reject, skip."`
    OverrideCategorySlug *string `json:"override_category_slug,omitempty" jsonschema:"Category slug to apply when rejecting the suggested category. Required when decision is reject."`
    Comment              *string `json:"comment,omitempty" jsonschema:"Explanation of the decision. Highly recommended for audit trail. Supports markdown."`
}
```

**Tool description:** `"Submit review decisions for pending transactions. You can approve the suggested category, reject it with an override, or skip. Always include a comment explaining your reasoning — this builds the audit trail and helps the family understand your decisions. Submit in batches for efficiency (up to 50 at a time)."`

**Response:**

```json
{
    "submitted": 5,
    "results": [
        {"review_id": "uuid", "status": "accepted"},
        {"review_id": "uuid", "status": "accepted"},
        {"review_id": "uuid", "status": "error", "error": "review item not found or already reviewed"}
    ]
}
```

**Behavior:**

- Maximum 50 decisions per call.
- Each decision is processed independently — one failure does not roll back others.
- Invalid `review_id` or already-reviewed items return per-item errors (not a top-level failure).
- Writes to audit log: `entity_type=review_queue`, `action=update`, `field=status`, with actor from API key context.
- If `comment` is provided, also creates a `transaction_comment` (Phase 21) on the associated transaction with the agent as author.
- Permission guard: checks Phase 23 write permission and API key scope before processing.

### 6.3 MCP Server Instructions Update

Add to the instructions string in `internal/mcp/server.go`:

```
TRANSACTION REVIEW:
- Use review_transactions to fetch pending items with instructions
- Submit decisions with submit_review — always include a comment
- Decisions: approve (suggested category is correct), reject (provide override_category_slug), skip (unsure)
- Check the review instructions carefully — they contain household-specific context
- Use get_transaction_history and list_transaction_comments to understand past decisions before reviewing
```

---

## 7. REST API

### 7.1 Review Endpoints

These endpoints extend the Phase 24 review API. API key authenticated.

#### `GET /api/v1/reviews/pending`

Described in Section 4.1 above.

#### `POST /api/v1/reviews/submit`

Submit review decisions in bulk. Same semantics as the MCP `submit_review` tool.

**Request:**

```json
{
    "reviews": [
        {
            "review_id": "uuid",
            "decision": "approve",
            "comment": "Correct categorization."
        },
        {
            "review_id": "uuid",
            "decision": "reject",
            "override_category_slug": "food_and_drink_groceries",
            "comment": "This is a Costco grocery run, not general shopping."
        }
    ]
}
```

**Response (200):**

```json
{
    "submitted": 2,
    "results": [
        {"review_id": "uuid", "status": "accepted"},
        {"review_id": "uuid", "status": "accepted"}
    ]
}
```

**Errors:**

- `400 INVALID_PARAMETER` — empty reviews array, array exceeds 50 items, invalid decision value, missing `override_category_slug` when decision is `reject`.
- `401 UNAUTHORIZED` — missing or invalid API key.
- `403 INSUFFICIENT_SCOPE` — read-only API key.

#### `GET /api/v1/reviews/instructions`

Returns the current review instructions (expanded with template variables).

**Response (200):**

```json
{
    "instructions": "...(expanded markdown)...",
    "template": "categorization",
    "updated_at": "2026-03-08T12:00:00Z"
}
```

### 7.2 Webhook Configuration Endpoints

Admin-only endpoints for webhook config management. These are admin API endpoints (session-authenticated), not REST API endpoints (API-key-authenticated), because webhook configuration is an admin function.

#### `GET /admin/api/review-webhooks`

Returns current webhook configuration.

**Response (200):**

```json
{
    "url": "https://agent.example.com/webhook",
    "secret_configured": true,
    "events": ["review_items_added"],
    "recent_deliveries": [
        {
            "id": "uuid",
            "event": "review_items_added",
            "status": "success",
            "attempts": 1,
            "created_at": "2026-03-08T14:30:00Z",
            "response_status": 200
        }
    ]
}
```

#### `POST /admin/api/review-webhooks`

Save webhook configuration.

**Request:**

```json
{
    "url": "https://agent.example.com/webhook",
    "secret": "",
    "events": ["review_items_added"]
}
```

If `secret` is empty and `url` is non-empty, a new secret is auto-generated and returned in the response.

**Response (200):**

```json
{
    "url": "https://agent.example.com/webhook",
    "secret": "abc123...64chars...",
    "events": ["review_items_added"],
    "message": "Webhook configuration saved. Secret was auto-generated."
}
```

The secret is only returned in full when it is first generated or explicitly changed. Subsequent GET requests show `secret_configured: true` but never return the secret itself.

#### `POST /admin/api/review-webhooks/test`

Sends a test webhook delivery to the configured URL.

**Response (200):**

```json
{
    "success": true,
    "status_code": 200,
    "response_time_ms": 142
}
```

### 7.3 Review Instructions Endpoints (Admin API)

#### `GET /admin/api/review-instructions`

Returns current review instructions and available templates.

**Response (200):**

```json
{
    "instructions": "...(raw markdown, not expanded)...",
    "template": "categorization",
    "templates": [
        {
            "slug": "categorization",
            "name": "Transaction Categorization",
            "description": "Guide agents to categorize transactions..."
        }
    ]
}
```

#### `POST /admin/api/review-instructions`

Save review instructions.

**Request:**

```json
{
    "instructions": "...(markdown)...",
    "template": "categorization"
}
```

**Response (200):**

```json
{
    "message": "Review instructions saved.",
    "character_count": 1523
}
```

### 7.4 Handler Files

- `internal/api/reviews.go` — `ListPendingReviewsHandler`, `SubmitReviewsHandler`, `GetReviewInstructionsHandler`
- `internal/admin/reviews.go` (new) — `ReviewWebhookGetHandler`, `ReviewWebhookSaveHandler`, `ReviewWebhookTestHandler`, `ReviewInstructionsGetHandler`, `ReviewInstructionsSaveHandler`

---

## 8. Dashboard UI

### 8.1 Review Settings Section

A new "Review Settings" card section is added to a dedicated `/admin/reviews/settings` page (linked from the review queue dashboard page added in Phase 24).

**Layout:**

```
┌──────────────────────────────────────────────┐
│ Review Settings                               │
│                                               │
│ ┌─────────────────────────────────────────┐   │
│ │ Review Instructions                      │   │
│ │                                          │   │
│ │ Template: [Categorization ▼]             │   │
│ │                                          │   │
│ │ ┌──────────────────────────────────────┐ │   │
│ │ │ Markdown editor textarea             │ │   │
│ │ │ (monospace, ~15 rows)                │ │   │
│ │ │                                      │ │   │
│ │ └──────────────────────────────────────┘ │   │
│ │ 1523/20000 characters                    │   │
│ │ [Save Instructions]                      │   │
│ └─────────────────────────────────────────┘   │
│                                               │
│ ┌─────────────────────────────────────────┐   │
│ │ Webhook Notifications                    │   │
│ │                                          │   │
│ │ URL:    [https://agent.example.com/hook] │   │
│ │ Secret: [••••••••••••] [Regenerate]      │   │
│ │ Events: ☑ New review items added         │   │
│ │                                          │   │
│ │ [Save Webhook] [Send Test]               │   │
│ └─────────────────────────────────────────┘   │
│                                               │
│ ┌─────────────────────────────────────────┐   │
│ │ Recent Deliveries                        │   │
│ │                                          │   │
│ │ Event          Status   Time     Code    │   │
│ │ review_added   ✓ 200    2m ago   —       │   │
│ │ review_added   ✗ 500    1h ago   timeout │   │
│ │ review_added   ✓ 200    2h ago   —       │   │
│ │                                          │   │
│ │ Showing last 20 deliveries               │   │
│ └─────────────────────────────────────────┘   │
└──────────────────────────────────────────────┘
```

### 8.2 Instructions Editor

- Template selector dropdown (Alpine.js `x-on:change` loads template content into textarea, with confirmation if textarea was modified).
- Character count display using Alpine.js `x-text`.
- Templates are embedded as JSON in the page template data (same pattern as Phase 23).
- The textarea value is POSTed as a standard form submission. No AJAX needed.

### 8.3 Webhook Configuration Card

- URL input field with HTTPS validation.
- Secret field: masked display of the first 8 characters (`bb_whk_a1b2...`). "Regenerate" button triggers a POST that generates a new secret and shows it once.
- Event checkboxes (only one event in Phase 25, but the UI supports multiple for future extensibility).
- "Send Test" button: uses Alpine.js `fetch()` to call `POST /admin/api/review-webhooks/test` and shows success/failure inline.

### 8.4 Delivery Log

- Table showing the last 20 webhook deliveries.
- Columns: Event, Status (green check / red X), Time (relative), HTTP code, Error.
- Loaded from the `webhook_deliveries` table.
- No pagination for MVP — just the most recent 20.

### 8.5 Template and Route

**Template file:** `internal/templates/pages/review_settings.html`

**Route:**

```go
// In admin authenticated routes:
r.Get("/reviews/settings", ReviewSettingsGetHandler(a, sm, tr, svc))
```

---

## 9. Security

### 9.1 HMAC Webhook Signing

Outgoing webhooks are HMAC-SHA256 signed using the `review_webhook_secret`. The receiver should:

1. Parse the `X-Breadbox-Signature` header to extract timestamp and signature.
2. Compute `HMAC-SHA256("{timestamp}.{raw_body}", secret)`.
3. Compare with constant-time comparison.
4. Reject if timestamp is older than 5 minutes (replay protection).

This is the same verification algorithm used for inbound Teller webhooks, making it easy to document and implement.

### 9.2 API Key Auth for Review Submissions

- `GET /api/v1/reviews/pending` — accessible to all API keys (read operation).
- `POST /api/v1/reviews/submit` — requires `full_access` scope API key (write operation). Read-only keys get `403 INSUFFICIENT_SCOPE`.
- `GET /api/v1/reviews/instructions` — accessible to all API keys (read operation).

### 9.3 MCP Permission Integration

- `review_transactions` tool — classified as `read`. Available in both `read_only` and `read_write` MCP modes. Accessible to read-only API keys.
- `submit_review` tool — classified as `write`. Only available in `read_write` MCP mode. Blocked for read-only API keys. Handler includes the Phase 23 `checkWritePermission` guard.

### 9.4 Rate Limiting

Phase 25 does not implement per-endpoint rate limiting (deferred to a future infrastructure phase). However, the review submission endpoints enforce:

- Maximum 50 decisions per `submit_review` / `POST /api/v1/reviews/submit` call.
- Review items that are already reviewed cannot be re-reviewed (prevents flooding the audit log).

### 9.5 Webhook Secret Storage

The `review_webhook_secret` is stored in plaintext in the `app_config` table (not encrypted). Rationale: it is a shared secret for HMAC signing, not a credential that grants access. The admin can view and regenerate it. This is consistent with how `teller_webhook_secret` is stored.

---

## 10. Service Layer

### 10.1 New Service Methods

```go
// internal/service/reviews.go (new file, extends Phase 24's review service)

// ListPendingReviews returns pending review items with optional filters.
func (s *Service) ListPendingReviews(ctx context.Context, params PendingReviewParams) (*PendingReviewResult, error)

// SubmitReviews processes a batch of review decisions.
func (s *Service) SubmitReviews(ctx context.Context, decisions []ReviewDecision, actor Actor) (*SubmitReviewResult, error)

// GetReviewInstructions returns the current review instructions with template variables expanded.
func (s *Service) GetReviewInstructions(ctx context.Context) (string, error)

// GetReviewInstructionsRaw returns the raw instructions (not expanded) for editing.
func (s *Service) GetReviewInstructionsRaw(ctx context.Context) (string, string, error) // instructions, template slug, error

// SaveReviewInstructions saves review instructions and template slug.
func (s *Service) SaveReviewInstructions(ctx context.Context, instructions string, templateSlug string) error

// GetWebhookConfig returns the current outgoing webhook configuration.
func (s *Service) GetWebhookConfig(ctx context.Context) (*WebhookConfig, error)

// SaveWebhookConfig saves outgoing webhook configuration.
func (s *Service) SaveWebhookConfig(ctx context.Context, cfg WebhookConfig) (*WebhookConfig, error)

// SendTestWebhook sends a test webhook delivery to the configured URL.
func (s *Service) SendTestWebhook(ctx context.Context) (*TestWebhookResult, error)
```

### 10.2 New Types

```go
// internal/service/types.go (additions)

type PendingReviewParams struct {
    Limit             int
    Cursor            string
    AccountID         *string
    UserID            *string
    CategorySlug      *string
    Since             *time.Time
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
    Decision             string  `json:"decision"` // "approve", "reject", "skip"
    OverrideCategorySlug *string `json:"override_category_slug,omitempty"`
    Comment              *string `json:"comment,omitempty"`
}

type SubmitReviewResult struct {
    Submitted int                   `json:"submitted"`
    Results   []ReviewDecisionResult `json:"results"`
}

type ReviewDecisionResult struct {
    ReviewID string `json:"review_id"`
    Status   string `json:"status"` // "accepted" or "error"
    Error    string `json:"error,omitempty"`
}

type WebhookConfig struct {
    URL    string   `json:"url"`
    Secret string   `json:"secret,omitempty"` // only returned on creation/regeneration
    Events []string `json:"events"`
    SecretConfigured bool `json:"secret_configured"`
}

type TestWebhookResult struct {
    Success        bool `json:"success"`
    StatusCode     int  `json:"status_code"`
    ResponseTimeMs int  `json:"response_time_ms"`
}
```

### 10.3 Webhook Dispatcher

```go
// internal/webhook/outgoing.go (new file)

// Dispatcher manages outgoing webhook deliveries with retry logic.
type Dispatcher struct {
    queries *db.Queries
    pool    *pgxpool.Pool
    logger  *slog.Logger
    client  *http.Client
    secret  string
    url     string
    version string
}

// NewDispatcher creates a new outgoing webhook dispatcher.
func NewDispatcher(queries *db.Queries, pool *pgxpool.Pool, logger *slog.Logger, version string) *Dispatcher

// Enqueue creates a new delivery record and triggers async delivery.
func (d *Dispatcher) Enqueue(ctx context.Context, event string, payload any) error

// ProcessPending processes any pending deliveries (called on startup and after enqueue).
func (d *Dispatcher) ProcessPending(ctx context.Context) error

// Cleanup removes deliveries older than 7 days.
func (d *Dispatcher) Cleanup(ctx context.Context) error
```

The dispatcher is initialized in `App` struct and passed to the sync engine so it can fire webhooks after auto-enqueue. The dispatcher reads the current `review_webhook_url` and `review_webhook_secret` from `app_config` at dispatch time (not cached), so config changes take effect immediately.

---

## 11. Implementation Tasks

Ordered by dependency. Each task is a reviewable unit of work.

### Task 1: Database Migration — Webhook Deliveries

- **File:** `internal/db/migrations/000XX_webhook_deliveries.sql`
- Create `webhook_deliveries` table per Section 3.6
- Run migration, verify table exists

### Task 2: sqlc Queries — Webhook Deliveries

- **File:** `internal/db/queries/webhook_deliveries.sql`
- Queries: `InsertWebhookDelivery`, `UpdateWebhookDeliveryStatus`, `ListRecentDeliveries`, `GetPendingDeliveries`, `CleanupOldDeliveries`
- Run `make sqlc`

### Task 3: Review Instruction Templates

- **File:** `internal/service/review_templates.go` (new)
- `ReviewInstructionTemplate` struct and `ReviewInstructionTemplates` slice
- Template content per Section 5.4

### Task 4: Service Layer — Review Instructions

- **File:** `internal/service/reviews.go` (extends Phase 24 file)
- Implement `GetReviewInstructions` (with template variable expansion)
- Implement `GetReviewInstructionsRaw`, `SaveReviewInstructions`
- Add template variable expansion logic
- Add types to `internal/service/types.go`

### Task 5: Service Layer — Pending Reviews and Submit

- **File:** `internal/service/reviews.go`
- Implement `ListPendingReviews` (dynamic SQL with filters, cursor pagination)
- Implement `SubmitReviews` (batch processing, audit log writes, comment creation)
- Write permission validation: decision values, required `override_category_slug` on reject

### Task 6: Service Layer — Webhook Config

- **File:** `internal/service/reviews.go` (or `internal/service/webhook_config.go`)
- Implement `GetWebhookConfig`, `SaveWebhookConfig`, `SendTestWebhook`
- Auto-generate secret if URL is set and secret is empty
- HTTPS URL validation

### Task 7: Outgoing Webhook Dispatcher

- **File:** `internal/webhook/outgoing.go` (new)
- `Dispatcher` struct with `Enqueue`, `ProcessPending`, `Cleanup`
- HMAC-SHA256 signing per Section 3.4
- Retry logic with exponential backoff per Section 3.5
- HTTP client with 10-second timeout

### Task 8: Integrate Dispatcher into App

- **File:** `cmd/breadbox/main.go` (or `internal/app/app.go`)
- Initialize `Dispatcher` in `App` struct
- Call `ProcessPending` on startup (recover from restart)
- Register `Cleanup` in cron scheduler (hourly)
- Wire dispatcher to Phase 24 auto-enqueue path (sync engine fires webhook after enqueue)

### Task 9: REST API — Pending Reviews and Submit

- **File:** `internal/api/reviews.go` (new)
- `ListPendingReviewsHandler` — `GET /api/v1/reviews/pending`
- `SubmitReviewsHandler` — `POST /api/v1/reviews/submit` (behind `RequireWriteScope` middleware)
- `GetReviewInstructionsHandler` — `GET /api/v1/reviews/instructions`
- Register routes in `internal/api/router.go`

### Task 10: MCP Tools — review_transactions and submit_review

- **File:** `internal/mcp/tools.go`
- Add `review_transactions` tool (read classification)
- Add `submit_review` tool (write classification, with `checkWritePermission` guard)
- Add input structs per Section 6
- Update MCP server instructions per Section 6.3

### Task 11: Admin Handlers — Review Settings Page

- **File:** `internal/admin/reviews.go` (new)
- `ReviewSettingsGetHandler` — loads instructions, webhook config, templates, recent deliveries
- `ReviewInstructionsSaveHandler` — `POST /admin/reviews/settings/instructions`
- `ReviewWebhookSaveHandler` — `POST /admin/reviews/settings/webhook`
- `ReviewWebhookTestHandler` — `POST /admin/api/review-webhooks/test`
- Admin API handlers for JSON endpoints: `POST /admin/api/review-webhooks`, `GET /admin/api/review-webhooks`

### Task 12: Dashboard Template — Review Settings

- **File:** `internal/templates/pages/review_settings.html` (new)
- Three-card layout: instructions editor, webhook config, delivery log
- Alpine.js for template loading, character count, test webhook button
- Register in `internal/admin/templates.go`

### Task 13: Admin Router Registration

- **File:** `internal/admin/router.go`
- Add `/reviews/settings` GET route (authenticated)
- Add `/reviews/settings/instructions` POST route
- Add `/reviews/settings/webhook` POST route
- Add `/admin/api/review-webhooks` GET/POST routes
- Add `/admin/api/review-webhooks/test` POST route

### Task 14: Update Docs

- **File:** `docs/data-model.md` — add `webhook_deliveries` table, new `app_config` keys
- **File:** `CLAUDE.md` — add review webhook and review instructions design decisions
- **File:** `docs/ROADMAP.md` — update Phase 25 status

---

## 12. Dependencies

### Depends On

- **Phase 24 (Review Queue):** The `review_queue` table, auto-enqueue logic, and base review service methods must exist. Phase 25 extends the review service, not replaces it.
- **Phase 21 (Comments & Audit Log):** `submit_review` creates transaction comments and audit log entries. The `Actor` type, `WriteAuditLog`, and `CreateComment` service methods are prerequisites.
- **Phase 23 (MCP Permissions):** The `review_transactions` and `submit_review` tools are subject to MCP mode and API key scope checks. The `checkWritePermission` guard and `BuildServer` tool filtering must be in place.

### Depended On By

- **Phase 26 (Built-in Agent):** The built-in agent uses the same review submission API (`SubmitReviews` service method) as external agents. Phase 25 establishes the contract that Phase 26 consumes.

### No Breaking Changes

- All new tables (webhook_deliveries). No schema changes to existing tables.
- All new REST endpoints. Existing endpoints unchanged.
- New MCP tools are additive. Existing tools unchanged.
- New `app_config` keys. Existing keys unchanged.
- New admin page. Existing pages unchanged.
