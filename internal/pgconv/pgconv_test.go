package pgconv

import (
	"testing"

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
