package crypto

import "testing"

func TestFingerprint_Stable(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	got := Fingerprint(key)
	if len(got) != FingerprintLength {
		t.Fatalf("len = %d, want %d", len(got), FingerprintLength)
	}
	// sha256("0123456789abcdef0123456789abcdef") = 2c3b... — only first 8 chars matter.
	if Fingerprint(key) != got {
		t.Fatalf("fingerprint not deterministic")
	}
}

func TestFingerprint_Different(t *testing.T) {
	a := Fingerprint([]byte("0123456789abcdef0123456789abcdef"))
	b := Fingerprint([]byte("0123456789abcdef0123456789abcde0"))
	if a == b {
		t.Fatalf("different keys produced same fingerprint: %s", a)
	}
}

func TestFingerprint_Empty(t *testing.T) {
	if got := Fingerprint(nil); got != "" {
		t.Errorf("Fingerprint(nil) = %q, want empty", got)
	}
	if got := Fingerprint([]byte{}); got != "" {
		t.Errorf("Fingerprint([]) = %q, want empty", got)
	}
}
