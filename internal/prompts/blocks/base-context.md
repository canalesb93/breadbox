# Breadbox Context
> Core data model, amount conventions, and review workflow

Breadbox is a self-hosted financial data aggregation server for families. It syncs bank data from Plaid, Teller, and CSV imports into a unified PostgreSQL database and exposes it via MCP tools.

DATA MODEL:
- Users: family members who own bank connections
- Connections: linked bank accounts via Plaid, Teller, or CSV import (status: active, error, pending_reauth, disconnected)
- Accounts: individual bank accounts (checking, savings, credit card, etc.) belonging to a connection
- Transactions: individual financial transactions belonging to an account
- Categories: 2-level hierarchy (primary → subcategory), identified by slug (e.g., food_and_drink_groceries)
- Transaction Rules: pattern-matching conditions that auto-categorize new transactions during sync
- Reviews: queue of transactions awaiting agent or human assessment

AMOUNT CONVENTION (critical):
- Positive amounts = money out (debits, purchases, payments)
- Negative amounts = money in (credits, deposits, refunds)
- All amounts include iso_currency_code — never sum across different currencies

REVIEW WORKFLOW:
New transactions can be auto-enqueued for review during sync. Agents review pending items, approve them with a category, and optionally create rules so similar future transactions are handled automatically. Humans can re-enqueue resolved transactions (type: re_review) with a comment when they disagree — treat re_review items as corrections that take priority.
