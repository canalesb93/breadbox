package csv

import (
	"testing"
)

func TestDetectDateFormat(t *testing.T) {
	tests := []struct {
		name    string
		samples []string
		want    string
		wantErr bool
	}{
		{
			name:    "MM/DD/YYYY",
			samples: []string{"01/15/2024", "02/28/2024", "12/31/2023", "03/01/2024", "11/15/2023"},
			want:    "01/02/2006",
		},
		{
			name:    "ISO 8601 YYYY-MM-DD",
			samples: []string{"2024-01-15", "2024-02-28", "2023-12-31", "2024-03-01"},
			want:    "2006-01-02",
		},
		{
			name:    "M/D/YYYY single digit",
			samples: []string{"1/5/2024", "2/8/2024", "3/1/2024", "4/9/2024"},
			want:    "1/2/2006",
		},
		{
			name:    "MM-DD-YYYY",
			samples: []string{"01-15-2024", "02-28-2024", "12-31-2023"},
			want:    "01-02-2006",
		},
		{
			name:    "YYYY/MM/DD",
			samples: []string{"2024/01/15", "2024/02/28", "2023/12/31"},
			want:    "2006/01/02",
		},
		{
			name:    "Mon DD YYYY",
			samples: []string{"Jan 15, 2024", "Feb 28, 2024", "Dec 31, 2023"},
			want:    "Jan 2, 2006",
		},
		{
			name:    "Month DD YYYY",
			samples: []string{"January 15, 2024", "February 28, 2024", "December 31, 2023"},
			want:    "January 2, 2006",
		},
		{
			name:    "90% threshold with one bad value",
			samples: []string{"2024-01-15", "2024-02-28", "2024-03-01", "2024-04-15", "2024-05-20", "2024-06-30", "2024-07-04", "2024-08-15", "2024-09-01", "bad-date"},
			want:    "2006-01-02",
		},
		{
			name:    "empty samples",
			samples: []string{},
			wantErr: true,
		},
		{
			name:    "only empty strings",
			samples: []string{"", "  ", ""},
			wantErr: true,
		},
		{
			name:    "no format matches",
			samples: []string{"not-a-date", "also-not", "nope"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectDateFormat(tt.samples)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("DetectDateFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	got, err := ParseDate("2024-03-15", "2006-01-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Year() != 2024 || got.Month() != 3 || got.Day() != 15 {
		t.Errorf("parsed date = %v, want 2024-03-15", got)
	}
}

func TestParseDateInvalid(t *testing.T) {
	_, err := ParseDate("not-a-date", "2006-01-02")
	if err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestParseDateWhitespace(t *testing.T) {
	got, err := ParseDate("  2024-03-15  ", "2006-01-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Day() != 15 {
		t.Errorf("whitespace not trimmed: got day %d", got.Day())
	}
}
