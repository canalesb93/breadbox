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
