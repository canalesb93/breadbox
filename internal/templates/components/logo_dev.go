//go:build !headless && !lite

package components

import (
	"net/url"
	"strings"
)

// logo.dev hotlinking — turn a counterparty's website into a brand-logo image
// URL served by the free https://img.logo.dev/{domain} API. The image rides in
// the avatar's <img src> and degrades to the gradient monogram (via the img's
// onerror) when logo.dev has no logo for the domain or no token is configured.
//
// The verified endpoint shape (curled against the public API):
//
//	https://img.logo.dev/{domain}?size=128&format=png&retina=true&fallback=404[&token=pk_…]
//
//   - A publishable token is required for a real logo; without it logo.dev
//     answers 401 (→ onerror → monogram). The token is therefore appended only
//     when configured, and the whole feature still degrades gracefully unset.
//   - fallback=404 makes UNKNOWN domains answer 404 rather than logo.dev's own
//     generic monogram, so an unknown counterparty degrades to *our* branded
//     gradient monogram instead of a foreign placeholder.
//   - size=128 + retina=true asks for a crisp 2× tile; the avatar renders it
//     object-contain at 36–40px.

const logoDevHost = "https://img.logo.dev/"

// LogoDevDomain derives the registrable host to hand logo.dev from a
// counterparty website URL: it strips the scheme, any leading "www.", the path,
// and the port, and lower-cases the result. A blank input, a host with no dot
// (e.g. "localhost", a bare word), or an unparseable URL all return "" — the
// caller then emits no logo.dev URL and the avatar shows its monogram.
//
//	""                              → ""
//	"https://www.amazon.com/foo"    → "amazon.com"
//	"amazon.com"                    → "amazon.com"
//	"HTTP://Netflix.com"            → "netflix.com"
//	"https://shop.example.co.uk:8080" → "shop.example.co.uk"  (non-www subdomain kept)
func LogoDevDomain(websiteURL string) string {
	raw := strings.TrimSpace(websiteURL)
	if raw == "" {
		return ""
	}
	// url.Parse only finds a Host when a scheme (or "//") is present; a bare
	// "amazon.com" parses entirely into Path. Prepend a scheme when none is
	// present so Hostname() resolves consistently.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	host = strings.TrimPrefix(host, "www.")
	// Require a dotted domain — a single label ("localhost", "amazon") is not a
	// hotlinkable brand domain.
	if host == "" || !strings.Contains(host, ".") {
		return ""
	}
	return host
}

// LogoDevURL builds the logo.dev hotlink image URL for a counterparty website,
// or "" when no registrable domain can be derived (→ the avatar renders its
// monogram). token is appended only when non-empty.
func LogoDevURL(websiteURL, token string) string {
	domain := LogoDevDomain(websiteURL)
	if domain == "" {
		return ""
	}
	// Built by hand (rather than url.Values.Encode, which sorts keys) so the
	// query order is stable and readable in page source, with token last.
	u := logoDevHost + domain + "?size=128&format=png&retina=true&fallback=404"
	if t := strings.TrimSpace(token); t != "" {
		u += "&token=" + url.QueryEscape(t)
	}
	return u
}
