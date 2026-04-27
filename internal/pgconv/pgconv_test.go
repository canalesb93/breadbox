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

func TestTextOr_Valid(t *testing.T) {
	got := TextOr(pgtype.Text{String: "hello", Valid: true}, "fallback")
	if got != "hello" {
		t.Errorf("TextOr(valid) = %q, want %q", got, "hello")
	}
}

func TestTextOr_EmptyValid(t *testing.T) {
	got := TextOr(pgtype.Text{String: "", Valid: true}, "fallback")
	if got != "" {
		t.Errorf("TextOr(empty valid) = %q, want empty — fallback only kicks in for NULL", got)
	}
}

func TestTextOr_Invalid(t *testing.T) {
	got := TextOr(pgtype.Text{Valid: false}, "fallback")
	if got != "fallback" {
		t.Errorf("TextOr(invalid) = %q, want %q", got, "fallback")
	}
}

func TestText(t *testing.T) {
	got := Text("hello")
	if !got.Valid || got.String != "hello" {
		t.Errorf("Text(hello) = %+v, want {hello true}", got)
	}
}

func TestText_Empty(t *testing.T) {
	got := Text("")
	if !got.Valid || got.String != "" {
		t.Errorf("Text(\"\") = %+v, want {\"\" true} — empty strings stay valid", got)
	}
}

func TestTextFromPtr_Nil(t *testing.T) {
	got := TextFromPtr(nil)
	if got.Valid {
		t.Errorf("TextFromPtr(nil) = %+v, want invalid", got)
	}
}

func TestTextFromPtr_Valid(t *testing.T) {
	s := "hello"
	got := TextFromPtr(&s)
	if !got.Valid || got.String != "hello" {
		t.Errorf("TextFromPtr(&hello) = %+v, want {hello true}", got)
	}
}

func TestTextFromPtr_EmptyPointer(t *testing.T) {
	s := ""
	got := TextFromPtr(&s)
	if !got.Valid || got.String != "" {
		t.Errorf("TextFromPtr(&\"\") = %+v, want {\"\" true}", got)
	}
}

func TestTextIfNotEmpty_NonEmpty(t *testing.T) {
	got := TextIfNotEmpty("hello")
	if !got.Valid || got.String != "hello" {
		t.Errorf("TextIfNotEmpty(hello) = %+v, want {hello true}", got)
	}
}

func TestTextIfNotEmpty_Empty(t *testing.T) {
	got := TextIfNotEmpty("")
	if got.Valid {
		t.Errorf("TextIfNotEmpty(\"\") = %+v, want invalid (NULL)", got)
	}
}

func TestDate(t *testing.T) {
	when := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	got := Date(when)
	if !got.Valid {
		t.Errorf("Date(%v).Valid = false, want true", when)
	}
	if !got.Time.Equal(when) {
		t.Errorf("Date(%v).Time = %v, want %v", when, got.Time, when)
	}
}

func TestTimestamptz(t *testing.T) {
	when := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
	got := Timestamptz(when)
	if !got.Valid {
		t.Errorf("Timestamptz(%v).Valid = false, want true", when)
	}
	if !got.Time.Equal(when) {
		t.Errorf("Timestamptz(%v).Time = %v, want %v", when, got.Time, when)
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

func TestInt4(t *testing.T) {
	got := Int4(42)
	if !got.Valid || got.Int32 != 42 {
		t.Errorf("Int4(42) = %+v, want {42 true}", got)
	}
}

func TestInt4_Zero(t *testing.T) {
	got := Int4(0)
	if !got.Valid || got.Int32 != 0 {
		t.Errorf("Int4(0) = %+v, want {0 true} — zero stays valid", got)
	}
}

func TestNumericCents(t *testing.T) {
	got := NumericCents(1050)
	if !got.Valid {
		t.Fatalf("NumericCents(1050).Valid = false, want true")
	}
	if got.Exp != -2 {
		t.Errorf("NumericCents(1050).Exp = %d, want -2", got.Exp)
	}
	if got.Int == nil || got.Int.Int64() != 1050 {
		t.Errorf("NumericCents(1050).Int = %v, want 1050", got.Int)
	}
	f, ok := NumericToFloat(got)
	if !ok || f != 10.50 {
		t.Errorf("NumericCents(1050) → NumericToFloat = (%v, %v), want (10.50, true)", f, ok)
	}
}

func TestNumericCents_Negative(t *testing.T) {
	got := NumericCents(-4200)
	f, ok := NumericToFloat(got)
	if !ok || f != -42.00 {
		t.Errorf("NumericCents(-4200) → NumericToFloat = (%v, %v), want (-42, true)", f, ok)
	}
}

func TestNumericCents_Zero(t *testing.T) {
	got := NumericCents(0)
	if !got.Valid {
		t.Errorf("NumericCents(0).Valid = false, want true — zero stays valid")
	}
	f, ok := NumericToFloat(got)
	if !ok || f != 0 {
		t.Errorf("NumericCents(0) → NumericToFloat = (%v, %v), want (0, true)", f, ok)
	}
}
