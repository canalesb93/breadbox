// Package components hosts templ components used by the admin UI.
package components

import (
	"fmt"
	"math"
	"strings"
	"time"

	"breadbox/internal/service"
)

const neutralAvatarColor = "oklch(0.65 0 0)"

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// firstChar returns the first alphanumeric character of s, uppercased — or "?"
// when s has none. Used for avatar letters.
func firstChar(s string) string {
	if s == "" {
		return "?"
	}
	for _, r := range s {
		c := strings.ToUpper(string(r))
		if c >= "A" && c <= "Z" {
			return c
		}
		if c >= "0" && c <= "9" {
			return c
		}
	}
	return strings.ToUpper(string([]rune(s)[0]))
}

func avatarURL(id *string) string {
	if id == nil || *id == "" {
		return "/avatars/unknown"
	}
	return "/avatars/" + *id
}

func formatAmount(amount float64) string {
	s := service.FormatCurrency(math.Abs(amount))
	if amount < 0 {
		return "-" + s
	}
	return s
}

// relativeDate renders a YYYY-MM-DD date as Today / Yesterday / N days ago /
// 1 week ago / absolute date. Falls back to the raw string on parse failure.
func relativeDate(s string) string {
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

// colorOrDefault returns *color when non-empty, else the neutral grey used
// across tx-row surfaces when no category color is available.
func colorOrDefault(color *string) string {
	if color != nil && *color != "" {
		return *color
	}
	return neutralAvatarColor
}

func amountClass(amount float64) string {
	if amount < 0 {
		return "bb-tx-amount tabular-nums bb-tx-amount--income"
	}
	return "bb-tx-amount tabular-nums"
}
