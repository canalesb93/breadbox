package api

import (
	"net/url"
	"testing"
	"time"
)

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		key        string
		defaultVal int
		min, max   int
		want       int
		wantErr    bool
	}{
		{"absent returns default", "", "limit", 100, 1, 500, 100, false},
		{"valid value", "limit=50", "limit", 100, 1, 500, 50, false},
		{"min boundary", "limit=1", "limit", 100, 1, 500, 1, false},
		{"max boundary", "limit=500", "limit", 100, 1, 500, 500, false},
		{"below min", "limit=0", "limit", 100, 1, 500, 0, true},
		{"above max", "limit=501", "limit", 100, 1, 500, 0, true},
		{"not a number", "limit=abc", "limit", 100, 1, 500, 0, true},
		{"negative", "limit=-1", "limit", 100, 1, 500, 0, true},
		{"float value", "limit=1.5", "limit", 100, 1, 500, 0, true},
		{"different default", "", "limit", 50, 1, 500, 50, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			got, err := parseIntParam(q, tt.key, tt.defaultVal, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseIntParam() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseIntParam() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseDateParam(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		key     string
		want    *time.Time
		wantErr bool
	}{
		{"absent returns nil", "", "start_date", nil, false},
		{"valid date", "start_date=2024-01-15", "start_date", timePtr(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)), false},
		{"invalid format", "start_date=01-15-2024", "start_date", nil, true},
		{"partial date", "start_date=2024-01", "start_date", nil, true},
		{"garbage", "start_date=not-a-date", "start_date", nil, true},
		{"different key", "end_date=2024-12-31", "end_date", timePtr(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			got, err := parseDateParam(q, tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDateParam() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.want == nil && got != nil {
				t.Errorf("parseDateParam() = %v, want nil", got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("parseDateParam() = nil, want %v", tt.want)
				}
				if !got.Equal(*tt.want) {
					t.Errorf("parseDateParam() = %v, want %v", *got, *tt.want)
				}
			}
		})
	}
}

func TestParseFloatParam(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		key     string
		want    *float64
		wantErr bool
	}{
		{"absent returns nil", "", "min_amount", nil, false},
		{"valid positive", "min_amount=42.50", "min_amount", float64Ptr(42.50), false},
		{"valid negative", "min_amount=-10.0", "min_amount", float64Ptr(-10.0), false},
		{"valid zero", "min_amount=0", "min_amount", float64Ptr(0), false},
		{"valid integer", "min_amount=100", "min_amount", float64Ptr(100), false},
		{"not a number", "min_amount=abc", "min_amount", nil, true},
		{"empty after equals", "min_amount=", "min_amount", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			got, err := parseFloatParam(q, tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFloatParam() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.want == nil && got != nil {
				t.Errorf("parseFloatParam() = %v, want nil", *got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("parseFloatParam() = nil, want %v", *tt.want)
				}
				if *got != *tt.want {
					t.Errorf("parseFloatParam() = %f, want %f", *got, *tt.want)
				}
			}
		})
	}
}

func TestParseBoolParam(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		key     string
		want    *bool
		wantErr bool
	}{
		{"absent returns nil", "", "pending", nil, false},
		{"true", "pending=true", "pending", boolPtr(true), false},
		{"false", "pending=false", "pending", boolPtr(false), false},
		{"invalid value", "pending=yes", "pending", nil, true},
		{"numeric 1", "pending=1", "pending", nil, true},
		{"numeric 0", "pending=0", "pending", nil, true},
		{"capitalized", "pending=True", "pending", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			got, err := parseBoolParam(q, tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseBoolParam() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.want == nil && got != nil {
				t.Errorf("parseBoolParam() = %v, want nil", *got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("parseBoolParam() = nil, want %v", *tt.want)
				}
				if *got != *tt.want {
					t.Errorf("parseBoolParam() = %v, want %v", *got, *tt.want)
				}
			}
		})
	}
}

func TestParseOptionalStringParam(t *testing.T) {
	tests := []struct {
		name  string
		query string
		key   string
		want  *string
	}{
		{"absent returns nil", "", "account_id", nil},
		{"present returns pointer", "account_id=abc123", "account_id", stringPtr("abc123")},
		{"empty value returns nil", "account_id=", "account_id", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			got := parseOptionalStringParam(q, tt.key)
			if tt.want == nil && got != nil {
				t.Errorf("parseOptionalStringParam() = %v, want nil", *got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("parseOptionalStringParam() = nil, want %v", *tt.want)
				}
				if *got != *tt.want {
					t.Errorf("parseOptionalStringParam() = %q, want %q", *got, *tt.want)
				}
			}
		})
	}
}

func TestParseMinLengthStringParam(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		key     string
		minLen  int
		want    *string
		wantErr bool
	}{
		{"absent returns nil", "", "search", 2, nil, false},
		{"valid length", "search=abc", "search", 2, stringPtr("abc"), false},
		{"exact min length", "search=ab", "search", 2, stringPtr("ab"), false},
		{"too short", "search=a", "search", 2, nil, true},
		{"empty value returns nil", "search=", "search", 2, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			got, err := parseMinLengthStringParam(q, tt.key, tt.minLen)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMinLengthStringParam() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.want == nil && got != nil {
				t.Errorf("parseMinLengthStringParam() = %v, want nil", *got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("parseMinLengthStringParam() = nil, want %v", *tt.want)
				}
				if *got != *tt.want {
					t.Errorf("parseMinLengthStringParam() = %q, want %q", *got, *tt.want)
				}
			}
		})
	}
}

func TestParseEnumParam(t *testing.T) {
	sortOptions := []string{"date", "amount", "name"}

	tests := []struct {
		name    string
		query   string
		key     string
		allowed []string
		want    *string
		wantErr bool
	}{
		{"absent returns nil", "", "sort_by", sortOptions, nil, false},
		{"valid value", "sort_by=date", "sort_by", sortOptions, stringPtr("date"), false},
		{"another valid value", "sort_by=amount", "sort_by", sortOptions, stringPtr("amount"), false},
		{"invalid value", "sort_by=invalid", "sort_by", sortOptions, nil, true},
		{"empty value returns nil", "sort_by=", "sort_by", sortOptions, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			got, err := parseEnumParam(q, tt.key, tt.allowed)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseEnumParam() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.want == nil && got != nil {
				t.Errorf("parseEnumParam() = %v, want nil", *got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("parseEnumParam() = nil, want %v", *tt.want)
				}
				if *got != *tt.want {
					t.Errorf("parseEnumParam() = %q, want %q", *got, *tt.want)
				}
			}
		})
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name string
		ss   []string
		want string
	}{
		{"empty", nil, ""},
		{"single", []string{"a"}, "a"},
		{"two", []string{"a", "b"}, "a, b"},
		{"three", []string{"date", "amount", "name"}, "date, amount, name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinStrings(tt.ss)
			if got != tt.want {
				t.Errorf("joinStrings() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test helpers.
func timePtr(t time.Time) *time.Time    { return &t }
func float64Ptr(f float64) *float64     { return &f }
func boolPtr(b bool) *bool              { return &b }
func stringPtr(s string) *string        { return &s }
