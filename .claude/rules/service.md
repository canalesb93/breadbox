---
paths:
  - "internal/service/**"
---

# Service layer

## Purpose

`internal/service/` is the **shared business-logic layer**. Both REST API handlers (`internal/api/`) and MCP tools (`internal/mcp/`) call into it — MCP does **not** loopback through HTTP.

- Takes `*db.Queries` (for sqlc calls) + `*pgxpool.Pool` (for dynamic SQL and transactions).
- Returns plain Go types, not `pgtype.*`. Convert at the boundary.
- Accepts either UUID or short_id for entity parameters — resolve via `internal/service/resolve.go`.

## pgtype conversion

Never leak `pgtype.Text`, `pgtype.Numeric`, `pgtype.Timestamptz`, etc. out of the service layer. Use helpers in `internal/pgconv/` (or inline) to produce Go primitives (`string`, `*string`, `float64`, `time.Time`, `*time.Time`).

Amounts: convert `pgtype.Numeric` → `float64` carefully. Always carry `iso_currency_code` alongside.

## Dynamic SQL

Transaction queries, rule listings, review queues, merchant aggregations, and bulk recategorize are written as hand-rolled SQL with positional `$N` params — **not** sqlc.

Pattern:
1. Build `WHERE` clauses and args incrementally based on filters.
2. Use `fmt.Sprintf` **only** for fixed identifiers (column names, JOIN clauses) — never for user input. User input goes through `$N`.
3. Use cursor pagination (`(date, id) < ($N, $N)`) for stable ordering, not OFFSET.
4. Scan into a row struct with a `toResponse()` method for DRY conversion.

See `internal/service/transactions.go` and `internal/service/rules.go` for canonical examples.

## Short-ID resolution

`ResolveEntityID(ctx, q, input)` functions in `resolve.go` accept either a UUID or an 8-char base62 short_id and return the canonical UUID. Every public service method that takes an entity ID should call these at the top.

## Field selection

`ParseFields(raw, aliases)` and `FilterFields(response, fields)` in `internal/service/fields.go` implement the `?fields=` query param. Aliases (`minimal`, `core`, `category`, etc.) are defined per entity. `id` and `short_id` are **always** included regardless of filter.

Filtering happens at the **handler** layer, not in the service method — the service returns the full struct.

## Actor pattern

Mutations that need auditing (rules, reports) accept an `Actor` struct (`Type`, `ID`, `Name`) sourced from request context. Helpers in `internal/middleware/` extract the actor for both admin sessions and API keys.

## Transactions (DB)

Multi-statement writes use `pool.BeginTx` + deferred rollback + explicit commit. Sync writes are wrapped in a single transaction for atomicity — see `.claude/rules/sync.md`.
