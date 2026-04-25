package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EncryptionKeyFilename is the basename of the auto-managed key inside DataDir.
const EncryptionKeyFilename = "encryption.key"

// Encryption key source labels — kept as constants so callers (logger, doctor)
// don't depend on string spelling.
const (
	EncryptionKeySourceEnv       = "env"
	EncryptionKeySourceFile      = "file"
	EncryptionKeySourceGenerated = "generated"
)

// resolveEncryptionKey implements the resolution order:
//
//  1. ENCRYPTION_KEY env var (decoded hex, 32 bytes) — source = "env".
//  2. ${dataDir}/encryption.key exists — source = "file".
//  3. Otherwise, generate a new 32-byte key with crypto/rand, atomic-write to
//     ${dataDir}/encryption.key with mode 0600, source = "generated".
//
// dataDir is created with mode 0700 if missing. Returns the live key bytes,
// the source label, and the resolved on-disk path (empty for source=="env").
//
// envKey is the raw value of ENCRYPTION_KEY (already trimmed). When empty, we
// fall through to file/generation.
func resolveEncryptionKey(envKey, dataDir string) ([]byte, string, string, error) {
	envKey = strings.TrimSpace(envKey)
	if envKey != "" {
		key, err := hex.DecodeString(envKey)
		if err != nil {
			return nil, "", "", fmt.Errorf("ENCRYPTION_KEY: invalid hex: %w", err)
		}
		if len(key) != 32 {
			return nil, "", "", fmt.Errorf("ENCRYPTION_KEY: expected 32 bytes, got %d", len(key))
		}
		return key, EncryptionKeySourceEnv, "", nil
	}

	if dataDir == "" {
		// Without a data dir we can't auto-manage the key; return no key and
		// let callers decide what to do.
		return nil, "", "", nil
	}

	keyPath := filepath.Join(dataDir, EncryptionKeyFilename)

	// 2: existing file.
	if data, err := os.ReadFile(keyPath); err == nil {
		key, perr := parseKeyFile(data)
		if perr != nil {
			return nil, "", keyPath, fmt.Errorf("encryption key file %q: %w", keyPath, perr)
		}
		return key, EncryptionKeySourceFile, keyPath, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, "", keyPath, fmt.Errorf("read encryption key file %q: %w", keyPath, err)
	}

	// 3: generate.
	if err := ensureDataDir(dataDir); err != nil {
		return nil, "", keyPath, fmt.Errorf("ensure data dir %q: %w", dataDir, err)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, "", keyPath, fmt.Errorf("generate encryption key: %w", err)
	}

	// Persist as hex so the file is human-inspectable and matches the env-var format.
	encoded := []byte(hex.EncodeToString(key) + "\n")
	if err := atomicWriteKeyFile(keyPath, encoded); err != nil {
		return nil, "", keyPath, fmt.Errorf("write encryption key file %q: %w", keyPath, err)
	}

	return key, EncryptionKeySourceGenerated, keyPath, nil
}

// parseKeyFile accepts either a 64-char hex string (with optional trailing
// whitespace) or 32 raw bytes and returns the 32-byte key.
func parseKeyFile(data []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 64 {
		key, err := hex.DecodeString(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid hex content: %w", err)
		}
		return key, nil
	}
	if len(data) == 32 {
		out := make([]byte, 32)
		copy(out, data)
		return out, nil
	}
	return nil, fmt.Errorf("expected 64-char hex or 32 raw bytes, got %d byte(s)", len(data))
}

// ensureDataDir creates the data directory with mode 0700 if it does not exist.
// Existing directories are left as-is (we do not chmod) so operators can mount
// volumes with their own perms.
func ensureDataDir(dir string) error {
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%q exists but is not a directory", dir)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.MkdirAll(dir, 0o700)
}

// atomicWriteKeyFile writes data to a sibling tmp file (mode 0600), fsyncs it,
// then renames into place. Avoids partial writes if the process dies mid-write.
func atomicWriteKeyFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, EncryptionKeyFilename+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		// Best-effort cleanup of leftover tmp file on error paths.
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// EncryptionKeyFingerprint returns the first 8 hex chars of sha256(key). Used
// in logs and doctor output so operators can tell two installs apart without
// ever exposing the key itself.
func EncryptionKeyFingerprint(key []byte) string {
	if len(key) == 0 {
		return ""
	}
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])[:8]
}

// defaultDataDir returns the per-environment fallback when BREADBOX_DATA_DIR
// is unset. Docker installs land at /data (matching docker-compose.prod.yml);
// local installs land at ./data so test runs and `make dev` stay self-contained.
func defaultDataDir(environment string) string {
	if environment == "docker" {
		return "/data"
	}
	return filepath.Join(".", "data")
}
