package admin

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestDummyHash_IsValidBcrypt(t *testing.T) {
	if len(dummyHash) == 0 {
		t.Fatal("dummyHash should not be empty")
	}
	// Verify it's a valid bcrypt hash by checking it matches the expected password.
	if err := bcrypt.CompareHashAndPassword(dummyHash, []byte("dummy-password-for-timing")); err != nil {
		t.Fatalf("dummyHash should be a valid bcrypt hash of the dummy password: %v", err)
	}
}

func TestDummyHash_TimingConsistency(t *testing.T) {
	// Verify that comparing against the dummy hash takes roughly the same time
	// as comparing against a real hash, preventing timing-based user enumeration.
	realHash, err := bcrypt.GenerateFromPassword([]byte("real-password"), 12)
	if err != nil {
		t.Fatalf("failed to generate real hash: %v", err)
	}

	const iterations = 3

	// Time dummy hash comparisons.
	start := time.Now()
	for i := 0; i < iterations; i++ {
		bcrypt.CompareHashAndPassword(dummyHash, []byte("wrong-password"))
	}
	dummyDuration := time.Since(start)

	// Time real hash comparisons.
	start = time.Now()
	for i := 0; i < iterations; i++ {
		bcrypt.CompareHashAndPassword(realHash, []byte("wrong-password"))
	}
	realDuration := time.Since(start)

	// Both should be within 2x of each other (same bcrypt cost).
	ratio := float64(dummyDuration) / float64(realDuration)
	if ratio < 0.5 || ratio > 2.0 {
		t.Errorf("timing ratio %.2f is too far from 1.0 (dummy=%v, real=%v)", ratio, dummyDuration, realDuration)
	}
}
