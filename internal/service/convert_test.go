package service

import (
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestFormatUUID(t *testing.T) {
	u := pgtype.UUID{
		Bytes: [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00},
		Valid: true,
	}

	got := formatUUID(u)
	want := "550e8400-e29b-41d4-a716-446655440000"
	if got != want {
		t.Errorf("formatUUID = %q, want %q", got, want)
	}
}

func TestFormatUUIDInvalid(t *testing.T) {
	u := pgtype.UUID{Valid: false}
	got := formatUUID(u)
	if got != "" {
		t.Errorf("formatUUID(invalid) = %q, want empty", got)
	}
}

func TestUuidPtr(t *testing.T) {
	u := pgtype.UUID{
		Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		Valid: true,
	}
	got := uuidPtr(u)
	if got == nil {
		t.Fatal("uuidPtr returned nil for valid UUID")
	}
	if *got == "" {
		t.Error("uuidPtr returned empty string for valid UUID")
	}
}

func TestUuidPtrInvalid(t *testing.T) {
	got := uuidPtr(pgtype.UUID{Valid: false})
	if got != nil {
		t.Errorf("uuidPtr(invalid) = %v, want nil", got)
	}
}

func TestTextPtr(t *testing.T) {
	txt := pgtype.Text{String: "hello", Valid: true}
	got := textPtr(txt)
	if got == nil || *got != "hello" {
		t.Errorf("textPtr = %v, want &hello", got)
	}
}

func TestTextPtrInvalid(t *testing.T) {
	got := textPtr(pgtype.Text{Valid: false})
	if got != nil {
		t.Errorf("textPtr(invalid) = %v, want nil", got)
	}
}

func TestNumericFloat(t *testing.T) {
	tests := []struct {
		name string
		n    pgtype.Numeric
		want *float64
	}{
		{
			name: "100.50 (Int=10050, Exp=-2)",
			n:    pgtype.Numeric{Int: big.NewInt(10050), Exp: -2, Valid: true},
			want: ptrFloat(100.50),
		},
		{
			name: "negative -42.00",
			n:    pgtype.Numeric{Int: big.NewInt(-4200), Exp: -2, Valid: true},
			want: ptrFloat(-42.00),
		},
		{
			name: "zero",
			n:    pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true},
			want: ptrFloat(0),
		},
		{
			name: "positive exponent (500 = 5 * 10^2)",
			n:    pgtype.Numeric{Int: big.NewInt(5), Exp: 2, Valid: true},
			want: ptrFloat(500),
		},
		{
			name: "invalid",
			n:    pgtype.Numeric{Valid: false},
			want: nil,
		},
		{
			name: "nil Int",
			n:    pgtype.Numeric{Int: nil, Valid: true},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := numericFloat(tt.n)
			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want non-nil")
			}
			if *got != *tt.want {
				t.Errorf("got %f, want %f", *got, *tt.want)
			}
		})
	}
}

func TestTimestampStr(t *testing.T) {
	ts := pgtype.Timestamptz{
		Time:  time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
		Valid: true,
	}
	got := timestampStr(ts)
	if got == nil {
		t.Fatal("got nil")
	}
	want := "2024-03-15T14:30:00Z"
	if *got != want {
		t.Errorf("timestampStr = %q, want %q", *got, want)
	}
}

func TestTimestampStrInvalid(t *testing.T) {
	got := timestampStr(pgtype.Timestamptz{Valid: false})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestDateStr(t *testing.T) {
	d := pgtype.Date{
		Time:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
	got := dateStr(d)
	if got == nil {
		t.Fatal("got nil")
	}
	if *got != "2024-03-15" {
		t.Errorf("dateStr = %q, want 2024-03-15", *got)
	}
}

func TestDateStrInvalid(t *testing.T) {
	got := dateStr(pgtype.Date{Valid: false})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestFormatDurationMs(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{0, "0ms"},
		{42, "42ms"},
		{999, "999ms"},
		{1000, "1.0s"},
		{1500, "1.5s"},
		{5250, "5.2s"},
		{59999, "60.0s"},
		{60000, "1m"},
		{90000, "1m 30s"},
		{120000, "2m"},
		{125000, "2m 5s"},
		{300000, "5m"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDurationMs(tt.ms)
			if got != tt.want {
				t.Errorf("formatDurationMs(%d) = %q, want %q", tt.ms, got, tt.want)
			}
		})
	}
}

func ptrFloat(f float64) *float64 {
	return &f
}
