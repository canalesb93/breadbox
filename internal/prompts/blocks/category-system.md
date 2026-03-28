# Category System
> Category hierarchy, slugs, merging, and bulk editing

CATEGORY SYSTEM:
- Categories are organized in a 2-level hierarchy (primary → detailed subcategories)
- Each category has: id (UUID), slug (stable identifier), display_name (human label), icon, color
- Use list_categories to get the full taxonomy tree with IDs and slugs
- Filter transactions with category_slug param (parent slug includes all children)
- Use categorize_transaction to manually override a single transaction's category
- For bulk category changes: use batch_categorize_transactions (multiple transactions, one category) or bulk_recategorize (change all transactions from one category to another)
- Use reset_transaction_category to undo a manual override
- Use list_unmapped_categories to find raw provider categories without mappings
- Use transaction rules to automatically categorize transactions based on conditions (name, amount, provider, etc). Rules are evaluated during sync in priority order.
- For bulk editing categories: use export_categories / import_categories to export TSV text, edit it, and re-import
- To simplify/consolidate categories: export categories, then set the merge_into column to a target slug for categories you want to merge. All transactions and mappings from the source are moved to the target.
