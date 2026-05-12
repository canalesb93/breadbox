---
paths:
  - "internal/api/router.go"
  - "docs/api-endpoints.md"
  - "openapi.yaml"
---

# API endpoint catalog upkeep

`docs/api-endpoints.md` is the terse human-readable index of every REST endpoint Breadbox exposes. It pairs with the canonical `openapi.yaml` (machine-readable, drift-tested) and `docs/api-reference.md` (long-form prose).

## The rule

**Any change to `internal/api/router.go` that adds, removes, renames, or re-scopes a route MUST update `docs/api-endpoints.md` in the same commit.**

Specifically, if you change:

- `r.Get / Post / Patch / Put / Delete("/...")` calls under `/api/v1/`
- The `RequireWriteScope` grouping (read ↔ write)
- The auth requirements for a route
- A route's request/response semantic (a sentence summary in the table is enough; full details belong in `openapi.yaml` + `docs/api-reference.md`)

…then `docs/api-endpoints.md` must reflect it in the same PR. The OpenAPI drift test catches the spec; nothing catches drift in this Markdown file except discipline.

## What goes in the table row

```
| METHOD | `/path/with/{params}` | R or W | One-sentence description in present tense |
```

- **Method** matches chi's call (`GET`, `POST`, `PATCH`, `PUT`, `DELETE`).
- **Path** is the public path (skip the `/api/v1` prefix — it's in the doc's preamble).
- **Scope** is `R` (any API key) or `W` (`full_access` required). Some endpoints are write-only even on read intent for sensitivity reasons (API keys list, login accounts list) — mark them `W` and that's fine.
- **Description** is 5–15 words, present tense, no marketing fluff. Be specific about side effects.

## Where the row lives

Group rows by resource — match the existing section order:

1. Health / meta
2. Accounts
3. Transactions (+ comments, tags-on-transactions)
4. Categories
5. Tags
6. Rules
7. Annotations (cross-reference)
8. Comments (cross-reference)
9. Connections (+ providers dispatch + per-provider link flows + CSV)
10. Sync
11. Users (+ Login accounts subsection)
12. Account links
13. Reports
14. API keys
15. Provider settings

Add a new top-level section if you're introducing a new resource family; don't bury new resources inside an unrelated section.

## Cross-references

If your change has a security or correctness implication (encryption at rest, one-time tokens, soft-vs-hard delete, FK protection), note it in the section's prose — not just the table. Examples already in the doc:

- "Empty sensitive fields on PUT preserve the stored value"
- "Response includes plaintext `setup_token` (one-time only)"
- "All sensitive fields are AES-256-GCM encrypted at rest"

## What does NOT belong in `docs/api-endpoints.md`

- Full request/response shapes (those live in `docs/api-reference.md` and `openapi.yaml`)
- MCP tool docs (separate surface — `docs/mcp-tools-reference.md`)
- Admin handlers under `/-/*` (this file is the **public REST** index)
- Webhook handlers (separate `/webhooks/:provider` surface)

## When in doubt

- A new endpoint that's pure parity with an existing one (e.g. another bulk variant): still add it. Discoverability matters.
- A renamed endpoint kept as a deprecated alias: keep both rows; tag the old one `*deprecated — use `<new>` instead*`.
- Internal-only endpoints (e.g. token-gated `/connect/*` for hosted-link surfaces): add them under a clearly marked section so readers don't think they're general-purpose.

If you forget and a PR lands without updating this file, open a follow-up PR with `docs(api-endpoints):` as the prefix. Don't let the index rot.
