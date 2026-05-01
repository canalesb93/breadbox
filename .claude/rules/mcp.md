---
paths:
  - "internal/mcp/**"
---

# MCP server

## Architecture

- SDK: `github.com/modelcontextprotocol/go-sdk` v1.4.0.
- Transport: Streamable HTTP at `/mcp` (API key auth) + stdio via `breadbox mcp-stdio` subcommand (no auth, local only).
- Tool handlers call the **service layer directly** — no HTTP round-trip, no re-serialization.

## Tool registry

`MCPServer.allTools []ToolDef` is the master list. Each `ToolDef` has:
- `Name` (snake_case verb_noun: `list_transactions`, `update_transactions`)
- `Description` (domain-rich — see below)
- `Classification`: `ToolRead` or `ToolWrite`
- `Handler` with typed input/output structs

`BuildServer(MCPServerConfig)` filters the tool list per request and returns a fresh `*mcpsdk.Server`. Write tools are suppressed when:
1. `mcp_mode == read_only` (global toggle in `app_config`)
2. Tool name is in `mcp_disabled_tools` JSON array
3. API key scope is `read_only`

## Audit sessions (transport-bound)

Every tool call is logged under an `mcp_sessions` row keyed off the transport connection — `MCP-Session-Id` for Streamable HTTP, a per-process fallback id for stdio. The dispatcher in `makeToolDefLogged` resolves the session via `MCPServer.ensureAuditSession`, which lazy-creates the row on first call and stamps it with `clientInfo` from the `initialize` request. There is no `create_session` tool — the binding is implicit.

Per-call labels travel via `tools/call._meta.reason` (`metaReason(req)`); the dispatcher pulls it and stamps it on the `mcp_tool_calls.reason` column. Tool input schemas no longer carry `session_id` or `reason` fields.

Handlers that need to bind a created row to the session (e.g. `submit_report` → `agent_reports.session_id`) read the resolved id from context via `auditSessionFromContext(ctx)` — the dispatcher stamps it before invoking the handler.

## Input/output conventions

Tool inputs are typed structs with `jsonschema` tags:

```go
type QueryTransactionsInput struct {
    Limit    int      `json:"limit,omitempty" jsonschema:"description=Max rows,maximum=500"`
    MinAmount *float64 `json:"min_amount,omitempty"`
}
```

Use `*float64` (not `float64`) for optional numeric filters so zero is a valid filter value.

Outputs go through `jsonResult()` which calls `compactIDsBytes()` — collapses each object's own `id`/`short_id` pair so the `id` field carries the 8-char short value and the separate `short_id` key is dropped.

FK fields (`account_id`, `transaction_id`, `rule_id`, `user_id`, `connection_id`, …) carry the **referenced row's `short_id`** directly. Resolution happens at the SQL layer in service queries (JOINs that select the FK target's `short_id`) — not in the byte rewriter, which only touches the row's own pair. There is no separate `*_short_id` sibling on responses. Categories and tags are referenced by `*_slug` instead — slugs are the canonical handle for those entities.

Agents see one compact 8-char ID per reference. REST consumers see the same shape (FK fields carry shorts) since the SQL resolution is shared; only the row's own-`id` rewriting is MCP-only.

Errors: return with `IsError: true` and a text content block `{"error": "message"}`. Never panic.

## Descriptions

Tool descriptions are **domain-rich**, not generic. Include:
- Amount sign convention (positive = debit, negative = credit/refund)
- What each filter does and how filters compose
- Pagination behavior (cursor vs offset)
- Limits (500 for list/batch tools)

Good descriptions are load-bearing — they're the only documentation agents see at tool-call time.

## Server instructions

`MCPServerConfig.Instructions` is domain-rich onboarding: data model overview, amount conventions, category system, recommended query patterns. Templates in `internal/mcp/templates.go` (`spend_review`, `monthly_analysis`, `reporting`) are presets users can load and edit. User-edited text stored as `mcp_custom_instructions` in `app_config`.

## Resource

`breadbox://overview` returns live stats: users, accounts by type, connections with counts, 30-day spending summary with top 5 categories, pending transaction count.

## Permissions admin page

`/mcp` dashboard has four cards: global mode, per-tool enable/disable, server instructions, API key scope info. Nav icon: `bot`.

## Tag-based review workflow

The "review queue" is just transactions tagged `needs-review`. A seeded system rule (NULL conditions, `trigger=on_create`, action `add_tag: needs-review`) auto-tags every newly-synced transaction. Disable that rule to opt out of auto-review.

Agents follow a uniform loop: `query_transactions(tags=["needs-review"])` to find work, `update_transactions(operations=[…])` to set category + remove the tag (and pair the change with a `comment` for the audit trail) atomically per transaction. Max 50 ops per call.

`update_transactions` is the universal per-row write — tag adds, tag removes, category sets, category resets (`reset_category: true`), and comments all flow through it. The bare-row and bulk variants (`add_transaction_tag`, `remove_transaction_tag`, `categorize_transaction`, `reset_transaction_category`, `add_transaction_comment`, `bulk_recategorize`, `batch_categorize_transactions`) were collapsed into it during the MCP overhaul.

Annotations are read via `list_annotations`. Tag *vocabulary* admin (introducing, renaming, deleting tag definitions) goes through `create_tag`, `update_tag`, `delete_tag`.

## Reference data: dual surface (resources + tool mirrors)

Bounded reference data is exposed two ways:

- **Resources (preferred)** — `breadbox://overview`, `://accounts`, `://categories`, `://tags`, `://users`, `://rules`, `://sync-status`. Surfaced in Claude.ai's paperclip menu and the Inspector resource picker. Application-driven, user-controlled.
- **Tool mirrors (compat)** — `get_overview`, `list_accounts`, `list_categories`, `list_tags`, `list_users`, `list_transaction_rules`, `get_sync_status`. Same payload, called as tools. Kept because not every MCP client implements the resources/* methods — without these, those clients can't read this data at all.

Both surfaces share the same service-layer call path (no logic duplication), so payload shape stays in sync. When adding a new bounded reference resource, register both: a resource handler in `resources.go` and a tool mirror in `tools_reads.go`.

## Resource templates (drill-downs)

Per-entity detail views are exposed as MCP resource templates registered with `Server.AddResourceTemplate`:

- `breadbox://transaction/{short_id}` — `{transaction, annotations}`
- `breadbox://account/{short_id}` — `{account, recent_transactions}` (capped at 25)
- `breadbox://user/{short_id}` — `{user, accounts}`

Template handlers parse the trailing `{short_id}` via `extractTemplateParam`, resolve through the standard service `Get*` methods (which accept either UUID or short_id), and return `mcpsdk.ResourceNotFoundError(uri)` on miss. URIs come back to chat as clickable items via `resource_link` content blocks emitted by tools (planned in a follow-up PR).
