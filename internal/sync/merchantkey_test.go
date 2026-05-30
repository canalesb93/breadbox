//go:build !lite

package sync

import "testing"

func TestNormalizeMerchant(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"NETFLIX.COM 866-579-7172 CA", "netflix"},
		{"SQ *BLUE BOTTLE", "blue bottle"},
		{"PAYPAL *SPOTIFY", "spotify"},
		{"AMZN MKTP US*2X4Y1", "amzn mktp"},
		{"AMZN MKTP US*9Z8B3", "amzn mktp"},
		{"Spotify", "spotify"},
		{"  Netflix.com  ", "netflix"},
		{"COSTCO WHSE #1234", "costco whse"},
		{"", ""},
		{"PAYMENT", "payment"}, // normalizes but is generic — rejected by MerchantKey, not here
	}
	for _, c := range cases {
		if got := NormalizeMerchant(c.in); got != c.want {
			t.Errorf("NormalizeMerchant(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMerchantKey_FallbackChain(t *testing.T) {
	cases := []struct {
		name string
		pmn  string // provider_merchant_name (rung 1)
		pn   string // provider_name (rung 2)
		want string
	}{
		{"enriched merchant wins", "Spotify", "PAYPAL *SPOTIFY 866-555-1212", "spotify"},
		{"fall through generic rung1 to rung2", "PAYMENT", "NETFLIX.COM", "netflix"},
		{"both generic -> empty", "PAYMENT", "ACH DEBIT", ""},
		{"blank merchant uses raw descriptor", "", "BLUE BOTTLE COFFEE", "blue bottle coffee"},
		{"no signal -> empty", "", "", ""},
		{"AMZN drift merges via dumb key", "", "AMZN MKTP US*9Z8B3", "amzn mktp"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MerchantKey(c.pmn, c.pn); got != c.want {
				t.Errorf("MerchantKey(%q, %q) = %q, want %q", c.pmn, c.pn, got, c.want)
			}
		})
	}
}

func TestMerchantKey_RejectsGenericAndJunk(t *testing.T) {
	// Bare generic descriptors must never anchor a series.
	for _, g := range []string{"paypal", "zelle", "venmo", "ach", "payment", "transfer", "atm", "wire"} {
		if k := MerchantKey(g, g); k != "" {
			t.Errorf("MerchantKey(%q) = %q, want empty (generic must be rejected)", g, k)
		}
	}
	// All-digit and too-short keys are not usable.
	if k := MerchantKey("12345", "67890"); k != "" {
		t.Errorf("MerchantKey(all-digits) = %q, want empty", k)
	}
	if k := MerchantKey("ab", "x"); k != "" {
		t.Errorf("MerchantKey(too-short) = %q, want empty", k)
	}
	// Exact-key match only: a real merchant containing a generic substring survives.
	if k := MerchantKey("Payless ShoeSource", ""); k == "" {
		t.Error("MerchantKey(\"Payless ShoeSource\") was rejected; substring match leaked")
	}
	if k := MerchantKey("Credit Karma", ""); k == "" {
		t.Error("MerchantKey(\"Credit Karma\") was rejected; substring match leaked")
	}
}
