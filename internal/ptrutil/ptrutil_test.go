package ptrutil

import "testing"

func TestDerefString(t *testing.T) {
	if got := Deref[string](nil); got != "" {
		t.Errorf("Deref(nil) = %q, want \"\"", got)
	}
	s := "hello"
	if got := Deref(&s); got != "hello" {
		t.Errorf("Deref(&%q) = %q, want %q", s, got, s)
	}
	empty := ""
	if got := Deref(&empty); got != "" {
		t.Errorf("Deref(&\"\") = %q, want \"\"", got)
	}
}

func TestDerefInt(t *testing.T) {
	if got := Deref[int](nil); got != 0 {
		t.Errorf("Deref[int](nil) = %d, want 0", got)
	}
	n := 42
	if got := Deref(&n); got != 42 {
		t.Errorf("Deref(&42) = %d, want 42", got)
	}
}

func TestDerefOr(t *testing.T) {
	if got := DerefOr[string](nil, "fallback"); got != "fallback" {
		t.Errorf("DerefOr(nil, \"fallback\") = %q, want \"fallback\"", got)
	}
	s := "actual"
	if got := DerefOr(&s, "fallback"); got != "actual" {
		t.Errorf("DerefOr(&%q, \"fallback\") = %q, want %q", s, got, s)
	}
	empty := ""
	if got := DerefOr(&empty, "fallback"); got != "" {
		t.Errorf("DerefOr(&\"\", \"fallback\") = %q, want \"\" (empty pointer wins over default)", got)
	}
}
