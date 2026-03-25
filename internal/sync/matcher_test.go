package sync

import "testing"

func TestNameSimilarityScore(t *testing.T) {
	tests := []struct {
		name         string
		depName      string
		depMerchant  string
		priName      string
		priMerchant  string
		wantScore    int
		wantFieldLen int // expected number of matched fields
	}{
		{
			name:         "exact merchant match",
			depName:      "STARBUCKS #1234",
			depMerchant:  "Starbucks",
			priName:      "STARBUCKS STORE #1234",
			priMerchant:  "Starbucks",
			wantScore:    3,
			wantFieldLen: 1,
		},
		{
			name:         "merchant contains match",
			depName:      "AMAZON MARKETPLACE",
			depMerchant:  "Amazon",
			priName:      "AMZN MKTP US",
			priMerchant:  "Amazon.com",
			wantScore:    2,
			wantFieldLen: 1,
		},
		{
			name:         "exact name match no merchant",
			depName:      "TRADER JOE'S #123",
			depMerchant:  "",
			priName:      "TRADER JOE'S #123",
			priMerchant:  "",
			wantScore:    2,
			wantFieldLen: 1,
		},
		{
			name:         "name contains match",
			depName:      "DOORDASH CHIPOTLE",
			depMerchant:  "",
			priName:      "DD *DOORDASH CHIPOTLE SAN FRANCISCO",
			priMerchant:  "",
			wantScore:    1,
			wantFieldLen: 1,
		},
		{
			name:         "no match at all",
			depName:      "UBER TRIP",
			depMerchant:  "Uber",
			priName:      "LYFT RIDE",
			priMerchant:  "Lyft",
			wantScore:    0,
			wantFieldLen: 0,
		},
		{
			name:         "case insensitive merchant",
			depName:      "COSTCO WHOLESALE",
			depMerchant:  "COSTCO WHOLESALE",
			priName:      "COSTCO WHSE #1234",
			priMerchant:  "Costco Wholesale",
			wantScore:    3,
			wantFieldLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, fields := nameSimilarityScore(tt.depName, tt.depMerchant, tt.priName, tt.priMerchant)
			if score != tt.wantScore {
				t.Errorf("score = %d, want %d", score, tt.wantScore)
			}
			if len(fields) != tt.wantFieldLen {
				t.Errorf("matched fields = %v (len %d), want len %d", fields, len(fields), tt.wantFieldLen)
			}
		})
	}
}

func TestPickBestCandidate(t *testing.T) {
	tests := []struct {
		name        string
		depName     string
		depMerchant string
		candidates  []matchCandidate
		wantNil     bool
		wantIdx     int // expected index if not nil
	}{
		{
			name:        "single candidate returns it",
			depName:     "STARBUCKS",
			depMerchant: "",
			candidates: []matchCandidate{
				{Name: "STARBUCKS STORE", MerchantName: ""},
			},
			wantNil: false,
			wantIdx: 0,
		},
		{
			name:        "two candidates same score returns nil (ambiguous)",
			depName:     "PAYMENT",
			depMerchant: "",
			candidates: []matchCandidate{
				{Name: "PAYMENT RECEIVED", MerchantName: ""},
				{Name: "PAYMENT SENT", MerchantName: ""},
			},
			wantNil: true,
		},
		{
			name:        "two candidates different scores picks best",
			depName:     "STARBUCKS #123",
			depMerchant: "Starbucks",
			candidates: []matchCandidate{
				{Name: "SOME COFFEE SHOP", MerchantName: ""},
				{Name: "STARBUCKS RESERVE", MerchantName: "Starbucks"},
			},
			wantNil: false,
			wantIdx: 1,
		},
		{
			name:        "no name similarity still picks single best zero",
			depName:     "ABC",
			depMerchant: "",
			candidates: []matchCandidate{
				{Name: "XYZ", MerchantName: ""},
				{Name: "DEF", MerchantName: ""},
			},
			wantNil: true, // both score 0, ambiguous
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := pickBestCandidate(tt.depName, tt.depMerchant, tt.candidates)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got candidate %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected a candidate, got nil")
			}
			if result.Name != tt.candidates[tt.wantIdx].Name {
				t.Errorf("picked %q, want %q", result.Name, tt.candidates[tt.wantIdx].Name)
			}
		})
	}
}
