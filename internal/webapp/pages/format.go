//go:build !headless && !lite

// Package pages holds the v3 webapp page templates and their small view-formatting
// helpers. Pages compose layout + components and read service types directly.
package pages

import (
	"math"
	"strconv"
	"strings"
)

// LoginForm is the data the login template needs (sticky values + error on re-render).
type LoginForm struct {
	Username string
	Next     string
	Error    string
}

// Money renders an amount + currency code with grouped digits. nil → em dash.
func Money(amount *float64, cur *string) string {
	if amount == nil {
		return "—"
	}
	code := "USD"
	if cur != nil && *cur != "" {
		code = strings.ToUpper(*cur)
	}
	return code + " " + groupDigits(*amount)
}

func groupDigits(v float64) string {
	neg := math.Signbit(v)
	s := strconv.FormatFloat(math.Abs(v), 'f', 2, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	n := len(intPart)
	for i, d := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	out := b.String() + "." + frac
	if neg {
		return "-" + out
	}
	return out
}

// TitleCase converts snake_case enums (account types/subtypes) to display text.
func TitleCase(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	parts := strings.Fields(s)
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// Deref returns *s, or fallback when nil/empty.
func Deref(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}
