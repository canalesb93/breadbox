package csv

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ParseAmount strips currency symbols, thousands separators, and parenthetical
// negatives, then parses the result as a decimal.
func ParseAmount(raw string) (decimal.Decimal, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return decimal.Zero, fmt.Errorf("empty amount")
	}

	// Strip currency symbols.
	s = strings.NewReplacer("$", "", "€", "", "£", "", "¥", "").Replace(s)
	s = strings.TrimSpace(s)

	// Handle parenthetical negatives: (123.45) → -123.45
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = "-" + s[1:len(s)-1]
	}

	// Remove thousands separators (commas).
	s = strings.ReplaceAll(s, ",", "")

	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse amount %q: %w", raw, err)
	}
	return d, nil
}

// ParseDualColumns handles Capital One style where debit and credit are in
// separate columns. Returns positive for debits, negative for credits.
func ParseDualColumns(debitStr, creditStr string) (decimal.Decimal, error) {
	debitStr = strings.TrimSpace(debitStr)
	creditStr = strings.TrimSpace(creditStr)

	hasDebit := debitStr != ""
	hasCredit := creditStr != ""

	if !hasDebit && !hasCredit {
		return decimal.Zero, fmt.Errorf("both debit and credit columns are empty")
	}

	if hasDebit && hasCredit {
		debit, err := ParseAmount(debitStr)
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse debit: %w", err)
		}
		credit, err := ParseAmount(creditStr)
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse credit: %w", err)
		}
		return debit.Sub(credit), nil
	}

	if hasDebit {
		d, err := ParseAmount(debitStr)
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse debit: %w", err)
		}
		return d, nil
	}

	// Only credit
	c, err := ParseAmount(creditStr)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse credit: %w", err)
	}
	return c.Neg(), nil
}

// NormalizeSign adjusts the sign so that positive = debit (money out).
// If positiveIsDebit is true, the amount is already in the correct convention.
// If false (bank convention: positive = credit), negate it.
func NormalizeSign(amount decimal.Decimal, positiveIsDebit bool) decimal.Decimal {
	if positiveIsDebit {
		return amount
	}
	return amount.Neg()
}
