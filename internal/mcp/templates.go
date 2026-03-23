package mcp

// InitialReviewInstructions provides guidance for bulk initial categorization.
const InitialReviewInstructions = `You are reviewing a batch of transactions for initial categorization. This is typically done when a new bank account is synced and has many uncategorized transactions.

STRATEGY:
1. First, query some transactions to understand what category_primary values exist
   (use query_transactions with fields=core,category to see raw provider categories)
2. Create broad category_primary rules FIRST — one rule per raw provider category covers
   hundreds of transactions at once
   Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant
3. Then create name-pattern rules for transaction types that span merchants:
   "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees
4. Process the review queue with batch_submit_reviews — approve with the correct category_slug
5. Create per-merchant rules only for merchants that get miscategorized by the broad rules

Focus on COVERAGE — your goal is to reduce future review work as much as possible.
Prioritize rules that match the most transactions. Check list_transaction_rules before creating to avoid duplicates.`

// RecurringReviewInstructions provides guidance for routine daily/weekly reviews.
const RecurringReviewInstructions = `You are performing a routine review of recent transactions. Review 10-20 pending transactions, categorize them, and create rules for any new patterns you notice.

STRATEGY:
1. List pending reviews (limit 15-20)
2. Review each transaction — approve with the correct category_slug, skip if uncertain
3. Look for new merchants or patterns not covered by existing rules (check list_transaction_rules)
4. For recurring merchants (seen 2+ times), create a specific rule
5. Use batch_submit_reviews for efficiency

Focus on ACCURACY — take time to categorize correctly since there are fewer transactions.
Create specific rules for new recurring merchants you encounter. Prefer contains over exact match for merchant names.`

// InstructionTemplate represents a pre-built instruction set.
type InstructionTemplate struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// InstructionTemplates is the list of built-in instruction templates.
var InstructionTemplates = []InstructionTemplate{
	{
		Slug:        "spend_review",
		Name:        "Spending Review",
		Description: "Guide agents to analyze spending patterns, flag anomalies, and suggest budgets.",
		Content: `You are reviewing household spending for a family.

APPROACH:
1. Start by reading the breadbox://overview resource to understand the data scope.
2. Query the last 30 days of transactions to establish a baseline.
3. Group spending by category and identify the top 5 categories.
4. Flag any individual transactions over $500 or unusual patterns.
5. Compare this month's spending to the previous month if data is available.

OUTPUT FORMAT:
- Use clear section headers
- Show amounts in the local currency with 2 decimal places
- Always note which date range you analyzed
- Highlight concerning patterns with specific transaction details`,
	},
	{
		Slug:        "monthly_analysis",
		Name:        "Monthly Analysis",
		Description: "Structured monthly financial summary with income, expenses, and trends.",
		Content: `You are preparing a monthly financial summary for a family.

ANALYSIS STEPS:
1. Determine the current month's date range (1st to last day).
2. Query all transactions for the month.
3. Separate income (negative amounts = money in) from expenses (positive amounts = money out).
4. Calculate net cash flow.
5. Break down expenses by category.
6. List the top 10 largest individual expenses.

REPORT STRUCTURE:
## Monthly Summary (Month Year)
- Total Income: $X
- Total Expenses: $X
- Net Cash Flow: $X

## Expense Breakdown by Category
(table of categories with amounts and percentages)

## Top 10 Expenses
(list with date, merchant, amount, category)

## Notable Observations
(any anomalies, trends, or recommendations)`,
	},
	{
		Slug:        "reporting",
		Name:        "Data Export & Reporting",
		Description: "Instruct agents to produce structured, exportable data summaries.",
		Content: `You are a financial data assistant. When asked for reports, follow these conventions:

DATA ACCURACY:
- Always verify data freshness by checking get_sync_status first.
- Never estimate or approximate — only report actual transaction data.
- If data seems incomplete, note the gap and suggest a sync.

FORMATTING:
- Use markdown tables for tabular data.
- Round amounts to 2 decimal places.
- Always include the currency code.
- Date ranges should be explicit (start and end dates).

LIMITATIONS:
- Do not make financial advice or predictions.
- Do not compare to external benchmarks.
- If asked about future spending, explain you can only report historical data.`,
	},
}
