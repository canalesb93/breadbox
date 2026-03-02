# CSV Import Specification

## 1. Overview

CSV import lets users manually upload bank transaction data from CSV/TSV exports downloaded from their bank's website. It's a fallback for institutions not covered by Plaid or Teller, or for historical data that predates a connection.

**When to use CSV import:**
- Bank not supported by Plaid or Teller
- Importing historical transactions before the API connection was established
- Credit union or small institution with only CSV export
- One-time import of data from another financial tool

**Limitations vs Plaid/Teller:**
- No automatic sync — manual upload only
- No balance tracking (balances are not imported)
- No webhook support
- No link/reauth flow
- Deduplication is hash-based, not provider-ID-based (less precise)

## 2. Import Flow

Multi-step wizard at `/admin/connections/import-csv`:

### Step 1: Select Member + Upload

- User selects a family member from dropdown
- User uploads a CSV file (max 10MB, `.csv`/`.tsv`/`.txt` extensions)
- Server parses the file, returns column headers and first 10 rows for preview
- Auto-detects delimiter (comma, tab, semicolon, pipe) and encoding
- Strips BOM (UTF-8, UTF-16) if present
- Rejects files with fewer than 2 rows (header + 1 data row) or more than 50,000 rows

### Step 2: Column Mapping + Preview

- User maps columns to fields: date (required), amount (required), description (required), category (optional), merchant_name (optional)
- Auto-detection: if headers match a pre-built template, mapping is pre-filled
- Amount sign convention toggle: "Positive = debit" (default, matches Plaid) or "Positive = credit"
- Account name field (free text, defaults to "CSV Import")
- Preview table shows first 10 rows with mapped values and parsed dates/amounts
- Validation errors shown inline (unparseable dates, invalid amounts)

### Step 3: Confirm + Import

- Summary: row count, date range, account name, member
- "Import" button triggers the actual import
- Progress indicator (not real-time — import is fast enough to block)

### Step 4: Results

- Success: "Imported N transactions (M new, K updated)" with link to connection detail
- Partial failure: show count of skipped rows with reasons (unparseable date, missing required field)
- Full failure: error message with link to retry

## 3. Column Mapping

### Required Fields

| Field | Maps To | Notes |
|-------|---------|-------|
| `date` | `transactions.date` | Parsed from various formats (see Section 6) |
| `amount` | `transactions.amount` | Parsed from various formats (see Section 5) |
| `description` | `transactions.name` | Trimmed, max 500 chars |

### Optional Fields

| Field | Maps To | Notes |
|-------|---------|-------|
| `category` | `transactions.personal_finance_category_primary` | Stored as-is (not mapped to Plaid categories) |
| `merchant_name` | `transactions.merchant_name` | Trimmed, max 200 chars |

### Auto-Detection

Headers are compared case-insensitively against known patterns:

- **date**: `date`, `transaction date`, `posting date`, `post date`, `trans date`
- **amount**: `amount`, `transaction amount`, `debit/credit`, `value`
- **description**: `description`, `transaction description`, `memo`, `payee`, `name`, `details`
- **category**: `category`, `type`, `transaction type`
- **merchant_name**: `merchant`, `merchant name`, `payee name`

If all three required fields match, the template is auto-selected.

## 4. Pre-Built Templates

Each template specifies exact header names and sign convention for a specific bank's CSV export format.

### Chase (Credit Card)

- **Headers**: `Transaction Date`, `Post Date`, `Description`, `Category`, `Type`, `Amount`, `Memo`
- **Date column**: `Transaction Date`
- **Amount column**: `Amount`
- **Description column**: `Description`
- **Category column**: `Category`
- **Sign convention**: Negative = debit (charges), Positive = credit (payments)
- **Date format**: `MM/DD/YYYY`

### Chase (Checking/Savings)

- **Headers**: `Details`, `Posting Date`, `Description`, `Amount`, `Type`, `Balance`, `Check or Slip #`
- **Date column**: `Posting Date`
- **Amount column**: `Amount`
- **Description column**: `Description`
- **Sign convention**: Negative = debit, Positive = credit
- **Date format**: `MM/DD/YYYY`

### Bank of America

- **Headers**: `Date`, `Description`, `Amount`, `Running Bal.`
- **Date column**: `Date`
- **Amount column**: `Amount`
- **Description column**: `Description`
- **Sign convention**: Negative = debit, Positive = credit
- **Date format**: `MM/DD/YYYY`

### Wells Fargo

- **Headers**: (no header row — 5 positional columns)
- **Column order**: Date, Amount, *, *, Description
- **Sign convention**: Negative = debit, Positive = credit
- **Date format**: `MM/DD/YYYY`
- **Note**: Wells Fargo CSVs have no header row. Detection relies on column count (5) and date format in first column.

### Capital One

- **Headers**: `Transaction Date`, `Posted Date`, `Card No.`, `Description`, `Category`, `Debit`, `Credit`
- **Date column**: `Transaction Date`
- **Amount**: `Debit` column (positive values) with `Credit` column as negative. If both present, combine: amount = debit - credit.
- **Description column**: `Description`
- **Category column**: `Category`
- **Sign convention**: Debit/Credit are separate columns, both positive values
- **Date format**: `YYYY-MM-DD`

### Amex

- **Headers**: `Date`, `Description`, `Amount`, `Extended Details`, `Appears On Your Statement As`, `Address`, `City/State`, `Zip Code`, `Country`, `Reference`, `Category`
- **Date column**: `Date`
- **Amount column**: `Amount`
- **Description column**: `Description`
- **Category column**: `Category`
- **Sign convention**: Positive = debit (charges), Negative = credit (payments)
- **Date format**: `MM/DD/YYYY`

## 5. Amount Handling

### Parsing

Amounts are parsed with the following normalizations (applied in order):

1. Strip leading/trailing whitespace
2. Remove currency symbols (`$`, `€`, `£`, `¥`)
3. Remove thousands separators (commas in `1,234.56`)
4. Handle parenthetical negatives: `(123.45)` → `-123.45`
5. Parse as decimal with `shopspring/decimal`

### Sign Normalization

After parsing, the sign is adjusted based on the user's selected convention:

- **"Positive = debit"** (Plaid convention, default): store as-is (positive amount = money spent)
- **"Positive = credit"**: negate the amount (so positive CSV values become negative in DB, meaning credit/income)

The stored value follows the same convention as Plaid: **positive = debit** (money going out).

### Split Debit/Credit Columns

Some banks (e.g., Capital One) use separate debit and credit columns:
- If both columns have a value, use `debit - credit`
- If only debit has a value, use it as-is (positive = debit)
- If only credit has a value, negate it (stored as negative = credit)

## 6. Date Parsing

### Supported Formats

Dates are parsed by trying each format in order until one succeeds:

1. `MM/DD/YYYY` (e.g., `01/15/2024`) — most common US bank format
2. `YYYY-MM-DD` (e.g., `2024-01-15`) — ISO 8601
3. `M/D/YYYY` (e.g., `1/5/2024`) — US short
4. `MM-DD-YYYY` (e.g., `01-15-2024`)
5. `DD/MM/YYYY` (e.g., `15/01/2024`) — European (tried last due to ambiguity)
6. `YYYY/MM/DD` (e.g., `2024/01/15`)
7. `Mon DD, YYYY` (e.g., `Jan 15, 2024`)
8. `Month DD, YYYY` (e.g., `January 15, 2024`)

### Auto-Detection Strategy

Rather than trying formats per-row, the parser:

1. Takes the first 20 non-empty date values from the file
2. Tries each format against all 20 values
3. Picks the format that successfully parses the most values (must parse at least 90%)
4. If no format reaches 90%, returns an error asking the user to check the date column

This avoids the MM/DD vs DD/MM ambiguity problem — if all dates parse as both, the US format (MM/DD) wins by priority order.

## 7. Deduplication

### Hash Algorithm

Each imported transaction gets a deterministic `external_transaction_id`:

```
SHA-256(account_id | date | amount | description)
```

Where:
- `account_id` is the UUID of the CSV account (scoped to prevent cross-account collisions)
- `date` is the parsed date in `YYYY-MM-DD` format
- `amount` is the normalized amount as a string (e.g., `123.45`)
- `description` is the trimmed, lowercased description
- `|` is a literal pipe character delimiter

### Upsert Behavior

The generated `external_transaction_id` is passed to the existing `UpsertTransaction` query, which uses `ON CONFLICT (external_transaction_id) DO UPDATE`. This means:

- **First import**: all rows are inserted as new transactions
- **Re-import same file**: all rows match existing transactions, counts as "updated" (no-op in practice since values are identical)
- **Re-import with edits**: changed rows (different amount or description for the same date+amount+description hash) won't match — they'll be inserted as new transactions. This is intentional; the hash is the identity.

### Limitations

- Two transactions on the same date with the same amount and description will collide (treated as one). This is rare but possible (e.g., two identical $5.00 charges at the same merchant on the same day).
- Changing the sign convention toggle and re-importing will create duplicates (different amount → different hash).

## 8. Connection Model

### CSV as a Connection

Each CSV import creates (or reuses) a `bank_connections` row:

| Field | Value |
|-------|-------|
| `provider` | `csv` |
| `institution_name` | User-provided account name (e.g., "Chase Checking CSV") |
| `external_id` | Auto-generated UUID (one per connection) |
| `encrypted_credentials` | NULL (no credentials needed) |
| `status` | `active` |
| `user_id` | Selected family member |

### Account

Each CSV connection has exactly one account:

| Field | Value |
|-------|-------|
| `name` | Same as connection `institution_name` |
| `type` | `depository` (default) |
| `subtype` | `checking` (default) |
| `mask` | NULL |
| `balance_*` | NULL (CSV doesn't provide balances) |

### Multiple Imports

A CSV connection can receive multiple imports over time:
- Each import creates a `sync_logs` entry with `trigger = manual`
- New transactions are inserted, existing ones are updated (via hash-based dedup)
- The connection's `last_synced_at` is updated after each import

### Re-Import Flow

From the connection detail page, CSV connections show an "Import More" button that links to the import wizard with the `connection_id` pre-filled. This skips member selection and account naming (reuses existing values).

## 9. Provider Interface

The CSV provider implements the `Provider` interface with most methods stubbed:

| Method | Behavior |
|--------|----------|
| `CreateLinkSession` | Returns `provider.ErrNotSupported` |
| `ExchangeToken` | Returns `provider.ErrNotSupported` |
| `SyncTransactions` | Returns `provider.ErrNotSupported` (CSV uses direct import, not sync engine) |
| `GetBalances` | Returns `provider.ErrNotSupported` |
| `HandleWebhook` | Returns `provider.ErrNotSupported` |
| `CreateReauthSession` | Returns `provider.ErrNotSupported` |
| `RemoveConnection` | Returns `nil` (no external resource to clean up) |

The CSV import flow bypasses the provider interface entirely — it uses the service layer directly to create connections, accounts, and transactions. The provider stub exists only so the provider registry has a `"csv"` entry (used for connection display and validation).
