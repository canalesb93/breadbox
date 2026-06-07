//go:build !lite

package csv

import (
	"reflect"
	"testing"
)

func TestHeaderFingerprintStableAndOrderSensitive(t *testing.T) {
	a := HeaderFingerprint([]string{"Date", "Amount", "Description"})
	// Case/whitespace insensitive, same order → same fingerprint.
	b := HeaderFingerprint([]string{" date ", "AMOUNT", "description"})
	if a != b {
		t.Errorf("fingerprint should ignore case/whitespace: %s != %s", a, b)
	}
	// Different column order → different fingerprint (mapping is index-based).
	c := HeaderFingerprint([]string{"Amount", "Date", "Description"})
	if a == c {
		t.Error("fingerprint should be order-sensitive")
	}
}

func TestExtractMaskFromColumn(t *testing.T) {
	headers := []string{"Transaction Date", "Card No.", "Description", "Debit"}
	rows := [][]string{
		{"2026-01-02", "", "STARBUCKS", "5.00"},        // empty mask cell
		{"2026-01-03", "1234567890124321", "AMZN", ""}, // full PAN → last4
	}
	if got := ExtractMask("capone.csv", headers, rows); got != "4321" {
		t.Errorf("ExtractMask column = %q, want 4321", got)
	}
}

func TestExtractMaskFromFilename(t *testing.T) {
	headers := []string{"Date", "Amount", "Description"}
	cases := map[string]string{
		"Chase_x4321_Activity.csv":      "4321",
		"statement-ending 9876.csv":     "9876",
		"acct_5555_2026.csv":            "5555",
		"plain_transactions.csv":        "",
		"/Users/me/Downloads/card-0001": "0001",
	}
	for fn, want := range cases {
		if got := ExtractMask(fn, headers, nil); got != want {
			t.Errorf("ExtractMask(%q) = %q, want %q", fn, got, want)
		}
	}
}

func TestFilenameTokens(t *testing.T) {
	got := FilenameTokens("Chase_Sapphire_transactions_2026.csv")
	want := []string{"chase", "sapphire"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilenameTokens = %v, want %v", got, want)
	}
}

func TestInstitutionTokens(t *testing.T) {
	got := InstitutionTokens("Chase Sapphire Credit Card")
	want := []string{"chase", "sapphire"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("InstitutionTokens = %v, want %v", got, want)
	}
}
