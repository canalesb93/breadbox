package teller

// categoryMapping holds both the primary and detailed category for a Teller
// transaction category.
type categoryMapping struct {
	primary  string
	detailed string
}

// tellerCategories maps Teller transaction categories to Plaid-compatible
// primary and detailed categories used throughout Breadbox.
var tellerCategories = map[string]categoryMapping{
	"accommodation":  {"TRAVEL", "TRAVEL_LODGING"},
	"advertising":    {"GENERAL_SERVICES", "GENERAL_SERVICES_OTHER_GENERAL_SERVICES"},
	"bar":            {"FOOD_AND_DRINK", "FOOD_AND_DRINK_BEER_WINE_AND_LIQUOR"},
	"charity":        {"GOVERNMENT_AND_NON_PROFIT", "GOVERNMENT_AND_NON_PROFIT_DONATIONS"},
	"clothing":       {"SHOPPING", "SHOPPING_CLOTHING_AND_ACCESSORIES"},
	"dining":         {"FOOD_AND_DRINK", "FOOD_AND_DRINK_RESTAURANT"},
	"education":      {"GENERAL_SERVICES", "GENERAL_SERVICES_EDUCATION"},
	"electronics":    {"SHOPPING", "SHOPPING_ELECTRONICS"},
	"entertainment":  {"ENTERTAINMENT", "ENTERTAINMENT_OTHER_ENTERTAINMENT"},
	"fuel":           {"TRANSPORTATION", "TRANSPORTATION_GAS"},
	"general":        {"GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_OTHER_GENERAL_MERCHANDISE"},
	"groceries":      {"FOOD_AND_DRINK", "FOOD_AND_DRINK_GROCERIES"},
	"health":         {"MEDICAL", "MEDICAL_OTHER_MEDICAL"},
	"home":           {"HOME_IMPROVEMENT", "HOME_IMPROVEMENT_OTHER_HOME_IMPROVEMENT"},
	"income":         {"INCOME", "INCOME_OTHER_INCOME"},
	"insurance":      {"LOAN_PAYMENTS", "LOAN_PAYMENTS_INSURANCE_PAYMENT"},
	"investment":     {"TRANSFER_IN", "TRANSFER_IN_INVESTMENT_AND_RETIREMENT_FUNDS"},
	"loan":           {"LOAN_PAYMENTS", "LOAN_PAYMENTS_OTHER_PAYMENT"},
	"office":         {"GENERAL_SERVICES", "GENERAL_SERVICES_OTHER_GENERAL_SERVICES"},
	"phone":          {"RENT_AND_UTILITIES", "RENT_AND_UTILITIES_TELEPHONE"},
	"service":        {"GENERAL_SERVICES", "GENERAL_SERVICES_OTHER_GENERAL_SERVICES"},
	"shopping":       {"SHOPPING", "SHOPPING_OTHER_SHOPPING"},
	"software":       {"GENERAL_SERVICES", "GENERAL_SERVICES_OTHER_GENERAL_SERVICES"},
	"sport":          {"ENTERTAINMENT", "ENTERTAINMENT_SPORTING_EVENTS_AMUSEMENT_PARKS_AND_MUSEUMS"},
	"tax":            {"GOVERNMENT_AND_NON_PROFIT", "GOVERNMENT_AND_NON_PROFIT_TAX_PAYMENT"},
	"transport":      {"TRANSPORTATION", "TRANSPORTATION_OTHER_TRANSPORTATION"},
	"transportation": {"TRANSPORTATION", "TRANSPORTATION_OTHER_TRANSPORTATION"},
	"utilities":      {"RENT_AND_UTILITIES", "RENT_AND_UTILITIES_OTHER_UTILITIES"},
}

// mapCategory converts a Teller category string to a Breadbox primary and
// detailed category. Unknown categories default to GENERAL_MERCHANDISE with
// no detailed value.
func mapCategory(tellerCategory string) (primary string, detailed *string) {
	if m, ok := tellerCategories[tellerCategory]; ok {
		return m.primary, &m.detailed
	}
	return "GENERAL_MERCHANDISE", nil
}
