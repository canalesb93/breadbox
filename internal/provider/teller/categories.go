package teller

// tellerCategories maps Teller transaction categories to Plaid-compatible
// primary categories used throughout Breadbox.
var tellerCategories = map[string]string{
	"accommodation":  "TRAVEL",
	"advertising":    "GENERAL_SERVICES",
	"bar":            "FOOD_AND_DRINK",
	"charity":        "GENERAL_SERVICES",
	"clothing":       "SHOPPING",
	"dining":         "FOOD_AND_DRINK",
	"education":      "GENERAL_SERVICES",
	"electronics":    "SHOPPING",
	"entertainment":  "ENTERTAINMENT",
	"fuel":           "TRANSPORTATION",
	"general":        "GENERAL_MERCHANDISE",
	"groceries":      "FOOD_AND_DRINK",
	"health":         "MEDICAL",
	"home":           "HOME_IMPROVEMENT",
	"income":         "INCOME",
	"insurance":      "LOAN_PAYMENTS",
	"investment":     "TRANSFER_IN",
	"loan":           "LOAN_PAYMENTS",
	"office":         "GENERAL_SERVICES",
	"phone":          "GENERAL_SERVICES",
	"service":        "GENERAL_SERVICES",
	"shopping":       "SHOPPING",
	"software":       "GENERAL_SERVICES",
	"sport":          "ENTERTAINMENT",
	"tax":            "GOVERNMENT_AND_NON_PROFIT",
	"transport":      "TRANSPORTATION",
	"transportation": "TRANSPORTATION",
	"utilities":      "RENT_AND_UTILITIES",
}

// mapCategory converts a Teller category string to a Breadbox primary category.
// Unknown categories default to GENERAL_MERCHANDISE.
func mapCategory(tellerCategory string) string {
	if primary, ok := tellerCategories[tellerCategory]; ok {
		return primary
	}
	return "GENERAL_MERCHANDISE"
}
