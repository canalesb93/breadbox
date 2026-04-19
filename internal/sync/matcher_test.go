package sync

import (
	"strings"
	"testing"
)

// nameSimilarityCase is shared by both scoring variants so behavior stays
// in lock-step (nameSimilarityScoreLowered is purely an allocation
// optimization of nameSimilarityScore and must score identically).
type nameSimilarityCase struct {
	name         string
	depName      string
	depMerchant  string
	priName      string
	priMerchant  string
	wantScore    int
	wantFieldLen int
	wantField    string // first matched field, if any
}

var nameSimilarityCases = []nameSimilarityCase{
	{
		name:         "exact merchant match",
		depName:      "STARBUCKS #1234",
		depMerchant:  "Starbucks",
		priName:      "STARBUCKS STORE #1234",
		priMerchant:  "Starbucks",
		wantScore:    3,
		wantFieldLen: 1,
		wantField:    "merchant_name",
	},
	{
		name:         "merchant contains match",
		depName:      "AMAZON MARKETPLACE",
		depMerchant:  "Amazon",
		priName:      "AMZN MKTP US",
		priMerchant:  "Amazon.com",
		wantScore:    2,
		wantFieldLen: 1,
		wantField:    "merchant_name",
	},
	{
		name:         "merchant contains reversed (pri shorter)",
		depName:      "AMAZON.COM PURCHASE",
		depMerchant:  "Amazon.com",
		priName:      "AMZN",
		priMerchant:  "Amazon",
		wantScore:    2,
		wantFieldLen: 1,
		wantField:    "merchant_name",
	},
	{
		name:         "exact name match no merchant",
		depName:      "TRADER JOE'S #123",
		depMerchant:  "",
		priName:      "TRADER JOE'S #123",
		priMerchant:  "",
		wantScore:    2,
		wantFieldLen: 1,
		wantField:    "name",
	},
	{
		name:         "exact name match case insensitive",
		depName:      "Trader Joe's #123",
		depMerchant:  "",
		priName:      "TRADER JOE'S #123",
		priMerchant:  "",
		wantScore:    2,
		wantFieldLen: 1,
		wantField:    "name",
	},
	{
		name:         "name contains match",
		depName:      "DOORDASH CHIPOTLE",
		depMerchant:  "",
		priName:      "DD *DOORDASH CHIPOTLE SAN FRANCISCO",
		priMerchant:  "",
		wantScore:    1,
		wantFieldLen: 1,
		wantField:    "name",
	},
	{
		name:         "name contains reversed (pri shorter)",
		depName:      "DD *DOORDASH CHIPOTLE SAN FRANCISCO",
		depMerchant:  "",
		priName:      "DOORDASH CHIPOTLE",
		priMerchant:  "",
		wantScore:    1,
		wantFieldLen: 1,
		wantField:    "name",
	},
	{
		name:         "name contains case insensitive",
		depName:      "doordash chipotle",
		depMerchant:  "",
		priName:      "DD *DOORDASH CHIPOTLE SAN FRANCISCO",
		priMerchant:  "",
		wantScore:    1,
		wantFieldLen: 1,
		wantField:    "name",
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
		name:         "case insensitive merchant exact",
		depName:      "COSTCO WHOLESALE",
		depMerchant:  "COSTCO WHOLESALE",
		priName:      "COSTCO WHSE #1234",
		priMerchant:  "Costco Wholesale",
		wantScore:    3,
		wantFieldLen: 1,
		wantField:    "merchant_name",
	},
	{
		name:         "dep merchant empty skips merchant branches",
		depName:      "STARBUCKS STORE",
		depMerchant:  "",
		priName:      "STARBUCKS STORE",
		priMerchant:  "Starbucks",
		wantScore:    2,
		wantFieldLen: 1,
		wantField:    "name",
	},
	{
		name:         "pri merchant empty skips merchant branches",
		depName:      "STARBUCKS STORE",
		depMerchant:  "Starbucks",
		priName:      "STARBUCKS STORE",
		priMerchant:  "",
		wantScore:    2,
		wantFieldLen: 1,
		wantField:    "name",
	},
	{
		name:         "different merchants still exact name match",
		depName:      "AMAZON PAYMENTS",
		depMerchant:  "Amazon",
		priName:      "AMAZON PAYMENTS",
		priMerchant:  "Stripe",
		wantScore:    2,
		wantFieldLen: 1,
		wantField:    "name",
	},
	{
		name:         "different merchants name contains",
		depName:      "PAYPAL TRANSFER FROM ACME CO",
		depMerchant:  "PayPal",
		priName:      "ACME CO",
		priMerchant:  "Acme",
		wantScore:    1,
		wantFieldLen: 1,
		wantField:    "name",
	},
	{
		name:         "merchant wins over name even when name also exact",
		depName:      "STARBUCKS #1234",
		depMerchant:  "Starbucks",
		priName:      "STARBUCKS #1234",
		priMerchant:  "Starbucks",
		wantScore:    3,
		wantFieldLen: 1,
		wantField:    "merchant_name",
	},
}

func TestNameSimilarityScore(t *testing.T) {
	for _, tt := range nameSimilarityCases {
		t.Run(tt.name, func(t *testing.T) {
			score, fields := nameSimilarityScore(tt.depName, tt.depMerchant, tt.priName, tt.priMerchant)
			if score != tt.wantScore {
				t.Errorf("score = %d, want %d", score, tt.wantScore)
			}
			if len(fields) != tt.wantFieldLen {
				t.Fatalf("matched fields = %v (len %d), want len %d", fields, len(fields), tt.wantFieldLen)
			}
			if tt.wantField != "" && fields[0] != tt.wantField {
				t.Errorf("matched field = %q, want %q", fields[0], tt.wantField)
			}
		})
	}
}

// TestNameSimilarityScoreLowered_ParityWithUnlowered asserts the lowered
// variant returns identical results to the canonical function for every
// case. The lowered form is purely an allocation optimization; any
// behavioral divergence is a bug.
func TestNameSimilarityScoreLowered_ParityWithUnlowered(t *testing.T) {
	for _, tt := range nameSimilarityCases {
		t.Run(tt.name, func(t *testing.T) {
			wantScore, wantFields := nameSimilarityScore(tt.depName, tt.depMerchant, tt.priName, tt.priMerchant)
			gotScore, gotFields := nameSimilarityScoreLowered(
				tt.depName, strings.ToLower(tt.depName),
				tt.depMerchant, strings.ToLower(tt.depMerchant),
				tt.priName, tt.priMerchant,
			)
			if gotScore != wantScore {
				t.Errorf("lowered score = %d, canonical = %d", gotScore, wantScore)
			}
			if len(gotFields) != len(wantFields) {
				t.Fatalf("lowered fields = %v, canonical = %v", gotFields, wantFields)
			}
			for i := range gotFields {
				if gotFields[i] != wantFields[i] {
					t.Errorf("lowered fields[%d] = %q, canonical = %q", i, gotFields[i], wantFields[i])
				}
			}
		})
	}
}

func TestBuildMatchedOn(t *testing.T) {
	tests := []struct {
		name        string
		depName     string
		depMerchant string
		priName     string
		priMerchant string
		want        []string
	}{
		{
			name:        "merchant exact",
			depName:     "STARBUCKS #1",
			depMerchant: "Starbucks",
			priName:     "STARBUCKS #2",
			priMerchant: "Starbucks",
			want:        []string{"merchant_name"},
		},
		{
			name:    "name exact",
			depName: "TRADER JOE'S",
			priName: "TRADER JOE'S",
			want:    []string{"name"},
		},
		{
			name:        "no similarity returns nil",
			depName:     "UBER TRIP",
			depMerchant: "Uber",
			priName:     "LYFT RIDE",
			priMerchant: "Lyft",
			want:        nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMatchedOn(tt.depName, tt.depMerchant, tt.priName, tt.priMerchant)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
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
			name:        "empty candidates returns nil",
			depName:     "STARBUCKS",
			depMerchant: "",
			candidates:  nil,
			wantNil:     true,
		},
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
		{
			name:        "three candidates one clear winner at top score",
			depName:     "STARBUCKS #123",
			depMerchant: "Starbucks",
			candidates: []matchCandidate{
				{Name: "STARBUCKS RESERVE", MerchantName: "Starbucks"}, // 3
				{Name: "SOME OTHER STARBUCKS", MerchantName: "Peet's"}, // 1 (name contains)
				{Name: "UNRELATED", MerchantName: "Dunkin"},            // 0
			},
			wantNil: false,
			wantIdx: 0,
		},
		{
			name:        "three candidates last one wins",
			depName:     "AMAZON MARKETPLACE",
			depMerchant: "Amazon",
			candidates: []matchCandidate{
				{Name: "UNRELATED ONE", MerchantName: ""},      // 0
				{Name: "AMZN MKTP", MerchantName: "Amazon.com"}, // 2 (merchant contains)
				{Name: "AMAZON.COM", MerchantName: "Amazon"},    // 3 (merchant exact)
			},
			wantNil: false,
			wantIdx: 2,
		},
		{
			name:        "three-way tie at top score returns nil",
			depName:     "PAYMENT",
			depMerchant: "",
			candidates: []matchCandidate{
				{Name: "PAYMENT RECEIVED", MerchantName: ""},
				{Name: "PAYMENT SENT", MerchantName: ""},
				{Name: "PAYMENT REFUND", MerchantName: ""},
			},
			wantNil: true,
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

