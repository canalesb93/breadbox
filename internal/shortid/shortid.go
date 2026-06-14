// Package shortid generates compact, URL-safe identifiers for use as
// token-efficient aliases for UUIDs. Short IDs are 8-character base62
// strings (0-9A-Za-z) generated from 6 random bytes (~48 bits of entropy).
package shortid

import (
	"crypto/rand"
	"math/big"
	"strings"
)

const (
	// alphabet is the base62 character set: digits, uppercase, lowercase.
	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// Length is the number of characters in a generated short ID.
	Length = 8
	// byteCount is the number of random bytes used (48 bits of entropy).
	byteCount = 6
)

// maxValue is 62^Length, the exclusive upper bound of the base62 code space.
// 6 random bytes span [0, 2^48), which is larger than this bound, so reducing a
// raw draw modulo 62^Length would bias the lowest-valued ~29% of IDs to roughly
// double likelihood. Generate rejection-samples against this bound to keep the
// distribution uniform and the collision probability at its theoretical floor.
var maxValue = new(big.Int).Exp(big.NewInt(62), big.NewInt(Length), nil)

// Generate returns a cryptographically random 8-character base62 string.
func Generate() (string, error) {
	b := make([]byte, byteCount)
	n := new(big.Int)
	// Rejection sampling: discard draws at or above 62^Length so every code
	// point is equally likely. With 48 bits of entropy over a ~47.6-bit space
	// the acceptance rate is ~78%, so this loops ~1.3 times on average.
	for {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		n.SetBytes(b)
		if n.Cmp(maxValue) < 0 {
			break
		}
	}
	result := make([]byte, Length)
	base := big.NewInt(62)
	mod := new(big.Int)
	for i := Length - 1; i >= 0; i-- {
		n.DivMod(n, base, mod)
		result[i] = alphabet[mod.Int64()]
	}
	return string(result), nil
}

// IsShortID returns true if s looks like a short ID (8 base62 chars)
// rather than a UUID.
func IsShortID(s string) bool {
	if len(s) != Length {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool {
		return !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	}) == -1
}
