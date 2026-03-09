package service

import (
	"testing"
	"time"
)

func TestEncodeDecode(t *testing.T) {
	date := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	id := "550e8400-e29b-41d4-a716-446655440000"

	cursor := EncodeCursor(date, id)

	gotDate, gotID, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}

	if gotDate.Format("2006-01-02") != "2024-03-15" {
		t.Errorf("date = %v, want 2024-03-15", gotDate)
	}
	if gotID != id {
		t.Errorf("id = %q, want %q", gotID, id)
	}
}

func TestDecodeBadBase64(t *testing.T) {
	_, _, err := DecodeCursor("!!!not-base64!!!")
	if err != ErrInvalidCursor {
		t.Errorf("got %v, want ErrInvalidCursor", err)
	}
}

func TestDecodeBadJSON(t *testing.T) {
	// Valid base64 but not JSON.
	_, _, err := DecodeCursor("bm90LWpzb24")
	if err != ErrInvalidCursor {
		t.Errorf("got %v, want ErrInvalidCursor", err)
	}
}

func TestDecodeBadDate(t *testing.T) {
	// {"d":"not-a-date","i":"abc"} in base64
	_, _, err := DecodeCursor("eyJkIjoibm90LWEtZGF0ZSIsImkiOiJhYmMifQ")
	if err != ErrInvalidCursor {
		t.Errorf("got %v, want ErrInvalidCursor", err)
	}
}

func TestDecodeEmptyID(t *testing.T) {
	// {"d":"2024-01-01","i":""} in base64
	_, _, err := DecodeCursor("eyJkIjoiMjAyNC0wMS0wMSIsImkiOiIifQ")
	if err != ErrInvalidCursor {
		t.Errorf("got %v, want ErrInvalidCursor", err)
	}
}

func TestTimestampCursorRoundTrip(t *testing.T) {
	ts := time.Date(2024, 6, 15, 14, 30, 45, 123456789, time.UTC)
	id := "test-id-123"

	cursor := encodeTimestampCursor(ts, id)

	gotTS, gotID, err := decodeTimestampCursor(cursor)
	if err != nil {
		t.Fatalf("decodeTimestampCursor: %v", err)
	}

	if !gotTS.Equal(ts) {
		t.Errorf("timestamp = %v, want %v", gotTS, ts)
	}
	if gotID != id {
		t.Errorf("id = %q, want %q", gotID, id)
	}
}

func TestTimestampCursorBadInput(t *testing.T) {
	_, _, err := decodeTimestampCursor("!!!bad!!!")
	if err != ErrInvalidCursor {
		t.Errorf("got %v, want ErrInvalidCursor", err)
	}
}

func TestTimestampCursorEmptyID(t *testing.T) {
	// {"t":"2024-06-15T14:30:45Z","i":""} in base64
	_, _, err := decodeTimestampCursor("eyJ0IjoiMjAyNC0wNi0xNVQxNDozMDo0NVoiLCJpIjoiIn0")
	if err != ErrInvalidCursor {
		t.Errorf("got %v, want ErrInvalidCursor", err)
	}
}
