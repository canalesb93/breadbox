# Headless API Completeness Plan

Goal: every workflow currently locked behind the admin UI becomes callable via `/api/v1/*` so a CLI / MCP / external service can fully drive Breadbox without any HTML surface.

This is the working spec for the `feat/headless-api` integration branch. Each numbered bundle below ships as its own PR targeting `feat/headless-api`. When all bundles land, a single PR squash-merges `feat/headless-api → main`.

## Branch model

```
main
 └── feat/headless-api  (long-lived integration branch)
      ├── feat/headless-api/01-rate-limit       → PR → squash-merged
      ├── feat/headless-api/02-txn-update       → PR → squash-merged
      ├── ...
      └── feat/headless-api/22-bootstrap        → PR → squash-merged
            ↓
  Final PR: feat/headless-api → main
```

Sub-PRs are **not** a Graphite stack — each is independent off the integration tip. Squash-merge each sub-PR into integration. Final integration → main is the only PR that reaches main.

## Quality bar

Every sub-PR must:

- Build cleanly: `go build ./... && go vet ./...`
- Pass unit tests: `go test ./...`
- Pass integration tests for the touched layer: `make test-integration`
- Include integration tests for new endpoints when reasonable (follow `internal/api/api_integration_test.go` patterns; `//go:build integration`)
- Update `docs/api-reference.md` with the new endpoints
- Use the existing error envelope (`writeServiceError` / `mw.WriteError`) — do **not** invent new shapes
- Honor scope middleware — read endpoints under `RequireReadScope`, mutating endpoints under `RequireWriteScope`
- Compact IDs in responses (use `compactIDs()` pattern from MCP / existing handlers)
- No `category_override=true` overrides without honoring the existing protection

## Bundles

| # | Bundle | Files | Phase |
|---|---|---|---|
| 01 | Rate-limit middleware on `/api/v1/*` | `internal/middleware/`, `cmd/breadbox/serve.go` | 1 |
| 02 | `POST /transactions/update` (atomic multi-field) | `internal/api/transactions.go`, `router.go` | 1 |
| 03 | `DELETE /transactions/{id}` + restore (soft-delete) | `internal/api/transactions.go`, `router.go` | 1 |
| 04 | Doc fix (`limit` default), `POST /sync` body docs, `POST /sync` tests | `docs/api-reference.md`, tests | 1 |
| 05 | Tags CRUD (`POST/PATCH/DELETE /tags`, `GET /tags/{slug}`) | `internal/api/tags.go`, `router.go` | 2 |
| 06 | `GET /transactions/{id}/annotations` | `internal/api/transactions.go`, `router.go` | 2 |
| 07 | Error envelope cleanup (`writeServiceError` everywhere) | `comments.go`, `reports.go` | 2 |
| 08 | Connections read & manage (GET detail, DELETE, per-conn sync, paused, sync-interval) | `internal/api/connections.go`, `router.go` | 3 |
| 09 | Plaid re-auth (`/connections/{id}/reauth` + complete) | `internal/api/connections.go`, `router.go` | 3 |
| 10 | Sync visibility (`GET /sync/logs`, `/sync/logs/{id}`, `/sync/health`, `/sync/health/providers`) | `internal/api/sync.go`, `router.go` | 3 |
| 11 | Plaid link flow (`POST /connections/plaid/link-token` + `/exchange`) | `connections.go`, `router.go` | 4 |
| 12 | Teller setup (`POST /connections/teller`) | `connections.go`, `router.go` | 4 |
| 13 | CSV preview + import (multipart) | `connections.go` or new file, `router.go` | 4 |
| 14 | Provider config (`GET/PUT /settings/providers/{plaid,teller}`) | new file, `router.go` | 4 |
| 15 | Categories tests (import/export, merge response) | tests only | 5 |
| 16 | Accounts: `PATCH /accounts/{id}` + `GET /accounts/{id}/detail` | `internal/api/accounts.go`, `router.go` | 5 |
| 17 | Rules: `POST /rules/batch` + `GET /rules/{id}/sync-history` + apply tests | `internal/api/rules.go`, `router.go`, tests | 5 |
| 18 | Users CRUD: `GET/POST/PATCH/DELETE /users`, `/users/{id}/wipe-data` | `internal/api/users.go`, `router.go` | 5 |
| 19 | Login accounts: `GET/POST/PATCH/DELETE /users/{id}/login` | `internal/api/users.go` (or new), `router.go` | 5 |
| 20 | API key mgmt + reports parity + account-links matches pagination | `internal/api/`, `router.go` | 5 |
| 21 | OpenAPI 3.1 spec (hand-authored) + drift check | `openapi.yaml`, `Makefile` | 6 |
| 22 | (Optional) `/headless/bootstrap` endpoint | `internal/api/`, `router.go` | 6 |

## Wave plan

Bundles execute in waves; bundles in the same wave run in parallel via subagents using isolated worktrees. Next wave waits for the prior wave's PRs to merge into `feat/headless-api`.

| Wave | Parallel bundles |
|------|---|
| W1 | 01 + 04 |
| W2 | 02 |
| W3 | 03 + 07 |
| W4 | 05 + 06 |
| W5 | 08 + 10 |
| W6 | 09 + 14 |
| W7 | 11, 12 (may serialize on `connections.go`) |
| W8 | 13 + 15 |
| W9 | 16 + 17 + 18 |
| W10 | 19 + 20 + 21 (+ 22) |

## Sub-PR conventions

- **Branch:** `feat/headless-api/<NN>-<slug>`
- **Base:** `feat/headless-api` (never `main`)
- **Title:** `headless-api: <bundle title>`
- **Body:** what the bundle adds, links to plan section, test summary
- **Labels:** `headless-api`
- **No auto-merge.** The orchestrator merges manually after review.
