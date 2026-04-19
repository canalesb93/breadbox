package components

import (
	"testing"
	"time"
)

func TestDeref(t *testing.T) {
	if got := deref(nil); got != "" {
		t.Errorf("deref(nil) = %q, want \"\"", got)
	}
	s := "hello"
	if got := deref(&s); got != "hello" {
		t.Errorf("deref(&\"hello\") = %q, want \"hello\"", got)
	}
	empty := ""
	if got := deref(&empty); got != "" {
		t.Errorf("deref(&\"\") = %q, want \"\"", got)
	}
}

func TestFirstChar(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "?"},
		{"apple", "A"},
		{"APPLE", "A"},
		{"2024 sales", "2"},
		{"  leading space", "L"},
		{"!@#hello", "H"},
		{"!!!", "!"}, // no ASCII letter/digit — first rune, uppercased
		{"émilie", "M"}, // non-ASCII leader skipped; first A–Z wins
		{"é", "É"},     // no A–Z/0–9 at all — first rune uppercased
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := firstChar(tc.in); got != tc.want {
				t.Errorf("firstChar(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2024-03-15", "Mar 15, 2024"},
		{"2024-12-31", "Dec 31, 2024"},
		{"not-a-date", "not-a-date"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := formatDate(tc.in); got != tc.want {
				t.Errorf("formatDate(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRelativeDateAt(t *testing.T) {
	// Fix "now" to a known moment so assertions are deterministic.
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		date string
		want string
	}{
		{"today", "2026-04-18", "Today"},
		{"yesterday", "2026-04-17", "Yesterday"},
		{"2 days ago", "2026-04-16", "2 days ago"},
		{"6 days ago", "2026-04-12", "6 days ago"},
		{"7 days ago → 1 week", "2026-04-11", "1 week ago"},
		{"13 days ago → 1 week", "2026-04-05", "1 week ago"},
		{"14 days ago → absolute", "2026-04-04", "Apr 4, 2026"},
		{"far past", "2020-01-15", "Jan 15, 2020"},
		{"future dates fall through to absolute", "2026-05-01", "May 1, 2026"},
		{"invalid input returned as-is", "not-a-date", "not-a-date"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := relativeDateAt(tc.date, now); got != tc.want {
				t.Errorf("relativeDateAt(%q) = %q, want %q", tc.date, got, tc.want)
			}
		})
	}
}

func TestFormatAmount(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "$0.00"},
		{1.5, "$1.50"},
		{-1.5, "-$1.50"},
		{1234.56, "$1,234.56"},
		{-1234.56, "-$1,234.56"},
		{1000000, "$1,000,000.00"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := formatAmount(tc.in); got != tc.want {
				t.Errorf("formatAmount(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestAvatarURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "/avatars/unknown"},
		{"abc123", "/avatars/abc123"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := avatarURL(tc.in); got != tc.want {
				t.Errorf("avatarURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty string", "", ""},
		{"all caps becomes title case", "STARBUCKS COFFEE", "Starbucks Coffee"},
		{"all lowercase becomes title case", "starbucks coffee", "Starbucks Coffee"},
		{"mixed case left untouched", "McDonald's", "McDonald's"},
		{"mixed case name like iTunes left untouched", "iTunes", "iTunes"},
		{"short non-article words uppercased", "us bank", "US Bank"},
		{"small words stay lowercase in middle", "BANK OF AMERICA", "Bank of America"},
		{"first small word capitalized", "the coffee shop", "The Coffee Shop"},
		{"first small word capitalized from all caps", "THE HOME DEPOT", "The Home Depot"},
		{"abbreviations with periods uppercased", "h.e.b grocery", "H.E.B Grocery"},
		{"abbreviation with trailing period", "h.e.b.", "H.E.B."},
		{"abbreviation already capitalized stays", "H.E.B.", "H.E.B."},
		{"single word all caps", "WALMART", "Walmart"},
		{"all caps multi word", "WHOLE FOODS MARKET", "Whole Foods Market"},
		{"two-letter acronym uppercased", "ab pharmacy", "AB Pharmacy"},
		{"three-letter acronym title-cased not all-uppered", "US ATM FEE", "US Atm Fee"},
		{"two-letter small words stay lowercase mid-phrase", "UP AND AT EM", "UP and at EM"},
		{"single small word at start gets capitalized", "the", "The"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := titleCase(tc.in); got != tc.want {
				t.Errorf("titleCase(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPluralS(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "s"},
		{1, ""},
		{2, "s"},
		{-1, "s"}, // only n==1 is singular
	}
	for _, tc := range tests {
		if got := pluralS(tc.n); got != tc.want {
			t.Errorf("pluralS(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
	// int64 path — sqlc-derived counts (e.g. transaction totals) hit this
	// branch of the generic and must produce the same output as int.
	int64Cases := []struct {
		n    int64
		want string
	}{
		{0, "s"},
		{1, ""},
		{2, "s"},
	}
	for _, tc := range int64Cases {
		if got := pluralS(tc.n); got != tc.want {
			t.Errorf("pluralS(int64=%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestExportedWrappersDelegate(t *testing.T) {
	// Each exported wrapper must return the same value as its lowercase
	// counterpart so the admin funcMap and .templ files stay in lock-step.
	if got, want := FirstChar("apple"), firstChar("apple"); got != want {
		t.Errorf("FirstChar = %q, want %q", got, want)
	}
	if got, want := FormatDate("2024-03-15"), formatDate("2024-03-15"); got != want {
		t.Errorf("FormatDate = %q, want %q", got, want)
	}
	if got, want := FormatAmount(-1.5), formatAmount(-1.5); got != want {
		t.Errorf("FormatAmount = %q, want %q", got, want)
	}
	if got, want := TitleCase("STARBUCKS"), titleCase("STARBUCKS"); got != want {
		t.Errorf("TitleCase = %q, want %q", got, want)
	}
	if got, want := PluralS(2), pluralS(2); got != want {
		t.Errorf("PluralS = %q, want %q", got, want)
	}
	// RelativeDate goes through time.Now() — just assert it returns non-empty
	// for a valid date.
	if got := RelativeDate("2024-01-01"); got == "" {
		t.Error("RelativeDate returned empty string for valid input")
	}
}

func TestCommaInt(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{5, "5"},
		{123, "123"},
		{1000, "1,000"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{1000000000, "1,000,000,000"},
	}
	for _, tc := range tests {
		if got := CommaInt(tc.in); got != tc.want {
			t.Errorf("CommaInt(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCommaAmount(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{1.5, "1.50"},
		{12.34, "12.34"},
		{1234.56, "1,234.56"},
		{1000000, "1,000,000.00"},
		{1234567.89, "1,234,567.89"},
	}
	for _, tc := range tests {
		if got := commaAmount(tc.in); got != tc.want {
			t.Errorf("commaAmount(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatBalance(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want string
	}{
		{"zero", 0, "$0.00"},
		{"small", 12.34, "$12.34"},
		{"under 1K rounds cents", 999.995, "$1000.00"}, // edge: cents round up to 1.00, not yet ≥1K branch
		{"exactly 1K uses commas", 1000, "$1,000.00"},
		{"mid thousands", 12345.67, "$12,345.67"},
		{"just under 1M", 999999.99, "$999,999.99"},
		{"exactly 1M abbreviates", 1_000_000, "$1.0M"},
		{"1.25M abbreviates with one decimal", 1_250_000, "$1.2M"},
		{"10M abbreviates", 10_500_000, "$10.5M"},
		{"negative treated as absolute", -1234.56, "$1,234.56"},
		{"negative over 1M abbreviates", -2_500_000, "$2.5M"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatBalance(tc.in); got != tc.want {
				t.Errorf("FormatBalance(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatIntervalMinutes(t *testing.T) {
	tests := []struct {
		name    string
		minutes int
		want    string
	}{
		{"zero → N/A", 0, "N/A"},
		{"negative → N/A", -5, "N/A"},
		{"sub-hour minutes", 15, "15m"},
		{"59 minutes", 59, "59m"},
		{"exactly one hour", 60, "1h"},
		{"two hours", 120, "2h"},
		{"four hours", 240, "4h"},
		{"hours plus leftover minutes", 90, "1h 30m"},
		{"hours plus leftover minutes (2h 5m)", 125, "2h 5m"},
		{"exactly 24 hours", 1440, "24h"},
		{"exactly 2 days", 2880, "2d"},
		{"7 days", 10080, "7d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatIntervalMinutes(tc.minutes); got != tc.want {
				t.Errorf("FormatIntervalMinutes(%d) = %q, want %q", tc.minutes, got, tc.want)
			}
		})
	}
}

func TestAmount2f(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{1.5, "1.50"},
		{-1.2, "-1.20"},
		{1234.5678, "1234.57"}, // %.2f rounds half-away-from-zero
	}
	for _, tc := range tests {
		if got := amount2f(tc.in); got != tc.want {
			t.Errorf("amount2f(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
