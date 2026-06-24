//go:build !headless && !lite

package components

import "testing"

func TestLogoDevDomain(t *testing.T) {
	cases := map[string]string{
		"":                                   "",
		"   ":                                "",
		"amazon.com":                         "amazon.com",
		"https://amazon.com":                 "amazon.com",
		"https://www.amazon.com":             "amazon.com",
		"https://www.amazon.com/foo/bar?x=1": "amazon.com",
		"http://www.amazon.com":              "amazon.com",
		"HTTP://Netflix.com":                 "netflix.com",
		"www.netflix.com":                    "netflix.com",
		// Non-www subdomains are kept (only a leading www. is stripped).
		"https://shop.example.co.uk":      "shop.example.co.uk",
		"https://shop.example.co.uk:8080": "shop.example.co.uk",
		"sub.domain.example.com":          "sub.domain.example.com",
		// Single-label / non-domains → no hotlink.
		"localhost":         "",
		"amazon":            "",
		"http://localhost":  "",
		"https://localhost": "",
		// Stripping "www." from "www.co" leaves "co" (no dot) → rejected.
		"www.co":     "",
		"ftp://x.io": "x.io",
	}

	for in, want := range cases {
		if got := LogoDevDomain(in); got != want {
			t.Errorf("LogoDevDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogoDevURL(t *testing.T) {
	const base = "https://img.logo.dev/amazon.com?size=128&format=png&retina=true&fallback=404"

	// No website → no URL (avatar shows monogram).
	if got := LogoDevURL("", "pk_x"); got != "" {
		t.Errorf("LogoDevURL(empty) = %q, want \"\"", got)
	}
	// A non-domain → no URL.
	if got := LogoDevURL("localhost", "pk_x"); got != "" {
		t.Errorf("LogoDevURL(localhost) = %q, want \"\"", got)
	}

	// No token → bare URL (logo.dev 401s → onerror → monogram, by design).
	if got := LogoDevURL("https://www.amazon.com/cart", ""); got != base {
		t.Errorf("LogoDevURL(no token) = %q, want %q", got, base)
	}

	// Token present → appended last, URL-escaped.
	wantTok := base + "&token=pk_live_123"
	if got := LogoDevURL("amazon.com", "pk_live_123"); got != wantTok {
		t.Errorf("LogoDevURL(token) = %q, want %q", got, wantTok)
	}

	// A token with reserved characters is query-escaped.
	if got := LogoDevURL("amazon.com", "a b&c"); got != base+"&token=a+b%26c" {
		t.Errorf("LogoDevURL(escaped token) = %q", got)
	}

	// Whitespace-only token is treated as absent.
	if got := LogoDevURL("amazon.com", "   "); got != base {
		t.Errorf("LogoDevURL(blank token) = %q, want %q", got, base)
	}
}
