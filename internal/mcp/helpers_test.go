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
