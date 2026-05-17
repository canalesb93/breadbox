//go:build !lite

package appconfig

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"breadbox/internal/crypto"
	"breadbox/internal/db"
)

// Writer is the minimal write surface used by the encrypted helpers.
// *db.Queries satisfies this; tests can inject their own.
type Writer interface {
	SetAppConfig(ctx context.Context, arg db.SetAppConfigParams) error
}

// ReadEncrypted reads key from app_config, hex-decodes the value, and
// AES-256-GCM-decrypts it using encKey. Returns ("", false, nil) when the
// key is absent or stored value is empty. Returns an error on
// hex/decrypt failure (treat as "the stored value is corrupt").
func ReadEncrypted(ctx context.Context, r Reader, key string, encKey []byte) (string, bool, error) {
	raw, ok := Read(ctx, r, key)
	if !ok || raw == "" {
		return "", false, nil
	}
	ciphertext, err := hex.DecodeString(raw)
	if err != nil {
		return "", false, fmt.Errorf("appconfig: hex-decode %q: %w", key, err)
	}
	plain, err := crypto.Decrypt(ciphertext, encKey)
	if err != nil {
		return "", false, fmt.Errorf("appconfig: decrypt %q: %w", key, err)
	}
	return string(plain), true, nil
}

// WriteEncrypted AES-256-GCM-encrypts plaintext using encKey and writes the
// hex-encoded ciphertext to app_config at key. Passing an empty plaintext
// is allowed and stores the empty string (effectively clearing the key).
func WriteEncrypted(ctx context.Context, w Writer, key, plaintext string, encKey []byte) error {
	if plaintext == "" {
		return w.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   key,
			Value: pgtype.Text{String: "", Valid: true},
		})
	}
	ciphertext, err := crypto.Encrypt([]byte(plaintext), encKey)
	if err != nil {
		return fmt.Errorf("appconfig: encrypt %q: %w", key, err)
	}
	return w.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   key,
		Value: pgtype.Text{String: hex.EncodeToString(ciphertext), Valid: true},
	})
}
