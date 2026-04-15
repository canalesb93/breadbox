# Category System
> Category hierarchy, slugs, merging, and taxonomy management

CATEGORIES:
- 2-level hierarchy: primary → detailed subcategories
- Each has: id (UUID), slug (stable identifier, e.g., food_and_drink_groceries), display_name, icon, color
- Use list_categories to get the full taxonomy tree
- Filter transactions by category_slug — parent slugs include all children

CATEGORIZATION:
- update_transactions: the preferred write for routine work — combines set_category, tag changes, and a comment into one atomic operation per transaction (max 50 per call). Use this when finishing a review: set the category AND remove the needs-review tag in one write.
- categorize_transaction: manually override a single transaction (sets category_override=true). Use only when you don't need the compound op.
- batch_categorize_transactions: override multiple transactions at once (max 500) without touching tags.
- reset_transaction_category: remove a manual override, let rules re-resolve the category
- bulk_recategorize: move ALL transactions matching filters from one category to another

TAXONOMY MANAGEMENT:
- export_categories / import_categories: bulk edit via TSV
- To merge/consolidate: set the merge_into column in the TSV to move all transactions from one category to another, then delete the source
- Transaction rules (via create_transaction_rule) handle automatic categorization during sync
