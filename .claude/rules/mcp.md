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

Agents follow a uniform loop: `query_transactions(tags=["needs-review"])` to find work, `update_transactions(operations=[…])` to set category + remove the tag (an optional note lands on the `tag_removed` annotation) atomically per transaction. Max 50 ops per call.

Tag/annotation tools: `list_tags`, `add_transaction_tag`, `remove_transaction_tag`, `list_annotations`, plus tag CRUD admin tools (`create_tag`, `update_tag`, `delete_tag`).
