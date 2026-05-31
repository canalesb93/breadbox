# API endpoints

A terse catalog of every REST endpoint Breadbox exposes. The canonical machine-readable spec is [`openapi.yaml`](../openapi.yaml); this file is the human-readable index. **Keep both in sync** — see `.claude/rules/api-endpoints.md` for the upkeep rule.

All endpoints live under `/api/v1/` and require `X-API-Key` unless noted. Scope column: **R** = readable with any key, **W** = requires `full_access` scope. Auth and CSRF details live in `.claude/rules/api.md`.

## Health / meta

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/health` | none | Basic liveness, no DB ping |
| GET | `/health/live` | none | Alias of `/health` |
| GET | `/health/ready` | none | DB + scheduler readiness |
| GET | `/api/v1/version` | none | Build version + upgrade check |
| GET | `/api/v1/headless/bootstrap` | R | Setup readiness report (consumed by `breadbox doctor`) |

## Auth

Unauthenticated device-code dance the CLI uses to mint API keys on a remote host without a paste-mode token. The `device_code` returned by the initiate call is the credential the polling endpoint accepts; the `user_code` is the short human-facing string the operator types on the browser approval page (`GET /auth/device`, session-gated, not on the public REST surface).

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| POST | `/auth/device-code` | none | Mint a pending device-code pair; returns `device_code`, `user_code`, `verification_url`, `expires_in`, `interval` |
| POST | `/auth/device-code/poll` | none | Poll status; `200 {status: "authorization_pending"}` or `200 {status: "approved", token: "bb_..."}`, with `400 EXPIRED` / `400 DENIED` / `404 INVALID_DEVICE_CODE` envelopes for terminal states |

## Accounts

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/accounts` | R | List all household accounts |
| GET | `/accounts/{id}` | R | Single account summary |
| GET | `/accounts/{id}/detail` | R | Detail incl. last 25 transactions and per-currency balances |
| PATCH | `/accounts/{id}` | W | Update `display_name`, `is_excluded`, `is_dependent_linked` |

## Transactions

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/transactions` | R | Cursor-paginated list with filters; supports `?fields=` selection |
| GET | `/transactions/count` | R | Count matching the same filters |
| GET | `/transactions/summary` | R | Aggregates by category / month / week / day |
| GET | `/transactions/merchants` | R | Merchant-level stats (count, total, avg) |
| GET | `/transactions/{id}` | R | Single transaction |
| GET | `/transactions/{id}/annotations` | R | Activity-timeline rows; mirrors MCP `list_annotations` |
| GET | `/transactions/{transaction_id}/comments` | R | List comments on a transaction |
| POST | `/transactions/update` | W | Atomic multi-field batch (category + tags + comment per row, max 50) |
| POST | `/transactions/batch-categorize` | W | Set category on many transactions (max 500) |
| POST | `/transactions/bulk-recategorize` | W | Server-side recategorize by filter |
| PATCH | `/transactions/{id}/category` | W | Set category (`category_override='user'`) |
| DELETE | `/transactions/{id}/category` | W | Reset category to provider default |
| DELETE | `/transactions/{id}` | W | Soft-delete (sets `deleted_at`) |
| POST | `/transactions/{id}/restore` | W | Restore a soft-deleted transaction |
| POST | `/transactions/{id}/tags` | W | Attach a tag to a transaction |
| DELETE | `/transactions/{id}/tags/{slug}` | W | Detach a tag from a transaction |
| PATCH | `/transactions/{id}/metadata/{key}` | W | Upsert one free-form metadata key |
| DELETE | `/transactions/{id}/metadata/{key}` | W | Remove one metadata key |
| PUT | `/transactions/{id}/metadata` | W | Replace the entire metadata object |
| DELETE | `/transactions/{id}/metadata` | W | Clear metadata to `{}` |
| POST | `/transactions/{id}/flag` | W | Flag for attention (optional `reason` → comment) |
| DELETE | `/transactions/{id}/flag` | W | Clear the flag |
| POST | `/transactions/{transaction_id}/comments` | W | Add a comment |
| PUT | `/transactions/{transaction_id}/comments/{id}` | W | Edit a comment |
| DELETE | `/transactions/{transaction_id}/comments/{id}` | W | Delete a comment |

## Categories

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/categories` | R | List all categories |
| GET | `/categories/{id}` | R | Single category |
| GET | `/categories/export` | R | Export as TSV |
| POST | `/categories` | W | Create a category |
| POST | `/categories/import` | W | Import categories from TSV |
| POST | `/categories/{id}/merge` | W | Merge one category into another (transactions migrate, source removed) |
| PUT | `/categories/{id}` | W | Replace category |
| DELETE | `/categories/{id}` | W | Delete (blocked if any transactions reference it; clear them first) |

## Tags

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/tags` | R | List tag vocabulary |
| GET | `/tags/{slug}` | R | Single tag (accepts UUID, short_id, or slug) |
| POST | `/tags` | W | Create a tag (slug must match `^[a-z0-9][a-z0-9\-:]*[a-z0-9]$`) |
| PATCH | `/tags/{slug}` | W | Partial update of display_name/description/color/icon/lifecycle |
| DELETE | `/tags/{slug}` | W | Delete a tag |

## Subscriptions (recurring series)

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/series` | R | List recurring series; optional `?status=active\|candidate\|paused\|cancelled` |
| GET | `/series/explain` | R | Near-miss feed: recurring-looking merchants not yet a series + the detector's verdict (why) |
| GET | `/series/{id}` | R | Single series (accepts UUID or short_id) |
| POST | `/series` | W | Create a missed series (`merchant_key`+`create_if_missing`) or assign by `series_id`; links `transaction_ids` (≤50), optional `confirm` |
| POST | `/series/{id}/transactions` | W | Link transactions (≤50, NULL-fill only) to a series; optional `confirm` |
| DELETE | `/series/{id}/transactions/{txid}` | W | Unlink a transaction from a series; strips inherited tags, recomputes rollups; errors if not a member |
| POST | `/series/{id}/rekey` | W | Correct a series' `merchant_key` (+ repoint members); errors on collision / sticky-reject |
| POST | `/series/{id}/split` | W | Move `transaction_ids` (≤50, current members) into a new series under `new_merchant_key` |
| POST | `/series/{id}/type` | W | Set the series type (`subscription`/`bill`/`loan`/`other`); sticky override of the inferred type |
| POST | `/series/{id}/tags` | W | Attach a tag to a series; linked transactions inherit it |
| DELETE | `/series/{id}/tags/{slug}` | W | Detach a tag; strips series-inherited copies from members (keeps user-added) |
| PATCH | `/series/{id}` | W | Partial update — edit attributes (`name`, `expected_amount`+`currency`, `amount_tolerance`, `cadence`, `expected_day`, `category_id`, `user_id`) and/or apply a `verdict` (`confirm`/`reject`/`pause`/`cancel`); edits apply first; user confirmation outranks agent |

## Rules

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/rules` | R | Cursor-paginated list |
| GET | `/rules/{id}` | R | Single rule |
| GET | `/rules/{id}/sync-history` | R | Last N sync runs that triggered this rule |
| POST | `/rules` | W | Create a rule |
| POST | `/rules/batch` | W | Bulk create; per-op results, `on_error: continue\|abort` |
| POST | `/rules/preview` | W | Dry-run — show which transactions would match |
| POST | `/rules/{id}/apply` | W | Apply one rule retroactively (respects `category_override`) |
| POST | `/rules/apply-all` | W | Apply every active rule retroactively |
| PUT | `/rules/{id}` | W | Replace a rule |
| DELETE | `/rules/{id}` | W | Delete a rule |

## Annotations

Annotations are read-only via the transaction endpoint: `GET /transactions/{id}/annotations` above. Writes happen implicitly through comments, tag adds/removes, category changes, and rule applications — never directly.

## Comments

See the Transactions table — comments are nested under `/transactions/{transaction_id}/comments`.

## Connections

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/connections` | R | List active connections |
| GET | `/connections/{id}` | R | Connection detail (provider, status, paused, sync interval, account count) |
| GET | `/connections/{id}/status` | R | Status + last sync info |
| GET | `/providers` | R | List installed providers (`{name, configured, needs_link_session}`) |
| GET | `/providers/{name}` | R | Single provider entry — capability flags for one provider |
| POST | `/providers/{name}/link-session` | W | Start a link/auth session — returns a provider token when needed; `204` when no link step |
| POST | `/providers/{name}/test` | W | Round-trip a credentials check; returns `{ok, message}` |
| DELETE | `/providers/{name}` | W | Disable a provider — clears credentials from `app_config`, drops the live provider entry |
| POST | `/connections` | W | Create from a provider payload (`provider` discriminator: `plaid`, `teller`) |
| POST | `/connections/{id}/sync` | W | Trigger sync for one connection (202; runs async) |
| POST | `/connections/{id}/paused` | W | Pause or resume scheduled syncs |
| POST | `/connections/{id}/sync-interval` | W | Set or clear per-connection interval override |
| POST | `/connections/{id}/reauth` | W | Start re-auth flow; returns a fresh link token |
| POST | `/connections/{id}/reauth-complete` | W | Mark connection active again after the user finishes re-auth |
| DELETE | `/connections/{id}` | W | Soft-disconnect (wipes encrypted tokens, transactions soft-deleted) |
| POST | `/connections/link` | W | Mint a hosted-link URL — agent shares it, user opens in browser to run Plaid/Teller |
| POST | `/connections/{id}/relink` | W | Mint a re-auth hosted-link URL for one connection (always single-use) |
| GET | `/connections/link/{id}` | R | Poll a hosted-link session — status, result connection IDs |
| POST | `/connections/csv/preview` | W | Preview a CSV (multipart or JSON+base64) — no persist |
| POST | `/connections/csv/import` | W | Import a CSV — creates connection if absent, deduplicates by provider txn id |
| POST | `/connections/plaid/link-token` | W | *Deprecated — use `POST /providers/plaid/link-session`.* Returns a fresh Plaid Link token |
| POST | `/connections/plaid/exchange` | W | *Deprecated — use `POST /connections` with `provider: "plaid"`.* Exchanges Plaid `public_token` |
| POST | `/connections/teller` | W | *Deprecated — use `POST /connections` with `provider: "teller"`.* Registers from the Teller enrollment payload |

Prefer `POST /providers/{name}/link-session` + `POST /connections` for new integrations — the OpenAPI spec treats them as canonical and the per-provider routes above are kept only as shims.

`POST /connections/{id}/relink` pins the new session to the connection in the path. The endpoint deliberately does not accept `user_id` or `provider` on the body — both are derived from the connection row, the session is always `action="relink"` and `single_use=true`, and re-auth against an already-disconnected connection returns `409 CONNECTION_DISCONNECTED`.

### Hosted-link page (token-scoped, page-internal)

These endpoints are called by the standalone `/link/{token}` page only. The token in the path is the credential — no API key required, no admin session, no rate limiter. They are intentionally not modeled in `openapi.yaml` (the spec covers the agent-facing `/api/v1/*` surface only); the drift test scopes itself to `/api/v1/*` and ignores this section.

| Method | Path | Description |
|--------|------|-------------|
| GET  | `/link/{token}` | Standalone HTML page; user opens it to add a bank |
| GET  | `/_link/{token}/session` | Redacted session view for the page (flips pending→active on first call) |
| POST | `/_link/{token}/providers/{name}/start` | Page-scoped start of a provider link session |
| GET  | `/_link/{token}/providers/teller/config` | Public Teller bootstrap (application_id + environment); cert/key never exposed |
| POST | `/_link/{token}/connections` | Page-scoped connection create (attributes to session's user) |
| POST | `/_link/{token}/reauth-complete` | Page-scoped re-auth completion (only valid for `action="relink"` sessions; reactivates the pinned connection and burns the token) |
| POST | `/_link/{token}/complete` | User-initiated "I'm done"; consumes the token (idempotent — already-completed returns 204) |
| POST | `/_link/{token}/fail` | Page reports a provider-side SDK failure |

The bearer middleware returns `401 INVALID_TOKEN` for unknown tokens, `410 EXPIRED` once past `expires_at`, `410 CONSUMED` after `/complete`, and `410 GONE` for any other terminal state. Scope-pinning lives in each handler: a session minted with `provider="plaid"` will `403 FORBIDDEN` any `/_link/.../providers/teller/start` or `/_link/.../connections` body that names a different provider.

## Sync

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| POST | `/sync` | W | Trigger sync — all active connections, or one via `{"connection_id": "..."}` |
| GET | `/sync/logs` | R | Paginated history; filters `connection_id`, `status`, `trigger`, `from`, `to` |
| GET | `/sync/logs/{id}` | R | Single log + per-account rows |
| GET | `/sync/health` | R | Aggregate stats (success rate, p50 duration) |
| GET | `/sync/health/providers` | R | Per-provider stats |
| GET | `/sync/stats` | R | Totals matching the same filter set as `/sync/logs` |

## Users

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/users` | R | List household members |
| GET | `/users/{id}` | R | Single member |
| POST | `/users` | W | Create a household member |
| PATCH | `/users/{id}` | W | Update name / email |
| DELETE | `/users/{id}` | W | Delete (blocked by FK; hit `/wipe-data` first if needed) |
| POST | `/users/{id}/wipe-data` | W | **Destructive.** Removes all rows attributed to this user |

### Login accounts (sensitive — all write-scope)

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/users/{user_id}/login` | W | List logins for the user |
| POST | `/users/{user_id}/login` | W | Create a login. **Response includes plaintext `setup_token` (one-time only)** |
| PATCH | `/users/{user_id}/login/{login_id}` | W | Update role |
| DELETE | `/users/{user_id}/login/{login_id}` | W | Remove a login |
| POST | `/users/{user_id}/login/{login_id}/regenerate-token` | W | Issue a fresh setup token |
| GET | `/login-accounts` | W | List every login account (flat — no parent user_id) |
| DELETE | `/login-accounts/{id}` | W | Delete a login by its own id |
| POST | `/login-accounts/{id}/reset-password` | W | Issue a fresh setup token (flat alias of `regenerate-token`) |

## Account links (dependent-account mirroring)

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/account-links` | R | List links |
| GET | `/account-links/{id}` | R | Single link |
| GET | `/account-links/{id}/matches` | R | Cursor-paginated matched-transaction list |
| POST | `/account-links` | W | Create a link between a primary and dependent account |
| PUT | `/account-links/{id}` | W | Update link |
| DELETE | `/account-links/{id}` | W | Delete link |
| POST | `/account-links/{id}/reconcile` | W | Re-run matcher across both accounts |
| POST | `/transaction-matches/{id}/confirm` | W | Confirm a candidate match |
| POST | `/transaction-matches/{id}/reject` | W | Reject a candidate match |
| POST | `/transaction-matches/manual` | W | Create a manual match between two transactions |

## Reports

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/reports` | R | List agent reports |
| GET | `/reports/unread-count` | R | Unread badge count |
| GET | `/reports/{id}` | R | Single report |
| POST | `/reports` | W | Create a report (typically from an agent) |
| PATCH | `/reports/{id}/read` | W | Mark report read |
| PATCH | `/reports/{id}/unread` | W | Mark report unread |
| POST | `/reports/read-all` | W | Mark every report read |
| DELETE | `/reports/{id}` | W | Delete a report |

## API keys (write-scope only — listing is sensitive)

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/api-keys` | W | List keys (hashed prefixes only — never plaintext) |
| POST | `/api-keys` | W | Create. **Response includes plaintext `key` once.** Accepts optional `actor_type` (`user`/`agent`/`system`, default `agent`) and `actor_name` |
| DELETE | `/api-keys/{id}` | W | Soft-revoke a key |
| GET | `/keys/me` | R | Whoami: return the calling key's id, prefix, scope, actor_type, actor_name |

## Provider settings

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/settings/providers` | R | Redacted view (`{configured, secret_set, certificate_set}`) — never raw secrets |
| PUT | `/settings/providers/plaid` | W | Set/update Plaid `client_id`, `secret`, `environment`, `webhook_url` |
| PUT | `/settings/providers/teller` | W | Set/update Teller `application_id`, cert + private key PEM |

Empty sensitive fields on PUT preserve the stored value (so you can update `webhook_url` without retyping the secret). All sensitive fields are AES-256-GCM encrypted at rest.

## App config

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/config` | W | List every app_config row with effective source (`env` / `db` / `default`); secret values are masked unless `?reveal=true` |
| GET | `/config/{key}` | W | Get a single config value; same masking and `?reveal=true` semantics |
| PUT | `/config/{key}` | W | Write a value into the app_config table (body: `{"value":"..."}`) |
| DELETE | `/config/{key}` | W | Drop the row (effective value falls back to env or compile-in default) |

Secret-flagged keys are masked on read. A denylist of keys (`ENCRYPTION_KEY`, `teller_cert_pem`, `teller_key_pem`) may never be revealed and refuse writes via this surface — manage them through env vars or the admin UI.

## Webhook events

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/webhook-events` | W | Paginated list of recent webhook events; filters `provider`, `status`, `page`, `limit` |
| POST | `/webhook-events/{id}/replay` | W | Re-trigger the manual sync the event would have caused; events without a connection are reported as `triggered: false` |

## Workflows

Workflows are scheduled Claude Agent SDK runs that call breadbox MCP to enrich, categorize, or report on data (the REST surface for the Workflows product; renamed from `/agents/*`). Runs are append-only (no delete). Settings carry Anthropic credentials (encrypted at rest) and global caps.

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/workflows` | R | List all agent definitions with last_run inlined (bare JSON array) |
| GET | `/workflows/{slug}` | R | One definition; accepts slug, short_id, or UUID |
| POST | `/workflows` | W | Create an agent definition |
| PATCH | `/workflows/{slug}` | W | PATCH-merge update; omitted fields are unchanged |
| DELETE | `/workflows/{slug}` | W | Delete the definition; historical runs preserved (FK SET NULL) |
| POST | `/workflows/{slug}/enable` | W | Flip enabled=true |
| POST | `/workflows/{slug}/disable` | W | Flip enabled=false |
| POST | `/workflows/{slug}/run` | W | Trigger an immediate synchronous run; optional body fields `{prompt_prefix}` (prepend, ≤2000) and `{prompt}` (full override, ≤40000) — `prompt` wins when both set; 503 `CONCURRENCY_LOCKED` when another run is in progress |
| POST | `/workflows/test` | W | Run the diagnostic smoke test (tiny "say OK" prompt, no MCP servers, ~5¢ cap); 422 `AUTH_NOT_CONFIGURED` / `AGENT_BINARY_NOT_FOUND` |
| POST | `/workflows/cleanup` | W | Run the agent cleanup pass on demand; returns `{runs_deleted, transcripts_deleted, transcripts_scanned, retention_days, transcript_dir}` |
| GET | `/workflows/{slug}/runs` | R | Offset-paginated run history; `?limit=50&offset=0` (max 200); filters: `status`, `trigger`, `hit_cap` (`max_turns`/`max_budget`/`any`), `start`, `end` |
| GET | `/workflows/runs` | R | Cross-agent run history with `agent_slug`+`agent_name` per row; same filters as `/workflows/{slug}/runs` plus optional `agent=<slug>` to narrow to one definition |
| GET | `/workflows/prompt-blocks` | R | Parsed view of the embedded `prompts/agents/*.md` library — id, title, description, group (`strategy`/`depth`/`integration`/`knowledge`), and full markdown content |
| GET | `/workflow-presets` | R | List the code-defined Workflow preset gallery, annotated with enabled-state |
| POST | `/workflow-presets/{slug}/enable` | W | Instantiate a workflow from a preset (409 if already enabled) |
| GET | `/workflows/runs/recent-errors` | R | Errored runs across all agents in the last `hours` (default 24, max 168); `?limit=5` (max 50); joined with `agent_slug`+`agent_name` for deep-link |
| GET | `/workflows/runs/{shortId}` | R | One run detail (by short_id or UUID) |
| PATCH | `/workflows/runs/{shortId}` | W | Set/clear the operator note on a run. Body `{ "note": "..." }`; empty string clears. Capped at 2000 chars |
| GET | `/workflows/runs/{shortId}/transcript` | R | Streams the NDJSON transcript; 404 when not yet written |
| GET | `/workflows/settings` | R | Agent subsystem config; token fields returned masked, never plaintext |
| GET | `/workflows/status` | R | Cheap readiness probe — `{auth_configured, binary_present, ready}` for onboarding hints (no API call) |
| PUT | `/workflows/settings` | W | Update settings; nil fields are unchanged, empty string for token fields clears them |

`subscription_token` and `anthropic_api_key` are AES-256-GCM encrypted at rest. GET returns a masked display string (`"sk-ant-oat01-XXXXXXXXX••••wxyz"`); the full value never leaves the server. A per-run scoped API key (`actor_type='agent'`) is minted at run start by the orchestrator and revoked at completion — it is not exposed via this surface.

## Rate limiting

All `/api/v1/*` routes (except `/health/*` and `/api/v1/version`) are rate-limited per API key. Defaults: **120 req/min, burst 60**. Env vars `API_RATE_LIMIT_RPM` / `API_RATE_LIMIT_BURST`. Over-limit responses: `429 RATE_LIMITED` with `X-RateLimit-Limit/Remaining/Reset` + `Retry-After`.

## Error envelope

Every error response uses:

```json
{ "error": { "code": "UPPER_SNAKE_CASE", "message": "..." } }
```

Common codes: `VALIDATION_ERROR`, `INVALID_PARAMETER`, `NOT_FOUND`, `UNAUTHORIZED`, `FORBIDDEN`, `INSUFFICIENT_SCOPE`, `RATE_LIMITED`, `CONFLICT`, `PROVIDER_ERROR`, `INTERNAL_ERROR`. Codes are stable contracts — never rename.

## See also

- [`openapi.yaml`](../openapi.yaml) — machine-readable spec; the drift test (`internal/api/openapi_drift_test.go`) fails CI when chi routes and the spec disagree
- [`docs/api-reference.md`](api-reference.md) — long-form prose with request/response examples
- [`.claude/rules/api.md`](../.claude/rules/api.md) — handler/auth conventions
- [`.claude/rules/api-endpoints.md`](../.claude/rules/api-endpoints.md) — upkeep rule for this file
