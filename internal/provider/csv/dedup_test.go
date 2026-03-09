package csv

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestGenerateExternalIDDeterministic(t *testing.T) {
	date := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	amount := decimal.NewFromFloat(42.50)

	id1 := GenerateExternalID("acc-123", date, amount, "Coffee Shop")
	id2 := GenerateExternalID("acc-123", date, amount, "Coffee Shop")

	if id1 != id2 {
		t.Errorf("same inputs produced different IDs: %s vs %s", id1, id2)
	}
}

func TestGenerateExternalIDCaseInsensitive(t *testing.T) {
	date := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	amount := decimal.NewFromFloat(42.50)

	id1 := GenerateExternalID("acc-123", date, amount, "Coffee Shop")
	id2 := GenerateExternalID("acc-123", date, amount, "COFFEE SHOP")
	id3 := GenerateExternalID("acc-123", date, amount, "coffee shop")

	if id1 != id2 || id2 != id3 {
		t.Errorf("case difference produced different IDs: %s, %s, %s", id1, id2, id3)
	}
}

func TestGenerateExternalIDWhitespaceTrimmed(t *testing.T) {
	date := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	amount := decimal.NewFromFloat(42.50)

	id1 := GenerateExternalID("acc-123", date, amount, "Coffee Shop")
	id2 := GenerateExternalID("acc-123", date, amount, "  Coffee Shop  ")

	if id1 != id2 {
		t.Errorf("whitespace produced different IDs: %s vs %s", id1, id2)
	}
}

func TestGenerateExternalIDDifferentInputs(t *testing.T) {
	date := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	amount := decimal.NewFromFloat(42.50)

	base := GenerateExternalID("acc-123", date, amount, "Coffee Shop")

	// Different account
	diff1 := GenerateExternalID("acc-456", date, amount, "Coffee Shop")
	if base == diff1 {
		t.Error("different account IDs produced same hash")
	}

	// Different date
	diff2 := GenerateExternalID("acc-123", date.AddDate(0, 0, 1), amount, "Coffee Shop")
	if base == diff2 {
		t.Error("different dates produced same hash")
	}

	// Different amount
	diff3 := GenerateExternalID("acc-123", date, decimal.NewFromFloat(99.99), "Coffee Shop")
	if base == diff3 {
		t.Error("different amounts produced same hash")
	}

	// Different description
	diff4 := GenerateExternalID("acc-123", date, amount, "Tea House")
	if base == diff4 {
		t.Error("different descriptions produced same hash")
	}
}

func TestGenerateExternalIDLength(t *testing.T) {
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	id := GenerateExternalID("acc", date, decimal.Zero, "test")

	// SHA-256 hex = 64 characters
	if len(id) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars: %s", len(id), id)
	}
}
