package components

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/a-h/templ"
)

// FormatBalance mirrors the html/template funcMap "formatBalance" helper so
// templ renders the same money strings as the legacy templates.
func FormatBalance(amount float64) string {
	abs := math.Abs(amount)
	if abs >= 1_000_000 {
		return fmt.Sprintf("$%.1fM", abs/1_000_000)
	}
	if abs >= 1_000 {
		whole := int(abs)
		cents := int((abs - float64(whole)) * 100)
		s := addThousandsCommas(fmt.Sprintf("%d", whole))
		return fmt.Sprintf("$%s.%02d", s, cents)
	}
	return fmt.Sprintf("$%.2f", abs)
}

// FormatAmount mirrors the funcMap "formatAmount" helper (handles negatives).
func FormatAmount(amount float64) string {
	neg := amount < 0
	abs := math.Abs(amount)
	whole := int(abs)
	cents := int(math.Round((abs - float64(whole)) * 100))
	s := addThousandsCommas(fmt.Sprintf("%d", whole))
	formatted := fmt.Sprintf("$%s.%02d", s, cents)
	if neg {
		return "-" + formatted
	}
	return formatted
}

func addThousandsCommas(s string) string {
	if len(s) <= 3 {
		return s
	}
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

// CommaInt adds thousands separators to an integer value.
func CommaInt(n int64) string {
	return addThousandsCommas(fmt.Sprintf("%d", n))
}

// AccountTypeIcon maps a Plaid/Teller account type to a Lucide icon name.
func AccountTypeIcon(t string) string {
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

// AccountTypeLabel maps (type, subtype) to a human-readable label.
func AccountTypeLabel(acctType, subtype string) string {
	if subtype != "" {
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

// FirstChar returns the first letter/digit of a string in uppercase. Falls
// back to "?" for empty strings.
func FirstChar(s string) string {
	if s == "" {
		return "?"
	}
	for _, r := range s {
		c := strings.ToUpper(string(r))
		if (c >= "A" && c <= "Z") || (c >= "0" && c <= "9") {
			return c
		}
	}
	return strings.ToUpper(string([]rune(s)[0]))
}

// RelativeDate formats a YYYY-MM-DD date as "Today", "Yesterday", "N days
// ago", etc., mirroring the funcMap "relativeDate" helper.
func RelativeDate(s string) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	d := t.In(now.Location())
	dateOnly := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
	days := int(today.Sub(dateOnly).Hours() / 24)
	switch {
	case days == 0:
		return "Today"
	case days == 1:
		return "Yesterday"
	case days >= 2 && days <= 6:
		return fmt.Sprintf("%d days ago", days)
	case days >= 7 && days <= 13:
		return "1 week ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}

// Deref returns the dereferenced string or an empty string if nil.
func Deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// categoryColorOrDefault resolves a nullable category color down to a concrete
// OKLCH value, falling back to the neutral default used across the admin UI.
func categoryColorOrDefault(c *string) string {
	if c == nil || *c == "" {
		return "oklch(0.65 0 0)"
	}
	return *c
}

// AvatarURL mirrors the funcMap "avatarURL" for user-id string inputs.
func AvatarURL(userID *string) string {
	if userID == nil || *userID == "" {
		return "/avatars/unknown"
	}
	return "/avatars/" + *userID
}

// --- inline-style helpers --------------------------------------------------
// templ validates `style=""` attribute values to prevent unsafe CSS injection,
// so these helpers construct fully sanitized strings via fmt.Sprintf and
// return templ.SafeCSS values. Inputs come from our own maps, so we know they
// don't carry user-controlled content.

// AllocationBarStyle builds the per-segment style for the net-worth allocation bar.
func AllocationBarStyle(slice AllocationSlice) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("width: %.1f%%; background: %s; opacity: 0.7;", slice.Percent, slice.Color))
}

// AllocationDotStyle builds the style for the small legend dots next to each
// allocation label.
func AllocationDotStyle(slice AllocationSlice) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("background: %s; opacity: 0.7;", slice.Color))
}

// CategoryAvatarStyle sets the --avatar-color custom property for the small
// category bubble on a transaction row.
func CategoryAvatarStyle(color *string) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("--avatar-color: %s", categoryColorOrDefault(color)))
}

// CategoryDotStyle sets the background color for the per-row category dot.
func CategoryDotStyle(color *string) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("background-color: %s", categoryColorOrDefault(color)))
}

// --- class helpers ---------------------------------------------------------

// AccountBalanceClass picks the class for the big balance number on an
// account card — liabilities are rendered in red-ish tone.
func AccountBalanceClass(isLiability bool) string {
	base := "text-lg font-bold tabular-nums tracking-tight"
	if isLiability {
		return base + " text-error/80"
	}
	return base
}

// TxAmountClass reproduces tx_row_compact's amount-tone logic: negative
// amounts are money in (income) and get the "--income" modifier.
func TxAmountClass(amount float64) string {
	base := "bb-tx-amount tabular-nums"
	if amount < 0 {
		return base + " bb-tx-amount--income"
	}
	return base
}

// ConnectionIconWrapperClass reproduces the dashboard connection-row icon
// wrapper's background tint logic.
func ConnectionIconWrapperClass(conn ConnectionHealthRow) string {
	base := "w-7 h-7 rounded-lg flex items-center justify-center"
	switch {
	case conn.Status == "error":
		return base + " bg-error/10"
	case conn.Status == "pending_reauth":
		return base + " bg-warning/10"
	case conn.IsStale:
		return base + " bg-warning/8"
	default:
		return base + " bg-success/8"
	}
}

// SyncRateClass picks the tone for the 24h sync-success-rate pill on the
// connection-health panel.
func SyncRateClass(rate float64) string {
	base := "flex items-center gap-1.5"
	switch {
	case rate >= 90:
		return base + " text-success"
	case rate >= 50:
		return base + " text-warning"
	default:
		return base + " text-error/80"
	}
}

// ConnectionAccountsLine renders the "N accounts · synced …" sub-label below
// a connection name when it's in a non-error state.
func ConnectionAccountsLine(conn ConnectionHealthRow) string {
	suffix := " accounts"
	if conn.AccountCount == 1 {
		suffix = " account"
	}
	return fmt.Sprintf("%d%s · synced %s", conn.AccountCount, suffix, conn.LastSyncedAt)
}
