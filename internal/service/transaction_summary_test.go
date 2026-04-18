package service

import (
	"context"
	"errors"
	"testing"
)

func TestGetTransactionSummary_InvalidGroupBy(t *testing.T) {
	svc := &Service{} // no pool needed for validation check
	_, err := svc.GetTransactionSummary(context.Background(), TransactionSummaryParams{GroupBy: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid group_by")
	}
	if !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestGetTransactionSummary_ValidGroupByValues(t *testing.T) {
	for _, gb := range []string{"category", "month", "week", "day", "category_month"} {
		if !validGroupBy[gb] {
			t.Errorf("expected %q to be a valid group_by", gb)
		}
	}
}
