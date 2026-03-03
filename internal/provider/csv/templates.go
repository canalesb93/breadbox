package csv

import "strings"

// Template describes a known bank CSV export format.
type Template struct {
	Name              string
	HeaderPatterns    []string // exact expected headers (case-insensitive match)
	DateColumn        string
	AmountColumn      string
	DescriptionColumn string
	CategoryColumn    string // empty = N/A
	MerchantColumn    string // empty = N/A
	PositiveIsDebit   bool
	DateFormat        string
	HasDebitCredit    bool   // Capital One style: separate debit/credit columns
	DebitColumn       string
	CreditColumn      string
}

var templates = []Template{
	{
		Name:              "Chase Credit Card",
		HeaderPatterns:    []string{"Transaction Date", "Post Date", "Description", "Category", "Type", "Amount", "Memo"},
		DateColumn:        "Transaction Date",
		AmountColumn:      "Amount",
		DescriptionColumn: "Description",
		CategoryColumn:    "Category",
		PositiveIsDebit:   false,
		DateFormat:        "01/02/2006",
	},
	{
		Name:              "Chase Checking",
		HeaderPatterns:    []string{"Details", "Posting Date", "Description", "Amount", "Type", "Balance", "Check or Slip #"},
		DateColumn:        "Posting Date",
		AmountColumn:      "Amount",
		DescriptionColumn: "Description",
		PositiveIsDebit:   false,
		DateFormat:        "01/02/2006",
	},
	{
		Name:              "Bank of America",
		HeaderPatterns:    []string{"Date", "Description", "Amount", "Running Bal."},
		DateColumn:        "Date",
		AmountColumn:      "Amount",
		DescriptionColumn: "Description",
		PositiveIsDebit:   false,
		DateFormat:        "01/02/2006",
	},
	{
		Name:              "Capital One",
		HeaderPatterns:    []string{"Transaction Date", "Posted Date", "Card No.", "Description", "Category", "Debit", "Credit"},
		DateColumn:        "Transaction Date",
		DescriptionColumn: "Description",
		CategoryColumn:    "Category",
		PositiveIsDebit:   true,
		DateFormat:        "2006-01-02",
		HasDebitCredit:    true,
		DebitColumn:       "Debit",
		CreditColumn:      "Credit",
	},
	{
		Name:              "Amex",
		HeaderPatterns:    []string{"Date", "Description", "Amount", "Extended Details", "Appears On Your Statement As", "Address", "City/State", "Zip Code", "Country", "Reference", "Category"},
		DateColumn:        "Date",
		AmountColumn:      "Amount",
		DescriptionColumn: "Description",
		CategoryColumn:    "Category",
		PositiveIsDebit:   true,
		DateFormat:        "01/02/2006",
	},
	{
		Name:              "Wells Fargo",
		HeaderPatterns:    nil, // No header row — 5 positional columns
		DateColumn:        "0",
		AmountColumn:      "1",
		DescriptionColumn: "4",
		PositiveIsDebit:   false,
		DateFormat:        "01/02/2006",
	},
}

// DetectTemplate matches parsed headers against known bank templates.
// Returns nil if no template matches.
func DetectTemplate(headers []string) *Template {
	normalized := make([]string, len(headers))
	for i, h := range headers {
		normalized[i] = strings.ToLower(strings.TrimSpace(h))
	}

	for i := range templates {
		t := &templates[i]
		if t.HeaderPatterns == nil {
			continue // Skip positional templates (Wells Fargo)
		}
		if len(normalized) < len(t.HeaderPatterns) {
			continue
		}
		match := true
		for j, pattern := range t.HeaderPatterns {
			if j >= len(normalized) || normalized[j] != strings.ToLower(pattern) {
				match = false
				break
			}
		}
		if match {
			return t
		}
	}
	return nil
}

// headerPatterns maps field names to common header variations.
var headerPatterns = map[string][]string{
	"date":          {"date", "transaction date", "posting date", "post date", "trans date"},
	"amount":        {"amount", "transaction amount", "debit/credit", "value"},
	"description":   {"description", "transaction description", "memo", "payee", "name", "details"},
	"category":      {"category", "type", "transaction type"},
	"merchant_name": {"merchant", "merchant name", "payee name"},
}

// DetectColumns uses generic header pattern matching to suggest column mappings.
// Returns a map of field name → column index.
func DetectColumns(headers []string) map[string]int {
	result := make(map[string]int)
	normalized := make([]string, len(headers))
	for i, h := range headers {
		normalized[i] = strings.ToLower(strings.TrimSpace(h))
	}

	for field, patterns := range headerPatterns {
		for _, pattern := range patterns {
			for i, header := range normalized {
				if header == pattern {
					if _, exists := result[field]; !exists {
						result[field] = i
					}
				}
			}
		}
	}

	return result
}
