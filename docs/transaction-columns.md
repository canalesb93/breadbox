# Transaction columns — classification & contract

> **Status:** canonical · **Date:** 2026-06-19 · **Owner:** Ricardo
> **Parent plan:** [[rules-as-universal-substrate]] §4d · **Roadmap:** P3-T6
>
> This is the **canonical transaction-column contract** — every column on the
> `transactions` table, what class it belongs to, and whether a transaction
> rule may match on it. `docs/data-model.md` remains the **schema-of-record**
> (authoritative types, indexes, FKs); this doc is the **classification /
> contract layer** on top of it. The two must agree on the column set; if they
> drift, data-model.md wins on shape and this doc wins on classification.

## Doctrine recap

Provider data is **immutable substrate**: the `provider_*` columns, `amount`,
the date/datetime fields, `iso_currency_code`, and `pending` are written once
from the provider payload and never rewritten by Breadbox — they change only if
the provider re-reports the transaction. **Enrichment accrues on top** of that
substrate via rules, agents, and users (`category_id`, `series_id`, `flagged_at`,
`metadata`, and matcher-written `attributed_user_id`). Provenance bookkeeping was
**removed in P3**: the `category_override` source-enum is dropped, and category
writes follow plain **last-writer-wins** gated by `isNew || isChanged` (a rule
only writes when the row is new or the assigned value actually changes). There is
no longer a per-row "who set this" stamp on `transactions`; authorship lives in
the activity/event log, not on the ledger row.

## Column inventory

Grouped identity → raw-provider → enriched → lifecycle. Types are the Postgres
column types from the migration history (`internal/db/migrations/`), cross-checked
against the sqlc `Transaction` model (`internal/db/models.go`).

The **DSL tag** column applies only to rule-matchable columns and uses the
stability vocabulary from `docs/rule-dsl.md`: **raw-immutable** (verbatim provider
data — author here), **stable-derived** (a pure function of a raw-immutable column),
**mutable-display** (a label or prior-stage rule write — discouraged but allowed).

| Column | Type | Class | Rule-matchable | Condition field / DSL tag | Notes |
|---|---|---|---|---|---|
| `id` | `UUID` PK | identity | no | — | Internal surrogate; never a public selector or condition source. |
| `short_id` | `TEXT` NOT NULL UNIQUE | identity | no | — | 8-char base62 public alias (trigger-generated). Selector, not a condition. |
| `account_id` | `UUID` FK→accounts (SET NULL) | identity | **yes** | `account_id` | FK identity. The ID is stable; its display variant `account_name` is **mutable-display** (renames break it). Prefer the ID. |
| `provider_transaction_id` | `TEXT` NOT NULL UNIQUE | raw-provider | no | — | Provider's stable id; the upsert/dedup key. Not exposed as a condition field. |
| `provider_pending_transaction_id` | `TEXT` NULL | raw-provider | no | — | Links a posted txn back to the pending one it replaced. Not a condition field. See naming reassessment. |
| `amount` | `NUMERIC(12,2)` NOT NULL | raw-provider | **yes** | `amount` · raw-immutable | Provider-reported amount; positive = money out. Supports `approx` (±tolerance) and `between`. Never compare across `iso_currency_code`. |
| `iso_currency_code` | `TEXT` NULL | raw-provider | no | — | Currency of `amount`. Not a condition field; it scopes amount comparisons rather than being matched directly. |
| `date` | `DATE` NOT NULL | raw-provider | **indirect** | (date-parts) · stable-derived | The posting date (tz-naive). Not a scalar condition field itself; matched via the derived `day_of_month` / `month` / `day_of_week` / `day_of_year` fields. |
| `authorized_date` | `DATE` NULL | raw-provider | no | — | Provider authorization date. Not a condition field. |
| `datetime` | `TIMESTAMPTZ` NULL | raw-provider | no | — | Provider-supplied posting timestamp. Not a condition field. |
| `authorized_datetime` | `TIMESTAMPTZ` NULL | raw-provider | no | — | Provider authorization timestamp. Not a condition field. |
| `provider_name` | `TEXT` NOT NULL | raw-provider | **yes** | `provider_name` · raw-immutable | Provider's raw transaction name. The primary durable text match. |
| `provider_merchant_name` | `TEXT` NULL | raw-provider | **yes** | `provider_merchant_name` · raw-immutable | Provider's raw merchant string; may be empty. |
| `provider_category_primary` | `TEXT` NULL | raw-provider | **yes** | `provider_category_primary` · raw-immutable | Provider's primary category — never rewritten by Breadbox. |
| `provider_category_detailed` | `TEXT` NULL | raw-provider | **yes** | `provider_category_detailed` · raw-immutable | Provider's detailed category — never rewritten by Breadbox. |
| `provider_category_confidence` | `TEXT` NULL | raw-provider | no | — | Provider's confidence label. Stored but not a condition field. |
| `provider_payment_channel` | `TEXT` NULL | raw-provider | no | — | Provider channel (`online`/`in store`/…). Stored but not a condition field. |
| `pending` | `BOOLEAN` NOT NULL DEFAULT FALSE | raw-provider | **yes** | `pending` · raw-immutable | Provider-reported pending flag. Classed raw-provider (provider truth), though it flips when the posted txn lands — a lifecycle flavor the DSL still treats as raw-immutable. |
| `category_id` | `UUID` FK→categories (SET NULL) | enriched | **yes** | `category` · mutable-display | Assigned category. Matched as the resolved slug (`category`), distinct from the raw `provider_category_*`. Mutates mid-pass as earlier `set_category` rules fire — discouraged as a primary condition. |
| `attributed_user_id` | `UUID` FK→users (SET NULL) | enriched | **yes** | `user_id` / `user_name` · mutable-display | Matcher-written attribution override, sourced from user-configured `account_links`. Queries filter on `COALESCE(attributed_user_id, bc.user_id)`. `user_id` (ID) is stable; `user_name` is mutable-display. |
| `series_id` | `UUID` FK→recurring_series (SET NULL) | enriched | **yes** | `series` / `in_series` · mutable-display | Recurring-series occurrence link, set only by `assign_series` rules / first-class agent assigns. `series` = the series `short_id`; `in_series` = bool. Pipeline-ordered — discouraged as a primary condition. |
| `metadata` | `JSONB` NOT NULL DEFAULT `'{}'` | enriched | **yes** | `metadata.<key>` · mutable-display | Free-form enrichment blob written by users/agents/rules. `clear` writes `'{}'`, never NULL. No stability guarantee. |
| `flagged_at` | `TIMESTAMPTZ` NULL | enriched | no | — | "Look at this" marker set by an agent/user (`flag`/`unflag` actions). The flag *reason* is a comment event, not a column. No condition field. |
| `deleted_at` | `TIMESTAMPTZ` NULL | lifecycle | no | — | Soft-delete tombstone. NULL = live. Active queries filter `deleted_at IS NULL`. |
| `created_at` | `TIMESTAMPTZ` NOT NULL DEFAULT NOW() | lifecycle | no | — | Row insert time. |
| `updated_at` | `TIMESTAMPTZ` NOT NULL DEFAULT NOW() | lifecycle | no | — | Row mutation time. |

26 columns. The provider's full raw payload is **not** on this table — it lives
1:1 in `transaction_provider_payloads.provider_raw` (moved off the hot ledger in
migration `20260614055107`), joinable by `transaction_id`.

### Notes on the tricky classifications

- **`pending`** straddles raw-provider and lifecycle: it *is* provider truth at
  sync time, but it transitions (pending → posted) over a transaction's life. It
  is filed under **raw-provider** because that's how the DSL treats it
  (raw-immutable) and because Breadbox never authors it — the provider does.
- **`attributed_user_id`** is **enriched**, not identity: it is not the
  transaction's own identity but a Breadbox-computed attribution, written by the
  matcher from user-configured `account_links` (and overridable by a user). The
  effective owner at query time is `COALESCE(attributed_user_id, bc.user_id)`.
- **`metadata`** is enriched even though it is schemaless — the keys are
  user/agent intent, not provider data.

### Condition fields with no backing `transactions` column

A few rule condition fields resolve via joins or derivation rather than a column
on this table, and so do not appear above: `provider` (the source system, from the
account's connection — raw-immutable), `account_name` / `user_name` (joined display
labels — mutable-display), `tags` (from `transaction_tags` — mutable-display), and
the date-part fields below.

## Derived matchable fields (not columns)

These are computed at evaluation time from immutable columns — they have no
stored scalar but are first-class condition fields. Plus the amount-tolerance
operators, which match `amount` without a separate field.

| Field / operator | Type | Derives from | DSL tag | Notes |
|---|---|---|---|---|
| `day_of_month` | numeric `1`–`31` | `date` | stable-derived | `approx` is **cyclic + clamped** to the txn's own month length (1st ≈ last day; "the 31st" fires on Feb 28/29). |
| `month` | numeric `1`–`12` | `date` | stable-derived | January = 1. |
| `day_of_week` | numeric `0`–`6` | `date` | stable-derived | `0` = Sunday … `6` = Saturday (Go `time.Weekday`). |
| `day_of_year` | numeric `1`–`366` | `date` | stable-derived | Drifts by one after Feb in leap years — for annual cadence use `month` + `day_of_month` instead. |
| `amount … approx` | operator | `amount` | raw-immutable | `abs(amount − value) ≤ tolerance`; requires a sibling `tolerance ≥ 0`. |
| `amount … between` | operator | `amount` | raw-immutable | `min ≤ amount ≤ max` (inclusive); requires `min` and `max`, `min ≤ max`. |

**Hygiene.** Author durable rules on **raw-immutable** and **stable-derived**
fields — they resolve identically on the create pass, on every re-sync, and on
retroactive apply. Enriched/mutable columns (`category`, `series`/`in_series`,
`tags`, `metadata.<key>`) are **discouraged-but-allowed** as condition sources:
their truth depends on pipeline order and on prior writes, so they break silently
when something upstream changes. See the stability contract in `docs/rule-dsl.md`.

## Naming reassessment — FOR USER REVIEW

**Proposals only — none of these are applied.** Column naming is the user's call;
this section exists so the decisions are captured in one place. Reconciled against
the prior floats in `docs/v1-schema-api-proposal.md` (§ transactions row of the
table-rename inventory).

| Current | Proposed | Rationale |
|---|---|---|
| `provider_pending_transaction_id` | `replaced_pending_provider_id` | Already floated in `v1-schema-api-proposal.md`. The current name reads as "a pending id from the provider"; the column actually points from a *posted* txn to the *pending* provider id it superseded. The proposed name states the relationship. **Still live — worth deciding before v1.** |
| `category_override` | ~~`category_source`~~ | **Moot.** The v1 proposal floated retyping it to a `category_source` ENUM (`none`/`rule`/`agent`/`user`); P3 **drops the column entirely** in favor of last-writer-wins, so there is nothing to rename. Listed only to close the loop with the prior proposal. |
| `provider_payment_channel` | (keep) | Consistent with the `provider_*` prefix doctrine; no change. |
| `datetime` / `authorized_datetime` | (consider `provider_datetime` / `provider_authorized_datetime`) | Minor: these are raw provider timestamps but lack the `provider_*` prefix the rest of the raw substrate carries. A prefix would make "raw vs enriched" unambiguous at the schema level (the stated intent of migration `20260424184638`). Low priority — flagged for consistency only. |

---

> **Footnote — columns removed before this contract.**
> `category_override` (the `none`/`agent`/`user` source-enum) was **dropped in P3**
> (migration `20260620031250_drop_category_override.sql`); category provenance is
> deferred — there is no per-row authorship stamp on `transactions` anymore
> (authorship lives in the activity/event log). Category writes are
> last-writer-wins; the sync engine only re-runs rules on `isNew||isChanged`
> transactions, so a manual edit on an unchanged row is not re-clobbered.
> `merchant_key` (the detector's normalized anchor) was dropped in **P2**
> (migration `20260620023318_recurring_series_thin.sql`) along with the fat
> `recurring_series` table — series membership is now rule-maintained.
