---
paths:
  - "internal/provider/**"
  - "internal/webhook/**"
---

# Providers

## Interface

`internal/provider.Provider` (in `provider.go`) is the abstraction. Implementations: `plaid/`, `teller/`, `simplefin/`, `csv/`.

Methods take a `Connection` **struct** (not an ID) so implementations can decrypt credentials internally without re-fetching from the DB.

### `ReconcilesPendingByPolling() bool`

A capability method on the interface: returns true for **poll-based** providers that re-return their full transaction window each sync (Teller, SimpleFIN), so the sync engine soft-deletes pending rows no longer present in the window. Returns false for cursor/webhook providers (Plaid) and import-only (CSV). The engine gates `cleanStalePending` on this instead of a hardcoded provider name — add a poll-based provider and it inherits the cleanup for free.

## Error sentinels

Provider-agnostic sentinels in `internal/provider/errors.go`:
- `ErrReauthRequired` — credentials invalid; UI should prompt reauth.
- `ErrSyncRetryable` — transient; sync engine retries with backoff.
- `ErrNotSupported` — CSV uses this for sync/webhook/reauth (import-only).

Each provider wraps upstream errors (Plaid item errors, Teller HTTP 401, etc.) into these sentinels. Keep the sync engine and UI provider-agnostic.

## Encryption

Access tokens and Teller PEM files are AES-256-GCM encrypted at rest via `internal/crypto/encrypt.go`. `ENCRYPTION_KEY` (64-char hex) required at startup if any provider is configured — **fail fast in `main.go`**, not at first sync.

Connection storage uses generic columns: `external_id` + `encrypted_credentials`, not provider-specific names. Unique constraint on `(provider, external_id)`.

## Plaid

- SDK: `github.com/plaid/plaid-go`.
- Pending → posted: Plaid removes the pending ID and creates a new posted ID linked via `pending_transaction_id`. Matcher handles the dedupe.
- Categories: raw Plaid primary/detailed strings stored in `category_primary`/`category_detailed` for audit; structured category assigned via rules during sync.

## Teller

- No SDK. Hand-written HTTP client with **mTLS**: app-level cert/key + per-connection access token via HTTP Basic Auth.
- Config env: `TELLER_APP_ID`, `TELLER_CERT_PATH`, `TELLER_KEY_PATH`, `TELLER_ENV`, `TELLER_WEBHOOK_SECRET`. All editable via `/providers` admin page; uploaded PEMs are encrypted and stored in `app_config`. Env paths take precedence over DB PEMs.
- `NewClientFromPEM(certPEM, keyPEM)` builds an mTLS client from in-memory bytes.
- **Amount sign is opposite Plaid**: Teller negative=debit, Plaid positive=debit. Provider negates before returning — downstream sees Plaid convention.
- Sync: date-range polling with 10-day overlap (no cursor). Post-sync, soft-delete stale *pending* transactions not returned by the API. Posted transactions are **never** auto-deleted.
- Raw Teller category strings (`dining`, `groceries`, etc.) stored directly in `category_primary`. Rules handle categorization.

## SimpleFIN

- No SDK. Token-paste, poll-only protocol (`internal/provider/simplefin/`). Full details in `docs/simplefin-integration.md`.
- **Connect = claim.** `ExchangeToken` receives the pasted base64 *setup token*, decodes it to a one-time claim URL, `POST`s for the **access URL** (`https://user:pass@host/path`), and stores the encrypted access URL as the credential. `CreateLinkSession` returns `ErrNotSupported`.
- **`external_id` is minted** (`internal/shortid`) — SimpleFIN exposes no stable upstream id and a single access URL spans many banks. Reauth keeps the row and rotates only `encrypted_credentials` (`UpdateBankConnectionCredentials`).
- **Sign is inverted vs Plaid**: SimpleFIN positive = money in; negate uniformly (no per-account-type rule, unlike Teller).
- **Sync**: date-range polling chunked into **≤90-day windows** (bridge cap). Cursor = last-sync RFC3339 timestamp. `ReconcilesPendingByPolling()` is true. New connections default to a **daily** `sync_interval_override_minutes` (bridge expects ≤24 req/day).
- **Account discovery on sync.** One access URL spans every bank at the bridge, and that set grows as the user links more banks there. So each sync returns the connection's full current account set in `SyncResult.Accounts`; the engine upserts it (metadata only, via `UpsertAccountMetadata` — never balances) before processing transactions, so banks added at the bridge after connect appear automatically on the next sync — no reconnect needed. Fixed-account-set providers (Plaid, CSV) leave `SyncResult.Accounts` nil and the engine skips the upsert.
- `HandleWebhook`/`CreateReauthSession` → `ErrNotSupported`; `RemoveConnection` → `nil` (no revoke endpoint). Account type defaults to `depository` (not exposed by SimpleFIN).
- **Opt-in**: gated by `SIMPLEFIN_ENABLED` env / `simplefin_enabled` app_config (no server-level credential). Admin-page connect/relink only in v1 — no REST endpoint, not in the hosted-link allowlist.

## CSV

- Import-only. Provider interface methods return `ErrNotSupported` for sync/webhook/reauth, `nil` for `RemoveConnection`.
- Actual import bypasses the provider interface — calls the service layer directly from `internal/admin/csv_import.go`.
- Dedup: `external_transaction_id = SHA-256(account_id|date|amount|description)` per account. Standard `UpsertTransaction` ON CONFLICT handles it.

## Hot reload

`app.ReinitProvider(name)` rebuilds a provider after dashboard config changes. The sync engine holds a shared `map[string]provider.Provider` reference — same map, updated values, so running schedules pick up new config on the next tick.

## Adding a new provider

1. New subdir under `internal/provider/`.
2. Implement the full `Provider` interface. Unsupported methods return `ErrNotSupported`.
3. Add to provider type enum (`plaid`, `teller`, `csv`, ...).
4. Register in `app.InitProviders`.
5. Admin card on `/providers`.
6. Document in `docs/<provider>-integration.md`.
