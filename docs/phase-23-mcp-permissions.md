# Phase 23: MCP Permissions & Custom Instructions

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## 1. Overview

Phase 23 gives admins control over what MCP-connected agents can do and how they behave. It introduces a global read-only/read-write mode toggle, per-tool enable/disable switches, custom server instructions (the "system prompt" agents see), reusable instruction templates, and API key scoping so that individual keys can be restricted to read-only access. All configuration is managed through a new `/admin/mcp` dashboard page.

---

## 2. Goals

1. **Least-privilege by default.** New installations start in read-only mode. Admins explicitly opt in to write operations (e.g., `trigger_sync`).
2. **Granular tool control.** Admins can disable individual MCP tools without code changes, immediately affecting all connected agents.
3. **Custom agent behavior.** Admins write markdown instructions that are injected into the MCP server's `Instructions` field, controlling how agents interpret and present financial data.
4. **Instruction templates.** Pre-built templates for common task types (spend review, monthly analysis, reporting) that admins can load, customize, and save.
5. **Scoped API keys.** Each API key has a scope (`full_access` or `read_only`). Read-only keys cannot invoke write tools or write REST endpoints, even if the global mode is read-write.

---

## 3. Data Model Changes

### 3.1 `api_keys` table — add `scope` column

```sql
-- Migration: 00017_api_key_scope.sql
-- +goose Up
ALTER TABLE api_keys ADD COLUMN scope TEXT NOT NULL DEFAULT 'full_access';

-- +goose Down
ALTER TABLE api_keys DROP COLUMN scope;
```

- **Column:** `scope TEXT NOT NULL DEFAULT 'full_access'`
- **Valid values:** `full_access`, `read_only`
- **Default:** `full_access` — existing keys retain full access after migration. Admins can downgrade individual keys via the dashboard.
- **Checked at:** middleware layer (REST) and MCP tool invocation time.

### 3.2 `app_config` keys — MCP settings

No new tables. All MCP configuration is stored in the existing `app_config` key-value table.

| Key | Type | Default | Description |
|---|---|---|---|
| `mcp_mode` | `read_only` \| `read_write` | `read_only` | Global MCP mode. Controls whether write tools are available. |
| `mcp_disabled_tools` | JSON string array | `[]` | List of tool names to hide from all agents, e.g. `["trigger_sync"]`. |
| `mcp_custom_instructions` | Text (markdown) | `""` (empty) | Custom instructions appended after the built-in instructions. |
| `mcp_instruction_template` | Text (slug) | `""` (empty) | Which built-in template is currently active (for UI state tracking). Empty means custom/none. |

#### Tool classification

Every MCP tool is classified as either **read** or **write**:

| Tool Name | Classification | Description |
|---|---|---|
| `list_accounts` | read | List bank accounts |
| `query_transactions` | read | Query transactions with filters |
| `count_transactions` | read | Count matching transactions |
| `list_categories` | read | List category taxonomy |
| `list_users` | read | List family members |
| `get_sync_status` | read | Check connection/sync status |
| `trigger_sync` | write | Trigger a manual bank sync |

The `breadbox://overview` resource is always available (read-only by nature).

When `mcp_mode` is `read_only`, all `write` tools are suppressed (not registered on the server). When a tool is in `mcp_disabled_tools`, it is suppressed regardless of mode. The two filters stack: a tool must pass both the mode filter AND the disabled-tools filter to be registered.

### 3.3 Updated `api_keys` in `data-model.md`

The `api_keys` table definition should be updated to include:

```
| scope         | TEXT        | NOT NULL | DEFAULT 'full_access' | 'full_access' or 'read_only'. Controls what operations this key can perform. |
```

### 3.4 Updated known configuration keys

Add to the "Known Configuration Keys" table in `data-model.md`:

| Key | Example Value | Description |
|---|---|---|
| `mcp_mode` | `read_only` | Global MCP mode: `read_only` or `read_write`. |
| `mcp_disabled_tools` | `["trigger_sync"]` | JSON array of disabled tool names. |
| `mcp_custom_instructions` | `(markdown text)` | Custom MCP server instructions. |
| `mcp_instruction_template` | `spend_review` | Active instruction template slug (UI state). |

---

## 4. MCP Permission Model

### 4.1 Permission check flow

When the MCP server is created (or when a new HTTP session starts), the following logic determines which tools are registered:

```
For each tool in the full tool registry:
  1. Is the tool in mcp_disabled_tools?        → skip (not registered)
  2. Is mcp_mode == "read_only" AND tool is write? → skip (not registered)
  3. Is the API key scope == "read_only" AND tool is write? → skip (not registered)
  4. Otherwise → register the tool
```

Steps 1-2 are applied at **server construction time** (they affect all sessions). Step 3 is applied at **tool invocation time** because the API key is only known per-request from the middleware.

### 4.2 Architecture: per-request MCP server

The current `NewHTTPHandler` already uses a factory function:

```go
// internal/mcp/server.go
func NewHTTPHandler(s *MCPServer) http.Handler {
    return mcpsdk.NewStreamableHTTPHandler(
        func(r *http.Request) *mcpsdk.Server {
            return s.server
        },
        nil,
    )
}
```

Phase 23 changes this so the factory can return a **filtered server** based on the requesting API key's scope. The approach:

1. **Shared base server** (`MCPServer`) holds all tool handler functions and the service layer reference.
2. **`MCPServer.BuildServer(cfg MCPConfig)`** creates a `*mcpsdk.Server` with only the permitted tools registered, using the current `mcp_mode`, `mcp_disabled_tools`, custom instructions, and the API key scope from the request context.
3. **The HTTP handler factory** calls `BuildServer` on each request, reading the API key scope from context (set by the existing `APIKeyAuth` middleware).

```go
// internal/mcp/server.go

// MCPConfig holds runtime MCP permissions loaded from app_config + API key.
type MCPConfig struct {
    Mode              string   // "read_only" or "read_write"
    DisabledTools     []string // tool names to suppress
    CustomInstructions string  // markdown appended to built-in instructions
    APIKeyScope       string   // "full_access" or "read_only" — from request context
}

// ToolDef holds a tool definition and its handler, plus metadata.
type ToolDef struct {
    Tool        *mcpsdk.Tool
    Handler     any // the typed handler function
    Classification string // "read" or "write"
}

// MCPServer holds the full tool registry and service layer.
type MCPServer struct {
    svc         *service.Service
    version     string
    allTools    []ToolDef
    resources   []ResourceDef // similar pattern for resources
}

// BuildServer creates a filtered *mcpsdk.Server for the given config.
func (s *MCPServer) BuildServer(cfg MCPConfig) *mcpsdk.Server {
    instructions := builtInInstructions
    if cfg.CustomInstructions != "" {
        instructions += "\n\nCUSTOM INSTRUCTIONS:\n" + cfg.CustomInstructions
    }

    server := mcpsdk.NewServer(
        &mcpsdk.Implementation{Name: "breadbox", Version: s.version},
        &mcpsdk.ServerOptions{Instructions: instructions},
    )

    disabledSet := toSet(cfg.DisabledTools)
    for _, td := range s.allTools {
        if disabledSet[td.Tool.Name] {
            continue
        }
        if td.Classification == "write" && (cfg.Mode == "read_only" || cfg.APIKeyScope == "read_only") {
            continue
        }
        // Register the tool (type-specific AddTool calls)
        s.registerTool(server, td)
    }

    s.registerResources(server)
    return server
}
```

### 4.3 Write-tool invocation guard

As an additional safety net, even if a write tool is somehow registered, the handler itself checks permissions before executing. This defends against race conditions where config changes between server creation and tool invocation.

```go
func (s *MCPServer) handleTriggerSync(ctx context.Context, req *mcpsdk.CallToolRequest, input triggerSyncInput) (*mcpsdk.CallToolResult, any, error) {
    // Permission guard — belt-and-suspenders
    if err := s.checkWritePermission(ctx); err != nil {
        return errorResult(err), nil, nil
    }
    // ... existing logic
}
```

The `checkWritePermission` function reads the API key scope from context and the current `mcp_mode` from the config/service layer.

### 4.4 Context propagation for API key scope

The existing `APIKeyAuth` middleware in `internal/middleware/apikey.go` already calls `svc.ValidateAPIKey()` which returns `*db.ApiKey`. Phase 23 stores the API key record (including scope) in the request context so downstream handlers (REST and MCP) can read it.

```go
// internal/middleware/context.go (new file)
type contextKey string
const apiKeyContextKey contextKey = "api_key"

func SetAPIKey(ctx context.Context, key *db.ApiKey) context.Context {
    return context.WithValue(ctx, apiKeyContextKey, key)
}

func GetAPIKey(ctx context.Context) *db.ApiKey {
    key, _ := ctx.Value(apiKeyContextKey).(*db.ApiKey)
    return key
}
```

Updated middleware:

```go
// internal/middleware/apikey.go
func APIKeyAuth(svc *service.Service) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            key := r.Header.Get("X-API-Key")
            // ... validation ...
            apiKey, err := svc.ValidateAPIKey(r.Context(), key)
            // ... error handling ...
            // Store in context for downstream use
            ctx := SetAPIKey(r.Context(), apiKey)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## 5. Custom Instructions

### 5.1 Storage

Custom instructions are stored as plain markdown text in `app_config` under the key `mcp_custom_instructions`. Maximum length: 10,000 characters (validated in the handler, not at DB level).

### 5.2 Injection

The built-in instructions (the current hardcoded string in `server.go`) remain as a constant. Custom instructions are appended with a clear separator:

```
<built-in instructions>

CUSTOM INSTRUCTIONS:
<admin-written markdown>
```

If `mcp_custom_instructions` is empty, only the built-in instructions are used (current behavior).

### 5.3 Instruction templates

Templates are hardcoded Go constants (not DB-stored). They provide starting points that admins can load into the editor, customize, and save. The `mcp_instruction_template` config key tracks which template was last loaded (for UI state only — the actual saved text is in `mcp_custom_instructions`).

```go
// internal/mcp/templates.go (new file)

// InstructionTemplate represents a pre-built instruction set.
type InstructionTemplate struct {
    Slug        string `json:"slug"`
    Name        string `json:"name"`
    Description string `json:"description"`
    Content     string `json:"content"`
}

var InstructionTemplates = []InstructionTemplate{
    {
        Slug:        "spend_review",
        Name:        "Spending Review",
        Description: "Guide agents to analyze spending patterns, flag anomalies, and suggest budgets.",
        Content: `You are reviewing household spending for a family.

APPROACH:
1. Start by reading the breadbox://overview resource to understand the data scope.
2. Query the last 30 days of transactions to establish a baseline.
3. Group spending by category and identify the top 5 categories.
4. Flag any individual transactions over $500 or unusual patterns.
5. Compare this month's spending to the previous month if data is available.

OUTPUT FORMAT:
- Use clear section headers
- Show amounts in the local currency with 2 decimal places
- Always note which date range you analyzed
- Highlight concerning patterns with specific transaction details`,
    },
    {
        Slug:        "monthly_analysis",
        Name:        "Monthly Analysis",
        Description: "Structured monthly financial summary with income, expenses, and trends.",
        Content: `You are preparing a monthly financial summary for a family.

ANALYSIS STEPS:
1. Determine the current month's date range (1st to last day).
2. Query all transactions for the month.
3. Separate income (negative amounts = money in) from expenses (positive amounts = money out).
4. Calculate net cash flow.
5. Break down expenses by category.
6. List the top 10 largest individual expenses.

REPORT STRUCTURE:
## Monthly Summary (Month Year)
- Total Income: $X
- Total Expenses: $X
- Net Cash Flow: $X

## Expense Breakdown by Category
(table of categories with amounts and percentages)

## Top 10 Expenses
(list with date, merchant, amount, category)

## Notable Observations
(any anomalies, trends, or recommendations)`,
    },
    {
        Slug:        "reporting",
        Name:        "Data Export & Reporting",
        Description: "Instruct agents to produce structured, exportable data summaries.",
        Content: `You are a financial data assistant. When asked for reports, follow these conventions:

DATA ACCURACY:
- Always verify data freshness by checking get_sync_status first.
- Never estimate or approximate — only report actual transaction data.
- If data seems incomplete, note the gap and suggest a sync.

FORMATTING:
- Use markdown tables for tabular data.
- Round amounts to 2 decimal places.
- Always include the currency code.
- Date ranges should be explicit (start and end dates).

LIMITATIONS:
- Do not make financial advice or predictions.
- Do not compare to external benchmarks.
- If asked about future spending, explain you can only report historical data.`,
    },
}
```

### 5.4 Template API

The admin page needs to load template content into the editor. This is handled client-side — templates are embedded as JSON in the page template data, and Alpine.js loads them into the textarea without a round-trip.

---

## 6. API Key Scoping

### 6.1 Scope values

| Scope | REST API | MCP Tools |
|---|---|---|
| `full_access` | All endpoints | All enabled tools (subject to global mode) |
| `read_only` | GET endpoints only | Read tools only (write tools hidden/blocked) |

### 6.2 REST API enforcement

The `APIKeyAuth` middleware stores the API key in context. A new middleware `RequireWriteScope` is added for write endpoints:

```go
// internal/middleware/scope.go (new file)

// RequireWriteScope rejects requests from read_only API keys.
func RequireWriteScope() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            apiKey := GetAPIKey(r.Context())
            if apiKey != nil && apiKey.Scope == "read_only" {
                WriteError(w, http.StatusForbidden, "INSUFFICIENT_SCOPE",
                    "This API key has read-only access and cannot perform write operations")
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Applied in `internal/api/router.go`:

```go
r.Route("/api/v1", func(r chi.Router) {
    r.Use(mw.APIKeyAuth(svc))
    // Read endpoints — all keys
    r.Get("/accounts", ListAccountsHandler(svc))
    r.Get("/accounts/{id}", GetAccountHandler(svc))
    r.Get("/transactions", ListTransactionsHandler(svc))
    // ...

    // Write endpoints — full_access keys only
    r.Group(func(r chi.Router) {
        r.Use(mw.RequireWriteScope())
        r.Post("/sync", TriggerSyncHandler(svc))
    })
})
```

### 6.3 MCP enforcement

For the MCP server, write tools are excluded from registration when the API key scope is `read_only` (see Section 4.2). Additionally, the handler-level guard in Section 4.3 provides defense-in-depth.

### 6.4 Key creation with scope

The `CreateAPIKey` service method and admin UI are updated to accept a scope parameter.

```go
// internal/service/apikeys.go
func (s *Service) CreateAPIKey(ctx context.Context, name string, scope string) (*CreateAPIKeyResult, error) {
    if scope == "" {
        scope = "full_access"
    }
    if scope != "full_access" && scope != "read_only" {
        return nil, fmt.Errorf("invalid scope: %s", scope)
    }
    // ... existing key generation ...
    apiKey, err := s.Queries.CreateApiKey(ctx, db.CreateApiKeyParams{
        Name:      name,
        KeyHash:   keyHash,
        KeyPrefix: keyPrefix,
        Scope:     scope,
    })
    // ...
}
```

### 6.5 Migration for existing keys

The migration in Section 3.1 sets `DEFAULT 'full_access'`, so all existing keys retain their current permissions. No manual migration action needed.

### 6.6 sqlc query updates

```sql
-- internal/db/queries/api_keys.sql

-- name: CreateApiKey :one
INSERT INTO api_keys (name, key_hash, key_prefix, scope) VALUES ($1, $2, $3, $4) RETURNING *;

-- All other queries remain unchanged — they SELECT * which automatically includes scope.
```

---

## 7. Admin Dashboard — `/admin/mcp`

### 7.1 Page structure

A new top-level nav item "MCP" is added to the sidebar (between "API Keys" and "Sync Logs" in the System section), using the `bot` Lucide icon.

The page has four card sections:

#### Card 1: Global Mode

- Radio buttons: "Read Only" (default) / "Read & Write"
- Description text explaining what each mode does
- Save button submits `POST /admin/mcp/mode`

#### Card 2: Tool Access

- Table listing all 7 tools with columns: Tool Name, Classification (read/write badge), Enabled (toggle switch)
- Write tools show a lock icon when global mode is read-only (cannot be individually enabled)
- Toggles submit `POST /admin/mcp/tools` with the full list of disabled tools
- The tool description is shown in a muted subtitle row

#### Card 3: Server Instructions

- Textarea (monospace, ~20 rows) for markdown editing
- Template selector dropdown above the textarea: "Custom" (default), "Spending Review", "Monthly Analysis", "Data Export & Reporting"
- Loading a template replaces the textarea content (with Alpine.js `x-on:change` confirmation if current content is non-empty and modified)
- Character count display (`x/10000`)
- Save button submits `POST /admin/mcp/instructions`
- Preview section below the textarea showing the full instructions agents will see (built-in + custom), rendered as preformatted text

#### Card 4: API Key Scopes

- This is an informational card pointing to the existing API Keys page
- Shows a count of full-access vs read-only keys
- Link to `/admin/api-keys` with note that scope is set during key creation

### 7.2 Routes

```go
// Added to internal/admin/router.go within the authenticated admin routes

r.Route("/mcp", func(r chi.Router) {
    r.Get("/", MCPSettingsGetHandler(a, sm, tr))
    r.Post("/mode", MCPSaveModeHandler(a, sm))
    r.Post("/tools", MCPSaveToolsHandler(a, sm))
    r.Post("/instructions", MCPSaveInstructionsHandler(a, sm))
})
```

### 7.3 Navigation update

```html
<!-- internal/templates/partials/nav.html — add after API Keys item -->
<li><a href="/admin/mcp"{{if eq .CurrentPage "mcp"}} class="menu-active"{{end}}>
  <i data-lucide="bot" class="w-4 h-4"></i> MCP</a>
</li>
```

### 7.4 API Key creation page update

The existing `api_key_new.html` page gets a scope selector:

```html
<!-- Added to the key creation form -->
<div class="form-control">
  <label class="label"><span class="label-text">Scope</span></label>
  <select name="scope" class="select select-bordered w-full">
    <option value="full_access">Full Access — read and write</option>
    <option value="read_only">Read Only — query data only</option>
  </select>
</div>
```

The API keys list page shows scope as a badge next to each key name.

### 7.5 Template file

New template: `internal/templates/pages/mcp_settings.html`

Registered in `internal/admin/templates.go` → `basePages` slice:

```go
"pages/mcp_settings.html",
```

---

## 8. Service Layer

### 8.1 New service methods

```go
// internal/service/mcp_config.go (new file)

// MCPConfig represents the MCP permission and instruction settings.
type MCPConfig struct {
    Mode               string   `json:"mode"`                // "read_only" or "read_write"
    DisabledTools      []string `json:"disabled_tools"`      // tool names
    CustomInstructions string   `json:"custom_instructions"` // markdown
    InstructionTemplate string  `json:"instruction_template"` // template slug
}

// GetMCPConfig loads MCP configuration from app_config.
func (s *Service) GetMCPConfig(ctx context.Context) (*MCPConfig, error) {
    // Read mcp_mode, mcp_disabled_tools, mcp_custom_instructions, mcp_instruction_template
    // from app_config. Return defaults for missing keys.
}

// SaveMCPMode saves the global MCP mode.
func (s *Service) SaveMCPMode(ctx context.Context, mode string) error {
    // Validate: must be "read_only" or "read_write"
    // Save to app_config key "mcp_mode"
}

// SaveMCPDisabledTools saves the list of disabled tool names.
func (s *Service) SaveMCPDisabledTools(ctx context.Context, tools []string) error {
    // Validate: each tool name must be in the known tool registry
    // JSON-encode and save to app_config key "mcp_disabled_tools"
}

// SaveMCPInstructions saves custom instructions and the template slug.
func (s *Service) SaveMCPInstructions(ctx context.Context, instructions string, templateSlug string) error {
    // Validate: len(instructions) <= 10000
    // Save to app_config keys "mcp_custom_instructions" and "mcp_instruction_template"
}
```

### 8.2 Updated `CreateAPIKey`

The signature changes from `CreateAPIKey(ctx, name string)` to `CreateAPIKey(ctx, name string, scope string)`. All existing callers (admin handlers, programmatic setup) are updated. The `CreateAPIKeyResult` and `APIKeyResponse` types gain a `Scope` field.

```go
// internal/service/types.go — updated

type APIKeyResponse struct {
    ID         string  `json:"id"`
    Name       string  `json:"name"`
    KeyPrefix  string  `json:"key_prefix"`
    Scope      string  `json:"scope"`
    LastUsedAt *string `json:"last_used_at"`
    RevokedAt  *string `json:"revoked_at"`
    CreatedAt  string  `json:"created_at"`
}
```

### 8.3 MCP config in `App`

The `App` struct does not cache MCP config — it is read from `app_config` on each MCP server construction (which happens per HTTP request via the factory function). This ensures config changes take effect immediately without requiring server restart or explicit reload.

For the `mcp-stdio` subcommand (which creates a single long-lived server), the config is read once at startup. Changes via the dashboard will not affect an already-running stdio session. This is acceptable — stdio is a development tool.

---

## 9. Implementation Tasks

### Task 1: Database migration

- **File:** `internal/db/migrations/00017_api_key_scope.sql`
- Add `scope` column to `api_keys` table
- Run `sqlc generate` to update generated code

### Task 2: sqlc query updates

- **File:** `internal/db/queries/api_keys.sql`
- Update `CreateApiKey` to include `scope` parameter
- Run `sqlc generate`

### Task 3: Context helpers for API key

- **File:** `internal/middleware/context.go` (new)
- `SetAPIKey(ctx, *db.ApiKey) context.Context`
- `GetAPIKey(ctx) *db.ApiKey`

### Task 4: Update API key middleware

- **File:** `internal/middleware/apikey.go`
- Store validated API key in request context via `SetAPIKey`
- **File:** `internal/middleware/scope.go` (new)
- `RequireWriteScope()` middleware for write REST endpoints

### Task 5: Update service layer — API keys

- **File:** `internal/service/apikeys.go`
- Update `CreateAPIKey` to accept `scope` parameter
- Update `apiKeyFromRow` to include `Scope` field
- **File:** `internal/service/types.go`
- Add `Scope` field to `APIKeyResponse`

### Task 6: Service layer — MCP config

- **File:** `internal/service/mcp_config.go` (new)
- `GetMCPConfig`, `SaveMCPMode`, `SaveMCPDisabledTools`, `SaveMCPInstructions`

### Task 7: MCP instruction templates

- **File:** `internal/mcp/templates.go` (new)
- `InstructionTemplate` struct and `InstructionTemplates` slice
- Template content for `spend_review`, `monthly_analysis`, `reporting`

### Task 8: MCP tool registry refactor

- **File:** `internal/mcp/server.go`
- Extract built-in instructions to a `const builtInInstructions`
- Add `MCPConfig` struct, `ToolDef` struct with classification
- Refactor `MCPServer` to hold `allTools []ToolDef` instead of registering directly
- Add `BuildServer(cfg MCPConfig) *mcpsdk.Server` method
- Update `NewHTTPHandler` to use factory with per-request config
- **File:** `internal/mcp/tools.go`
- Add `checkWritePermission(ctx)` helper
- Add permission guard to `handleTriggerSync` (and any future write handlers)

### Task 9: MCP handler integration with service layer

- **File:** `internal/mcp/server.go`
- `NewMCPServer` now takes `*service.Service` + `version` (unchanged) but stores tool defs instead of registering them immediately
- `NewHTTPHandler` receives `*service.Service` to call `GetMCPConfig` per request, plus reads API key scope from context

### Task 10: REST API router — scope enforcement

- **File:** `internal/api/router.go`
- Group write endpoints (currently just `POST /sync`) behind `RequireWriteScope()` middleware

### Task 11: Admin handlers — MCP settings page

- **File:** `internal/admin/mcp.go` (new)
- `MCPSettingsGetHandler` — loads MCP config, tool list, templates, API key scope counts
- `MCPSaveModeHandler` — saves `mcp_mode`
- `MCPSaveToolsHandler` — saves `mcp_disabled_tools`
- `MCPSaveInstructionsHandler` — saves `mcp_custom_instructions` and `mcp_instruction_template`

### Task 12: Admin handlers — API key scope

- **File:** `internal/admin/apikeys.go`
- Update `APIKeyCreatePageHandler` to read `scope` from form
- Update `APIKeyNewPageHandler` to pass scope options to template
- Update `APIKeysListPageHandler` to show scope badges
- Update JSON API handlers similarly (`CreateAPIKeyHandler`)

### Task 13: Templates

- **File:** `internal/templates/pages/mcp_settings.html` (new)
- Four-card layout: mode, tools, instructions, API key info
- Alpine.js for template loading, unsaved changes confirmation, character count
- **File:** `internal/templates/partials/nav.html`
- Add MCP nav item
- **File:** `internal/templates/pages/api_key_new.html`
- Add scope selector dropdown
- **File:** `internal/templates/pages/api_keys.html`
- Add scope badge column

### Task 14: Template registration

- **File:** `internal/admin/templates.go`
- Add `"pages/mcp_settings.html"` to `basePages` slice

### Task 15: Admin router registration

- **File:** `internal/admin/router.go`
- Add `/mcp` route group with GET + POST handlers

### Task 16: `mcp-stdio` subcommand update

- **File:** `cmd/breadbox/mcp_stdio.go` (or wherever the stdio command lives)
- Read MCP config from DB at startup and pass to `BuildServer`

### Task 17: Update docs

- **File:** `docs/data-model.md` — add `scope` column to `api_keys`, add new `app_config` keys
- **File:** `CLAUDE.md` — add MCP permissions design decisions
- **File:** `docs/ROADMAP.md` — mark Phase 23 tasks

### Task 18: Tests

- Service layer tests for `GetMCPConfig`, `SaveMCPMode`, `SaveMCPDisabledTools`, `SaveMCPInstructions`
- Service layer test for `CreateAPIKey` with scope
- Middleware test for `RequireWriteScope`
- MCP `BuildServer` test verifying tool filtering by mode, disabled list, and key scope
- Integration test: read-only key cannot trigger sync via REST or MCP

---

## 10. Dependencies

### Depends on

- **Phase 22 (Agent APIs):** Phase 22 may add new MCP tools (field selection, aggregation, category CRUD). Phase 23's tool registry must include any tools added by Phase 22. If Phase 22 adds write tools (category CRUD), they must be classified as `write` in the tool registry.

### Depended on by

- **Phase 25 (External Agent Support):** External agents authenticate via API keys. Phase 25 benefits from scoped keys to give review agents limited access.
- **Phase 29 (Multi-user):** Multi-user introduces per-user API keys. Phase 23's scope model (`full_access`/`read_only`) will be extended with user-level scoping, but the column and middleware are forward-compatible.

### Independent of

- Phases 21, 24, 26, 27, 28, 30 — no direct dependencies in either direction.
