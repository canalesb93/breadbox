# CSV import

Breadbox imports transactions from CSV/TSV exports. There are two paths:

- **v2 — drop anywhere (recommended).** Drag a CSV onto any page. A global overlay
  appears; on drop the import flow analyzes the file, detects which account it
  belongs to, dedupes against what's already there, shows a preview of exactly
  what will change, and applies only on confirm.
- **v1 — wizard page.** The legacy 4-step wizard at
  `/admin/connections/import-csv` (upload → map → preview → import). Still works;
  kept for now. v2 supersedes it.

This doc covers v2. v1 internals live in `internal/admin/csv_import.go`.

## The flow

```
drop CSV (any page)
  → POST /-/csv/v2/sessions            analyze: parse, detect template/mapping/
                                       currency, apply saved profile, rank accounts
  → [account step, if not auto-chosen]
      POST /-/csv/v2/sessions/{id}/resolve   bind/create account → classify rows
  → preview/edit
      GET   /-/csv/v2/sessions/{id}/rows           paginated rows + summary
      PATCH /-/csv/v2/sessions/{id}/rows/{rowId}   inline edit a row
      POST  /-/csv/v2/sessions/{id}/bulk           include/exclude/set-category/remap
  → POST /-/csv/v2/sessions/{id}/apply  upsert the included set, save the profile
```

All endpoints are admin-only (session-cookie auth), so they carry no OpenAPI
surface. The flow UI is `components.ImportModal` + `$store.csvImport` (drop
overlay + open/close) + the `csvImport` Alpine factory
(`static/js/admin/components/csv_import_v2.js`). See the live variant matrix at
`/design/c/csv-import`.

## Staging (durable, so preview == apply)

A drop creates a `csv_import_sessions` row holding the original bytes
(`raw_blob`), detected mapping, resolved account, and (after apply) a result
snapshot. Each parsed row is a `csv_import_rows` row carrying its parsed values,
classification, dedup hashes, and the user's include/exclude intent. Classifying
or remapping replaces the row set wholesale (delete + `:copyfrom`), so up to 50k
rows insert fast. Abandoned sessions are swept hourly past their 24h TTL
(`SweepExpiredImportSessions`, wired in `serve.go`).

Service layer: `internal/service/csv_import_v2.go` (lifecycle + apply),
`csv_classify.go` (dedup classification), `csv_account_match.go` (account
detection), `csv_profiles.go` (profile CRUD).

## Account detection

`MatchCSVAccounts` ranks existing accounts with a deterministic, explainable
weighted score (every point carries a reason string):

| Signal | Points |
|---|---|
| saved profile's default account | auto-preselect |
| mask / last-4 matches the account | +40 |
| transaction overlap (rows already present) | up to +40 |
| institution name appears in the account name | +15 |
| filename mentions the account | +5 |

The top account is **pre-selected** only when it scores ≥70 *and* leads the
runner-up by ≥20 (or a profile matched). Otherwise the user picks from the ranked
list or creates a new account inline. Confidence buckets: high ≥70, medium 40–69,
low <40.

## Deduplication

Per incoming row, against the **resolved account's** live transactions (any
provider — CSV, Plaid, Teller):

- **exact_dup** — the row's stable `provider_transaction_id`
  (`SHA-256(accountID|date|amount|desc)`) already exists, or its account-
  independent `content_hash` (`SHA-256(date|amount|desc)`) matches an existing
  row. Default: excluded.
- **probable_dup** — same amount, date within ±1 day, and a similar name (scored
  by the shared `internal/textmatch` similarity used by the sync matcher).
  Default: excluded.
- **new** — no match. Default: included.
- **error** — unparseable row; surfaced for inline fixing. Excluded.

The classifier loads candidates with one indexed query
(`transactions_dedup_lookup` on `(account_id, date, amount) WHERE deleted_at IS
NULL`). Apply upserts via `UpsertTransactionV2` (returns `(xmax = 0) AS inserted`
for reliable new-vs-updated counts) and is **idempotent** — re-dropping the same
file into the same account upserts to no-ops. A genuine same-key duplicate the
user force-imports gets an occurrence disambiguator so it inserts as a distinct
row.

Why re-dropping a later export "just adds the new ones": the overlapping rows hit
exact_dup (and the saved profile auto-resolves the account), so only the genuinely
new rows are classified `new` and imported.

## Profiles

A profile is a saved import recipe keyed by a **header fingerprint** (a hash of
the file's normalized header row). On a successful apply the profile is upserted:
created the first time a given header layout is imported, then bumped
(`times_used` / `last_used_at`) and updated to remember the resolved account. On a
future drop whose fingerprint matches, the flow auto-applies the saved
mapping/date-format/sign/currency and pre-selects the remembered account —
collapsing straight to a deduped preview.

Manage profiles via `GET/PATCH/DELETE /-/csv/v2/profiles[/{id}]`
(`internal/service/csv_profiles.go`). The user's rename survives future imports
(the upsert deliberately doesn't overwrite `name`).

## Currency

Detected from the file (currency symbols in the amount column), defaulting to the
target account's currency, confirmable in the flow. Amounts are stored as-is with
their `iso_currency_code`; nothing is converted (honoring the never-sum-across-
currency invariant).

## Schema

Additive migrations only (shared dev DB):

- `transactions.content_hash` + partial index `transactions_dedup_lookup`.
- `csv_import_profiles` (saved recipes; one per user+fingerprint).
- `csv_import_sessions` + `csv_import_rows` (durable staging).
- `csv_import_rows.category_id` (user-chosen Breadbox category applied at import).
