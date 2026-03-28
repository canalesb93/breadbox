# Breadbox Context
> Core data model and amount conventions

Breadbox is a self-hosted financial data aggregation server for families. It syncs bank data from Plaid, Teller, and CSV imports into a unified PostgreSQL database.

DATA MODEL:
- Users: family members who own bank connections
- Connections: linked bank accounts via Plaid, Teller, or CSV import (status: active, error, pending_reauth, disconnected)
- Accounts: individual bank accounts (checking, savings, credit card, etc.) belonging to a connection
- Transactions: individual financial transactions belonging to an account

AMOUNT CONVENTION:
- Positive amounts = money out (debits, purchases, payments)
- Negative amounts = money in (credits, deposits, refunds)
- All amounts include iso_currency_code — never sum across different currencies
