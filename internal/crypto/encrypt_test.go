package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func validKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestRoundTrip(t *testing.T) {
	key := validKey(t)
	plaintext := []byte("plaid-access-token-sandbox-abc123")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestRoundTripEmptyPlaintext(t *testing.T) {
	key := validKey(t)

	ciphertext, err := Encrypt([]byte{}, key)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty plaintext, got %d bytes", len(got))
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key := validKey(t)
	plaintext := []byte("same input")

	c1, _ := Encrypt(plaintext, key)
	c2, _ := Encrypt(plaintext, key)

	if bytes.Equal(c1, c2) {
		t.Error("two encryptions of the same plaintext produced identical ciphertext (nonce reuse)")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := validKey(t)
	key2 := validKey(t)

	ciphertext, _ := Encrypt([]byte("secret"), key1)

	_, err := Decrypt(ciphertext, key2)
	if err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := validKey(t)
	ciphertext, _ := Encrypt([]byte("secret"), key)

	// Flip a byte in the sealed portion (after the 12-byte nonce).
	ciphertext[len(ciphertext)-1] ^= 0xFF

	_, err := Decrypt(ciphertext, key)
	if err == nil {
		t.Error("expected error decrypting tampered ciphertext")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := validKey(t)

	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Error("expected error for ciphertext shorter than nonce")
	}
}

func TestEncryptInvalidKeySize(t *testing.T) {
	for _, size := range []int{0, 15, 31, 33, 64} {
		key := make([]byte, size)
		_, err := Encrypt([]byte("test"), key)
		if err == nil {
			t.Errorf("expected error for key size %d", size)
		}
	}
}

func TestDecryptInvalidKeySize(t *testing.T) {
	// Produce a valid ciphertext first so the input is otherwise well-formed —
	// this ensures the error comes from the key, not the ciphertext shape.
	ciphertext, err := Encrypt([]byte("test"), validKey(t))
	if err != nil {
		t.Fatalf("setup Encrypt: %v", err)
	}
	for _, size := range []int{0, 15, 31, 33, 64} {
		key := make([]byte, size)
		if _, err := Decrypt(ciphertext, key); err == nil {
			t.Errorf("expected error for key size %d", size)
		}
	}
}
