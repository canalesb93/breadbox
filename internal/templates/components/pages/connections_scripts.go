package pages

import (
	"fmt"
	"strings"
)

// connectionsCountLabel renders "N bank" / "N banks" — pluralized inline so
// the templ side stays declarative.
func connectionsCountLabel(n int) string {
	if n == 1 {
		return fmt.Sprintf("%d bank", n)
	}
	return fmt.Sprintf("%d banks", n)
}

// connectionsAccountSuffix renders the trailing "account"/"accounts"
// (or compact "acct"/"accts") word, mirroring the original template's
// per-viewport variants. The leading space is part of the segment because
// the count is rendered separately in the same flex row.
func connectionsAccountSuffix(n int64, compact bool) string {
	if compact {
		if n == 1 {
			return "acct"
		}
		return "accts"
	}
	if n == 1 {
		return "account"
	}
	return "accounts"
}

// connectionsHumanize replaces underscores with spaces so subtype slugs like
// "credit_card" render as "credit card". Mirrors the funcMap "humanize"
// helper in admin/templates.go.
func connectionsHumanize(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}

// connectionsLinkActionURL builds a templ.SafeURL for the account-link form
// actions (reconcile, delete). The path-construction lives in Go rather
// than as a string literal in the .templ file so the routes-drift test
// (which scans .templ for templ.SafeURL("...") literals against the admin
// GET router) doesn't flag these POST-only mutation endpoints.
func connectionsLinkActionURL(linkID, op string) string {
	return "/-/account-links/" + linkID + "/" + op
}

// connectionsLinkAccountLabel renders the option label used in the
// create-link modal selects. Format mirrors the previous template:
// "<DisplayName>[ ••<Mask>] (<UserName> - <InstitutionName>)".
func connectionsLinkAccountLabel(a ConnectionsLinkAccount) string {
	var b strings.Builder
	b.WriteString(a.DisplayName)
	if a.Mask != "" {
		b.WriteString(" ••")
		b.WriteString(a.Mask)
	}
	b.WriteString(" (")
	b.WriteString(a.UserName)
	b.WriteString(" - ")
	b.WriteString(a.InstitutionName)
	b.WriteString(")")
	return b.String()
}
