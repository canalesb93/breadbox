# Category Mapping Design

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## Table of Contents

1. [Overview](#1-overview)
2. [Data Model](#2-data-model)
3. [Default Category Set](#3-default-category-set)
4. [Provider Mapping Design](#4-provider-mapping-design)
5. [User Customization](#5-user-customization)
6. [Migration Strategy](#6-migration-strategy)
7. [API & MCP Impact](#7-api--mcp-impact)
8. [Dashboard UX](#8-dashboard-ux)
9. [Edge Cases](#9-edge-cases)
10. [Implementation Phases](#10-implementation-phases)

---

## 1. Overview

### The Problem

Today, transactions store raw provider category strings (`FOOD_AND_DRINK_GROCERIES`) directly. This has three problems:

1. **No user control.** A family might want to split "Food & Drink" into "Groceries" vs "Dining Out" as top-level categories, or merge "General Merchandise" and "Shopping" into one bucket. They can't.
2. **Provider coupling.** Every consumer (dashboard, MCP, API) works with Plaid's internal taxonomy. CSV imports have no normalization at all.
3. **Poor display.** `RENT_AND_UTILITIES_GAS` is not something you want to show a human or an AI agent crafting a budget summary.

### The Solution

A user-owned category taxonomy stored in the database, with per-provider mapping tables that translate provider categories into the user's categories during sync. Transactions store a FK to the user's category, not raw strings. The system ships with a sensible default taxonomy (derived from Plaid's) that users can fully customize.

### Design Principles

- **Categories belong to the user, not the provider.** The category table is the user's taxonomy. Providers are just input sources that get mapped.
- **Provider categories are raw material.** They're stored for auditability but never shown to end users or AI agents directly.
- **Mapping happens at sync time.** When a transaction arrives, its provider category is looked up in the mapping table and resolved to a user category before being written. This means category changes propagate to new transactions immediately.
- **Retroactive re-mapping is explicit.** Changing a mapping doesn't auto-update old transactions. There's a "re-map" action for that.
- **Per-transaction overrides win.** A user can manually override any transaction's category. Overrides are never clobbered by sync or re-map.

---

## 2. Data Model

### 2.1 `categories` Table

The user's canonical category taxonomy. Two-level hierarchy: primary (group) and detailed (specific).

```sql
CREATE TABLE categories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,           -- machine key: "groceries", "dining_out"
    display_name    TEXT NOT NULL,                   -- human label: "Groceries", "Dining Out"
    parent_id       UUID NULL REFERENCES categories(id) ON DELETE CASCADE,
    icon            TEXT NULL,                       -- Lucide icon name: "shopping-cart", "utensils"
    color           TEXT NULL,                       -- hex color for charts: "#4ade80"
    sort_order      INTEGER NOT NULL DEFAULT 0,      -- display ordering within siblings
    is_system       BOOLEAN NOT NULL DEFAULT FALSE,  -- true for seeded defaults (deletable, just a hint)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX categories_parent_id_idx ON categories(parent_id);
CREATE INDEX categories_slug_idx ON categories(slug);
```

**Key decisions:**

- **Two levels only.** `parent_id IS NULL` = primary category. `parent_id IS NOT NULL` = detailed category. No deeper nesting. This matches Plaid's model and keeps queries simple.
- **Slug is the stable key.** Display names can change freely. Slugs are what mappings and filters reference. Slugs use `lowercase_with_underscores`.
- **`is_system` is advisory.** Users can rename, re-icon, recolor, or delete system categories. It's just a UI hint ("this came from the defaults").
- **Self-referencing FK with CASCADE.** Deleting a primary category deletes its children. Transactions referencing deleted categories get handled separately (see Section 2.3).

### 2.2 `category_mappings` Table

Maps provider-specific category strings to user categories. One row per (provider, provider_category) pair.

```sql
CREATE TABLE category_mappings (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider            provider_type NOT NULL,       -- reuse existing enum: plaid, teller, csv
    provider_category   TEXT NOT NULL,                 -- raw string from provider: "FOOD_AND_DRINK_GROCERIES", "groceries", etc.
    category_id         UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_category)
);

CREATE INDEX category_mappings_provider_idx ON category_mappings(provider);
CREATE INDEX category_mappings_category_id_idx ON category_mappings(category_id);
```

**Key decisions:**

- **Mappings are at the detailed level.** A Plaid `FOOD_AND_DRINK_GROCERIES` maps to the user's `groceries` detailed category. The primary category is always derived from `parent_id` — there's no separate primary mapping. This is simpler and prevents primary/detailed conflicts.
- **Primary-only fallback.** If a transaction has only a primary category (e.g., `FOOD_AND_DRINK` with no detailed), we also store a mapping row for the primary string. The mapping table doesn't distinguish — it just maps strings to categories.
- **`ON DELETE CASCADE` on category_id.** If a user deletes a category, its mappings disappear. Unmapped provider categories will then hit the fallback logic (Section 4.3).
- **Provider-agnostic.** The `provider` column uses the existing `provider_type` enum. CSV gets its own mappings (or more likely, no mappings — CSV categories pass through as-is and get matched by string).

### 2.3 Transaction Schema Changes

Add a FK to the categories table. Keep the raw provider strings for auditability.

```sql
-- New columns on transactions table
ALTER TABLE transactions
    ADD COLUMN category_id          UUID NULL REFERENCES categories(id) ON DELETE SET NULL,
    ADD COLUMN category_override    BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX transactions_category_id_idx ON transactions(category_id);
```

**What each column does:**

| Column | Purpose |
|--------|---------|
| `category_id` | FK to the user's category. This is THE category for display, filtering, and API/MCP responses. |
| `category_override` | `true` if the user manually set this transaction's category. Manual overrides are never touched by sync or re-mapping. |
| `category_primary` | (existing) Raw primary category string from the provider. Kept for auditability and re-mapping. Never shown to end users. |
| `category_detailed` | (existing) Raw detailed category string from the provider. Kept for auditability and re-mapping. Never shown to end users. |

**`ON DELETE SET NULL`** on `category_id`: if a category is deleted, transactions become uncategorized rather than deleted. The raw provider strings remain, so re-mapping can recategorize them.

### 2.4 Entity Relationship Summary

```
categories (self-referencing: parent_id → id)
    ↑
    │ category_id (FK, SET NULL)
    │
transactions ─── still has category_primary, category_detailed (raw TEXT, no FK)

category_mappings
    ├── provider (enum)
    ├── provider_category (TEXT)
    └── category_id → categories(id)
```

---

## 3. Default Category Set

### 3.1 Seeding Strategy

The app ships with a default taxonomy seeded by a goose migration. The taxonomy is derived from Plaid's 16 primary + ~104 detailed categories but cleaned up for human readability.

The migration inserts rows with `is_system = TRUE`. Users can modify or delete any of them.

### 3.2 Default Taxonomy

The default set groups Plaid's categories into 16 primaries with human-friendly display names. Slugs are stable machine keys derived from Plaid's taxonomy but lowercased and simplified.

| Slug | Display Name | Icon | Plaid Primary |
|------|-------------|------|---------------|
| `income` | Income | `wallet` | INCOME |
| `transfer_in` | Transfers In | `arrow-down-circle` | TRANSFER_IN |
| `transfer_out` | Transfers Out | `arrow-up-circle` | TRANSFER_OUT |
| `loan_payments` | Loan Payments | `landmark` | LOAN_PAYMENTS |
| `bank_fees` | Bank Fees | `building-2` | BANK_FEES |
| `entertainment` | Entertainment | `tv` | ENTERTAINMENT |
| `food_and_drink` | Food & Drink | `utensils` | FOOD_AND_DRINK |
| `general_merchandise` | Shopping | `shopping-bag` | GENERAL_MERCHANDISE |
| `home_improvement` | Home | `home` | HOME_IMPROVEMENT |
| `medical` | Medical | `heart-pulse` | MEDICAL |
| `personal_care` | Personal Care | `sparkles` | PERSONAL_CARE |
| `general_services` | Services | `wrench` | GENERAL_SERVICES |
| `government_and_non_profit` | Government & Donations | `building` | GOVERNMENT_AND_NON_PROFIT |
| `transportation` | Transportation | `car` | TRANSPORTATION |
| `travel` | Travel | `plane` | TRAVEL |
| `rent_and_utilities` | Rent & Utilities | `zap` | RENT_AND_UTILITIES |

Each primary has its detailed subcategories seeded as children. For example, `food_and_drink` gets children:

| Slug | Display Name | Plaid Detailed |
|------|-------------|----------------|
| `food_and_drink_groceries` | Groceries | FOOD_AND_DRINK_GROCERIES |
| `food_and_drink_restaurant` | Restaurants | FOOD_AND_DRINK_RESTAURANT |
| `food_and_drink_coffee` | Coffee Shops | FOOD_AND_DRINK_COFFEE |
| `food_and_drink_fast_food` | Fast Food | FOOD_AND_DRINK_FAST_FOOD |
| `food_and_drink_delivery` | Food Delivery | FOOD_AND_DRINK_FOOD_DELIVERY_SERVICES |
| `food_and_drink_beer_wine_and_liquor` | Beer, Wine & Liquor | FOOD_AND_DRINK_BEER_WINE_AND_LIQUOR |
| `food_and_drink_other` | Other Food & Drink | FOOD_AND_DRINK_OTHER_FOOD_AND_DRINK |

The full set is ~120 rows (16 primaries + ~104 detailed). Implemented as a Go seed function (not raw SQL) so it can be version-controlled and updated cleanly.

### 3.3 Seeding the Mappings

The same seed function also populates `category_mappings` for all three providers:

- **Plaid:** 1:1 mapping. `(plaid, "FOOD_AND_DRINK_GROCERIES")` → `food_and_drink_groceries` category. Also maps each primary string: `(plaid, "FOOD_AND_DRINK")` → `food_and_drink` category.
- **Teller:** Uses the existing `tellerCategories` Go map. `(teller, "groceries")` → `food_and_drink_groceries`, `(teller, "dining")` → `food_and_drink_restaurant`, etc.
- **CSV:** No default mappings. CSV categories are freeform. Users add mappings as needed, or the unmapped fallback handles it.

---

## 4. Provider Mapping Design

### 4.1 Mapping Resolution During Sync

When the sync engine upserts a transaction, it resolves the category through this chain:

```
1. If transaction has category_override = true → SKIP (keep existing category_id)
2. Look up provider_category in category_mappings:
   a. Try detailed category first: (provider, "FOOD_AND_DRINK_GROCERIES")
   b. If no match, try primary category: (provider, "FOOD_AND_DRINK")
   c. If no match → unmapped fallback (Section 4.3)
3. Set category_id to the resolved category
```

This lookup is a simple indexed query. For bulk sync performance, the sync engine loads the full mapping table for the relevant provider into a `map[string]UUID` at the start of each sync run. The mapping table is small (hundreds of rows max) and fits entirely in memory.

### 4.2 Implementation in Sync Engine

The sync engine's `upsertTransaction` method gains a `categoryResolver` dependency:

```go
type categoryResolver struct {
    mappings map[string]pgtype.UUID // "plaid:FOOD_AND_DRINK_GROCERIES" → category UUID
}

func (r *categoryResolver) Resolve(provider, detailed, primary *string) pgtype.UUID {
    if detailed != nil {
        if id, ok := r.mappings[*provider+":"+*detailed]; ok {
            return id
        }
    }
    if primary != nil {
        if id, ok := r.mappings[*provider+":"+*primary]; ok {
            return id
        }
    }
    return pgtype.UUID{} // NULL — unmapped
}
```

Loaded once per sync run:

```go
func loadMappings(ctx context.Context, pool *pgxpool.Pool, provider string) (*categoryResolver, error) {
    rows, _ := pool.Query(ctx,
        "SELECT provider_category, category_id FROM category_mappings WHERE provider = $1", provider)
    // build map...
}
```

### 4.3 Unmapped Category Fallback

When a provider category has no mapping:

1. **Transaction gets `category_id = NULL`.** It's uncategorized.
2. **The raw `category_primary` and `category_detailed` strings are still stored.** Nothing is lost.
3. **Dashboard shows an "Unmapped Categories" section** on the category management page (see Section 8). This lists all distinct `(category_primary, category_detailed)` pairs from transactions where `category_id IS NULL AND category_override = FALSE`. The user can click to create a mapping.

This is a conscious choice: we don't auto-create categories for unknown provider strings. That would pollute the user's taxonomy with provider junk. Instead, we surface unmapped categories and let the user decide.

### 4.4 Mapping Granularity

Mappings work at the **string level** — whatever the provider sends. For Plaid, this means:

- If the provider sends both primary and detailed, the detailed mapping wins (more specific).
- If the provider sends only primary, the primary mapping is used.
- Plaid detailed strings always start with the primary name, but the mapping table doesn't enforce this. It's just strings.

For Teller, the provider sends a single category string (`"groceries"`, `"dining"`). The Teller provider maps this to a `(primary, detailed)` pair in Go code before it reaches the sync engine. So the mapping table sees the translated Plaid-compatible strings. **However**, we also seed direct Teller mappings `(teller, "groceries")` → user category, so the `mapCategory()` Go function becomes unnecessary once the mapping table is in place. The Go map stays as documentation/fallback but the DB mappings are authoritative.

**Decision: Keep the Go-level Teller mapping.** The `mapCategory()` function in `teller/categories.go` continues to translate Teller categories to Plaid-compatible strings. The sync engine then resolves those Plaid-compatible strings through the mapping table using `provider = "plaid"` keys. This means Teller transactions go through two translation steps (Teller→Plaid strings→user category), but it simplifies the mapping table: users only need to customize Plaid-format mappings unless they want Teller-specific overrides. The `teller`-provider mappings in the DB exist as an override layer — if present, they short-circuit the Plaid fallback.

**Revised resolution order for Teller:**

```
1. Look up (teller, raw_teller_category) in mappings  → if found, done
2. mapCategory() translates to Plaid strings
3. Look up (plaid, plaid_detailed) in mappings         → if found, done
4. Look up (plaid, plaid_primary) in mappings           → if found, done
5. Unmapped
```

This gives users the option to override at either the Teller or Plaid level. Most users will never touch this — the defaults just work.

---

## 5. User Customization

### 5.1 Custom Categories

Users can create, rename, recolor, re-icon, reorder, and delete categories freely.

- **Create:** Add a new primary or detailed category. User provides display name; slug is auto-generated from display name (lowercased, spaces→underscores, special chars stripped). Slug uniqueness is enforced.
- **Rename:** Change `display_name` at any time. Slug stays the same. All transactions and mappings continue to work.
- **Recolor/re-icon:** Cosmetic changes, no impact on data.
- **Reorder:** Change `sort_order` to control display ordering.
- **Delete:** Category is removed. Transactions with that category get `category_id = NULL` (via `ON DELETE SET NULL`). Mappings are also removed (via `ON DELETE CASCADE`). User is warned before deleting.

### 5.2 Merge Categories

To merge category B into category A:

1. `UPDATE transactions SET category_id = A.id WHERE category_id = B.id`
2. `UPDATE category_mappings SET category_id = A.id WHERE category_id = B.id`
3. `DELETE FROM categories WHERE id = B.id`

This is exposed as a single "Merge into..." action in the dashboard.

### 5.3 Split Categories

Splitting is the inverse: create a new detailed category under a primary, then reassign transactions. This is a manual process — the user creates the new category, then either:

- Updates mappings so future transactions from certain provider categories go to the new category
- Manually reassigns specific transactions

No automatic splitting based on rules (that's rule-based auto-categorization, out of scope for v1).

### 5.4 Per-Transaction Category Override

Users can change any transaction's category from the transaction detail view (dashboard) or via the REST API.

- Sets `category_id` to the chosen category and `category_override = TRUE`.
- Overridden transactions are **never** touched by sync or re-mapping operations.
- Users can "reset to automatic" which clears the override flag and re-resolves the category from the mapping table.

### 5.5 Bulk Re-mapping

When a user changes a mapping (e.g., changes `(plaid, "FOOD_AND_DRINK_COFFEE")` from "Coffee Shops" to "Dining Out"), existing transactions are NOT automatically updated. Instead:

- The dashboard shows a prompt: "42 existing transactions match this mapping. Apply to existing transactions?"
- If yes: `UPDATE transactions SET category_id = $new WHERE category_id = $old AND category_override = FALSE AND category_detailed = 'FOOD_AND_DRINK_COFFEE'`
- If no: only future transactions use the new mapping.

### 5.6 Future: Rule-Based Auto-Categorization

The data model supports this without changes. A future `category_rules` table could hold conditions (merchant name patterns, amount ranges, etc.) and a target `category_id`. Rules would run after provider mapping and before manual override in the resolution chain. The `category_override` flag already distinguishes manual vs automatic assignments.

---

## 6. Migration Strategy

### 6.1 Schema Migration

A single goose migration that:

1. Creates the `categories` table
2. Creates the `category_mappings` table
3. Adds `category_id` and `category_override` columns to `transactions`
4. Creates indexes

### 6.2 Data Seed Migration

A separate goose migration (Go, not SQL) that:

1. Inserts the default category taxonomy (~120 rows)
2. Inserts the default provider mappings (~130 rows for Plaid + ~27 rows for Teller)

Using a Go migration ensures the seed data is version-controlled alongside the code and can use the same slug-generation logic.

### 6.3 Backfill Existing Transactions

A third goose migration (Go) that backfills `category_id` for all existing transactions:

```sql
UPDATE transactions t
SET category_id = cm.category_id
FROM category_mappings cm
WHERE cm.provider = 'plaid'        -- Teller categories are already Plaid-normalized
  AND cm.provider_category = t.category_detailed
  AND t.category_id IS NULL
  AND t.deleted_at IS NULL;
```

Then a second pass for transactions with only primary categories:

```sql
UPDATE transactions t
SET category_id = cm.category_id
FROM category_mappings cm
WHERE cm.provider = 'plaid'
  AND cm.provider_category = t.category_primary
  AND t.category_id IS NULL
  AND t.deleted_at IS NULL;
```

This handles the majority of existing data. Any remaining `category_id IS NULL` transactions are truly unmapped (provider sent no category, or it was an unrecognized string).

### 6.4 Backward Compatibility

The existing `category_primary` and `category_detailed` TEXT columns remain. They continue to be populated by sync. They serve as:

1. **Audit trail:** The raw provider data is always preserved.
2. **Re-mapping source:** If a user wants to re-resolve categories, the raw strings are the input.
3. **Fallback:** If a bug causes `category_id` to be NULL, the raw strings are still queryable.

The REST API and MCP responses gain new fields but keep the old ones (see Section 7).

---

## 7. API & MCP Impact

### 7.1 REST API Changes

**Transaction responses** gain new fields:

```json
{
  "id": "...",
  "category": {
    "id": "uuid",
    "slug": "food_and_drink_groceries",
    "display_name": "Groceries",
    "primary_slug": "food_and_drink",
    "primary_display_name": "Food & Drink",
    "icon": "shopping-cart",
    "color": "#4ade80"
  },
  "category_override": false,
  "category_primary_raw": "FOOD_AND_DRINK",
  "category_detailed_raw": "FOOD_AND_DRINK_GROCERIES",
  ...
}
```

The old `category_primary` and `category_detailed` fields are renamed to `category_primary_raw` and `category_detailed_raw` to avoid confusion. This is a breaking change but an acceptable one at this stage.

**New filter parameters:**

- `category_slug` — filter by category slug (replaces current `category` filter on raw strings)
- `category_primary_slug` — filter by primary category slug
- The old `category` and `category_detailed` filters continue to work against raw strings for backward compatibility, but are documented as deprecated.

**New endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/categories` | List all categories (tree structure) |
| `GET` | `/api/v1/categories/:id` | Get single category with transaction count |
| `POST` | `/api/v1/categories` | Create category |
| `PUT` | `/api/v1/categories/:id` | Update category |
| `DELETE` | `/api/v1/categories/:id` | Delete category |
| `GET` | `/api/v1/category-mappings` | List mappings (filterable by provider) |
| `PUT` | `/api/v1/category-mappings` | Bulk upsert mappings |
| `PATCH` | `/api/v1/transactions/:id/category` | Override transaction category |
| `DELETE` | `/api/v1/transactions/:id/category` | Reset to automatic category |

### 7.2 MCP Tool Changes

**`list_categories` tool:** Returns the user's category taxonomy instead of raw distinct strings. Response shape:

```json
[
  {
    "slug": "food_and_drink",
    "display_name": "Food & Drink",
    "icon": "utensils",
    "subcategories": [
      {"slug": "food_and_drink_groceries", "display_name": "Groceries"},
      {"slug": "food_and_drink_restaurant", "display_name": "Restaurants"}
    ]
  }
]
```

This is a much better experience for AI agents. Instead of parsing `FOOD_AND_DRINK_GROCERIES`, they see "Groceries" under "Food & Drink".

**`query_transactions` tool:** The `category` and `category_detailed` input parameters are updated to accept category slugs. The tool description is updated to reference the new taxonomy. Response includes the structured category object.

**New MCP tool: `categorize_transaction`:**

```
Input:  { transaction_id, category_slug }
Output: { success: true }
```

This lets AI agents categorize transactions directly. Sets `category_override = true`.

**MCP server instructions** updated to describe the category system, explain that categories are user-customizable, and recommend using `list_categories` before filtering by category.

### 7.3 `breadbox://overview` Resource

The overview resource gains a `categories` section:

```json
{
  "categories": {
    "total_primary": 16,
    "total_detailed": 104,
    "custom_count": 3,
    "unmapped_transaction_count": 12
  }
}
```

---

## 8. Dashboard UX

### 8.1 Category Management Page

New top-level nav item: **Categories** (`/admin/categories`).

**Main view:** A tree-list of all categories. Each row shows:

- Icon + color swatch
- Display name
- Slug (muted)
- Transaction count
- Actions: Edit, Delete, Merge Into

The tree is collapsible by primary category. Drag-to-reorder for `sort_order` (Alpine.js + Sortable.js, or just up/down buttons to keep it simple).

**"Add Category" button** at the top. Modal form: display name, parent (dropdown of primaries or "none" for new primary), icon picker, color picker.

**"Unmapped Categories" alert card** at the top of the page if any transactions have `category_id IS NULL AND category_override = FALSE`. Shows count and links to the mapping editor filtered to show unmapped strings.

### 8.2 Provider Mapping Editor

Sub-tab or section within the Categories page: **Mappings** (`/admin/categories/mappings`).

**Layout:** A filterable table:

| Provider | Provider Category | → | User Category | Actions |
|----------|------------------|---|---------------|---------|
| plaid | FOOD_AND_DRINK_GROCERIES | → | Groceries | Edit / Remove |
| plaid | FOOD_AND_DRINK_COFFEE | → | Coffee Shops | Edit / Remove |
| teller | groceries | → | Groceries | Edit / Remove |

**Filters:** Provider dropdown, user category dropdown, "unmapped only" toggle.

**"Unmapped only" view:** Shows distinct `(category_primary, category_detailed)` pairs from transactions that have no mapping. Each row has a "Map to..." dropdown to create a mapping inline.

**Bulk edit:** A textarea/JSON editor for power users. Export all mappings as JSON, edit, re-import. Format:

```json
{
  "plaid": {
    "FOOD_AND_DRINK_GROCERIES": "food_and_drink_groceries",
    "FOOD_AND_DRINK_COFFEE": "food_and_drink_coffee"
  },
  "teller": {
    "groceries": "food_and_drink_groceries"
  }
}
```

### 8.3 Transaction Category Override

On the transaction list and detail pages:

- Category column shows the display name with icon, not the raw string.
- Clicking the category opens an inline dropdown to change it (category override).
- A small "reset" icon appears next to overridden categories.

### 8.4 Dashboard Category Breakdown

The existing dashboard can gain a "Spending by Category" summary card. Out of scope for the category mapping feature itself, but the data model supports it trivially:

```sql
SELECT c.display_name, c.icon, c.color, SUM(t.amount)
FROM transactions t
JOIN categories c ON t.category_id = c.id
WHERE t.date >= $1 AND t.amount > 0
GROUP BY c.id ORDER BY SUM(t.amount) DESC
```

---

## 9. Edge Cases

### 9.1 Provider Taxonomy Updates

Plaid occasionally adds new categories. When a new Plaid category appears that has no mapping:

- Transaction gets `category_id = NULL`
- Raw strings are preserved
- "Unmapped Categories" alert surfaces it in the dashboard
- User creates a mapping (or we add it in a future app update's seed migration)

Seed migrations are **additive only** — they use `ON CONFLICT DO NOTHING` so they never overwrite user customizations.

### 9.2 Conflicting Mappings

The `UNIQUE(provider, provider_category)` constraint prevents duplicate mappings. One provider category maps to exactly one user category. If a user wants different behavior (e.g., "Plaid's GENERAL_SERVICES should map to different user categories depending on merchant"), that's a rule-based categorization problem — out of scope for v1, solved by per-transaction overrides for now.

### 9.3 Category Deletion with Active Transactions

When deleting a category that has transactions:

1. Dashboard shows: "This category has 47 transactions. They will become uncategorized."
2. Optionally offer "Move transactions to:" with a category picker.
3. If confirmed without moving: `category_id` becomes NULL via `ON DELETE SET NULL`. Transactions can be re-mapped later.

### 9.4 Renaming Slugs

Slugs are **immutable after creation.** Users can change display names freely, but slugs are permanent machine identifiers. This prevents breaking API consumers, MCP tool references, and external integrations. If a user truly needs a different slug, they create a new category and merge.

### 9.5 CSV Categories

CSV imports store whatever the user mapped as `category_primary`/`category_detailed`. These raw strings can be mapped via `(csv, "raw_string")` entries in `category_mappings`. Since CSV categories are completely freeform, the "Unmapped Categories" workflow is especially important here — after a CSV import, the user reviews unmapped categories and assigns them.

### 9.6 Empty/Null Provider Categories

Some transactions arrive with no category at all (both `category_primary` and `category_detailed` are NULL). These get `category_id = NULL`. They don't appear in the "Unmapped Categories" list (there's nothing to map). Users can only categorize them via per-transaction override.

---

## 10. Implementation Phases

### Phase A: Schema + Seed (1 session)

1. Write goose migration: `categories` table, `category_mappings` table, new columns on `transactions`
2. Write Go seed function with full default taxonomy + Plaid/Teller mappings
3. Write backfill migration for existing transactions
4. Update sqlc queries for new tables

**Checkpoint:** Migrations apply cleanly. `SELECT count(*) FROM categories` returns ~120. `SELECT count(*) FROM category_mappings` returns ~150. Existing transactions have `category_id` populated.

### Phase B: Sync Engine Integration (1 session)

1. Implement `categoryResolver` in sync engine
2. Load mappings at start of each sync run
3. Resolve `category_id` during `upsertTransaction`
4. Respect `category_override` flag (skip resolution if true)
5. Update CSV import to resolve categories through mapping table

**Checkpoint:** New synced transactions get `category_id` populated. Manual sync works. CSV import resolves categories.

### Phase C: Service Layer + API (1 session)

1. CRUD service methods for categories
2. CRUD service methods for category mappings
3. Update `TransactionResponse` with structured category object
4. Update `ListTransactions` and `CountTransactions` to support `category_slug` filter
5. New REST endpoints for categories and mappings
6. `PATCH/DELETE /transactions/:id/category` for overrides

**Checkpoint:** REST API returns structured categories. New endpoints work. Category filtering works.

### Phase D: MCP Updates (1 session)

1. Update `list_categories` tool to return user taxonomy
2. Update `query_transactions` to use category slugs
3. Add `categorize_transaction` tool
4. Update MCP server instructions
5. Update `breadbox://overview` resource

**Checkpoint:** MCP tools return human-friendly category names. AI agent can categorize transactions.

### Phase E: Dashboard UI (1-2 sessions)

1. Category management page (tree list, CRUD)
2. Provider mapping editor (table + inline editing)
3. Unmapped categories alert + workflow
4. Transaction category display + inline override
5. "Unmapped Categories" counter in nav/dashboard

**Checkpoint:** Full category management from the dashboard. Can create, edit, delete, merge categories. Can edit mappings. Can override transaction categories.

---

## Appendix: Full Default Taxonomy

The complete default taxonomy with all ~104 detailed categories. Each detailed category's slug is the lowercased version of its Plaid constant.

<details>
<summary>Click to expand full taxonomy</summary>

**INCOME**
- `income_dividends` — Dividends
- `income_interest_earned` — Interest Earned
- `income_retirement_pension` — Retirement & Pension
- `income_tax_refund` — Tax Refund
- `income_unemployment` — Unemployment
- `income_wages` — Wages & Salary
- `income_other_income` — Other Income

**TRANSFER_IN**
- `transfer_in_cash_advances_and_loans` — Cash Advances & Loans
- `transfer_in_deposit` — Deposits
- `transfer_in_investment_and_retirement_funds` — Investment & Retirement
- `transfer_in_savings` — Savings
- `transfer_in_account_transfer` — Account Transfers
- `transfer_in_other_transfer_in` — Other Transfers In

**TRANSFER_OUT**
- `transfer_out_investment_and_retirement_funds` — Investment & Retirement
- `transfer_out_savings` — Savings
- `transfer_out_withdrawal` — Withdrawals
- `transfer_out_account_transfer` — Account Transfers
- `transfer_out_other_transfer_out` — Other Transfers Out

**LOAN_PAYMENTS**
- `loan_payments_car_payment` — Car Payment
- `loan_payments_credit_card_payment` — Credit Card Payment
- `loan_payments_personal_loan_payment` — Personal Loan
- `loan_payments_mortgage_payment` — Mortgage
- `loan_payments_student_loan_payment` — Student Loans
- `loan_payments_other_payment` — Other Loan Payments
- `loan_payments_insurance_payment` — Insurance

**BANK_FEES**
- `bank_fees_atm_fees` — ATM Fees
- `bank_fees_foreign_transaction_fees` — Foreign Transaction Fees
- `bank_fees_insufficient_funds` — Insufficient Funds
- `bank_fees_interest_charge` — Interest Charges
- `bank_fees_overdraft_fees` — Overdraft Fees
- `bank_fees_other_bank_fees` — Other Bank Fees

**ENTERTAINMENT**
- `entertainment_casinos_and_gambling` — Casinos & Gambling
- `entertainment_music_and_audio` — Music & Audio
- `entertainment_sporting_events_amusement_parks_and_museums` — Events, Parks & Museums
- `entertainment_tv_and_movies` — TV & Movies
- `entertainment_video_games` — Video Games
- `entertainment_other_entertainment` — Other Entertainment

**FOOD_AND_DRINK**
- `food_and_drink_beer_wine_and_liquor` — Beer, Wine & Liquor
- `food_and_drink_coffee` — Coffee Shops
- `food_and_drink_fast_food` — Fast Food
- `food_and_drink_groceries` — Groceries
- `food_and_drink_restaurant` — Restaurants
- `food_and_drink_vending_machines` — Vending Machines
- `food_and_drink_other_food_and_drink` — Other Food & Drink

**GENERAL_MERCHANDISE**
- `general_merchandise_bookstores_and_newsstands` — Books & Newsstands
- `general_merchandise_clothing_and_accessories` — Clothing & Accessories
- `general_merchandise_convenience_stores` — Convenience Stores
- `general_merchandise_department_stores` — Department Stores
- `general_merchandise_discount_stores` — Discount Stores
- `general_merchandise_electronics` — Electronics
- `general_merchandise_gifts_and_novelties` — Gifts & Novelties
- `general_merchandise_office_supplies` — Office Supplies
- `general_merchandise_online_marketplaces` — Online Marketplaces
- `general_merchandise_pet_supplies` — Pet Supplies
- `general_merchandise_sporting_goods` — Sporting Goods
- `general_merchandise_superstores` — Superstores
- `general_merchandise_tobacco_and_vape` — Tobacco & Vape
- `general_merchandise_other_general_merchandise` — Other Shopping

**HOME_IMPROVEMENT**
- `home_improvement_furniture` — Furniture
- `home_improvement_hardware` — Hardware
- `home_improvement_repair_and_maintenance` — Repair & Maintenance
- `home_improvement_security` — Security
- `home_improvement_other_home_improvement` — Other Home Improvement

**MEDICAL**
- `medical_dental_care` — Dental
- `medical_eye_care` — Eye Care
- `medical_nursing_care` — Nursing Care
- `medical_pharmacies_and_supplements` — Pharmacies & Supplements
- `medical_primary_care` — Primary Care
- `medical_veterinary_services` — Veterinary
- `medical_other_medical` — Other Medical

**PERSONAL_CARE**
- `personal_care_gyms_and_fitness_centers` — Gyms & Fitness
- `personal_care_hair_and_beauty` — Hair & Beauty
- `personal_care_laundry_and_dry_cleaning` — Laundry & Dry Cleaning
- `personal_care_other_personal_care` — Other Personal Care

**GENERAL_SERVICES**
- `general_services_accounting_and_financial_planning` — Accounting & Financial Planning
- `general_services_automotive` — Automotive Services
- `general_services_childcare` — Childcare
- `general_services_consulting_and_legal` — Consulting & Legal
- `general_services_education` — Education
- `general_services_insurance` — Insurance
- `general_services_postage_and_shipping` — Postage & Shipping
- `general_services_storage` — Storage
- `general_services_other_general_services` — Other Services

**GOVERNMENT_AND_NON_PROFIT**
- `government_and_non_profit_donations` — Donations
- `government_and_non_profit_government_departments_and_agencies` — Government Agencies
- `government_and_non_profit_tax_payment` — Tax Payments
- `government_and_non_profit_other_government_and_non_profit` — Other Government

**TRANSPORTATION**
- `transportation_bikes_and_scooters` — Bikes & Scooters
- `transportation_gas` — Gas
- `transportation_parking` — Parking
- `transportation_public_transit` — Public Transit
- `transportation_taxis_and_ride_shares` — Taxis & Ride Shares
- `transportation_tolls` — Tolls
- `transportation_other_transportation` — Other Transportation

**TRAVEL**
- `travel_flights` — Flights
- `travel_lodging` — Lodging
- `travel_rental_cars` — Rental Cars
- `travel_other_travel` — Other Travel

**RENT_AND_UTILITIES**
- `rent_and_utilities_gas_and_electricity` — Gas & Electricity
- `rent_and_utilities_internet_and_cable` — Internet & Cable
- `rent_and_utilities_rent` — Rent
- `rent_and_utilities_sewage_and_waste_management` — Sewage & Waste
- `rent_and_utilities_telephone` — Telephone
- `rent_and_utilities_water` — Water
- `rent_and_utilities_other_utilities` — Other Utilities

</details>
