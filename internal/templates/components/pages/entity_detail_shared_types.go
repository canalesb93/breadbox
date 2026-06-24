//go:build !headless && !lite

package pages

import "time"

// entityChargeDate formats an AdminTransactionRow.Date ("2006-01-02") as the
// leading date label in a linked-charge list ("Jan 2, 2006"); raw on parse fail.
// Shared by the series and counterparty detail pages (entityChargeRow).
func entityChargeDate(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("Jan 2, 2006")
	}
	return s
}
