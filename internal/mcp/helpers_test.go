package mcp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOptStr(t *testing.T) {
	if optStr("") != nil {
		t.Error("optStr(\"\") should return nil")
	}
	p := optStr("hello")
	if p == nil {
		t.Fatal("optStr(non-empty) should return non-nil pointer")
	}
	if *p != "hello" {
		t.Errorf("expected %q, got %q", "hello", *p)
	}
}

func TestParseOptionalDate(t *testing.T) {
	t.Run("empty returns nil without error", func(t *testing.T) {
		got, err := parseOptionalDate("start_date", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("valid date parses", func(t *testing.T) {
		got, err := parseOptionalDate("start_date", "2024-03-15")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil pointer")
		}
		want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("expected %v, got %v", want, *got)
		}
	})

	t.Run("invalid date returns wrapped error with field name", func(t *testing.T) {
		_, err := parseOptionalDate("end_date", "not-a-date")
		if err == nil {
			t.Fatal("expected error")
		}
		if msg := err.Error(); !strings.Contains(msg, "end_date") {
			t.Errorf("expected error to mention field name, got: %q", msg)
		}
		// Ensure the underlying time.Parse error is wrapped so callers can unwrap.
		var parseErr *time.ParseError
		if !errors.As(err, &parseErr) {
			t.Errorf("expected wrapped *time.ParseError, got: %v", err)
		}
	})
}

func TestParseDateRange(t *testing.T) {
	t.Run("both empty returns nil pair", func(t *testing.T) {
		start, end, err := parseDateRange("", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if start != nil || end != nil {
			t.Errorf("expected nil pair, got start=%v end=%v", start, end)
		}
	})

	t.Run("only start parses", func(t *testing.T) {
		start, end, err := parseDateRange("2024-03-15", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if start == nil || end != nil {
			t.Fatalf("expected start set, end nil; got start=%v end=%v", start, end)
		}
		want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
		if !start.Equal(want) {
			t.Errorf("expected %v, got %v", want, *start)
		}
	})

	t.Run("both set parse", func(t *testing.T) {
		start, end, err := parseDateRange("2024-01-01", "2024-12-31")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if start == nil || end == nil {
			t.Fatal("expected both non-nil")
		}
	})

	t.Run("invalid start surfaces field name", func(t *testing.T) {
		_, _, err := parseDateRange("nope", "2024-12-31")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "start_date") {
			t.Errorf("expected start_date in error, got: %q", err.Error())
		}
	})

	t.Run("invalid end surfaces field name", func(t *testing.T) {
		_, _, err := parseDateRange("2024-01-01", "nope")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "end_date") {
			t.Errorf("expected end_date in error, got: %q", err.Error())
		}
	})
}
