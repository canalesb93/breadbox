//go:build !headless && !lite

package admin

import "testing"

// TestCounterpartyLogoDevURL covers the token-gated emission of a logo.dev
// hotlink for a counterparty avatar. The full resolution chain is:
// explicit logo_url → (enabled + token + derivable domain) logo.dev → monogram.
func TestCounterpartyLogoDevURL(t *testing.T) {
	site := func(s string) *string { return &s }
	const wantURL = "https://img.logo.dev/amazon.com?size=128&format=png&retina=true&fallback=404&token=pk_test"

	t.Run("enabled+token+domain → URL", func(t *testing.T) {
		if got := counterpartyLogoDevURL(true, "", site("https://www.amazon.com"), "pk_test"); got != wantURL {
			t.Fatalf("got %q, want %q", got, wantURL)
		}
	})
	t.Run("no token → monogram (no URL)", func(t *testing.T) {
		if got := counterpartyLogoDevURL(true, "", site("https://amazon.com"), ""); got != "" {
			t.Fatalf("got %q, want \"\" (token required)", got)
		}
	})
	t.Run("disabled → no URL", func(t *testing.T) {
		if got := counterpartyLogoDevURL(false, "", site("https://amazon.com"), "pk_test"); got != "" {
			t.Fatalf("got %q, want \"\" (toggle off)", got)
		}
	})
	t.Run("manual logo_url wins → no logo.dev URL", func(t *testing.T) {
		if got := counterpartyLogoDevURL(true, "https://cdn.example.com/a.png", site("https://amazon.com"), "pk_test"); got != "" {
			t.Fatalf("got %q, want \"\" (manual override wins)", got)
		}
	})
	t.Run("no website → no URL", func(t *testing.T) {
		if got := counterpartyLogoDevURL(true, "", nil, "pk_test"); got != "" {
			t.Fatalf("got %q, want \"\" (no domain)", got)
		}
	})
	t.Run("non-domain website → no URL", func(t *testing.T) {
		if got := counterpartyLogoDevURL(true, "", site("localhost"), "pk_test"); got != "" {
			t.Fatalf("got %q, want \"\" (single-label host)", got)
		}
	})
}
