//go:build !lite

package csv

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// GenerateExternalID creates a deterministic external transaction ID by
// hashing SHA-256(accountID|YYYY-MM-DD|amount|lower(trim(description))).
//
// The account id is baked in so this id is stable for idempotent upserts into a
// RESOLVED account: re-importing the same file into the same account reproduces
// identical ids and UpsertTransaction no-ops.
func GenerateExternalID(accountID string, date time.Time, amount decimal.Decimal, description string) string {
	input := fmt.Sprintf("%s|%s|%s|%s",
		accountID,
		date.Format("2006-01-02"),
		amount.String(),
		strings.ToLower(strings.TrimSpace(description)),
	)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:])
}

// GenerateExternalIDWithOccurrence is GenerateExternalID with a disambiguator
// appended, used when a user force-imports a row that is byte-for-byte identical
// to one already imported (same date+amount+description). Occurrence 0 is
// identical to GenerateExternalID so the common path stays idempotent.
func GenerateExternalIDWithOccurrence(accountID string, date time.Time, amount decimal.Decimal, description string, occurrence int) string {
	if occurrence == 0 {
		return GenerateExternalID(accountID, date, amount, description)
	}
	input := fmt.Sprintf("%s|%s|%s|%s|#%d",
		accountID,
		date.Format("2006-01-02"),
		amount.String(),
		strings.ToLower(strings.TrimSpace(description)),
		occurrence,
	)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:])
}

// GenerateContentHash creates an account-INDEPENDENT fingerprint of a
// transaction's core content: SHA-256(YYYY-MM-DD|amount|lower(trim(desc))). It
// lets the CSV import classifier detect that a row already exists in a target
// account regardless of which provider (CSV, Plaid, Teller) created it.
func GenerateContentHash(date time.Time, amount decimal.Decimal, description string) string {
	input := fmt.Sprintf("%s|%s|%s",
		date.Format("2006-01-02"),
		amount.String(),
		strings.ToLower(strings.TrimSpace(description)),
	)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:])
}
