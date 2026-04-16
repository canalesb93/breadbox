---
paths:
  - "internal/sync/**"
---

# Sync engine

## Trigger paths

- `cron` — scheduled via `robfig/cron`. Fires at the minimum configured interval; checks each connection's staleness individually.
- `webhook` — provider push (Plaid `TRANSACTIONS_*`, Teller webhook events). Verified via HMAC where applicable.
- `manual` — admin "Sync Now" button or `POST /api/v1/connections/{id}/sync`.
- `initial` — first sync after connection creation (runs inline during onboarding).

## Atomicity

Each connection's sync runs inside a **single DB transaction**. Upserts, deletions, rule application, hit-count increments, and sync-log updates all commit together. A crash mid-sync rolls back cleanly.

## Per-sync timeout

Configurable context deadline (default 5 minutes). Exceeded timeouts mark the sync log as `error` with the deadline message.

## Orphaned sync logs

On startup, `MarkOrphanedSyncLogs()` sets any `in_progress` log to `error` with "interrupted by server restart". Prevents stuck connections after a crash or force-kill.

## Pause vs status

- `paused BOOLEAN` is orthogonal to `status`. **Only cron respects pause**; manual and webhook sync bypass it.
- `status` reflects last sync outcome: `active`, `error`, `pending_reauth`, `disconnected`.
- Per-connection `sync_interval_override_minutes` lets users customize cadence.

## In-sync hooks

After each transaction upsert inside the sync transaction:

1. **Transaction rules** — `ApplyRulesToTransaction` matches active non-expired rules filtered by `trigger × isNew`. Typed actions (`set_category`, `add_tag`, `add_comment`) execute atomically inside the same DB tx. `set_category` skips `category_override=true` rows. Batched hit-count increments via `BatchIncrementHitCounts`. Each fired action writes an annotation (`rule_applied`, `category_set`, `tag_added`, `comment`) for the unified audit log.
2. **Review tagging** — handled entirely by the rule engine. The seeded system rule (NULL conditions, `trigger=on_create`, action `add_tag: needs-review`) attaches `needs-review` to every newly-synced transaction. Disable that rule to opt out. No separate enqueue path.

Everything lives inside the sync tx — either it all commits or nothing does.

## Post-sync reconciliation

`matcher.ReconcileForConnection()` runs **after** the sync tx commits. Matches cross-connection duplicate transactions (e.g., same charge on a shared card) by date + exact amount:

- Single candidate → auto-match (`confidence: auto`).
- Multiple candidates → name similarity tiebreaker.
- Ambiguous → skip for manual review via admin UI or MCP `confirm_match`/`reject_match`.

Matched pairs go in `transaction_matches`; attribution flows through `transactions.attributed_user_id`.

## Account exclusion

`accounts.excluded = TRUE` skips the **transaction upsert** only — balances still refresh. Useful for accounts the user doesn't want cluttering totals but still wants synced.

`accounts.is_dependent_linked = TRUE` flags accounts linked to a primary cardholder account. Their transactions are excluded from totals at query time via `AND a.is_dependent_linked = FALSE`.

## Retries

`internal/provider/retry.go` wraps sync calls with exponential backoff on `ErrSyncRetryable`. `ErrReauthRequired` is **not** retried — it flips status to `pending_reauth` immediately.
