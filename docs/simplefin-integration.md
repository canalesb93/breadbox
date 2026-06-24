# SimpleFIN integration

SimpleFIN ([simplefin.org](https://www.simplefin.org/protocol.html)) is a
token-paste, poll-only bank-data protocol — the aggregator the self-hosting
community commonly uses (via the hosted [SimpleFIN Bridge](https://bridge.simplefin.org)
or a self-run server). It is the simplest provider Breadbox supports: no OAuth,
no SDK, no webhooks.

Provider package: `internal/provider/simplefin/`. It implements the standard
`internal/provider.Provider` interface and is structurally between CSV (stubs the
unsupported methods) and Teller (real HTTP client + date-range polling).

## How it differs from Plaid/Teller

| Aspect | SimpleFIN |
|---|---|
| Connect | **Token paste.** User gets a one-time base64 *setup token* from their bridge's `/create` page and pastes it. No SDK popup, no `CreateLinkSession`. |
| Credential | The claimed **access URL** `https://user:pass@host/path` (HTTP Basic creds embedded), stored AES-GCM encrypted. |
| Fetch | **Poll only.** `GET {accessURL}/accounts?start-date&end-date&pending=1` returns accounts with nested transactions. No cursor, no webhooks. |
| Scope | **One access URL spans every bank** the user linked at the bridge (multi-bank aggregator) → one Breadbox connection, many accounts. |
| Amount sign | **Inverted.** SimpleFIN positive = money in; Breadbox positive = debit (money out). The mapper negates uniformly. |
| Reauth | Paste a **new** setup token; the old access URL 403s once revoked. |
| Enablement | **Opt-in toggle** (`SIMPLEFIN_ENABLED` env / `simplefin_enabled` app_config). No server-level credential. |

## The connect flow (claim)

1. User pastes the setup token in the admin **Add connection** screen.
2. `ExchangeToken` base64-decodes it to a one-time **claim URL** and `POST`s to it
   (unauthenticated, empty body). A `200` returns the **access URL**; a `403`
   means the token was already used or is invalid.
3. The access URL is AES-GCM encrypted (`internal/crypto`) and stored in
   `bank_connections.encrypted_credentials`. The embedded `user:pass` is the
   credential — it never appears in `external_id` or logs.
4. `GET {accessURL}/accounts?balances-only=1` discovers the accounts.

### Connection identity

SimpleFIN exposes no stable upstream connection id (the access URL host is the
shared bridge; re-claiming yields a brand-new URL). So the provider **mints its
own opaque `external_id`** (`internal/shortid`) at connect time, satisfying the
`UNIQUE (provider, external_id)` constraint without leaking the secret URL.
Reauth keeps the same row and only rotates `encrypted_credentials`.

## Sync

`SyncTransactions` polls `/accounts` over a date range, **chunked into ≤90-day
windows** (the bridge caps a single request's range at 90 days):

- Empty cursor → ~1 year backfill (≈5 windowed requests).
- Non-empty cursor → from `lastSync − 10 days` (overlap for late-posting txns) to
  now. The cursor is the RFC3339 timestamp of the last sync (same scheme as
  Teller).

Each transaction's amount is **negated** (uniform — SimpleFIN has no per-account
sign rule), `pending` is taken from the `pending` flag / `posted == 0`, and the
raw JSON is preserved in `Transaction.Raw`. Because SimpleFIN re-returns the full
window each sync, the engine soft-deletes stale pending rows via the
`ReconcilesPendingByPolling()` capability (shared with Teller — see
`.claude/rules/providers.md`).

### Account discovery on every sync

One access URL spans every bank at the bridge, and the user can link more banks
there *after* connecting Breadbox. So `SyncTransactions` also returns the
connection's **full current account set** in `SyncResult.Accounts`, deduped by
external ID across every fetch window (a single window can omit accounts, so the
set is unioned rather than captured from one window). Before processing
transactions, the sync engine upserts that set with `UpsertAccountMetadata` —
**metadata only, never balances** (those are owned by the balance-refresh path)
and `connection_id` is set only on INSERT, so an existing account keeps its
connection. A bank added at the bridge after connect therefore appears in
Breadbox on the next sync automatically — no reconnect, no new token. Providers
whose account set is fixed at connect time (Plaid, CSV) leave
`SyncResult.Accounts` nil and the engine skips the upsert.

### Mixed-owner bridges

A single SimpleFIN token can span accounts belonging to **different household
members** (e.g. one person links both their and their partner's banks at the
bridge). The connection has a single owner (`bank_connections.user_id`), but
individual accounts can be reassigned to another member via the per-account
owner override (`accounts.owner_user_id`) — editable from the account-detail
Settings page or `PATCH /api/v1/accounts/{id}`. Attribution resolves at read
time through `COALESCE(t.attributed_user_id, a.owner_user_id, bc.user_id)`, so
reassigning an account re-routes its existing transactions across per-user
totals with no backfill. See `docs/data-model.md` § `accounts`.

### Rate limits

The SimpleFIN Bridge expects **≤ 24 requests/day** ("daily updates"). New
SimpleFIN connections are therefore given a **daily** `sync_interval_override_minutes`
at connect time so the default cron cadence doesn't blow the budget. Severe
overages disable the access token at the bridge.

## Reauth

`CreateReauthSession` returns `ErrNotSupported` (there's nothing server-mintable).
The admin reauth page renders a token-paste form instead of launching an SDK; on
submit, `POST /-/connections/{id}/reauth-complete` claims the new token and
updates `encrypted_credentials` in place on the existing connection
(`UpdateBankConnectionCredentials`), flipping status back to `active`.

## Unsupported / limitations (v1)

- `CreateLinkSession`, `HandleWebhook`, `CreateReauthSession` → `ErrNotSupported`.
  `RemoveConnection` is a no-op (SimpleFIN has no documented revoke endpoint; the
  user disables the token at the bridge).
- **Account type** is not exposed by SimpleFIN, so accounts default to
  `depository`. Credit cards therefore sync as depository in v1.
- **No REST endpoint** (`POST /api/v1/connections/simplefin`) and no hosted-link
  support — connect/relink is admin-page-only. A REST handler mirroring
  `connections_teller_setup.go` is a clean fast-follow if automation demand
  appears.
- Soft errors the server reports in the `errors`/`errlist` array are logged, not
  surfaced on the connection (no channel without an interface change).

## Testing

- Unit tests: `internal/provider/simplefin/simplefin_test.go` run against a fake
  bridge HTTP server (claim flow, amount negation, 90-day windowing, pending
  mapping, 403 → reauth, balances, errlist parsing). No DB required.
- **Demo token** for manual testing (decodes to the bridge's DEMO claim URL):
  `aHR0cHM6Ly9iZXRhLWJyaWRnZS5zaW1wbGVmaW4ub3JnL3NpbXBsZWZpbi9jbGFpbS9ERU1PLXYyLTFBOEY4QkM3QkUxQTAyMTM5QkUw`
  Enable SimpleFIN in `/settings/providers`, then paste this on the Add-connection
  screen.
