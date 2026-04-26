// Package components hosts templ-generated admin UI components.
//
// Helpers here mirror the admin funcMap (internal/admin/templates.go) so a
// component produces the same HTML whether it's rendered standalone, via
// another templ component, or through the renderComponent bridge. Keep the
// two in lock-step — drift shows up as different rows between pages.
package components

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/service"
)

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// firstChar returns the first A–Z/0–9 rune of s uppercased, or "?" when s
// has none. Used for avatar letter fallbacks.
func firstChar(s string) string {
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

func formatDate(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("Jan 2, 2006")
	}
	return s
}

// relativeDate returns "Today", "Yesterday", "N days ago" (2–6), "1 week ago"
// (7–13), or an absolute "Jan 2, 2006" date otherwise. Mirrors the admin
// funcMap of the same name so components and html/template stay aligned.
func relativeDate(s string) string {
	return relativeDateAt(s, time.Now())
}

// relativeDateAt is the testable core of relativeDate — takes an explicit
// "now" so tests don't depend on wall-clock time.
func relativeDateAt(s string, now time.Time) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
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

func formatAmount(amount float64) string {
	formatted := service.FormatCurrency(math.Abs(amount))
	if amount < 0 {
		return "-" + formatted
	}
	return formatted
}

func avatarURL(id string) string {
	if id == "" {
		return "/avatars/unknown"
	}
	return "/avatars/" + id
}

// titleCase mirrors admin.titleCaseMerchant: ALL-CAPS or all-lowercase input
// is rewritten to Title Case; mixed-case input is returned unchanged so
// already-styled names like "McDonald's" aren't mangled.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	upper := strings.ToUpper(s)
	lower := strings.ToLower(s)
	if s != upper && s != lower {
		return s
	}
	smallWords := map[string]bool{
		"a": true, "an": true, "and": true, "as": true, "at": true,
		"by": true, "for": true, "in": true, "of": true, "on": true,
		"or": true, "the": true, "to": true, "vs": true, "via": true,
	}
	words := strings.Fields(lower)
	for i, w := range words {
		// Abbreviations with periods ("h.e." → "H.E.") uppercase every segment.
		if strings.Contains(w, ".") {
			parts := strings.Split(w, ".")
			for j, p := range parts {
				if len(p) > 0 {
					parts[j] = strings.ToUpper(p)
				}
			}
			words[i] = strings.Join(parts, ".")
			continue
		}
		// Short non-article words are likely acronyms (ATM, US, AB).
		if len(w) <= 2 && !smallWords[w] {
			words[i] = strings.ToUpper(w)
			continue
		}
		if i == 0 || !smallWords[w] {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// pluralInt constrains pluralS to signed int types so the same helper works
// for `len(slice)` (int) and sqlc-derived counts (int64) without casting.
type pluralInt interface {
	~int | ~int32 | ~int64
}

func pluralS[T pluralInt](n T) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func amount2f(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

// FormatBalance formats an amount for balance display. Values >= $1M are
// abbreviated ($1.2M); values >= $1K use comma-thousands ($1,234.56); all
// others use two-decimal notation ($123.45). Sign is ignored — always absolute.
// Mirrors the "formatBalance" admin funcMap entry; keep them in sync.
func FormatBalance(amount float64) string {
	abs := math.Abs(amount)
	if abs >= 1_000_000 {
		return fmt.Sprintf("$%.1fM", abs/1_000_000)
	}
	if abs >= 1_000 {
		return fmt.Sprintf("$%s", commaAmount(abs))
	}
	return fmt.Sprintf("$%.2f", abs)
}

// commaAmount formats a non-negative float as "1,234.56" (no $ prefix).
func commaAmount(f float64) string {
	whole := int64(f)
	cents := int(math.Round((f - float64(whole)) * 100))
	s := CommaInt(whole)
	return fmt.Sprintf("%s.%02d", s, cents)
}

// CommaInt formats a non-negative integer with thousands-separator commas:
// 1234567 → "1,234,567". Used by FormatBalance and templates that need
// a plain integer count formatted with commas.
func CommaInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	out := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(c)
	}
	return out
}

// FormatIntervalMinutes renders a sync interval in minutes as a short human
// label: "24h", "4h", "30m", "1d". Mirrors the "formatIntervalMinutes" admin
// funcMap entry; keep them in sync.
func FormatIntervalMinutes(minutes int) string {
	if minutes <= 0 {
		return "N/A"
	}
	if minutes >= 1440 && minutes%1440 == 0 {
		d := minutes / 1440
		if d == 1 {
			return "24h"
		}
		return fmt.Sprintf("%dd", d)
	}
	if minutes >= 60 && minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	if minutes >= 60 {
		return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%dm", minutes)
}

// Exported wrappers for the shared formatting helpers. Used by the admin
// funcMap (internal/admin/templates.go) and other packages so there's one
// canonical implementation. Keep these in lock-step with their lowercase
// counterparts above — drift surfaces as different rows between pages.

// FirstChar returns the first A–Z/0–9 rune of s uppercased, or "?" when s has
// none. See firstChar.
func FirstChar(s string) string { return firstChar(s) }

// FormatDate formats a "2006-01-02" date string as "Jan 2, 2006". Returns the
// input unchanged if it doesn't parse. See formatDate.
func FormatDate(s string) string { return formatDate(s) }

// RelativeDate renders a "2006-01-02" date as "Today", "Yesterday", etc.
// See relativeDate.
func RelativeDate(s string) string { return relativeDate(s) }

// FormatAmount renders a signed amount with currency prefix (e.g. "-$1.50").
// See formatAmount.
func FormatAmount(amount float64) string { return formatAmount(amount) }

// CommaAmount formats a non-negative float as "1,234.56" (no currency prefix
// or sign). See commaAmount.
func CommaAmount(f float64) string { return commaAmount(f) }

// TitleCase rewrites ALL-CAPS or all-lowercase input to Title Case, leaving
// mixed-case input untouched. See titleCase.
func TitleCase(s string) string { return titleCase(s) }

// PluralS returns "" for n == 1, else "s". See pluralS.
func PluralS[T pluralInt](n T) string { return pluralS(n) }

// StatusBadge renders the inline HTML for a connection-status badge. Same
// markup as the admin funcMap "statusBadge" entry — reused by templ pages
// (via @templ.Raw) and the funcMap helper so all three sites stay aligned.
func StatusBadge(status string) string {
	switch status {
	case "active":
		return `<span class="badge badge-soft badge-success badge-sm">Active</span>`
	case "pending_reauth":
		return `<span class="badge badge-soft badge-warning badge-sm">Reauth Needed</span>`
	case "error":
		return `<span class="badge badge-soft badge-error badge-sm">Error</span>`
	default:
		return `<span class="badge badge-ghost badge-sm">Disconnected</span>`
	}
}

// ErrorMessage maps Plaid/Teller connection error codes to human-friendly
// strings. Mirrors the admin funcMap "errorMessage" entry. Unknown codes
// pass through unchanged.
func ErrorMessage(code string) string {
	switch code {
	case "ITEM_LOGIN_REQUIRED":
		return "Your bank login has changed. Please re-authenticate."
	case "INSUFFICIENT_CREDENTIALS":
		return "Additional credentials are needed. Please re-authenticate."
	case "INVALID_CREDENTIALS":
		return "Your bank credentials are incorrect. Please re-authenticate."
	case "MFA_NOT_SUPPORTED":
		return "This connection requires MFA which is not supported. Please reconnect."
	case "NO_ACCOUNTS":
		return "No accounts found for this connection."
	case "enrollment.disconnected":
		return "This bank connection has been disconnected."
	}
	return code
}

// PageRange returns the page numbers to display in a paginator. For totals
// up to 7 it returns every page. Beyond that, it returns first, last,
// current, and current's neighbors, inserting 0 as an ellipsis sentinel
// wherever a gap would appear. Used by the admin funcMap and any templ
// component that renders pagination, so all paginators stay aligned.
func PageRange(current, total int) []int {
	if total <= 7 {
		out := make([]int, total)
		for i := range out {
			out[i] = i + 1
		}
		return out
	}
	seen := map[int]bool{}
	add := func(p int) {
		if p >= 1 && p <= total {
			seen[p] = true
		}
	}
	add(1)
	add(total)
	for d := -1; d <= 1; d++ {
		add(current + d)
	}
	sorted := make([]int, 0, len(seen))
	for p := range seen {
		sorted = append(sorted, p)
	}
	slices.Sort(sorted)
	result := make([]int, 0, len(sorted)*2)
	for i, p := range sorted {
		if i > 0 && p > sorted[i-1]+1 {
			result = append(result, 0) // ellipsis
		}
		result = append(result, p)
	}
	return result
}
