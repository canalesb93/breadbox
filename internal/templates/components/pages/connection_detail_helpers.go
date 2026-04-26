package pages

import "fmt"

// connDetailHeaderTileBg returns the icon-tile background class by provider.
func connDetailHeaderTileBg(provider string) string {
	switch provider {
	case "plaid":
		return "bg-info/10"
	case "teller":
		return "bg-success/10"
	default:
		return "bg-warning/10"
	}
}

// connDetailSuccessRateClass returns the success-rate text color class.
func connDetailSuccessRateClass(rate float64) string {
	switch {
	case rate >= 90:
		return "text-success"
	case rate >= 50:
		return "text-warning"
	default:
		return "text-error"
	}
}

// connDetailAvgDuration renders the avg-duration figure. Mirrors the
// nested template branches in the original markup so the dash-fallback
// span survives byte-for-byte. Emitted via @templ.Raw because it can
// produce literal &mdash;.
func connDetailAvgDuration(sec float64) string {
	switch {
	case sec > 60:
		return fmt.Sprintf("%.0fm", sec/60.0)
	case sec > 1:
		return fmt.Sprintf("%.1fs", sec)
	case sec > 0:
		return fmt.Sprintf("%.0fms", sec*1000.0)
	default:
		return `<span class="text-base-content/30">&mdash;</span>`
	}
}

// connDetailBarStyle returns the inline style attribute value for a
// success-or-error bar in the 14-day timeline. Mirrors the original
// `style="height: <pct>px; min-height: 4px;"` calculation. `mine` is the
// count for this color, `other` is the count for the other color, and
// total is mine+other (or the row total).
func connDetailBarStyle(mine, other, total int) string {
	if other > 0 {
		// Pixel-equivalent height (52 * mine / total) so the two bars
		// add up to 52px when both colors are present.
		px := (float64(mine) * 52.0) / float64(total)
		return fmt.Sprintf("height: %.0fpx; min-height: 4px;", px)
	}
	return "height: 52px; min-height: 4px;"
}

// connDetailAccentBar returns the colored accent-bar class for an account
// card (left edge). Mirrors the source map of account types to bg colors.
func connDetailAccentBar(t string) string {
	switch t {
	case "depository":
		return "bg-info/50"
	case "credit":
		return "bg-warning/50"
	case "loan":
		return "bg-error/40"
	case "investment":
		return "bg-success/50"
	default:
		return "bg-base-300/50"
	}
}

// connDetailAccountTileBg returns the icon-tile background class for an
// account card.
func connDetailAccountTileBg(t string) string {
	switch t {
	case "depository":
		return "bg-info/8"
	case "credit":
		return "bg-warning/8"
	case "loan":
		return "bg-error/8"
	case "investment":
		return "bg-success/8"
	default:
		return "bg-base-200/60"
	}
}

// connDetailAccountIcon returns the lucide icon name for an account type.
// Mirrors the funcMap "accountTypeIcon" helper.
func connDetailAccountIcon(t string) string {
	switch t {
	case "depository":
		return "landmark"
	case "credit":
		return "credit-card"
	case "loan":
		return "file-text"
	case "investment":
		return "trending-up"
	default:
		return "wallet"
	}
}

// connDetailAccountIconColor returns the icon color class for an account type.
func connDetailAccountIconColor(t string) string {
	switch t {
	case "depository":
		return "text-info/60"
	case "credit":
		return "text-warning/70"
	case "loan":
		return "text-error/60"
	case "investment":
		return "text-success/60"
	default:
		return "text-base-content/40"
	}
}

// connDetailBalanceColor returns the balance text color class — credit/loan
// types render warning/error tints; everything else uses default.
func connDetailBalanceColor(t string) string {
	switch t {
	case "credit":
		return "text-warning/80"
	case "loan":
		return "text-error/80"
	default:
		return ""
	}
}

// connDetailAccountTypeLabel returns the human-readable label for an
// account's type/subtype combination. Mirrors the funcMap
// "accountTypeLabel" helper.
func connDetailAccountTypeLabel(acctType, subtype string, subtypeValid bool) string {
	if subtypeValid && subtype != "" {
		labels := map[string]string{
			"checking":     "Checking",
			"savings":      "Savings",
			"credit card":  "Credit Card",
			"credit_card":  "Credit Card",
			"money market": "Money Market",
			"money_market": "Money Market",
			"cd":           "CD",
			"paypal":       "PayPal",
			"student":      "Student Loan",
			"mortgage":     "Mortgage",
			"auto":         "Auto Loan",
			"401k":         "401(k)",
			"ira":          "IRA",
			"brokerage":    "Brokerage",
			"prepaid":      "Prepaid",
			"hsa":          "HSA",
		}
		if label, ok := labels[subtype]; ok {
			return label
		}
		return subtype
	}
	labels := map[string]string{
		"depository": "Bank Account",
		"credit":     "Credit Card",
		"loan":       "Loan",
		"investment": "Investment",
	}
	if label, ok := labels[acctType]; ok {
		return label
	}
	return acctType
}

// connDetailSyncIconBg returns the icon-circle background class for a sync log row.
func connDetailSyncIconBg(status string) string {
	switch status {
	case "success":
		return "bg-success/12"
	case "error":
		return "bg-error/12"
	default:
		return "bg-base-200"
	}
}

// connDetailErrTitle returns the title= attribute for the inline error
// paragraph (raw error string when available).
func connDetailErrTitle(sl SyncLogRow) string {
	if sl.ErrorMessageValid {
		return sl.ErrorMessageString
	}
	return ""
}
