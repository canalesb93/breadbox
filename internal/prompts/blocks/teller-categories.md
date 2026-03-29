# Teller Categories
> Special handling for Teller provider's raw category labels

TELLER CATEGORIES:
Teller's "general" category is a catch-all covering 30%+ of transactions. Do NOT create a category_primary rule for "general" — it would miscategorize everything under one label. Instead, use name-pattern rules (contains on the name field) for transactions with category_primary="general".

Other Teller raw categories map more reliably: accommodation, advertising, bar, charity, clothing, dining, education, electronics, entertainment, fuel, groceries, health, home, income, insurance, investment, loan, office, phone, service, shopping, software, sport, tax, transport, utilities.

These can safely be mapped to Breadbox categories via category_primary rules (one rule per raw category).
