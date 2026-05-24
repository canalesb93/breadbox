//go:build !headless && !lite

package components

import (
	"fmt"
	"math"
)

// AmountIntent picks the sign + color treatment. See Amount for the
// full doc.
type AmountIntent string

const (
	// AmountTransaction is the default. Negative = income (+, green);
	// positive/zero = expense (no sign, default color).
	AmountTransaction AmountIntent = ""
	// AmountBalance preserves the original sign; no color treatment.
	AmountBalance AmountIntent = "balance"
	// AmountCost renders the absolute value with no sign or color.
	AmountCost AmountIntent = "cost"
)

// AmountFormat picks the display precision. See Amount for the full doc.
type AmountFormat string

const (
	// AmountFormatStandard renders "$1,234.56".
	AmountFormatStandard AmountFormat = ""
	// AmountFormatAbbreviated renders "$1.2M" at ≥ $1M, otherwise standard.
	AmountFormatAbbreviated AmountFormat = "abbreviated"
	// AmountFormatCompact renders "$50" for whole, "$12.34" otherwise.
	AmountFormatCompact AmountFormat = "compact"
)

// AmountProps configures the canonical Amount component.
type AmountProps struct {
	Value     float64
	Intent    AmountIntent // default AmountTransaction
	Format    AmountFormat // default AmountFormatStandard
	Precision int          // decimals for AmountCost; ignored for other intents (defaults to 2)
	Pending   bool
	Class     string
}

// amountClasses returns the space-separated class list for an Amount
// span. Always includes tabular-nums; adds text-success for the
// transaction intent when the value is negative (income); adds the
// pending opacity modifier when Pending is set; appends caller-provided
// Class last so it can override sizing/weight but not the core classes.
func amountClasses(p AmountProps) string {
	cls := "tabular-nums"
	if (p.Intent == AmountTransaction || p.Intent == "") && p.Value < 0 {
		cls += " text-success"
	}
	if p.Pending {
		cls += " bb-tx-amount--pending"
	}
	if p.Class != "" {
		cls += " " + p.Class
	}
	return cls
}

// AmountText returns the formatted text for an Amount, exported so the
// few non-templ render paths (e.g. command-palette JSON, MCP previews
// over time) can call the same formatter without rendering markup.
//
// Sign rules:
//   - AmountTransaction (default): negative → "+$X" (income), else "$X".
//   - AmountBalance: signed — "-$X" for negative, "$X" otherwise.
//   - AmountCost: absolute — "$X" regardless of sign.
func AmountText(p AmountProps) string {
	abs := math.Abs(p.Value)
	formatted := amountFormatAbs(abs, p.Format, p.Precision)
	switch p.Intent {
	case AmountBalance:
		if p.Value < 0 {
			return "-" + formatted
		}
		return formatted
	case AmountCost:
		return formatted
	default: // AmountTransaction
		if p.Value < 0 {
			return "+" + formatted
		}
		return formatted
	}
}

// amountFormatAbs formats a non-negative value per the requested
// AmountFormat. precision is honored only for AmountFormatStandard; the
// abbreviated and compact shapes carry their own fixed precision.
func amountFormatAbs(abs float64, format AmountFormat, precision int) string {
	switch format {
	case AmountFormatAbbreviated:
		if abs >= 1_000_000 {
			return fmt.Sprintf("$%.1fM", abs/1_000_000)
		}
		return standardCurrency(abs, 2)
	case AmountFormatCompact:
		return compactCurrency(abs)
	default:
		decimals := precision
		if decimals <= 0 {
			decimals = 2
		}
		return standardCurrency(abs, decimals)
	}
}

// standardCurrency renders "$1,234.56" with thousand-separator commas
// and the requested decimal precision. Shares rounding behavior with
// service.FormatCurrency for the 2-decimal case so payloads stay
// bit-identical with existing call sites.
func standardCurrency(abs float64, decimals int) string {
	if decimals == 2 {
		whole := int64(abs)
		cents := int(math.Round((abs - float64(whole)) * 100))
		if cents >= 100 {
			whole += int64(cents / 100)
			cents %= 100
		}
		return fmt.Sprintf("$%s.%02d", commaInt64(whole), cents)
	}
	scale := math.Pow(10, float64(decimals))
	rounded := math.Round(abs*scale) / scale
	whole := int64(rounded)
	frac := rounded - float64(whole)
	fracInt := int64(math.Round(frac * scale))
	if fracInt >= int64(scale) {
		whole += fracInt / int64(scale)
		fracInt %= int64(scale)
	}
	return fmt.Sprintf("$%s.%0*d", commaInt64(whole), decimals, fracInt)
}

// compactCurrency renders "$50" when whole, else "$12.34".
func compactCurrency(abs float64) string {
	rounded := math.Round(abs*100) / 100
	whole := math.Trunc(rounded)
	if rounded == whole {
		return "$" + commaInt64(int64(whole))
	}
	return standardCurrency(abs, 2)
}

// commaInt64 inserts thousand-separator commas in a non-negative int64.
// Defers to CommaInt for values ≥ 1000 so all comma logic lives in one
// place (helpers.go).
func commaInt64(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return CommaInt(n)
}
