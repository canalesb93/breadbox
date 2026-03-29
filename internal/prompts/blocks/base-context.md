# Breadbox Context
> Core data model, amount conventions, and review workflow

Breadbox is a self-hosted financial data aggregation server for families. It syncs bank data from Plaid, Teller, and CSV imports into a unified PostgreSQL database and exposes it via MCP tools.

DATA MODEL:
- Users: family members who own bank connections
- Connections: linked bank accounts via Plaid, Teller, or CSV import (status: active, error, pending_reauth, disconnected)
- Accounts: individual bank accounts (checking, savings, credit card, etc.) belonging to a connection
- Transactions: individual financial transactions belonging to an account
- Categories: 2-level hierarchy (primary → subcategory), identified by slug (e.g., food_and_drink_groceries)
- Transaction Rules: pattern-matching conditions that pre-categorize new transactions during sync (agents still review them)
- Reviews: queue of transactions awaiting agent or human assessment — the primary workflow for ensuring accuracy

AMOUNT CONVENTION (critical):
- Positive amounts = money out (debits, purchases, payments)
- Negative amounts = money in (credits, deposits, refunds)
- All amounts include iso_currency_code — never sum across different currencies

REVIEW WORKFLOW:
New transactions are enqueued for review during sync. Rules may pre-categorize them, but agents still review every item — confirming correct pre-categorizations and fixing incorrect ones. This ensures accuracy while rules reduce cognitive load over time. Agents create new rules as they discover patterns, building the system's knowledge incrementally. Humans can re-enqueue resolved transactions (type: re_review) with a comment when they disagree — treat re_review items as corrections that take priority.
