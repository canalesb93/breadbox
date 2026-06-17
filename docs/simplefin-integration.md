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
| Connect | **Token paste, in Settings.** The single bridge token is pasted in **Settings → Providers → SimpleFIN** (a side drawer), not the per-bank Add-connection flow. No SDK popup, no `CreateLinkSession`. |
| Credential | The claimed **access URL** `https://user:pass@host/path` (HTTP Basic creds embedded), stored AES-GCM encrypted. |
| Fetch | **Poll only.** `GET {accessURL}/accounts?start-date&end-date&pending=1` returns accounts with nested transactions. No cursor, no webhooks. |
| Scope | **One access URL spans every bank** the user linked at the bridge (multi-bank aggregator) → **one singleton** Breadbox connection, many accounts. |
| Account growth | Banks added at the bridge **after** connect are discovered **automatically on the next sync** — no new token. The engine upserts the re-discovered account set before processing transactions (see "Sync" below). |
| Amount sign | **Inverted.** SimpleFIN positive = money in; Breadbox positive = debit (money out). The mapper negates uniformly. |
| Reauth / rotate | Paste a **fresh** setup token in the Settings drawer; it rotates the existing connection's credential in place (the old access URL 403s once revoked). |
| Enablement | **Connecting in Settings enables it** (`simplefin_enabled` app_config is set on first claim). `SIMPLEFIN_ENABLED=false` in env hard-disables. No manual on/off toggle. |

## Where the token lives (Settings, not Add-connection)

SimpleFIN is an **aggregator bridge**, not a per-bank login like Plaid/Teller, so
its single token is managed in **Settings → Providers → SimpleFIN** (a side
drawer), and the household has **at most one** active SimpleFIN connection (the
"singleton"). The drawer's form claims a pasted token and either creates that one
bridge connection (first time) or **rotates the stored credential in place** when
one already exists — there's never a second SimpleFIN connection, so re-pasting a
token can't strand accounts on an orphan row.

The **Add a connection** flow shows SimpleFIN as a *special, non-selectable row*
that links out to the bridge (to manage which banks are included) and to Settings
(to paste/rotate the token) — reinforcing that you don't add SimpleFIN banks one
at a time here. `internal/admin/providers.go::ProvidersSaveSimpleFINHandler` owns
the connect/rotate POST; `GetActiveConnectionByProvider` resolves the singleton.

## The connect flow (claim)

1. User pastes the setup token in **Settings → Providers → SimpleFIN**.
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

Each `/accounts` response carries the **full current account set**, so
`SyncTransactions` returns it in `SyncResult.Accounts` (deduped across windows).
The sync engine upserts that set — **metadata only, never balances** (the
`UpsertAccountMetadata` query; balances are owned by the balance-refresh path) —
onto the connection *before* processing transactions, then seeds its account
cache. The effect: a bank the user links at the bridge after connecting shows up
on the next sync, and its transactions resolve instead of being dropped on a
missing-account lookup. Existing accounts already on the connection are skipped
(no per-sync metadata write). Providers whose account set is fixed at connect
(Plaid, CSV) leave `SyncResult.Accounts` nil and the engine skips the step.

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
