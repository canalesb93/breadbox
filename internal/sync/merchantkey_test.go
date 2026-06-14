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

// TestMerchantKey_AllTokensGenericNet pins the safety net that rejects
// descriptors composed ENTIRELY of generic tokens even when the exact
// (lowercased) key is NOT in DefaultMerchantDenylist. These keys are caught
// only by allTokensGeneric — the prior tests all used words that also live in
// the denylist, so a regression that broke the net (or trimmed genericTokens)
// would let "AUTOPAY" / "EFT" / "DIRECT DEPOSIT" anchor a fake series.
func TestMerchantKey_AllTokensGenericNet(t *testing.T) {
	// Single generic tokens absent from DefaultMerchantDenylist — rejected
	// purely by the all-tokens-generic net.
	rejectedSingles := []string{
		"eft", "dda", "pmt", "bank", "online", "bill",
		"direct", "autopay", "recurring", "web",
	}
	for _, g := range rejectedSingles {
		if defaultDenylistSet[g] {
			t.Fatalf("test premise broken: %q is in the denylist, not isolating the net", g)
		}
		if k := MerchantKey(g, g); k != "" {
			t.Errorf("MerchantKey(%q) = %q, want empty (all-generic net)", g, k)
		}
	}

	// Multi-word descriptors where EVERY token is generic but the whole phrase
	// is not a denylist entry — must still be rejected.
	rejectedPhrases := []string{
		"DIRECT DEPOSIT", "AUTO PAY", "WEB PMT", "RECURRING PAYMENT", "POS PURCHASE",
	}
	for _, p := range rejectedPhrases {
		if k := MerchantKey(p, ""); k != "" {
			t.Errorf("MerchantKey(%q) = %q, want empty (multi-word all-generic)", p, k)
		}
	}

	// A single specific token rescues an otherwise-generic phrase: the net must
	// not over-reject real merchants whose leading word happens to be generic.
	survivors := map[string]string{
		"DIRECT TV":       "direct tv",
		"BANK OF AMERICA": "bank of america",
	}
	for in, want := range survivors {
		if k := MerchantKey(in, ""); k != want {
			t.Errorf("MerchantKey(%q) = %q, want %q (one specific token survives)", in, k, want)
		}
	}
}

// TestNormalizeMerchant_ProcessorPrefixGating pins that settlement-processor
// prefixes are stripped only when followed by the "*" separator, across the
// prefix table — and that a real merchant whose name merely begins with those
// letters (no "*") is left intact.
func TestNormalizeMerchant_ProcessorPrefixGating(t *testing.T) {
	stripped := map[string]string{
		"TST* CHIPOTLE": "chipotle",
		"DD *DOORDASH":  "doordash",
		"PP*GITHUB":     "github",
		"SP *NOTION":    "notion",
		"CHK*ACME CORP": "acme corp",
	}
	for in, want := range stripped {
		if got := NormalizeMerchant(in); got != want {
			t.Errorf("NormalizeMerchant(%q) = %q, want %q (prefix should strip)", in, got, want)
		}
	}

	// No "*" → the leading letters are part of the merchant, not a processor
	// tag, and must NOT be stripped.
	intact := map[string]string{
		"Sprouts Farmers Mkt": "sprouts farmers mkt",
		"DDR Memory":          "ddr memory",
	}
	for in, want := range intact {
		if got := NormalizeMerchant(in); got != want {
			t.Errorf("NormalizeMerchant(%q) = %q, want %q (no '*', must stay intact)", in, got, want)
		}
	}
}

// TestNormalizeMerchant_NeverReducesToEmpty pins the under-merge guard: the
// trailing-noise trim stops at the last remaining token, so a descriptor that
// is nothing but a noise token still yields that token rather than "".
func TestNormalizeMerchant_NeverReducesToEmpty(t *testing.T) {
	cases := map[string]string{
		"shell ca": "shell", // trailing state code dropped
		"CA":       "ca",    // lone state code survives (not reduced to empty)
		"#1234":    "#1234", // lone store-number token survives normalization
	}
	for in, want := range cases {
		if got := NormalizeMerchant(in); got != want {
			t.Errorf("NormalizeMerchant(%q) = %q, want %q", in, got, want)
		}
	}
}
