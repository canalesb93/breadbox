package textmatch

import (
	"reflect"
	"strings"
	"testing"
)

func TestScore(t *testing.T) {
	tests := []struct {
		name                           string
		aName, aMerchant, bName, bMerc string
		wantScore                      int
		wantFields                     []string
	}{
		{"merchant exact", "card 1234", "Starbucks", "POS 9", "starbucks", 3, []string{"merchant_name"}},
		{"merchant contains", "x", "Starbucks Coffee", "y", "starbucks", 2, []string{"merchant_name"}},
		{"name exact, no merchant", "AMAZON", "", "amazon", "", 2, []string{"name"}},
		{"name contains", "AMAZON MKTPLACE US", "", "amazon", "", 1, []string{"name"}},
		{"no similarity", "WALMART", "", "TARGET", "", 0, nil},
		{"merchant beats name", "AMAZON", "Whole Foods", "AMAZON", "whole foods market", 2, []string{"merchant_name"}},
		{"empty merchant falls to name", "Uber Trip", "", "UBER", "", 1, []string{"name"}},
		{"both empty", "", "", "", "", 2, []string{"name"}}, // EqualFold("","") is true
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, gotFields := Score(tt.aName, tt.aMerchant, tt.bName, tt.bMerc)
			if gotScore != tt.wantScore {
				t.Errorf("Score() score = %d, want %d", gotScore, tt.wantScore)
			}
			if !reflect.DeepEqual(gotFields, tt.wantFields) {
				t.Errorf("Score() fields = %v, want %v", gotFields, tt.wantFields)
			}
		})
	}
}

func TestScoreSymmetric(t *testing.T) {
	// Swapping the two sides must yield the same score (field labels are
	// field-based, not side-based, so they match too).
	cases := [][4]string{
		{"AMAZON MKTPLACE", "", "amazon", ""},
		{"x", "Starbucks Coffee", "y", "starbucks"},
		{"WALMART", "", "TARGET", ""},
	}
	for _, c := range cases {
		s1, f1 := Score(c[0], c[1], c[2], c[3])
		s2, f2 := Score(c[2], c[3], c[0], c[1])
		if s1 != s2 || !reflect.DeepEqual(f1, f2) {
			t.Errorf("asymmetric for %v: (%d,%v) vs (%d,%v)", c, s1, f1, s2, f2)
		}
	}
}

func TestScoreLoweredParity(t *testing.T) {
	// ScoreLowered must score identically to Score for every input — it is
	// purely an allocation optimization.
	cases := [][4]string{
		{"card 1234", "Starbucks", "POS 9", "starbucks"},
		{"AMAZON MKTPLACE US", "", "amazon", ""},
		{"WALMART", "Walmart", "TARGET", "Target"},
		{"Uber Trip", "", "UBER", ""},
		{"", "", "", ""},
	}
	for _, c := range cases {
		wantScore, wantFields := Score(c[0], c[1], c[2], c[3])
		gotScore, gotFields := ScoreLowered(
			c[0], strings.ToLower(c[0]),
			c[1], strings.ToLower(c[1]),
			c[2], c[3],
		)
		if gotScore != wantScore || !reflect.DeepEqual(gotFields, wantFields) {
			t.Errorf("parity mismatch for %v: lowered=(%d,%v) direct=(%d,%v)",
				c, gotScore, gotFields, wantScore, wantFields)
		}
	}
}
