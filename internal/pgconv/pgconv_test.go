package pgconv

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestFormatUUID_Valid(t *testing.T) {
	u := pgtype.UUID{
		Bytes: [16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0},
		Valid: true,
	}
	got := FormatUUID(u)
	want := "12345678-9abc-def0-1234-56789abcdef0"
	if got != want {
		t.Errorf("FormatUUID() = %q, want %q", got, want)
	}
}

func TestFormatUUID_Invalid(t *testing.T) {
	u := pgtype.UUID{Valid: false}
	got := FormatUUID(u)
	if got != "" {
		t.Errorf("FormatUUID(invalid) = %q, want empty", got)
	}
}

func TestFormatUUID_ZeroBytes(t *testing.T) {
	u := pgtype.UUID{
		Bytes: [16]byte{},
		Valid: true,
	}
	got := FormatUUID(u)
	want := "00000000-0000-0000-0000-000000000000"
	if got != want {
		t.Errorf("FormatUUID(zero) = %q, want %q", got, want)
	}
}

func TestFormatUUID_MaxBytes(t *testing.T) {
	u := pgtype.UUID{
		Bytes: [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		Valid: true,
	}
	got := FormatUUID(u)
	want := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if got != want {
		t.Errorf("FormatUUID(max) = %q, want %q", got, want)
	}
}

func TestParseUUID_Canonical(t *testing.T) {
	got, err := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatalf("ParseUUID: unexpected error: %v", err)
	}
	if !got.Valid {
		t.Fatal("ParseUUID: expected Valid=true")
	}
	if FormatUUID(got) != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("ParseUUID roundtrip mismatch: got %q", FormatUUID(got))
	}
}

func TestParseUUID_NoDashes(t *testing.T) {
	got, err := ParseUUID("550e8400e29b41d4a716446655440000")
	if err != nil {
		t.Fatalf("ParseUUID: unexpected error: %v", err)
	}
	if FormatUUID(got) != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("ParseUUID roundtrip mismatch: got %q", FormatUUID(got))
	}
}

func TestParseUUID_Invalid(t *testing.T) {
	if _, err := ParseUUID("not-a-uuid"); err == nil {
		t.Error("ParseUUID: expected error for invalid input")
	}
}

func TestNumericToFloat_Valid(t *testing.T) {
	var n pgtype.Numeric
	if err := n.Scan("123.45"); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got, ok := NumericToFloat(n)
	if !ok {
		t.Fatal("NumericToFloat: expected ok=true")
	}
	if got != 123.45 {
		t.Errorf("NumericToFloat = %v, want 123.45", got)
	}
}

func TestNumericToFloat_Negative(t *testing.T) {
	var n pgtype.Numeric
	if err := n.Scan("-42.5"); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got, ok := NumericToFloat(n)
	if !ok {
		t.Fatal("NumericToFloat: expected ok=true")
	}
	if got != -42.5 {
		t.Errorf("NumericToFloat = %v, want -42.5", got)
	}
}

func TestNumericToFloat_Zero(t *testing.T) {
	var n pgtype.Numeric
	if err := n.Scan("0"); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got, ok := NumericToFloat(n)
	if !ok {
		t.Fatal("NumericToFloat: expected ok=true for zero")
	}
	if got != 0 {
		t.Errorf("NumericToFloat = %v, want 0", got)
	}
}

func TestNumericToFloat_Null(t *testing.T) {
	n := pgtype.Numeric{Valid: false}
	got, ok := NumericToFloat(n)
	if ok {
		t.Error("NumericToFloat: expected ok=false for NULL")
	}
	if got != 0 {
		t.Errorf("NumericToFloat = %v, want 0", got)
	}
}

func TestNumericToFloat_NaN(t *testing.T) {
	n := pgtype.Numeric{NaN: true, Valid: true}
	_, ok := NumericToFloat(n)
	if ok {
		t.Error("NumericToFloat: expected ok=false for NaN")
	}
}

func TestTextPtr_Valid(t *testing.T) {
	got := TextPtr(pgtype.Text{String: "hello", Valid: true})
	if got == nil || *got != "hello" {
		t.Errorf("TextPtr = %v, want &\"hello\"", got)
	}
}

func TestTextPtr_Empty(t *testing.T) {
	got := TextPtr(pgtype.Text{String: "", Valid: true})
	if got == nil || *got != "" {
		t.Errorf("TextPtr(empty valid) = %v, want &\"\"", got)
	}
}

func TestTextPtr_Invalid(t *testing.T) {
	got := TextPtr(pgtype.Text{Valid: false})
	if got != nil {
		t.Errorf("TextPtr(invalid) = %v, want nil", got)
	}
}

func TestTimestampStr_Valid(t *testing.T) {
	ts := pgtype.Timestamptz{
		Time:  time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
		Valid: true,
	}
	got := TimestampStr(ts)
	want := "2024-03-15T14:30:00Z"
	if got != want {
		t.Errorf("TimestampStr = %q, want %q", got, want)
	}
}

func TestTimestampStr_NonUTC(t *testing.T) {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	ts := pgtype.Timestamptz{
		Time:  time.Date(2024, 3, 15, 7, 30, 0, 0, loc),
		Valid: true,
	}
	got := TimestampStr(ts)
	want := "2024-03-15T14:30:00Z"
	if got != want {
		t.Errorf("TimestampStr(non-UTC) = %q, want %q (should be normalized to UTC)", got, want)
	}
}

func TestTimestampStr_Invalid(t *testing.T) {
	got := TimestampStr(pgtype.Timestamptz{Valid: false})
	if got != "" {
		t.Errorf("TimestampStr(invalid) = %q, want empty", got)
	}
}

func TestTimestampStrPtr_Valid(t *testing.T) {
	ts := pgtype.Timestamptz{
		Time:  time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
		Valid: true,
	}
	got := TimestampStrPtr(ts)
	if got == nil {
		t.Fatal("TimestampStrPtr: expected non-nil")
	}
	want := "2024-03-15T14:30:00Z"
	if *got != want {
		t.Errorf("TimestampStrPtr = %q, want %q", *got, want)
	}
}

func TestTimestampStrPtr_Invalid(t *testing.T) {
	got := TimestampStrPtr(pgtype.Timestamptz{Valid: false})
	if got != nil {
		t.Errorf("TimestampStrPtr(invalid) = %v, want nil", got)
	}
}

func TestDateStrPtr_Valid(t *testing.T) {
	d := pgtype.Date{
		Time:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
	got := DateStrPtr(d)
	if got == nil {
		t.Fatal("DateStrPtr: expected non-nil")
	}
	if *got != "2024-03-15" {
		t.Errorf("DateStrPtr = %q, want 2024-03-15", *got)
	}
}

func TestDateStrPtr_Invalid(t *testing.T) {
	got := DateStrPtr(pgtype.Date{Valid: false})
	if got != nil {
		t.Errorf("DateStrPtr(invalid) = %v, want nil", got)
	}
}
