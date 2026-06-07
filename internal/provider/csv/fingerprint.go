//go:build !lite

package csv

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// NormalizeHeaders lowercases and trims each header. The order is preserved
// because column mapping is index-based — a bank reordering its columns is a
// genuinely different layout that should not reuse the same saved profile.
func NormalizeHeaders(headers []string) []string {
	out := make([]string, len(headers))
	for i, h := range headers {
		out[i] = strings.ToLower(strings.TrimSpace(h))
	}
	return out
}

// HeaderFingerprint returns a stable hash of the normalized, ordered header row.
// It is the key used to look up a saved CSV import profile: two files from the
// same source export the same header layout and therefore the same fingerprint.
func HeaderFingerprint(headers []string) string {
	norm := NormalizeHeaders(headers)
	sum := sha256.Sum256([]byte(strings.Join(norm, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// maskHeaderRe matches header names that plausibly carry a card/account number.
var maskHeaderRe = regexp.MustCompile(`(?i)\b(card\s*no\.?|card\s*number|account\s*(no\.?|number)|last\s*4)\b`)

// digitsRe extracts runs of digits.
var digitsRe = regexp.MustCompile(`\d+`)

// filenameMaskRe finds a 4-digit group preceded by a mask-ish hint in a
// filename, e.g. "x4321", "ending 4321", "acct_4321", "-4321".
// The trailing (?:\D|$) (rather than \b) rejects the case where the 4 digits are
// part of a longer run — "_" counts as a word char so \b would mis-fire on
// "4321_". RE2 has no lookahead, so we consume one non-digit outside the group.
var filenameMaskRe = regexp.MustCompile(`(?i)(?:x|ending|acct|account|card|no|#|-|_)\s*[-_ ]?(\d{4})(?:\D|$)`)

// lastFour returns the trailing 4 digits of s (digits only), or "" if there are
// fewer than 4 digits.
func lastFour(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteByte(byte(r))
		}
	}
	d := b.String()
	if len(d) < 4 {
		return ""
	}
	return d[len(d)-4:]
}

// ExtractMask attempts to recover the account's last-4 ("mask") from the file —
// first from a card/account-number column (using a sample of data rows), then
// from the filename. Returns "" when nothing convincing is found.
func ExtractMask(filename string, headers []string, sampleRows [][]string) string {
	// 1. A dedicated card/account-number column.
	for i, h := range headers {
		if !maskHeaderRe.MatchString(h) {
			continue
		}
		for _, row := range sampleRows {
			if i >= len(row) {
				continue
			}
			if m := lastFour(row[i]); m != "" {
				return m
			}
		}
	}

	// 2. The filename, with a mask-ish hint in front of the 4 digits.
	base := filename
	if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
		base = base[idx+1:]
	}
	if m := filenameMaskRe.FindStringSubmatch(base); m != nil {
		return m[1]
	}

	return ""
}

// stopTokens are filename fragments that carry no account-identifying signal.
var stopTokens = map[string]bool{
	"csv": true, "tsv": true, "txt": true, "transactions": true,
	"transaction": true, "activity": true, "statement": true, "export": true,
	"download": true, "history": true, "data": true, "report": true,
}

// tokenSplitRe splits a filename into alphabetic/numeric tokens.
var tokenSplitRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// FilenameTokens returns lowercased, meaningful tokens from a filename (no
// extension, no generic words, no pure numbers). Used to match a file against
// an account's name/nickname.
func FilenameTokens(filename string) []string {
	base := filename
	if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
		base = base[idx+1:]
	}
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		base = base[:idx]
	}
	var out []string
	for _, tok := range tokenSplitRe.Split(strings.ToLower(base), -1) {
		if len(tok) < 3 || stopTokens[tok] {
			continue
		}
		if digitsRe.MatchString(tok) && !regexp.MustCompile(`[a-z]`).MatchString(tok) {
			continue // pure number
		}
		out = append(out, tok)
	}
	return out
}

// InstitutionTokens returns lowercased significant tokens of an institution /
// account name, suitable for substring overlap scoring. Drops short and generic
// words ("the", "card", "checking", ...).
func InstitutionTokens(name string) []string {
	generic := map[string]bool{
		"the": true, "card": true, "credit": true, "checking": true,
		"savings": true, "account": true, "bank": true, "of": true,
		"and": true, "debit": true, "rewards": true, "cash": true,
	}
	var out []string
	for _, tok := range tokenSplitRe.Split(strings.ToLower(strings.TrimSpace(name)), -1) {
		if len(tok) < 3 || generic[tok] {
			continue
		}
		out = append(out, tok)
	}
	return out
}
