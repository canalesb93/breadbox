# Phase 26: Agentic Review — Built-in Agent

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## Table of Contents

1. [Overview](#1-overview)
2. [Goals](#2-goals)
3. [AI Provider Abstraction](#3-ai-provider-abstraction)
4. [Data Model](#4-data-model)
5. [Review Prompt System](#5-review-prompt-system)
6. [Agent Runner](#6-agent-runner)
7. [Cost Controls](#7-cost-controls)
8. [Trigger Modes](#8-trigger-modes)
9. [Dashboard UI](#9-dashboard-ui)
10. [Service Layer](#10-service-layer)
11. [Implementation Tasks](#11-implementation-tasks)
12. [Dependencies](#12-dependencies)

---

## 1. Overview

Phase 26 adds a built-in AI agent that reviews transactions automatically — no external agent setup required. The admin configures an AI provider (Anthropic Claude or OpenAI GPT), writes review instructions, and Breadbox calls the AI API internally when new items appear in the review queue (Phase 24). The agent uses transaction context, category taxonomy, and similar past decisions from the audit log (Phase 21) to make categorization decisions, then submits results back to the review queue as an `agent` actor. Cost controls (per-sync limits, daily budget caps, token usage tracking) prevent runaway spending.

---

## 2. Goals

| Goal | Description |
|------|-------------|
| Zero-config AI review | Non-technical users get AI-powered transaction review by entering a single API key. No MCP client, no external agent, no scripting. |
| Learning from history | The agent sees similar past decisions (from the audit log) so it improves over time as the family builds review history. |
| Cost transparency | Admins see exactly how many tokens and dollars the agent has consumed, with hard caps that prevent surprises. |
| Multiple AI providers | Support Anthropic Claude and OpenAI GPT from day one. Adding future providers is a matter of implementing a Go interface. |
| Non-destructive defaults | The agent submits review decisions that still require admin approval unless auto-approve is explicitly enabled. |

---

## 3. AI Provider Abstraction

### 3.1 Interface

A new package `internal/ai` defines a provider-agnostic interface for calling AI APIs. This is distinct from the bank `Provider` interface — it handles text generation, not financial data sync.

```go
// internal/ai/provider.go

package ai

import "context"

// Provider abstracts an AI/LLM API for transaction review.
type Provider interface {
    // Name returns the provider identifier (e.g., "anthropic", "openai").
    Name() string

    // Review sends a review prompt and returns the AI's structured response.
    // The prompt includes transaction data, instructions, and context.
    // Returns the response text and token usage stats.
    Review(ctx context.Context, req ReviewRequest) (*ReviewResponse, error)

    // ValidateKey tests whether the configured API key is valid.
    // Returns nil if the key works, an error with a human-readable message otherwise.
    ValidateKey(ctx context.Context) error
}

// ReviewRequest contains everything the AI needs to review transactions.
type ReviewRequest struct {
    // SystemPrompt is the system-level instruction (review instructions + context).
    SystemPrompt string

    // UserPrompt is the per-batch prompt containing transaction data and past decisions.
    UserPrompt string

    // Model is the specific model to use (e.g., "claude-sonnet-4-20250514", "gpt-4o").
    Model string

    // MaxTokens caps the response length.
    MaxTokens int
}

// ReviewResponse contains the AI's response and usage metadata.
type ReviewResponse struct {
    // Content is the raw text response from the AI.
    Content string

    // InputTokens is the number of tokens in the prompt.
    InputTokens int

    // OutputTokens is the number of tokens in the response.
    OutputTokens int

    // Model is the actual model used (may differ from requested if aliased).
    Model string
}
```

### 3.2 Anthropic Provider

```go
// internal/ai/anthropic/provider.go

package anthropic

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"

    "breadbox/internal/ai"
)

const apiBaseURL = "https://api.anthropic.com/v1/messages"

type Provider struct {
    apiKey     string
    httpClient *http.Client
}

func New(apiKey string) *Provider {
    return &Provider{
        apiKey:     apiKey,
        httpClient: &http.Client{},
    }
}

func (p *Provider) Name() string { return "anthropic" }

func (p *Provider) Review(ctx context.Context, req ai.ReviewRequest) (*ai.ReviewResponse, error) {
    // Build Anthropic Messages API request body.
    // POST https://api.anthropic.com/v1/messages
    // Headers: x-api-key, anthropic-version: 2023-06-01, content-type: application/json
    // Body: { model, max_tokens, system, messages: [{role: "user", content: req.UserPrompt}] }
    // Parse response: content[0].text, usage.input_tokens, usage.output_tokens
    // ...
}

func (p *Provider) ValidateKey(ctx context.Context) error {
    // Send a minimal request to verify the key works.
    // Use a tiny max_tokens (1) and a simple prompt.
    // ...
}
```

**Model defaults:**

| Provider | Default Model | Configurable |
|----------|--------------|-------------|
| Anthropic | `claude-sonnet-4-20250514` | Yes, via `app_config` |
| OpenAI | `gpt-4o` | Yes, via `app_config` |

### 3.3 OpenAI Provider

```go
// internal/ai/openai/provider.go

package openai

const apiBaseURL = "https://api.openai.com/v1/chat/completions"

type Provider struct {
    apiKey     string
    httpClient *http.Client
}

func New(apiKey string) *Provider {
    return &Provider{
        apiKey:     apiKey,
        httpClient: &http.Client{},
    }
}

func (p *Provider) Name() string { return "openai" }

func (p *Provider) Review(ctx context.Context, req ai.ReviewRequest) (*ai.ReviewResponse, error) {
    // Build OpenAI Chat Completions API request body.
    // POST https://api.openai.com/v1/chat/completions
    // Headers: Authorization: Bearer <key>, content-type: application/json
    // Body: { model, max_tokens, messages: [{role: "system", content: req.SystemPrompt}, {role: "user", content: req.UserPrompt}] }
    // Parse response: choices[0].message.content, usage.prompt_tokens, usage.completion_tokens
    // ...
}

func (p *Provider) ValidateKey(ctx context.Context) error {
    // List models endpoint to verify key validity.
    // GET https://api.openai.com/v1/models with Authorization header.
    // ...
}
```

### 3.4 No External Dependencies

Both providers use `net/http` directly (hand-written HTTP clients) — no SDK dependencies. This matches the existing Teller pattern of hand-written HTTP clients and keeps the dependency footprint small.

### 3.5 API Key Storage

AI provider API keys are encrypted with AES-256-GCM (same as Teller credentials and PEM certificates) and stored base64-encoded in `app_config`. The encryption uses the existing `internal/crypto` package and the `ENCRYPTION_KEY` environment variable.

```
app_config key: ai_api_key_anthropic
app_config value: base64(AES-256-GCM(api_key_plaintext))

app_config key: ai_api_key_openai
app_config value: base64(AES-256-GCM(api_key_plaintext))
```

Only one provider is active at a time, determined by the `ai_provider` config key. Both keys can be stored simultaneously so switching providers does not require re-entering the key.

### 3.6 Provider Initialization

The `ai.Provider` is constructed on-demand when the agent runner needs it (not at `App` startup). The runner reads the active provider name and encrypted API key from `app_config`, decrypts the key, and creates the appropriate provider instance. This avoids holding decrypted keys in memory when the agent is not running.

If the `ENCRYPTION_KEY` is not set and an AI key is configured, the agent runner logs a warning and skips review (same fail-fast pattern as Plaid/Teller providers).

---

## 4. Data Model

### 4.1 New Table: `ai_usage_log`

Tracks every AI API call for cost accounting and debugging.

```sql
-- Migration: 00XXX_ai_usage_log.sql (number depends on preceding migrations)
-- +goose Up
CREATE TABLE ai_usage_log (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        TEXT          NOT NULL,
    model           TEXT          NOT NULL,
    input_tokens    INTEGER       NOT NULL DEFAULT 0,
    output_tokens   INTEGER       NOT NULL DEFAULT 0,
    review_count    INTEGER       NOT NULL DEFAULT 0,
    trigger         TEXT          NOT NULL CHECK (trigger IN ('sync', 'manual', 'scheduled')),
    sync_log_id     UUID          NULL REFERENCES sync_logs (id) ON DELETE SET NULL,
    error_message   TEXT          NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX ai_usage_log_created_at_idx ON ai_usage_log (created_at DESC);
CREATE INDEX ai_usage_log_daily_idx ON ai_usage_log (DATE(created_at), provider);

-- +goose Down
DROP TABLE IF EXISTS ai_usage_log;
```

**Column notes:**

| Column | Description |
|--------|-------------|
| `provider` | AI provider name: `anthropic` or `openai`. |
| `model` | Actual model used (e.g., `claude-sonnet-4-20250514`, `gpt-4o`). |
| `input_tokens` | Total input tokens across all reviews in this batch. |
| `output_tokens` | Total output tokens across all reviews in this batch. |
| `review_count` | Number of review queue items processed in this batch. |
| `trigger` | What triggered this run: `sync` (post-sync hook), `manual` (dashboard button), `scheduled` (cron). |
| `sync_log_id` | FK to the sync that triggered this review. NULL for manual/scheduled triggers. SET NULL on sync log deletion. |
| `error_message` | Error text if the AI API call failed. NULL on success. |
| `created_at` | When the AI API call was made. |

### 4.2 `app_config` Keys

All AI-related configuration is stored in the existing `app_config` key-value table. No new tables beyond `ai_usage_log`.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ai_provider` | `anthropic` \| `openai` \| `""` | `""` (disabled) | Active AI provider. Empty string means built-in agent is disabled. |
| `ai_api_key_anthropic` | encrypted string | `""` | AES-256-GCM encrypted Anthropic API key, base64-encoded. |
| `ai_api_key_openai` | encrypted string | `""` | AES-256-GCM encrypted OpenAI API key, base64-encoded. |
| `ai_model` | string | `""` (use provider default) | Override model name. Empty uses provider default (`claude-sonnet-4-20250514` / `gpt-4o`). |
| `ai_review_prompt` | text (markdown) | `""` (use built-in default) | Custom review prompt template. See Section 5 for template variables. |
| `ai_auto_review` | `true` \| `false` | `false` | Whether to automatically run reviews after sync completes. |
| `ai_auto_approve` | `true` \| `false` | `false` | Whether to auto-approve agent decisions (skip human review). |
| `ai_max_reviews_per_sync` | integer | `50` | Maximum review queue items to process per sync trigger. |
| `ai_daily_budget_cents` | integer | `100` (=$1.00) | Maximum estimated daily AI API spend in cents. 0 = unlimited. |
| `ai_max_tokens` | integer | `4096` | Max output tokens per AI API call. |
| `ai_similar_decisions_count` | integer | `10` | Number of similar past decisions to include as context. |
| `ai_schedule_enabled` | `true` \| `false` | `false` | Whether the scheduled (cron) agent runner is enabled. |
| `ai_schedule_interval_minutes` | integer | `60` | How often the scheduled agent runner fires (minimum 15). |

### 4.3 No Changes to Existing Tables

The built-in agent uses the review queue (Phase 24) and audit log (Phase 21) as-is. It submits review decisions through the same service layer methods that external agents and dashboard users use. The `review_queue` table's `reviewer_type` and `reviewer_id` columns identify the built-in agent:

- `reviewer_type = 'agent'`
- `reviewer_id = 'builtin'`
- `reviewer_name = 'Breadbox AI'`

---

## 5. Review Prompt System

### 5.1 Prompt Structure

Each AI API call consists of a **system prompt** and a **user prompt**:

- **System prompt:** Review instructions (built-in defaults + custom instructions) that set the agent's behavior. Sent once per batch.
- **User prompt:** Transaction data, category list, and similar past decisions for the specific batch of items being reviewed.

### 5.2 Template Variables

The custom review prompt (`ai_review_prompt`) supports template variables that are interpolated before sending to the AI. Variables use `{{variable_name}}` syntax (Go `text/template`).

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `{{.CategoryList}}` | Formatted list of all categories (slug + display name, hierarchical) | `food_and_drink: Food & Drink\n  food_and_drink_groceries: Groceries\n  ...` |
| `{{.UserNames}}` | Comma-separated list of family member names | `Ricardo, Maria` |
| `{{.AccountSummary}}` | Brief list of accounts with types | `Chase Checking (checking), Amex Gold (credit)` |
| `{{.TransactionCount}}` | Number of transactions in this review batch | `12` |
| `{{.DateRange}}` | Date range of transactions being reviewed | `2026-03-01 to 2026-03-08` |

### 5.3 Default Review Prompt

Used when `ai_review_prompt` is empty. Stored as a Go constant in `internal/ai/prompt.go`:

```go
const DefaultReviewPrompt = `You are a financial transaction reviewer for a family's household finances.

TASK:
Review each transaction and determine the correct category. Consider the merchant name, amount, date, and any patterns from past decisions.

CATEGORIES:
{{.CategoryList}}

FAMILY MEMBERS: {{.UserNames}}
ACCOUNTS: {{.AccountSummary}}

GUIDELINES:
- Choose the most specific category that matches (prefer subcategories over parent categories).
- If a transaction has been categorized before by the family, follow their established pattern.
- If unsure between two categories, choose the more common one for that merchant.
- For ambiguous transactions, use the "uncategorized" category rather than guessing.
- Amounts follow the convention: positive = money out (debit), negative = money in (credit).

RESPONSE FORMAT:
Respond with a JSON array. Each element has:
- "transaction_id": the UUID of the transaction
- "category_slug": the category slug to assign
- "confidence": "high", "medium", or "low"
- "reasoning": a brief (1-2 sentence) explanation

Example:
[
  {
    "transaction_id": "550e8400-e29b-41d4-a716-446655440000",
    "category_slug": "food_and_drink_groceries",
    "confidence": "high",
    "reasoning": "Whole Foods is a grocery store."
  }
]

Only output the JSON array. No markdown fencing, no preamble.`
```

### 5.4 User Prompt Construction

The user prompt is built per-batch by the agent runner. It contains the actual transaction data and past decisions.

```go
// internal/ai/prompt.go

func BuildUserPrompt(items []ReviewItem, pastDecisions []PastDecision) string {
    // Format:
    // TRANSACTIONS TO REVIEW:
    // 1. ID: <uuid>
    //    Date: 2026-03-07
    //    Name: WHOLE FOODS MARKET
    //    Merchant: Whole Foods
    //    Amount: $45.67 USD
    //    Account: Chase Checking (Ricardo)
    //    Current Category: FOOD_AND_DRINK (provider-assigned)
    //
    // SIMILAR PAST DECISIONS (from your review history):
    // - "WHOLE FOODS" ($32.10, 2026-02-15) → food_and_drink_groceries (high confidence)
    //   Reasoning: "Whole Foods is a grocery store"
    // - "TARGET" ($87.43, 2026-02-20) → shopping (medium confidence)
    //   Reasoning: "Target purchases are typically general shopping"
}
```

### 5.5 Response Parsing

The agent runner expects a JSON array from the AI. Parsing is lenient:

1. Try to parse the raw response as JSON directly.
2. If that fails, look for a JSON array within markdown code fences (` ```json ... ``` `).
3. If that fails, look for `[` ... `]` boundaries in the text.
4. If all parsing fails, log the error in `ai_usage_log` and skip the batch.

```go
// internal/ai/parse.go

type ReviewDecision struct {
    TransactionID string `json:"transaction_id"`
    CategorySlug  string `json:"category_slug"`
    Confidence    string `json:"confidence"` // "high", "medium", "low"
    Reasoning     string `json:"reasoning"`
}

// ParseReviewResponse extracts review decisions from the AI response text.
// Returns parsed decisions and any parse error.
func ParseReviewResponse(content string) ([]ReviewDecision, error) {
    // Try direct JSON parse, then fenced, then bracket extraction.
    // Validate each decision: transaction_id must be UUID, category_slug must be non-empty.
    // Skip invalid entries (log warning), return valid ones.
}
```

### 5.6 Prompt Editor

The dashboard provides a markdown textarea for editing the review prompt (see Section 9.3). The editor shows:

- Current template with variable placeholders highlighted.
- A "Preview" panel showing the rendered prompt with sample data substituted.
- A "Reset to Default" button that restores `DefaultReviewPrompt`.

---

## 6. Agent Runner

### 6.1 Overview

The agent runner (`internal/ai/runner.go`) is the core orchestration component. It:

1. Loads pending review queue items.
2. Fetches similar past decisions from the audit log.
3. Builds the prompt.
4. Calls the AI API.
5. Parses the response.
6. Submits review decisions to the review queue.
7. Logs usage to `ai_usage_log`.

### 6.2 Runner Structure

```go
// internal/ai/runner.go

package ai

import (
    "context"
    "log/slog"

    "breadbox/internal/service"
)

// Runner orchestrates built-in AI review of transactions.
type Runner struct {
    svc    *service.Service
    logger *slog.Logger
}

func NewRunner(svc *service.Service, logger *slog.Logger) *Runner {
    return &Runner{svc: svc, logger: logger}
}

// Run processes pending review queue items using the configured AI provider.
// It is safe to call concurrently — it acquires a lock to prevent duplicate runs.
//
// trigger indicates what initiated this run: "sync", "manual", or "scheduled".
// syncLogID is the sync log that triggered this run (nil for manual/scheduled).
func (r *Runner) Run(ctx context.Context, trigger string, syncLogID *string) error {
    // 1. Load AI config from app_config (provider, model, limits, prompt).
    // 2. Check daily budget — abort if exceeded.
    // 3. Create AI provider instance (decrypt key, construct provider).
    // 4. Load pending review items (up to ai_max_reviews_per_sync).
    // 5. Load category list for prompt.
    // 6. Batch items into groups (see Section 6.3).
    // 7. For each batch:
    //    a. Load similar past decisions for the batch's transactions.
    //    b. Build system prompt (render template with variables).
    //    c. Build user prompt (transaction data + past decisions).
    //    d. Call AI provider.Review().
    //    e. Parse response into ReviewDecision structs.
    //    f. Submit each decision to review queue via service layer.
    //    g. Write audit log entries for each decision.
    //    h. Log usage to ai_usage_log.
    //    i. Check budget — stop if exceeded mid-run.
    // 8. Return summary (reviews processed, tokens used, errors).
}
```

### 6.3 Batching Strategy

Transactions are batched to balance context quality against context window limits:

| Setting | Value | Rationale |
|---------|-------|-----------|
| Batch size | 10 transactions | Fits comfortably in all supported models. Larger batches reduce API calls but increase risk of a single failure losing all results. |
| Max batches per run | `ceil(ai_max_reviews_per_sync / 10)` | Derived from the per-sync limit. |
| Context per transaction | ~200 tokens | ID, date, name, merchant, amount, account, current category. |
| Similar decisions | `ai_similar_decisions_count` total per batch | Shared across all transactions in the batch. |

Estimated token budget per batch:

| Component | Tokens (approx) |
|-----------|-----------------|
| System prompt (instructions + categories) | ~1,500 |
| 10 transactions | ~2,000 |
| 10 similar past decisions | ~1,000 |
| Response (10 decisions) | ~1,500 |
| **Total per batch** | **~6,000** |

This fits within even the smallest context windows (8K tokens). For models with larger contexts, the batch size could be increased, but 10 is a safe default.

### 6.4 Similar Decision Lookup

The runner queries the audit log for past review decisions that are similar to the current transactions. Similarity is based on merchant name and category:

```go
// internal/ai/runner.go

func (r *Runner) findSimilarDecisions(ctx context.Context, items []ReviewItem, limit int) ([]PastDecision, error) {
    // Query audit_log for entries where:
    //   entity_type = 'transaction'
    //   action = 'update'
    //   field = 'category_id' OR field = 'category_override'
    //   actor_type IN ('user', 'agent')
    // Then match against current items by merchant_name similarity.
    // Order by created_at DESC (most recent decisions first).
    // Limit to `limit` results.
}
```

The query uses a dynamic SQL approach (same pattern as `ListTransactions`):

```sql
SELECT al.entity_id, al.old_value, al.new_value, al.actor_type, al.actor_name,
       al.metadata, al.created_at,
       t.name AS transaction_name, t.merchant_name, t.amount
FROM audit_log al
JOIN transactions t ON t.id = al.entity_id
WHERE al.entity_type = 'transaction'
  AND al.action = 'update'
  AND al.field IN ('category_id', 'category_override')
  AND al.actor_type IN ('user', 'agent')
  AND t.merchant_name = ANY($1)  -- merchant names from current batch
ORDER BY al.created_at DESC
LIMIT $2;
```

If no exact merchant matches exist, the runner falls back to returning the most recent N decisions globally (to give the AI some sense of the family's categorization preferences).

### 6.5 Concurrency Guard

Only one agent run may execute at a time. The runner uses a `sync.Mutex` (same pattern as the per-connection lock in the sync engine):

```go
var runnerLock sync.Mutex

func (r *Runner) Run(ctx context.Context, trigger string, syncLogID *string) error {
    if !runnerLock.TryLock() {
        r.logger.Info("agent runner already in progress, skipping")
        return nil
    }
    defer runnerLock.Unlock()
    // ...
}
```

### 6.6 Error Handling

| Error | Behavior |
|-------|----------|
| AI API returns HTTP 401/403 (invalid key) | Log error, mark run as failed in `ai_usage_log`, do not retry. Set `ai_provider` status to errored in a flash message on next dashboard load. |
| AI API returns HTTP 429 (rate limited) | Log warning, stop processing remaining batches, record partial results. |
| AI API returns HTTP 5xx (server error) | Retry once with exponential backoff (2 seconds). If still failing, log error and stop. |
| AI response is unparseable | Log the raw response text in `ai_usage_log.error_message`, skip the batch, continue with next batch. |
| Parsed decision references unknown category slug | Skip that individual decision, log warning. Process remaining decisions in the batch. |
| Parsed decision references unknown transaction ID | Skip that individual decision, log warning. |
| Budget exceeded mid-run | Stop processing, log partial results. Remaining items stay in the review queue for the next run. |
| Context deadline exceeded | Stop processing. The runner respects the context timeout passed by the trigger. |

### 6.7 Review Decision Submission

Each parsed decision is submitted to the review queue via the service layer (Phase 24's `SubmitReview` method). The built-in agent uses a well-known actor:

```go
actor := service.Actor{
    Type: "agent",
    ID:   "builtin",
    Name: "Breadbox AI",
}
```

If `ai_auto_approve` is `true`, the decision is submitted with `status = "approved"`. Otherwise, it is submitted with `status = "suggested"` (pending human approval on the dashboard).

Each submission also writes an audit log entry and optionally a transaction comment with the AI's reasoning.

---

## 7. Cost Controls

### 7.1 Per-Sync Limit

`ai_max_reviews_per_sync` (default: 50) caps the number of review queue items processed per trigger event. This prevents a large sync (e.g., initial import of 1,000 transactions) from making hundreds of AI API calls.

When the limit is reached, remaining items stay in the queue for the next run.

### 7.2 Daily Budget Cap

`ai_daily_budget_cents` (default: 100 = $1.00) limits estimated daily AI spending. The runner checks the budget before each batch and stops if the estimated cost of the next batch would exceed the remaining budget.

**Cost estimation** uses per-model pricing tables hardcoded in the provider:

```go
// internal/ai/cost.go

// ModelPricing contains per-1M-token pricing in cents.
type ModelPricing struct {
    InputCentsPerMillion  float64
    OutputCentsPerMillion float64
}

var PricingTable = map[string]ModelPricing{
    // Anthropic
    "claude-sonnet-4-20250514":   {InputCentsPerMillion: 300, OutputCentsPerMillion: 1500},
    "claude-haiku-35-20241022":   {InputCentsPerMillion: 80,  OutputCentsPerMillion: 400},
    // OpenAI
    "gpt-4o":                     {InputCentsPerMillion: 250, OutputCentsPerMillion: 1000},
    "gpt-4o-mini":                {InputCentsPerMillion: 15,  OutputCentsPerMillion: 60},
}

// EstimateCostCents calculates the estimated cost in cents for the given usage.
func EstimateCostCents(model string, inputTokens, outputTokens int) float64 {
    pricing, ok := PricingTable[model]
    if !ok {
        return 0 // Unknown model — cannot estimate
    }
    return (float64(inputTokens) * pricing.InputCentsPerMillion / 1_000_000) +
           (float64(outputTokens) * pricing.OutputCentsPerMillion / 1_000_000)
}
```

**Daily usage query:**

```sql
SELECT COALESCE(SUM(
    (input_tokens * p.input_rate + output_tokens * p.output_rate) / 1000000.0
), 0) AS estimated_cost_cents
FROM ai_usage_log
WHERE DATE(created_at) = CURRENT_DATE
  AND error_message IS NULL;
```

In practice, the Go code computes cost from the `ai_usage_log` rows rather than doing the math in SQL, since the pricing table lives in Go.

### 7.3 Token Usage Tracking

Every AI API call records token counts in `ai_usage_log`. The dashboard displays:

- Today's usage (tokens + estimated cost).
- Last 7 days usage (daily breakdown).
- Last 30 days total.
- Average cost per review.

### 7.4 Budget Exceeded Behavior

When the daily budget is exceeded:

1. The runner logs `"daily budget exceeded, skipping AI review"` at INFO level.
2. No API calls are made.
3. Pending review items remain in the queue.
4. The dashboard shows a warning badge: "Daily budget reached — reviews paused until tomorrow."
5. The next day at midnight UTC, the budget resets (no explicit reset — the runner just queries today's usage).

---

## 8. Trigger Modes

### 8.1 Automatic (Post-Sync Hook)

When `ai_auto_review` is `true`, the sync engine triggers the agent runner after a successful sync. The hook fires after `Engine.Sync()` completes, not during the DB transaction.

**Integration point in `internal/sync/engine.go`:**

```go
// At the end of Engine.Sync(), after the sync log is updated:
if syncErr == nil && e.agentRunner != nil {
    // Fire and forget — do not block the sync response.
    go func() {
        runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
        defer cancel()
        if err := e.agentRunner.Run(runCtx, "sync", &syncLogIDStr); err != nil {
            e.logger.Error("post-sync agent review failed", "error", err)
        }
    }()
}
```

The sync engine receives the runner via a new field `agentRunner *ai.Runner` on the `Engine` struct, set during `App` initialization. If the runner is nil (AI not configured), the hook is a no-op.

The goroutine approach ensures the sync API response is not delayed by AI processing. The concurrency guard (Section 6.5) prevents overlapping runs if multiple syncs complete simultaneously.

### 8.2 Manual ("Review Now")

The dashboard provides a "Review Now" button that triggers an immediate agent run. This is a POST endpoint:

```
POST /admin/api/ai/review-now
```

The handler starts the runner in a background goroutine and returns immediately with a flash message: "AI review started. Results will appear in the review queue."

The admin can also trigger review for a specific connection's transactions by passing a `connection_id` parameter, which filters the review queue to items from that connection's accounts.

### 8.3 Scheduled (Cron)

When `ai_schedule_enabled` is `true`, a cron job runs the agent runner at the configured interval (`ai_schedule_interval_minutes`, minimum 15 minutes). This is useful for setups where syncs happen via webhooks (push-based) and the admin wants periodic AI review regardless of sync timing.

**Registration in `internal/sync/scheduler.go`:**

The agent runner cron job is registered alongside the sync cron job in the scheduler. It is a separate cron entry with its own interval.

```go
func (s *Scheduler) StartAgentRunner(runner *ai.Runner, intervalMinutes int) {
    spec := fmt.Sprintf("@every %dm", intervalMinutes)
    _, err := s.cron.AddFunc(spec, func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
        defer cancel()
        if err := runner.Run(ctx, "scheduled", nil); err != nil {
            s.logger.Error("scheduled agent review failed", "error", err)
        }
    })
    if err != nil {
        s.logger.Error("failed to add agent runner cron job", "error", err)
    }
}
```

### 8.4 Trigger Priority

If multiple triggers fire simultaneously (e.g., sync completes while a scheduled run is starting), the concurrency guard ensures only one runs. The second trigger is silently skipped — the items it would have reviewed are still in the queue and will be picked up by the next run.

---

## 9. Dashboard UI

### 9.1 Page: AI Settings (`/admin/ai`)

A new top-level nav item "AI Review" is added to the sidebar in the System section, using the `sparkles` Lucide icon.

**File:** `internal/templates/pages/ai_settings.html`

The page has five card sections.

### 9.2 Card 1: AI Provider

Configuration for the active AI provider.

**Layout:**

```
┌──────────────────────────────────────────────────────┐
│ AI Provider                                           │
│                                                       │
│ Provider:  ○ None (disabled)                         │
│            ○ Anthropic (Claude)                      │
│            ○ OpenAI (GPT)                            │
│                                                       │
│ API Key:   [••••••••••••••sk-abc123] [Test] [Clear]  │
│            ✓ Key validated successfully               │
│                                                       │
│ Model:     [claude-sonnet-4-20250514    ▼]           │
│            Leave blank for provider default           │
│                                                       │
│ [Save Provider Settings]                              │
└──────────────────────────────────────────────────────┘
```

**Behavior:**

- Selecting "None" disables the built-in agent entirely.
- The API key field is a password input. When a key is already stored, it shows a masked version (last 6 characters). Submitting an empty field preserves the existing key (same pattern as Plaid secret).
- The "Test" button calls `POST /admin/api/ai/test-provider` which runs `ValidateKey()` on the configured provider and returns success/failure.
- The "Clear" button removes the stored key.
- Model dropdown shows common models for the selected provider. A custom text input is available for models not in the list.

### 9.3 Card 2: Review Prompt

Markdown editor for the review instructions template.

**Layout:**

```
┌──────────────────────────────────────────────────────┐
│ Review Prompt                                         │
│                                                       │
│ ┌──────────────────────────────────────────────────┐ │
│ │ You are a financial transaction reviewer...       │ │
│ │                                                    │ │
│ │ (textarea, monospace, ~20 rows)                   │ │
│ │                                                    │ │
│ └──────────────────────────────────────────────────┘ │
│                                                       │
│ Available variables: {{.CategoryList}},               │
│ {{.UserNames}}, {{.AccountSummary}},                 │
│ {{.TransactionCount}}, {{.DateRange}}                │
│                                                       │
│ 1,234 / 10,000 characters                            │
│                                                       │
│ [Reset to Default]  [Save Prompt]                    │
└──────────────────────────────────────────────────────┘
```

**Behavior:**

- Empty textarea means use the built-in default prompt.
- Character count updates live via Alpine.js.
- "Reset to Default" loads `DefaultReviewPrompt` into the textarea (with confirmation if modified).
- Max 10,000 characters, validated server-side.

### 9.4 Card 3: Cost Controls

```
┌──────────────────────────────────────────────────────┐
│ Cost Controls                                         │
│                                                       │
│ Max reviews per sync:       [50          ]            │
│ Daily budget:               [$1.00       ]            │
│ Max output tokens per call: [4096        ]            │
│ Similar past decisions:     [10          ]            │
│                                                       │
│ [Save Controls]                                       │
└──────────────────────────────────────────────────────┘
```

### 9.5 Card 4: Trigger Settings

```
┌──────────────────────────────────────────────────────┐
│ Trigger Settings                                      │
│                                                       │
│ ☐ Auto-review after sync                             │
│   Run AI review automatically when new transactions   │
│   are synced from bank connections.                   │
│                                                       │
│ ☐ Auto-approve decisions                             │
│   ⚠ Caution: When enabled, the AI's category         │
│   decisions are applied immediately without human     │
│   review.                                             │
│                                                       │
│ ☐ Scheduled review                                   │
│   Run every [60] minutes                             │
│                                                       │
│ [Review Now]  ← manual trigger button                │
│                                                       │
│ [Save Trigger Settings]                               │
└──────────────────────────────────────────────────────┘
```

**"Review Now" button:** Uses Alpine.js + fetch to POST to `/admin/api/ai/review-now` and shows a toast notification. The button is disabled while a run is in progress (checked via a lightweight status endpoint).

### 9.6 Card 5: Usage Stats

```
┌──────────────────────────────────────────────────────┐
│ Usage Stats                                           │
│                                                       │
│ Today:        142 reviews │ 45,230 tokens │ ~$0.23   │
│ This week:    891 reviews │ 287K tokens   │ ~$1.47   │
│ This month:  2,340 reviews│ 754K tokens   │ ~$3.89   │
│                                                       │
│ Daily Budget: ████████░░ $0.23 / $1.00               │
│                                                       │
│ Last 7 Days:                                          │
│ ┌──────────────────────────────────────┐              │
│ │ Mon  █████  $0.45                    │              │
│ │ Tue  ███    $0.31                    │              │
│ │ Wed  ██████ $0.52                    │              │
│ │ Thu  ████   $0.38                    │              │
│ │ Fri  ██     $0.23                    │              │
│ │ Sat  ░      $0.00                    │              │
│ │ Sun  ░      $0.00 (today)            │              │
│ └──────────────────────────────────────┘              │
│                                                       │
│ Avg cost per review: $0.0017                          │
└──────────────────────────────────────────────────────┘
```

The bar chart is rendered with inline CSS (DaisyUI progress bars or simple divs with percentage widths). No Chart.js dependency — that comes in Phase 27.

### 9.7 Handler File

**File:** `internal/admin/ai.go` (new)

```go
func AISettingsGetHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc
func AISaveProviderHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc
func AISavePromptHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc
func AISaveControlsHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc
func AISaveTriggersHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc
func AITestProviderHandler(a *app.App) http.HandlerFunc
func AIReviewNowHandler(a *app.App) http.HandlerFunc
```

### 9.8 Routes

```go
// Added to internal/admin/router.go within the authenticated admin routes
r.Route("/ai", func(r chi.Router) {
    r.Get("/", AISettingsGetHandler(a, sm, tr))
    r.Post("/provider", AISaveProviderHandler(a, sm))
    r.Post("/prompt", AISavePromptHandler(a, sm))
    r.Post("/controls", AISaveControlsHandler(a, sm))
    r.Post("/triggers", AISaveTriggersHandler(a, sm))
})

// Admin API routes (JSON responses)
r.Post("/api/ai/test-provider", AITestProviderHandler(a))
r.Post("/api/ai/review-now", AIReviewNowHandler(a))
```

### 9.9 Navigation Update

```html
<!-- internal/templates/partials/nav.html — add in System section -->
<li><a href="/admin/ai"{{if eq .CurrentPage "ai"}} class="menu-active"{{end}}>
  <i data-lucide="sparkles" class="w-4 h-4"></i> AI Review</a>
</li>
```

---

## 10. Service Layer

### 10.1 New Package: `internal/ai/`

| File | Contents |
|------|----------|
| `internal/ai/provider.go` | `Provider` interface, `ReviewRequest`, `ReviewResponse` types |
| `internal/ai/runner.go` | `Runner` struct with `Run()` method |
| `internal/ai/prompt.go` | `DefaultReviewPrompt`, `BuildSystemPrompt()`, `BuildUserPrompt()`, template rendering |
| `internal/ai/parse.go` | `ParseReviewResponse()`, `ReviewDecision` type |
| `internal/ai/cost.go` | `ModelPricing`, `PricingTable`, `EstimateCostCents()` |
| `internal/ai/anthropic/provider.go` | Anthropic Claude API client |
| `internal/ai/openai/provider.go` | OpenAI GPT API client |

### 10.2 New Service Methods

```go
// internal/service/ai_config.go (new file)

// AIConfig holds the built-in agent configuration.
type AIConfig struct {
    Provider              string  `json:"provider"`               // "anthropic", "openai", or ""
    HasAPIKey             bool    `json:"has_api_key"`             // true if encrypted key is stored
    Model                 string  `json:"model"`                  // override model or ""
    ReviewPrompt          string  `json:"review_prompt"`           // custom prompt or ""
    AutoReview            bool    `json:"auto_review"`
    AutoApprove           bool    `json:"auto_approve"`
    MaxReviewsPerSync     int     `json:"max_reviews_per_sync"`
    DailyBudgetCents      int     `json:"daily_budget_cents"`
    MaxTokens             int     `json:"max_tokens"`
    SimilarDecisionsCount int     `json:"similar_decisions_count"`
    ScheduleEnabled       bool    `json:"schedule_enabled"`
    ScheduleIntervalMin   int     `json:"schedule_interval_minutes"`
}

// GetAIConfig loads AI agent configuration from app_config.
func (s *Service) GetAIConfig(ctx context.Context) (*AIConfig, error)

// SaveAIProvider saves the AI provider selection and encrypted API key.
func (s *Service) SaveAIProvider(ctx context.Context, provider string, apiKey string, model string, encryptionKey []byte) error

// SaveAIPrompt saves the custom review prompt.
func (s *Service) SaveAIPrompt(ctx context.Context, prompt string) error

// SaveAIControls saves cost control settings.
func (s *Service) SaveAIControls(ctx context.Context, maxReviews int, dailyBudgetCents int, maxTokens int, similarDecisions int) error

// SaveAITriggers saves trigger mode settings.
func (s *Service) SaveAITriggers(ctx context.Context, autoReview bool, autoApprove bool, scheduleEnabled bool, scheduleIntervalMin int) error

// GetAIUsageStats returns usage statistics for display on the dashboard.
func (s *Service) GetAIUsageStats(ctx context.Context) (*AIUsageStats, error)

// GetDailyAIUsageCents returns the estimated cost in cents for today.
func (s *Service) GetDailyAIUsageCents(ctx context.Context) (float64, error)

// LogAIUsage records an AI API call in the usage log.
func (s *Service) LogAIUsage(ctx context.Context, entry AIUsageLogEntry) error
```

### 10.3 New Types

```go
// internal/service/types.go — additions

type AIUsageStats struct {
    TodayReviews      int     `json:"today_reviews"`
    TodayTokens       int     `json:"today_tokens"`
    TodayCostCents    float64 `json:"today_cost_cents"`
    WeekReviews       int     `json:"week_reviews"`
    WeekTokens        int     `json:"week_tokens"`
    WeekCostCents     float64 `json:"week_cost_cents"`
    MonthReviews      int     `json:"month_reviews"`
    MonthTokens       int     `json:"month_tokens"`
    MonthCostCents    float64 `json:"month_cost_cents"`
    DailyBudgetCents  int     `json:"daily_budget_cents"`
    AvgCostPerReview  float64 `json:"avg_cost_per_review"`
    DailyBreakdown    []DailyUsage `json:"daily_breakdown"`
}

type DailyUsage struct {
    Date       string  `json:"date"`
    Reviews    int     `json:"reviews"`
    Tokens     int     `json:"tokens"`
    CostCents  float64 `json:"cost_cents"`
}

type AIUsageLogEntry struct {
    Provider     string
    Model        string
    InputTokens  int
    OutputTokens int
    ReviewCount  int
    Trigger      string
    SyncLogID    *string
    ErrorMessage *string
}
```

### 10.4 Sync Engine Changes

The `Engine` struct gains an `agentRunner` field:

```go
// internal/sync/engine.go

type Engine struct {
    db          *db.Queries
    pool        *pgxpool.Pool
    providers   map[string]provider.Provider
    logger      *slog.Logger
    locks       gosync.Map
    agentRunner *ai.Runner // nil if AI not configured
}

func (e *Engine) SetAgentRunner(runner *ai.Runner) {
    e.agentRunner = runner
}
```

The `SetAgentRunner` method is called from `App` initialization after both the sync engine and service are constructed. This avoids a circular dependency (sync engine -> runner -> service -> sync engine) — the runner uses the service layer, not the sync engine directly.

### 10.5 App Changes

The `App` struct gains an `AgentRunner` field:

```go
// internal/app/app.go

type App struct {
    // ... existing fields ...
    AgentRunner *ai.Runner
}
```

In `App.New()`, after the service and sync engine are created:

```go
agentRunner := ai.NewRunner(svc, logger)
syncEngine.SetAgentRunner(agentRunner)
app.AgentRunner = agentRunner
```

The runner is always created but only acts when AI is configured (it checks `ai_provider` in `app_config` at runtime).

### 10.6 sqlc Queries

New query file: `internal/db/queries/ai_usage.sql`

```sql
-- name: InsertAIUsageLog :one
INSERT INTO ai_usage_log (provider, model, input_tokens, output_tokens, review_count, trigger, sync_log_id, error_message)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetDailyAIUsage :many
SELECT DATE(created_at) AS usage_date,
       SUM(input_tokens) AS total_input_tokens,
       SUM(output_tokens) AS total_output_tokens,
       SUM(review_count) AS total_reviews,
       COUNT(*) AS api_calls
FROM ai_usage_log
WHERE created_at >= $1
  AND error_message IS NULL
GROUP BY DATE(created_at)
ORDER BY DATE(created_at) DESC;

-- name: GetTodayAIUsage :one
SELECT COALESCE(SUM(input_tokens), 0) AS total_input_tokens,
       COALESCE(SUM(output_tokens), 0) AS total_output_tokens,
       COALESCE(SUM(review_count), 0) AS total_reviews
FROM ai_usage_log
WHERE DATE(created_at) = CURRENT_DATE
  AND error_message IS NULL;
```

---

## 11. Implementation Tasks

Ordered by dependency. Each task is a self-contained unit of work.

### Task 1: Database Migration

**File:** `internal/db/migrations/00XXX_ai_usage_log.sql`

- Create `ai_usage_log` table with indexes.
- Run migration, verify table exists.

### Task 2: sqlc Queries

**File:** `internal/db/queries/ai_usage.sql`

- Add `InsertAIUsageLog`, `GetDailyAIUsage`, `GetTodayAIUsage` queries.
- Run `make sqlc` to generate Go code.

### Task 3: AI Provider Interface and Cost Utilities

**Files:** `internal/ai/provider.go`, `internal/ai/cost.go`

- Define `Provider` interface, `ReviewRequest`, `ReviewResponse` types.
- Implement `ModelPricing`, `PricingTable`, `EstimateCostCents()`.

### Task 4: Anthropic Provider

**File:** `internal/ai/anthropic/provider.go`

- Implement `Provider` interface for Anthropic Messages API.
- `Review()`: POST to `/v1/messages`, parse response.
- `ValidateKey()`: minimal API call to verify key.
- Unit tests with HTTP mock.

### Task 5: OpenAI Provider

**File:** `internal/ai/openai/provider.go`

- Implement `Provider` interface for OpenAI Chat Completions API.
- `Review()`: POST to `/v1/chat/completions`, parse response.
- `ValidateKey()`: GET `/v1/models` to verify key.
- Unit tests with HTTP mock.

### Task 6: Prompt System

**Files:** `internal/ai/prompt.go`, `internal/ai/parse.go`

- `DefaultReviewPrompt` constant.
- `BuildSystemPrompt()` renders template with variables.
- `BuildUserPrompt()` formats transactions and past decisions.
- `ParseReviewResponse()` with lenient JSON extraction.
- `ReviewDecision` type.
- Unit tests for template rendering and response parsing.

### Task 7: Agent Runner

**File:** `internal/ai/runner.go`

- `Runner` struct with `Run()` method.
- Batching logic (10 items per batch).
- Similar decision lookup via audit log.
- Budget checking before each batch.
- Concurrency guard with `sync.Mutex`.
- AI provider instantiation (decrypt key, create provider).
- Review decision submission via service layer.
- Usage logging.

### Task 8: Service Layer — AI Config

**File:** `internal/service/ai_config.go` (new)

- `AIConfig` type.
- `GetAIConfig()`, `SaveAIProvider()`, `SaveAIPrompt()`, `SaveAIControls()`, `SaveAITriggers()`.
- `GetAIUsageStats()`, `GetDailyAIUsageCents()`, `LogAIUsage()`.
- API key encryption/decryption using `internal/crypto`.

### Task 9: Service Layer — AI Types

**File:** `internal/service/types.go`

- Add `AIUsageStats`, `DailyUsage`, `AIUsageLogEntry`, `AIConfig` types.

### Task 10: Sync Engine Integration

**File:** `internal/sync/engine.go`

- Add `agentRunner` field and `SetAgentRunner()` method.
- Add post-sync hook at the end of `Sync()` that fires the runner in a goroutine when `ai_auto_review` is enabled.

### Task 11: App Initialization

**Files:** `internal/app/app.go`

- Add `AgentRunner` field to `App` struct.
- Create `ai.Runner` in `App.New()` and wire it to the sync engine.

### Task 12: Scheduler Integration

**File:** `internal/sync/scheduler.go`

- Add `StartAgentRunner()` method for cron-based agent execution.
- Read `ai_schedule_enabled` and `ai_schedule_interval_minutes` from config.

### Task 13: Admin Handlers

**File:** `internal/admin/ai.go` (new)

- `AISettingsGetHandler`: loads AI config, usage stats, renders page.
- `AISaveProviderHandler`: validates + encrypts API key, saves provider settings.
- `AISavePromptHandler`: saves custom prompt (max 10,000 chars).
- `AISaveControlsHandler`: saves cost control settings.
- `AISaveTriggersHandler`: saves trigger mode settings.
- `AITestProviderHandler`: instantiates provider, calls `ValidateKey()`.
- `AIReviewNowHandler`: fires runner in background goroutine.

### Task 14: Admin Template

**File:** `internal/templates/pages/ai_settings.html` (new)

- Five-card layout: Provider, Prompt, Controls, Triggers, Usage Stats.
- Alpine.js for character count, test button, review-now button.
- DaisyUI components: cards, form controls, badges, progress bars.

### Task 15: Template Registration and Routing

**Files:** `internal/admin/templates.go`, `internal/admin/router.go`

- Add `"pages/ai_settings.html"` to `basePages` slice.
- Register `/ai` route group with GET + POST handlers.
- Register API routes for test-provider and review-now.

### Task 16: Navigation Update

**File:** `internal/templates/partials/nav.html`

- Add "AI Review" nav item with `sparkles` icon in the System section.

### Task 17: Update Docs

**Files:** `docs/data-model.md`, `CLAUDE.md`, `docs/ROADMAP.md`

- Add `ai_usage_log` table to data model spec.
- Add new `app_config` keys to the known configuration keys table.
- Add Phase 26 design decisions to `CLAUDE.md`.
- Mark Phase 26 tasks in `ROADMAP.md`.

---

## 12. Dependencies

### Depends On

| Phase | Dependency | Notes |
|-------|-----------|-------|
| Phase 24 | `review_queue` table and review API | The built-in agent reads pending items from the review queue and submits decisions through the review service layer. Without Phase 24, there is nothing to review. |
| Phase 25 | Review instructions config | Phase 25 establishes the review instructions system (configurable markdown prompts). Phase 26 extends this with template variables and AI-specific formatting. The `ai_review_prompt` in Phase 26 builds on the same concept. |
| Phase 21 | `audit_log` table | The similar-decision lookup queries the audit log for past categorization decisions. Without Phase 21, the agent has no historical context (it still works, just without the learning-from-history feature). |

### Depended On By

No phases currently depend on Phase 26.

### Independent Of

- Phase 22 (Agent APIs) — the built-in agent uses the service layer directly, not MCP or REST.
- Phase 23 (MCP Permissions) — the built-in agent bypasses MCP entirely; it has its own permission model (the AI config settings).
- Phase 27+ — no dependencies.

### Graceful Degradation

- If Phase 21 (audit log) is not complete, the runner skips the similar-decision lookup and sends an empty "past decisions" section in the prompt. The agent still functions, just without historical context.
- If no categories are configured (Phase 20), the agent cannot make categorization decisions. The runner detects an empty category list and skips the run with a log message.
