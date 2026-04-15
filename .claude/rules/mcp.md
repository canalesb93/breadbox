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
- `Name` (snake_case verb_noun: `list_transactions`, `submit_review`)
- `Description` (domain-rich — see below)
- `Classification`: `ToolRead` or `ToolWrite`
- `Handler` with typed input/output structs

`BuildServer(MCPServerConfig)` filters the tool list per request and returns a fresh `*mcpsdk.Server`. Write tools are suppressed when:
1. `mcp_mode == read_only` (global toggle in `app_config`)
2. Tool name is in `mcp_disabled_tools` JSON array
3. API key scope is `read_only`

## Input/output conventions

Tool inputs are typed structs with `jsonschema` tags:

```go
type QueryTransactionsInput struct {
    Limit    int      `json:"limit,omitempty" jsonschema:"description=Max rows,maximum=500"`
    MinAmount *float64 `json:"min_amount,omitempty"`
}
```

Use `*float64` (not `float64`) for optional numeric filters so zero is a valid filter value.

Outputs go through `jsonResult()` which calls `compactIDs()` — replaces every `id` field with the `short_id` value and drops the `short_id` field. Agents see compact 8-char IDs; REST API consumers still get both.

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

## Disabled-feature behavior

When review queue is off (`review_auto_enqueue=false`), review MCP tools return empty results with an explanatory note (read tools) or an error (write tools) — they don't just disappear from the registry. This keeps tool-count stable for agents.
