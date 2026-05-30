//go:build !lite

package sync

import (
	"regexp"
	"strings"
)

// MerchantKeyVersion is bumped whenever the normalizer or the seeded denylist
// changes semantics, so a future re-derive sweep knows whether stored
// transactions.merchant_key values are stale. Recorded in app_config as
// series_merchant_key_version.
const MerchantKeyVersion = 1

// merchantKeyMinLen is the specificity floor: a key shorter than this carries
// no merchant signal and is rejected (forcing fall-through / NULL).
const merchantKeyMinLen = 3

// DefaultMerchantDenylist is the built-in (US-bank) set of generic financial
// descriptors that must NEVER anchor a recurring series — they would collapse a
// hundred unrelated payees into one fake subscription. Matched by EXACT equality
// on the fully-normalized key (never substring), so "Payless" / "Checkr" /
// "Credit Karma" survive while a bare "paypal" / "ach" is rejected.
//
// This list is mirrored into app_config.series_merchant_denylist by the
// recurring_series migration. A non-US self-hoster REPLACES that config value
// wholesale (e.g. lastschrift, transferencia); the Go default here is the
// fallback when no config is loaded.
var DefaultMerchantDenylist = []string{
	"payment", "transfer", "deposit", "withdrawal", "purchase", "pos debit",
	"ach", "check", "fee", "interest", "refund", "venmo", "zelle", "cash app",
	"paypal", "square", "bank transfer", "online payment", "bill payment",
	"point of sale", "debit card", "credit", "atm", "wire",
}

// BuildDenylistSet turns a denylist slice into a lookup set keyed by the
// normalized form of each entry, so config-supplied lists match the same way
// the normalizer produces keys.
func BuildDenylistSet(list []string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, s := range list {
		k := strings.TrimSpace(strings.ToLower(s))
		if k != "" {
			set[k] = true
		}
	}
	return set
}

var defaultDenylistSet = BuildDenylistSet(DefaultMerchantDenylist)

// genericTokens is the all-tokens safety net: a normalized key is rejected when
// EVERY one of its tokens is generic, catching multi-word descriptors the exact
// list can't enumerate ("ach debit", "ach credit", "pos purchase") WITHOUT
// substring matching — a real merchant survives as long as one token is
// specific ("credit karma" keeps "karma", "payless shoesource" keeps both).
// Single, unambiguous banking/processor words only.
var genericTokens = map[string]bool{
	"payment": true, "transfer": true, "deposit": true, "withdrawal": true,
	"purchase": true, "pos": true, "debit": true, "credit": true, "ach": true,
	"check": true, "fee": true, "interest": true, "refund": true, "paypal": true,
	"venmo": true, "zelle": true, "square": true, "atm": true, "wire": true,
	"eft": true, "dda": true, "pmt": true, "bank": true, "online": true,
	"bill": true, "direct": true, "cash": true, "app": true, "autopay": true,
	"recurring": true, "auto": true, "pay": true, "web": true,
}

// allTokensGeneric reports whether every whitespace token of key is generic.
func allTokensGeneric(key string) bool {
	fields := strings.Fields(key)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if !genericTokens[f] {
			return false
		}
	}
	return true
}

// Leading settlement-processor prefixes ("SQ *BLUE BOTTLE", "PAYPAL *SPOTIFY")
// — strip so the merchant BEHIND the processor surfaces. Handled by
// normalization, not the denylist (the denylist only rejects bare generic keys).
var processorPrefixRe = regexp.MustCompile(`^(?:sq|tst|pp|sp|dd|chk|paypal|py|ext)\s*\*\s*`)

// Phone tails anywhere in the descriptor ("866-579-7172").
var phoneRe = regexp.MustCompile(`\s*\b\d{3}[-.\s]?\d{3}[-.\s]?\d{4}\b`)

// Trailing TLD on a single-token domain ("netflix.com" -> "netflix").
var tldRe = regexp.MustCompile(`\.(?:com|net|org|io|co|app|gov)$`)

// usStateCodes are 2-letter trailing tokens treated as location noise.
var usStateCodes = map[string]bool{
	"al": true, "ak": true, "az": true, "ar": true, "ca": true, "co": true, "ct": true,
	"de": true, "fl": true, "ga": true, "hi": true, "id": true, "il": true, "in": true,
	"ia": true, "ks": true, "ky": true, "la": true, "me": true, "md": true, "ma": true,
	"mi": true, "mn": true, "ms": true, "mo": true, "mt": true, "ne": true, "nv": true,
	"nh": true, "nj": true, "nm": true, "ny": true, "nc": true, "nd": true, "oh": true,
	"ok": true, "or": true, "pa": true, "ri": true, "sc": true, "sd": true, "tn": true,
	"tx": true, "ut": true, "vt": true, "va": true, "wa": true, "wv": true, "wi": true,
	"wy": true, "dc": true,
}

// NormalizeMerchant reduces a raw descriptor to a dumb, under-merge-biased
// merchant stem. It is deliberately conservative: when unsure it splits (leaves
// distinguishing tokens) rather than fusing two merchants. SQL-expressible by
// construction (lower / regexp / token-trim) so a future backfill can produce
// byte-identical keys. May return a generic word or ""; specificity is enforced
// separately by MerchantKey.
func NormalizeMerchant(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	s = processorPrefixRe.ReplaceAllString(s, "")
	s = phoneRe.ReplaceAllString(s, " ")

	fields := strings.Fields(s)
	// Drop trailing noise tokens (store numbers, refs, state codes), never the
	// last remaining token — under-merge, but never reduce to empty here.
	for len(fields) > 1 && isNoiseToken(fields[len(fields)-1]) {
		fields = fields[:len(fields)-1]
	}
	s = strings.Join(fields, " ")

	if !strings.Contains(s, " ") {
		s = tldRe.ReplaceAllString(s, "")
	}
	return strings.TrimSpace(s)
}

// isNoiseToken reports whether a trailing token is location/reference noise:
// pure digits, a "#1234" store number, a 2-letter US state, or a processor ref
// containing "*" plus a digit ("us*2x4y1").
func isNoiseToken(tok string) bool {
	if tok == "" {
		return true
	}
	if usStateCodes[tok] {
		return true
	}
	if strings.HasPrefix(tok, "#") {
		return true
	}
	if isAllDigits(strings.TrimPrefix(tok, "#")) {
		return true
	}
	if strings.Contains(tok, "*") && containsDigit(tok) {
		return true
	}
	return false
}

// MerchantKey derives the normalized detection anchor from a transaction's
// provider fields using the built-in denylist. Returns "" when no usable,
// specific signal exists (blank, ATM/wire with no name, or a descriptor that
// normalizes to a bare generic word) — those charges are excluded from
// auto-detection.
func MerchantKey(providerMerchantName, providerName string) string {
	return MerchantKeyWithDenylist(providerMerchantName, providerName, defaultDenylistSet)
}

// MerchantKeyWithDenylist is MerchantKey with a caller-supplied denylist set
// (from app_config.series_merchant_denylist). Evaluates the fallback chain:
// enriched merchant first, then raw descriptor; the first rung yielding a
// usable, specific key wins; otherwise "".
func MerchantKeyWithDenylist(providerMerchantName, providerName string, denylist map[string]bool) string {
	for _, raw := range []string{providerMerchantName, providerName} {
		key := NormalizeMerchant(raw)
		if isUsableMerchantKey(key, denylist) {
			return key
		}
	}
	return ""
}

// isUsableMerchantKey enforces the specificity gate: long enough, not all
// digits, and not a generic descriptor.
func isUsableMerchantKey(key string, denylist map[string]bool) bool {
	if len(key) < merchantKeyMinLen {
		return false
	}
	if isAllDigits(key) {
		return false
	}
	if denylist[key] {
		return false
	}
	if allTokensGeneric(key) {
		return false
	}
	return true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
