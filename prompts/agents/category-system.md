---
title: Category System
description: Category hierarchy, slugs, merging, and taxonomy management
icon: shapes
---

## Category System

### Categories

- 2-level hierarchy: primary → detailed subcategories
- Each has: `id` (UUID), `slug` (stable identifier, e.g. `food_and_drink_groceries`), `display_name`, `icon`, `color`
- Use `list_categories` to get the full taxonomy tree
- Filter transactions by `category_slug` — parent slugs include all children

### Categorization

- `update_transactions`: the **only** per-row write — combines set-category, tag changes, comment, flag, and category reset into one atomic operation per transaction (max 50 per call). It writes `category_id` directly (last-writer-wins, no provenance). Use it for everything: set the category on one transaction, or set the category AND remove the `needs-review` tag in one write when finishing a review.
  - Reset a category (let rules re-resolve it): `update_transactions` with `reset_category: true` on that row.
  - Recategorize many: page through them with `query_transactions`, then `update_transactions` in batches of ≤50 — or, for a durable pattern, author a `create_transaction_rule` so future syncs categorize automatically.

### Taxonomy management

- `export_categories` / `import_categories`: bulk edit via TSV
- To merge/consolidate: set the `merge_into` column in the TSV to move all transactions from one category to another, then delete the source
- Transaction rules (via `create_transaction_rule`) handle automatic categorization during sync
