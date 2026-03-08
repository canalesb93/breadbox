# Phase 28: Dashboard — Transaction UX & Filtering

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## 1. Overview

Phase 28 upgrades the admin transaction list page (`/admin/transactions`) from a basic filterable table into a power-user transaction browsing and management tool. It adds composable multi-select filters, saved filter presets, inline bulk actions, a transaction detail slide-out panel, full-text search with debounce, and CSV export of filtered results.

This phase builds on the existing dynamic query builder in `internal/service/transactions.go`, the current filter bar and expandable-row pattern in `transactions.html`, and the Alpine.js + DaisyUI interaction patterns used throughout the dashboard.

---

## 2. Goals

- **Composable filtering:** Users can combine any number of filters (category, account, user, date range, amount range, pending status, search) and see results update via the URL, making filters bookmarkable and shareable.
- **Saved presets:** Users can save frequently used filter combinations (e.g., "Groceries this month", "Large transactions > $500") and load them with one click.
- **Bulk actions:** Select multiple transactions and apply batch operations (re-categorize, add comment) without editing one by one.
- **Rich detail panel:** A slide-out panel replaces the current inline expand row, showing full transaction details and supporting inline edits (category override, notes).
- **Search:** Type-ahead search on description and merchant name with debounce, integrated into the filter bar.
- **CSV export:** Download the current filtered result set as a CSV file for use in spreadsheets or external tools.

---

## 3. Filter Bar Design

### 3.1 Current State

The current filter bar (`transactions.html`) is a `<form>` with `method="GET"` submitting to `/admin/transactions`. It has individual `<select>` and `<input>` elements for: start date, end date, connection, account, family member, category (primary only), min amount, max amount, pending, and search. A "Filter" button submits the form, and a "Clear" link resets all filters. Filters are preserved in query parameters across pagination.

### 3.2 Enhanced Filter Bar

**Layout:** Keep the `<form method="GET">` approach — all filters are URL query parameters. This preserves bookmarkability, back-button behavior, and pagination compatibility without JavaScript state management.

**Changes from current implementation:**

1. **Multi-select for category, account, user, connection.** Replace single `<select>` elements with multi-select dropdowns. Since DaisyUI does not ship a multi-select component, implement using Alpine.js dropdown menus with checkbox items inside a `dropdown` component.

2. **Category hierarchy.** If Phase 20B (category taxonomy) is complete, the category filter shows a two-level tree: primary categories as group headers, detailed categories as checkable items underneath. If Phase 20B is not yet complete, continue showing the flat list of distinct `category_primary` values. Use query param `category_slug` (list) for the new system, or `category` (single string) for the legacy flat list.

3. **Date range with presets.** Keep the two date inputs but add a `<select>` with quick-pick presets: "Today", "This week", "This month", "Last 30 days", "Last 90 days", "This year", "Custom". Selecting a preset populates the start/end date inputs via Alpine.js. Selecting "Custom" shows the date inputs (already visible by default).

4. **Amount range.** Keep the two number inputs (min/max) unchanged. They work well as-is.

5. **Search input.** Move the search input to the top-left of the filter bar, prominently positioned. Add a magnifying glass icon (`search` Lucide icon) inside the input. The search input submits with the form on Enter like today. No client-side debounce is needed for form submission — debounce is only relevant for the future AJAX-based filtering (out of scope for Phase 28).

6. **Active filter badges.** Below the filter bar, show a row of `badge` elements for each active filter with an "x" button to remove that individual filter. Clicking the "x" re-submits the form with that parameter removed. A "Clear all" link at the end removes all filters.

7. **Filter bar collapse.** On mobile, the filter bar is collapsed behind a "Filters" toggle button. Use DaisyUI `collapse` with Alpine.js toggle. Active filter count shown on the toggle button: "Filters (3)".

### 3.3 Multi-Select Dropdown Component

Since DaisyUI lacks a built-in multi-select, build a reusable Alpine.js pattern:

```html
<div class="dropdown" x-data="{ open: false }">
  <label tabindex="0" class="btn btn-sm btn-ghost gap-1" @click="open = !open">
    Accounts
    <span class="badge badge-sm" x-show="selectedCount > 0" x-text="selectedCount"></span>
    <i data-lucide="chevron-down" class="w-3 h-3"></i>
  </label>
  <div class="dropdown-content z-10 bg-base-100 shadow-lg rounded-box w-64 max-h-60 overflow-y-auto p-2"
       x-show="open" @click.outside="open = false" x-cloak>
    <label class="flex items-center gap-2 px-2 py-1 hover:bg-base-200 rounded cursor-pointer"
           x-for="item in items" :key="item.value">
      <input type="checkbox" :name="paramName" :value="item.value"
             class="checkbox checkbox-xs" :checked="item.selected">
      <span class="text-sm" x-text="item.label"></span>
    </label>
  </div>
</div>
```

For the `account_id` multi-select, the query parameter format is repeated keys: `?account_id=uuid1&account_id=uuid2`. The Go handler parses via `r.URL.Query()["account_id"]` (string slice).

### 3.4 URL State Management

All filter state lives in URL query parameters. No Alpine.js store or localStorage is used for filter state. This guarantees:

- Bookmarkable URLs
- Browser back/forward works
- Pagination links carry all active filters (already implemented)
- Presets can be represented as simple URLs

**Query parameters (full list):**

| Parameter | Type | Description |
|---|---|---|
| `page` | int | Page number (1-based) |
| `start_date` | string (YYYY-MM-DD) | Inclusive start date |
| `end_date` | string (YYYY-MM-DD) | Inclusive end date |
| `account_id` | string[] (repeated) | Filter by account UUID(s) |
| `user_id` | string[] (repeated) | Filter by family member UUID(s) |
| `connection_id` | string[] (repeated) | Filter by connection UUID(s) |
| `category` | string[] (repeated) | Filter by category primary (legacy) or slug |
| `min_amount` | float | Minimum transaction amount |
| `max_amount` | float | Maximum transaction amount |
| `pending` | string ("true"/"false") | Filter by pending status |
| `search` | string | Full-text search on name/merchant_name |
| `sort` | string ("asc"/"desc") | Sort order for date |
| `preset` | string (UUID) | Load saved preset (server resolves to filters) |

### 3.5 Multi-Value Filter Service Layer Changes

The current `AdminTransactionListParams` uses `*string` for `AccountID`, `UserID`, `ConnectionID`, and `Category`. These need to change to `[]string` to support multi-select:

```go
type AdminTransactionListParams struct {
    Page          int
    PageSize      int
    StartDate     *time.Time
    EndDate       *time.Time
    AccountIDs    []string   // was *string
    UserIDs       []string   // was *string
    ConnectionIDs []string   // was *string
    Categories    []string   // was *string
    MinAmount     *float64
    MaxAmount     *float64
    Pending       *bool
    Search        *string
    SortOrder     string
}
```

The dynamic query builder uses `ANY($N)` for multi-value filters:

```sql
AND t.account_id = ANY($1)   -- where $1 is a UUID array
```

**Backward compatibility:** If only one value is provided, the behavior is identical to today. Empty slices mean "no filter" (same as nil pointer today).

---

## 4. Saved Presets

### 4.1 Data Model

Add a new `filter_presets` table:

```sql
CREATE TABLE filter_presets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    filters     JSONB NOT NULL,        -- serialized filter parameters
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,  -- show in quick-access bar
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Why a table instead of `app_config`:** Presets are structured entities with their own lifecycle (CRUD, ordering, defaults). Stuffing JSON arrays into `app_config` would make querying and ordering cumbersome.

### 4.2 Filters JSON Structure

The `filters` JSONB column stores the filter parameters as a JSON object:

```json
{
  "start_date": "2026-01-01",
  "end_date": "2026-01-31",
  "account_ids": ["uuid1", "uuid2"],
  "user_ids": [],
  "connection_ids": [],
  "categories": ["groceries", "dining_out"],
  "min_amount": null,
  "max_amount": 500.00,
  "pending": null,
  "search": "",
  "sort": "desc"
}
```

Null/empty values mean "no filter applied for this dimension."

### 4.3 Preset UI

**Save current filters:** A "Save as preset" button appears in the filter bar action area (next to Filter/Clear buttons). Clicking it opens a DaisyUI `modal` dialog with:

- Name input (required, max 100 chars)
- "Show in quick bar" checkbox (sets `is_default`)
- Save / Cancel buttons

**Quick-access bar:** Below the page title and above the filter bar, show preset badges for all presets where `is_default = true`. Clicking a badge navigates to `/admin/transactions?preset={id}`. The active preset badge is highlighted with `badge-primary`.

**Manage presets:** A "Manage presets" link (gear icon) opens a `modal` listing all presets with:

- Drag-to-reorder (Alpine.js sortable, updates `sort_order`)
- Rename inline
- Toggle default visibility
- Delete with confirmation

### 4.4 Preset Resolution

When `?preset={id}` is present in the URL:

1. Server loads the preset by ID
2. Extracts filter parameters from the `filters` JSONB
3. Merges them into `AdminTransactionListParams` (any explicit URL params override the preset values)
4. The preset name is passed to the template for display ("Viewing preset: Groceries Jan 2026")

### 4.5 Default Presets

No system-provided presets are seeded. The presets table starts empty. Users create their own.

---

## 5. Search

### 5.1 Current State

Search is already implemented. The `search` query parameter triggers `ILIKE` matching on `t.name` and `t.merchant_name` in the dynamic query builder. A GIN trigram index (`transactions_name_merchant_gin_idx`) supports this.

### 5.2 Enhancements

1. **Prominent placement.** Move the search input to the top of the filter bar, full width on mobile, left-aligned on desktop. Use DaisyUI `input` with a leading search icon.

2. **Minimum length.** Keep the 2-character minimum from the REST API. The admin handler currently has no minimum — add validation to skip the search filter if the search string is less than 2 characters (silently ignore, don't error).

3. **No client-side debounce.** Since the filter bar is a standard form submission (full page load), debounce is unnecessary. If future work moves to AJAX-based filtering, add 300ms debounce at that point.

4. **Search result highlighting.** Optionally, the template can bold the matching substring in the Name/Merchant columns using a Go template function. This is a nice-to-have and can be deferred.

### 5.3 Fields Searched

- `transactions.name` (transaction description)
- `transactions.merchant_name`

No changes to the set of searched fields.

---

## 6. Bulk Actions

### 6.1 Selection Model

**Checkbox per row:** Add a checkbox column as the first column of the transaction table. Each checkbox stores the transaction ID.

**Select all on page:** A checkbox in the table header selects/deselects all visible transactions on the current page. This is a client-side operation (Alpine.js).

**Select all matching filter:** When all on the current page are selected, show a banner: "All 50 transactions on this page are selected. Select all {N} transactions matching this filter?" Clicking "Select all {N}" sets a flag that the bulk action should apply to the full filter result set (server-side), not just the visible page.

**Selection state:** Managed in Alpine.js via an `x-data` object on the page:

```js
x-data="{
  selected: [],              // array of transaction IDs
  selectAll: false,          // header checkbox state
  selectAllMatching: false,  // true = apply to full filter set
  totalMatching: {{ .Total }},

  toggleAll() {
    if (this.selectAll) {
      this.selected = [/* all IDs on page */];
    } else {
      this.selected = [];
      this.selectAllMatching = false;
    }
  },

  get selectedCount() {
    return this.selectAllMatching ? this.totalMatching : this.selected.length;
  }
}"
```

### 6.2 Available Bulk Actions

Display a floating action bar at the bottom of the page when `selected.length > 0`:

```html
<div class="fixed bottom-4 left-1/2 -translate-x-1/2 z-40 bg-base-100 shadow-xl rounded-box px-4 py-2 flex items-center gap-3 border border-base-300"
     x-show="selected.length > 0" x-cloak x-transition>
  <span class="text-sm font-medium" x-text="selectedCount + ' selected'"></span>
  <button class="btn btn-sm btn-primary" @click="showBulkCategorize = true">
    <i data-lucide="tag" class="w-4 h-4"></i> Categorize
  </button>
  <button class="btn btn-sm btn-ghost" @click="selected = []; selectAllMatching = false">
    Cancel
  </button>
</div>
```

**Actions:**

| Action | Description | Endpoint |
|---|---|---|
| Bulk categorize | Set category on N transactions (overrides existing) | `POST /admin/api/transactions/bulk/categorize` |
| Bulk add comment | Add a comment to N transactions (requires Phase 21) | `POST /admin/api/transactions/bulk/comment` |

### 6.3 Bulk Categorize Flow

1. User selects transactions and clicks "Categorize"
2. A DaisyUI `modal` opens with a category dropdown (same two-level tree as the filter bar)
3. The modal shows: "Apply category '{name}' to {N} transactions?"
4. User confirms
5. Client sends POST to `/admin/api/transactions/bulk/categorize`:

```json
{
  "transaction_ids": ["uuid1", "uuid2"],
  "category_slug": "groceries",
  "select_all_matching": false,
  "filters": {}
}
```

If `select_all_matching` is true, `transaction_ids` is ignored and `filters` contains the current filter set. The server applies the same dynamic query builder to identify the target transactions.

6. Server response: `{ "updated_count": 42 }`
7. Page reloads to reflect changes, toast confirms: "42 transactions categorized as Groceries"

### 6.4 Bulk Categorize Service Layer

```go
type BulkCategorizeParams struct {
    TransactionIDs     []string
    CategorySlug       string
    SelectAllMatching  bool
    Filters            AdminTransactionListParams  // used when SelectAllMatching is true
}

func (s *Service) BulkCategorizeTransactions(ctx context.Context, params BulkCategorizeParams) (int64, error)
```

The implementation runs an `UPDATE transactions SET category_primary = $1, category_override = TRUE, updated_at = NOW() WHERE id = ANY($2)` (or builds the filter-based WHERE clause when `SelectAllMatching` is true). If the category system (Phase 20B) is implemented, it sets `category_id` instead of `category_primary`.

### 6.5 Bulk Comment (Phase 21 Dependency)

This action is only available if Phase 21 (transaction comments) has been implemented. If the `transaction_comments` table does not exist, the "Add comment" button is hidden. The bulk comment action inserts a comment record for each selected transaction with the same text.

### 6.6 Confirmation Flow

All bulk actions require explicit confirmation via a modal dialog. The modal states the action, the number of affected transactions, and has "Confirm" / "Cancel" buttons. No `alert()` or `confirm()` — inline DaisyUI modals only, consistent with the existing pattern in `connection_detail.html`.

---

## 7. Transaction Detail Panel

### 7.1 Current State

Transactions currently expand inline using an Alpine.js `expanded` toggle on each `<tbody>`. The expanded row shows a `bb-info-grid` with merchant, transaction ID, account, family member, created/updated timestamps. There are no edit capabilities.

### 7.2 Slide-Out Panel Design

Replace the inline expand with a DaisyUI `drawer` component that slides in from the right side of the page. This provides more space for detail and edit controls.

**Trigger:** Clicking a transaction row opens the panel. The clicked row gets a visual highlight (`bg-primary/10`).

**Panel width:** `w-96` (24rem) on desktop, full-width on mobile.

**Implementation:** Use a right-side drawer within the page content area:

```html
<div class="drawer drawer-end" x-data="txnDetail">
  <input id="txn-detail-drawer" type="checkbox" class="drawer-toggle" x-model="open" />
  <div class="drawer-content">
    <!-- Transaction table goes here -->
  </div>
  <div class="drawer-side z-30">
    <label for="txn-detail-drawer" class="drawer-overlay"></label>
    <div class="bg-base-100 min-h-full w-96 p-6 border-l border-base-300">
      <!-- Detail content loaded via Alpine.js -->
    </div>
  </div>
</div>
```

### 7.3 Panel Contents

The panel shows all transaction fields in a structured layout:

**Header section:**
- Transaction name (large text)
- Merchant name (smaller, muted)
- Amount with currency (large, right-aligned, colored for debit/credit)
- Date
- Pending badge (if applicable)

**Details section** (`bb-info-grid`):
- Account name (link to account detail)
- Institution name
- Family member
- Payment channel
- Category (with edit button)
- Transaction ID (monospace, truncated with copy button)
- External transaction ID
- Authorized date/datetime (if present)
- Created at / Updated at timestamps

**Edit section:**
- **Category override:** A dropdown to change the transaction's category. Sends `POST /admin/api/transactions/{id}/category` with `{ "category_slug": "..." }`. Sets `category_override = true`.
- **Notes/comments:** A textarea for adding notes (requires Phase 21). If Phase 21 is not complete, this section is hidden.

### 7.4 Data Loading

**Option A (recommended): Embed data in the row, read on click.**

The transaction table already renders most fields. Store additional detail fields as `data-*` attributes on the `<tr>` element. When a row is clicked, Alpine.js reads the data attributes and populates the panel. No AJAX request needed for the initial view.

```html
<tr @click="openDetail($el.dataset)"
    data-id="{{.ID}}"
    data-account-id="{{.AccountID}}"
    data-account-name="{{.AccountName}}"
    data-institution="{{.InstitutionName}}"
    data-user="{{.UserName}}"
    data-date="{{.Date}}"
    data-name="{{.Name}}"
    data-merchant="{{if .MerchantName}}{{.MerchantName}}{{end}}"
    data-amount="{{printf "%.2f" .Amount}}"
    data-currency="{{if .IsoCurrencyCode}}{{.IsoCurrencyCode}}{{end}}"
    data-category="{{if .CategoryPrimary}}{{.CategoryPrimary}}{{end}}"
    data-pending="{{.Pending}}"
    data-created="{{.CreatedAt}}"
    data-updated="{{.UpdatedAt}}"
    class="cursor-pointer hover:bg-base-300">
```

This avoids an extra HTTP request per click and keeps the panel snappy. The trade-off is that some fields not shown in the table (authorized_date, payment_channel) need to be added to the `AdminTransactionRow` struct and the admin query.

### 7.5 Panel Close

- Click the drawer overlay
- Press Escape (Alpine.js `@keydown.escape.window`)
- Click a close button ("x" icon) in the panel header

---

## 8. CSV Export

### 8.1 Endpoint

`GET /admin/transactions/export.csv`

This is a new admin route (authenticated, same auth middleware as all `/admin/` routes). It accepts the same filter query parameters as the transaction list page. It streams all matching transactions (no pagination limit) as a CSV response.

### 8.2 Response Headers

```
Content-Type: text/csv; charset=utf-8
Content-Disposition: attachment; filename="breadbox-transactions-2026-03-08.csv"
```

The filename includes the current date. If filters are active, the filename could optionally include a hint: `breadbox-transactions-groceries-2026-03-08.csv`.

### 8.3 CSV Fields

| Column Header | Source Field | Notes |
|---|---|---|
| Date | `date` | YYYY-MM-DD format |
| Description | `name` | Raw transaction name |
| Merchant | `merchant_name` | May be empty |
| Amount | `amount` | Decimal, 2 places, no currency symbol |
| Currency | `iso_currency_code` | e.g., "USD" |
| Category | `category_primary` | Primary category string |
| Account | Account display name | `COALESCE(display_name, name)` |
| Institution | `institution_name` | From bank_connections |
| Family Member | `user_name` | From users via bank_connections |
| Pending | `pending` | "true" or "false" |
| Transaction ID | `id` | UUID |

### 8.4 Implementation

The export handler reuses the existing `AdminTransactionListParams` parsing logic from `TransactionListHandler` but:

- Sets `PageSize` to a large value (e.g., 100,000) or removes pagination entirely
- Streams rows directly to the `http.ResponseWriter` using Go's `encoding/csv` package
- Does not load all rows into memory at once — use the `rows.Next()` loop to write each row as it's scanned

### 8.5 UI Integration

Add an "Export CSV" button in the filter bar action area, next to "Filter" and "Clear":

```html
<a href="/admin/transactions/export.csv?{{currentFilterQueryString}}"
   class="btn btn-sm btn-outline self-end">
  <i data-lucide="download" class="w-4 h-4"></i>
  Export CSV
</a>
```

The link includes all current filter parameters so the export matches what the user sees. A Go template function `filterQueryString` builds the query string from the current filter values.

### 8.6 Limits

- Maximum 100,000 rows per export (return 413 if exceeded, with a message suggesting narrower filters)
- No background jobs — the export is synchronous. For the expected data volumes (family finance, typically < 50K transactions total), this is acceptable.

---

## 9. Service Layer Changes

### 9.1 Modified Functions

| Function | Change |
|---|---|
| `ListTransactionsAdmin` | Support `[]string` for AccountIDs, UserIDs, ConnectionIDs, Categories. Use `= ANY($N)` SQL. |
| `AdminTransactionListParams` | Change `*string` fields to `[]string` for multi-value filters. |
| `AdminTransactionRow` | Add `PaymentChannel`, `AuthorizedDate`, `CategoryDetailed` fields for the detail panel. |

### 9.2 New Functions

| Function | Purpose |
|---|---|
| `BulkCategorizeTransactions(ctx, BulkCategorizeParams) (int64, error)` | Update category on multiple transactions by ID list or filter. |
| `ExportTransactionsCSV(ctx, w io.Writer, params AdminTransactionListParams) error` | Stream filtered transactions as CSV rows. Reuses the same query builder. |
| `CreateFilterPreset(ctx, name string, filters json.RawMessage, isDefault bool) (*FilterPreset, error)` | Insert a new filter preset. |
| `ListFilterPresets(ctx) ([]FilterPreset, error)` | List all presets ordered by sort_order. |
| `GetFilterPreset(ctx, id string) (*FilterPreset, error)` | Fetch a single preset by ID. |
| `UpdateFilterPreset(ctx, id string, updates FilterPresetUpdate) error` | Update name, filters, is_default, sort_order. |
| `DeleteFilterPreset(ctx, id string) error` | Delete a preset. |

### 9.3 New Types

```go
type FilterPreset struct {
    ID        string          `json:"id"`
    Name      string          `json:"name"`
    Filters   json.RawMessage `json:"filters"`
    IsDefault bool            `json:"is_default"`
    SortOrder int             `json:"sort_order"`
    CreatedAt string          `json:"created_at"`
    UpdatedAt string          `json:"updated_at"`
}

type FilterPresetUpdate struct {
    Name      *string          `json:"name"`
    Filters   *json.RawMessage `json:"filters"`
    IsDefault *bool            `json:"is_default"`
    SortOrder *int             `json:"sort_order"`
}

type BulkCategorizeParams struct {
    TransactionIDs    []string
    CategorySlug      string
    SelectAllMatching bool
    Filters           AdminTransactionListParams
}
```

---

## 10. Dashboard UI

### 10.1 Page Layout

```
┌─────────────────────────────────────────────────────────────┐
│  Transactions                               [Export CSV]    │
├─────────────────────────────────────────────────────────────┤
│  [Preset 1] [Preset 2] [Preset 3]  ...  [Manage ⚙]        │  ← Quick preset bar
├─────────────────────────────────────────────────────────────┤
│  🔍 Search transactions...                                  │  ← Prominent search
│                                                             │
│  [Date range ▾] [Accounts ▾] [Members ▾] [Categories ▾]   │  ← Multi-select filters
│  [Amount: min __ max __] [Pending ▾]  [Filter] [Clear]     │
├─────────────────────────────────────────────────────────────┤
│  Active: [Groceries ×] [Jan 2026 ×] [Alice ×]  Clear all  │  ← Filter badges
├─────────────────────────────────────────────────────────────┤
│  ☐ │ Date       │ Description        │ Amount │ Account    │  ← Table with checkbox
│  ☐ │ 2026-01-15 │ Whole Foods        │ -85.42 │ Checking   │
│  ☐ │ 2026-01-14 │ Shell Gas Station  │ -45.00 │ Credit     │
│  ...                                                        │
├─────────────────────────────────────────────────────────────┤
│  ◄ Prev    Page 1 of 20 (982 total)    Next ►              │  ← Pagination
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────┐  ← Slide-out detail panel
│  ✕  Transaction Detail          │     (drawer-end)
│                                 │
│  Whole Foods Market             │
│  Whole Foods                    │
│                                 │
│  -$85.42 USD                    │
│  January 15, 2026               │
│                                 │
│  Account:  Chase Checking →     │
│  Member:   Alice                │
│  Category: Groceries [Edit]     │
│  Channel:  in_store             │
│  ID:       abc123... [Copy]     │
│                                 │
│  Created:  2026-01-15T...       │
│  Updated:  2026-01-15T...       │
└─────────────────────────────────┘

┌──────────────────────────────────────────────┐  ← Floating bulk action bar
│  3 selected   [Categorize]  [Cancel]         │     (fixed bottom, centered)
└──────────────────────────────────────────────┘
```

### 10.2 Responsive Behavior

- **Mobile (< lg):** Filter bar collapses behind a toggle. Multi-select dropdowns go full-width. Detail panel is full-width overlay. Bulk action bar remains fixed at bottom. Table scrolls horizontally. Checkbox column always visible.
- **Tablet (lg):** Filter bar wraps naturally with `flex-wrap`. Detail panel is `w-96`. Table fits with horizontal scroll if needed.
- **Desktop (xl+):** All filters visible. Detail panel slides in from right, table content area shrinks to accommodate.

### 10.3 CSS Additions

Add to `input.css`:

```css
.bb-filter-badges {
    @apply flex flex-wrap gap-2 mb-4;
}

.bb-filter-badge {
    @apply badge badge-outline gap-1;
}

.bb-bulk-bar {
    @apply fixed bottom-4 left-1/2 -translate-x-1/2 z-40
           bg-base-100 shadow-xl rounded-box
           px-4 py-2 flex items-center gap-3
           border border-base-300;
}

.bb-detail-panel {
    @apply bg-base-100 min-h-full w-96 max-w-full p-6
           border-l border-base-300 overflow-y-auto;
}
```

---

## 11. Implementation Tasks

Ordered by dependency. File references are relative to the project root.

### 11.1 Database Migration

1. **Create migration for `filter_presets` table.** Add `internal/db/migrations/00XXX_filter_presets.sql` with the schema from Section 4.1.
2. **Add sqlc queries** for filter preset CRUD in `internal/db/queries/filter_presets.sql`.

### 11.2 Service Layer

3. **Modify `AdminTransactionListParams`** in `internal/service/types.go`: change `AccountID *string`, `UserID *string`, `ConnectionID *string`, `Category *string` to `AccountIDs []string`, `UserIDs []string`, `ConnectionIDs []string`, `Categories []string`.
4. **Update `ListTransactionsAdmin`** in `internal/service/transactions.go`: use `= ANY($N)` for multi-value filters. Update the count query similarly.
5. **Add `AdminTransactionRow` fields:** `PaymentChannel *string`, `AuthorizedDate *string`, `CategoryDetailed *string`. Update the SELECT and scan in `ListTransactionsAdmin`.
6. **Add `ExportTransactionsCSV` function** in `internal/service/transactions.go`: streaming CSV writer using `encoding/csv`, reuses the dynamic query builder without LIMIT.
7. **Add `BulkCategorizeTransactions` function** in `internal/service/transactions.go`.
8. **Add filter preset service functions** in `internal/service/filter_presets.go`: `CreateFilterPreset`, `ListFilterPresets`, `GetFilterPreset`, `UpdateFilterPreset`, `DeleteFilterPreset`.

### 11.3 Admin Handlers

9. **Update `TransactionListHandler`** in `internal/admin/transactions.go`: parse multi-value query params (`r.URL.Query()["account_id"]`), load presets for quick-access bar, resolve `?preset=` param.
10. **Add `ExportTransactionsCSVHandler`** in `internal/admin/transactions.go`: new handler for `GET /admin/transactions/export.csv`.
11. **Add `BulkCategorizeHandler`** in `internal/admin/transactions.go`: new handler for `POST /admin/api/transactions/bulk/categorize`.
12. **Add filter preset CRUD handlers** in `internal/admin/filter_presets.go`: handlers for `POST /admin/api/filter-presets`, `GET /admin/api/filter-presets`, `PUT /admin/api/filter-presets/{id}`, `DELETE /admin/api/filter-presets/{id}`.

### 11.4 Routes

13. **Register new routes** in `internal/admin/router.go`:
    - `GET /admin/transactions/export.csv` (inside the authenticated `/admin` route group)
    - `POST /admin/api/transactions/bulk/categorize`
    - `POST /admin/api/transactions/bulk/comment` (Phase 21 dependent)
    - `GET /admin/api/filter-presets`
    - `POST /admin/api/filter-presets`
    - `PUT /admin/api/filter-presets/{id}`
    - `DELETE /admin/api/filter-presets/{id}`

### 11.5 Templates

14. **Rewrite `transactions.html`** in `internal/templates/pages/transactions.html`: implement the full layout from Section 10.1 with multi-select filter dropdowns, preset bar, active filter badges, checkbox column, detail drawer, bulk action bar. Heavy Alpine.js usage.
15. **Add `filterQueryString` template function** in `internal/admin/templates.go`: builds URL query string from current filter values for the export link.

### 11.6 CSS

16. **Add new component classes** to `input.css`: `.bb-filter-badges`, `.bb-filter-badge`, `.bb-bulk-bar`, `.bb-detail-panel` (Section 10.3).
17. **Run `make css`** to regenerate `static/css/styles.css`.

### 11.7 Testing

18. **Service layer tests** for multi-value filtering in `internal/service/transactions_test.go`.
19. **Service layer tests** for `BulkCategorizeTransactions`.
20. **Service layer tests** for filter preset CRUD.
21. **Handler tests** for CSV export (verify headers, content format).

---

## 12. Dependencies

### 12.1 Phase 20B — Category Taxonomy

Phase 28 works **with or without** Phase 20B:

- **Without Phase 20B:** Category filter shows flat list of `category_primary` strings (current behavior). Bulk categorize updates `category_primary` directly. Detail panel shows raw category strings.
- **With Phase 20B:** Category filter shows hierarchical tree from `categories` table. Bulk categorize sets `category_id` + `category_override = true`. Detail panel shows display names with icons/colors.

The implementation should check for the existence of the `categories` table (or a feature flag) and adapt accordingly. The recommended approach is to code both paths now and toggle based on whether `ListCategories()` returns results.

### 12.2 Phase 21 — Transaction Comments

The "Bulk add comment" action and the "Notes" section in the detail panel depend on Phase 21. If Phase 21 is not implemented:

- Hide the "Add comment" bulk action button
- Hide the notes/comments section in the detail panel
- No error, no stub — just absent UI

### 12.3 No Other Dependencies

Phase 28 does not depend on or block any other phase. It is a pure dashboard UX improvement that consumes existing service layer functionality.

---

## 13. Open Questions

1. **AJAX vs. form submission for filters:** This spec uses traditional form submission (full page reload on filter change). Should we move to AJAX-based filtering with Alpine.js `fetch()` and partial DOM updates? Pros: faster UX, no page flash. Cons: significant complexity increase, state management. **Recommendation:** Ship with form submission first, consider AJAX as a follow-up if performance is a concern.

2. **Select all matching:** The "select all N matching" feature for bulk actions requires the server to process potentially large update sets. Should we add a confirmation with an explicit count? **Recommendation:** Yes, always show the count and require confirmation.

3. **Preset sharing:** Should presets be exportable/importable (e.g., as URL query strings)? **Recommendation:** Not for Phase 28. Presets are already URL-representable since they map directly to query params. A "Copy as URL" button on presets would be a low-effort follow-up.
