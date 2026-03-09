package csv

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string // decimal string
		wantErr bool
	}{
		{"plain positive", "123.45", "123.45", false},
		{"plain negative", "-42.00", "-42", false},
		{"dollar sign", "$1,234.56", "1234.56", false},
		{"euro sign", "€100.00", "100", false},
		{"pound sign", "£50.99", "50.99", false},
		{"yen sign", "¥1000", "1000", false},
		{"parenthetical negative", "(500.00)", "-500", false},
		{"parenthetical with dollar", "($1,200.00)", "-1200", false},
		{"thousands separator", "1,234,567.89", "1234567.89", false},
		{"whitespace", "  42.50  ", "42.5", false},
		{"zero", "0.00", "0", false},
		{"empty string", "", "", true},
		{"only spaces", "   ", "", true},
		{"garbage", "abc", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAmount(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want, _ := decimal.NewFromString(tt.want)
			if !got.Equal(want) {
				t.Errorf("ParseAmount(%q) = %s, want %s", tt.input, got, want)
			}
		})
	}
}

func TestParseDualColumns(t *testing.T) {
	tests := []struct {
		name    string
		debit   string
		credit  string
		want    string
		wantErr bool
	}{
		{"debit only", "100.00", "", "100", false},
		{"credit only", "", "50.00", "-50", false},
		{"both present", "100.00", "25.00", "75", false},
		{"both empty", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDualColumns(tt.debit, tt.credit)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want, _ := decimal.NewFromString(tt.want)
			if !got.Equal(want) {
				t.Errorf("ParseDualColumns(%q, %q) = %s, want %s", tt.debit, tt.credit, got, want)
			}
		})
	}
}

func TestNormalizeSign(t *testing.T) {
	amount := decimal.NewFromFloat(100)

	// Positive is debit — no change.
	got := NormalizeSign(amount, true)
	if !got.Equal(amount) {
		t.Errorf("positiveIsDebit=true: got %s, want %s", got, amount)
	}

	// Positive is credit — negate.
	got = NormalizeSign(amount, false)
	want := decimal.NewFromFloat(-100)
	if !got.Equal(want) {
		t.Errorf("positiveIsDebit=false: got %s, want %s", got, want)
	}
}
