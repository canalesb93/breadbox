package crypto

import (
	"crypto/sha256"
	"encoding/hex"
)

// FingerprintLength is the number of hex characters shown in a key
// fingerprint. Eight hex chars = 32 bits of entropy — enough to spot a
// changed key by eye, short enough to read aloud or print next to a label.
const FingerprintLength = 8

// Fingerprint returns the first 8 hex characters of sha256(key). It is a
// non-reversible identifier for a raw 32-byte AES key: safe to print in
// logs, the admin UI, and on install output so users can sanity-check that
// a host migration or `.env` restore is still using the same key.
//
// Returns the empty string when key is nil or empty so call sites can
// distinguish "no key configured" from "fingerprint unavailable".
func Fingerprint(key []byte) string {
	if len(key) == 0 {
		return ""
	}
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])[:FingerprintLength]
}
