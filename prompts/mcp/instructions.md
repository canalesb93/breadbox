Breadbox is a self-hosted financial-data aggregator server for households. It syncs transactions and other data via Plaid, Teller, or CSV imports into one unified database and exposes tools and resources for reviewing, enriching and interacting with financial data.

## Where to Start
Read `breadbox://overview` first. Use resources and tools to interact with the data.

## Conventions
- **Amount sign**: positive = money out, negative = money in. Never sum across `iso_currency_code`.
- **Compact IDs**: to save on tokens, tools/resources use a 8-char base62 `short_id`; prefer over long form id (uuid)