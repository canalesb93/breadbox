# Teller Categories
> Special handling for Teller provider's raw category labels

TELLER CATEGORIES:
Teller's "general" category is a useless catch-all covering 30%+ of transactions. Do NOT create a category_primary rule for "general" — it will miscategorize everything. Instead, use name-pattern rules (contains on the name field) for transactions with category_primary="general".

Known Teller raw categories: accommodation, advertising, bar, charity, clothing, dining, education, electronics, entertainment, fuel, general, groceries, health, home, income, insurance, investment, loan, office, phone, service, shopping, software, sport, tax, transport, utilities
