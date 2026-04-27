package strutil

import "testing"

func TestDeref(t *testing.T) {
	if got := Deref(nil); got != "" {
		t.Errorf("Deref(nil) = %q, want \"\"", got)
	}
	s := "hello"
	if got := Deref(&s); got != "hello" {
		t.Errorf("Deref(&\"hello\") = %q, want \"hello\"", got)
	}
	empty := ""
	if got := Deref(&empty); got != "" {
		t.Errorf("Deref(&\"\") = %q, want \"\"", got)
	}
}

func TestDerefOr(t *testing.T) {
	if got := DerefOr(nil, "fallback"); got != "fallback" {
		t.Errorf("DerefOr(nil, \"fallback\") = %q, want \"fallback\"", got)
	}
	s := "hello"
	if got := DerefOr(&s, "fallback"); got != "hello" {
		t.Errorf("DerefOr(&\"hello\", \"fallback\") = %q, want \"hello\"", got)
	}
	// Empty pointed-to string overrides the default — the pointer is non-nil.
	empty := ""
	if got := DerefOr(&empty, "fallback"); got != "" {
		t.Errorf("DerefOr(&\"\", \"fallback\") = %q, want \"\"", got)
	}
}
