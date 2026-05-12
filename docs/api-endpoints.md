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
| PATCH | `/transactions/{id}/category` | W | Set category (`category_override=true`) |
| DELETE | `/transactions/{id}/category` | W | Reset category to provider default |
| DELETE | `/transactions/{id}` | W | Soft-delete (sets `deleted_at`) |
| POST | `/transactions/{id}/restore` | W | Restore a soft-deleted transaction |
| POST | `/transactions/{id}/tags` | W | Attach a tag to a transaction |
| DELETE | `/transactions/{id}/tags/{slug}` | W | Detach a tag from a transaction |
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
- [`docs/headless-api-plan.md`](headless-api-plan.md) — design context for the headless control plane
- [`.claude/rules/api.md`](../.claude/rules/api.md) — handler/auth conventions
- [`.claude/rules/api-endpoints.md`](../.claude/rules/api-endpoints.md) — upkeep rule for this file
