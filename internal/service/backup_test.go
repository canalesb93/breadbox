package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseDatabaseName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"postgres://user:pass@localhost:5432/breadbox?sslmode=disable", "breadbox"},
		{"postgres://user:pass@localhost/mydb", "mydb"},
		{"postgres://user:pass@localhost/", "unknown"},
		{"not-a-url", "unknown"},
	}
	for _, tt := range tests {
		got := ParseDatabaseName(tt.input)
		if got != tt.want {
			t.Errorf("ParseDatabaseName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseTriggerFromFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// New bundled format
		{"breadbox_manual_20260404_120000.tar.gz", "manual"},
		{"breadbox_scheduled_20260404_020000.tar.gz", "scheduled"},
		// Legacy single-dump format (pre-#688)
		{"breadbox_manual_20260404_120000.sql.gz", "manual"},
		{"breadbox_scheduled_20260404_020000.sql.gz", "scheduled"},
		{"random_file.sql.gz", "unknown"},
		{"breadbox_unknown_20260404.sql.gz", "unknown"},
	}
	for _, tt := range tests {
		got := parseTriggerFromFilename(tt.input)
		if got != tt.want {
			t.Errorf("parseTriggerFromFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBackupService_EnsureBackupDir(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")

	bs := NewBackupService("postgres://test:test@localhost/test", backupDir, "", slog.Default())
	if err := bs.EnsureBackupDir(); err != nil {
		t.Fatalf("EnsureBackupDir: %v", err)
	}

	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("stat backup dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("backup dir is not a directory")
	}
}

func TestBackupService_ListBackups_Empty(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	backups, err := bs.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected 0 backups, got %d", len(backups))
	}
}

func TestBackupService_ListBackups_WithFiles(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	// Create fake backup files. Mix legacy .sql.gz and new .tar.gz so we
	// confirm ListBackups recognizes both.
	for _, name := range []string{
		"breadbox_manual_20260401_120000.sql.gz",
		"breadbox_scheduled_20260402_020000.tar.gz",
		"not_a_backup.txt",
	} {
		path := filepath.Join(dir, name)
		if err := createFakeGzFile(path); err != nil {
			t.Fatalf("create fake file %s: %v", name, err)
		}
	}

	backups, err := bs.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}

	// Should be sorted newest first.
	if backups[0].Trigger != "scheduled" {
		t.Errorf("expected first backup trigger=scheduled, got %s", backups[0].Trigger)
	}
	if backups[1].Trigger != "manual" {
		t.Errorf("expected second backup trigger=manual, got %s", backups[1].Trigger)
	}
}

func TestBackupService_DeleteBackup(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	filename := "breadbox_manual_20260401_120000.sql.gz"
	path := filepath.Join(dir, filename)
	if err := createFakeGzFile(path); err != nil {
		t.Fatalf("create fake file: %v", err)
	}

	if err := bs.DeleteBackup(filename); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("backup file should have been deleted")
	}
}

func TestBackupService_DeleteBackup_NotFound(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	err := bs.DeleteBackup("nonexistent.sql.gz")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestBackupService_GetBackupPath_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	tests := []string{
		"../../../etc/passwd.sql.gz",
		"foo/bar.sql.gz",
		"..\\evil.sql.gz",
	}
	for _, name := range tests {
		_, err := bs.GetBackupPath(name)
		if err == nil {
			t.Errorf("expected error for path traversal: %s", name)
		}
	}
}

func TestBackupService_GetBackupPath_InvalidExtension(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	_, err := bs.GetBackupPath("evil.sh")
	if err == nil {
		t.Fatal("expected error for invalid extension")
	}
}

func TestBackupService_CleanupOldBackups(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	// Create a file and set its mtime to 10 days ago.
	oldFile := filepath.Join(dir, "breadbox_scheduled_20260301_020000.sql.gz")
	if err := createFakeGzFile(oldFile); err != nil {
		t.Fatalf("create fake file: %v", err)
	}
	oldTime := time.Now().AddDate(0, 0, -10)
	os.Chtimes(oldFile, oldTime, oldTime)

	// Create a recent file.
	newFile := filepath.Join(dir, "breadbox_manual_20260404_120000.sql.gz")
	if err := createFakeGzFile(newFile); err != nil {
		t.Fatalf("create fake file: %v", err)
	}

	deleted, err := bs.CleanupOldBackups(7) // Keep 7 days
	if err != nil {
		t.Fatalf("CleanupOldBackups: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	// Old file should be gone.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("old backup should have been deleted")
	}

	// New file should remain.
	if _, err := os.Stat(newFile); err != nil {
		t.Fatal("new backup should still exist")
	}
}

func TestBackupService_CleanupOldBackups_DisabledWithZero(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	deleted, err := bs.CleanupOldBackups(0)
	if err != nil {
		t.Fatalf("CleanupOldBackups: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted when disabled, got %d", deleted)
	}
}

func TestBackupService_TotalBackupSize(t *testing.T) {
	dir := t.TempDir()
	bs := NewBackupService("", dir, "", slog.Default())

	// Create two files with known content.
	for _, name := range []string{
		"breadbox_manual_20260401_120000.sql.gz",
		"breadbox_manual_20260402_120000.sql.gz",
	} {
		if err := createFakeGzFile(filepath.Join(dir, name)); err != nil {
			t.Fatalf("create fake file: %v", err)
		}
	}

	total, err := bs.TotalBackupSize()
	if err != nil {
		t.Fatalf("TotalBackupSize: %v", err)
	}
	if total <= 0 {
		t.Fatal("expected positive total size")
	}
}

func TestBackupService_CreateBackup_NoPgDump(t *testing.T) {
	// This test verifies that CreateBackup fails gracefully when pg_dump is not in PATH.
	dir := t.TempDir()
	bs := NewBackupService("postgres://test:test@localhost/test", dir, "", slog.Default())

	// Override PATH to exclude pg_dump.
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", dir) // temp dir has no executables
	defer os.Setenv("PATH", originalPath)

	_, err := bs.CreateBackup(context.Background(), "manual")
	if err == nil {
		t.Fatal("expected error when pg_dump is not available")
	}
}

// TestIsUstarHeader covers the format-sniffing branch used to distinguish a
// new .tar.gz bundle from a legacy raw .sql.gz dump on restore.
func TestIsUstarHeader(t *testing.T) {
	// Build a real tar block containing a single file. The first 512 bytes
	// of the resulting buffer should sniff as ustar.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "x", Size: 1, Mode: 0o600}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if _, err := tw.Write([]byte("y")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !isUstarHeader(buf.Bytes()) {
		t.Fatal("expected ustar magic in tar header")
	}

	if isUstarHeader([]byte("-- some sql dump\n")) {
		t.Fatal("plain SQL should not be detected as ustar")
	}
	if isUstarHeader(make([]byte, 100)) {
		t.Fatal("short buffer should not be detected as ustar")
	}
}

// TestRestoreFromBundle_WritesEncryptionKey verifies the key-restore branch:
// when the bundle carries encryption.key, restoreFromBundle writes it to the
// configured destination with mode 0600.
//
// We can't drive the SQL restore without psql on PATH, so this exercises
// writeRestoredKey directly — which is the part of the restore that's unique
// to the auto-managed-key feature.
func TestWriteRestoredKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "encryption.key")

	bs := NewBackupService("", t.TempDir(), keyPath, slog.Default())

	want := []byte(hex.EncodeToString(bytes.Repeat([]byte{0x42}, 32)) + "\n")
	if err := bs.writeRestoredKey(want); err != nil {
		t.Fatalf("writeRestoredKey: %v", err)
	}

	got, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read restored key: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("restored key bytes differ: got %x, want %x", got, want)
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("perms: got %v, want 0600", mode)
	}

	// Idempotent: a second write with identical bytes should be a no-op and
	// must not error.
	if err := bs.writeRestoredKey(want); err != nil {
		t.Fatalf("writeRestoredKey (second): %v", err)
	}
}

// TestRestoreFromBundle_ExtractsKey runs the full bundle parser using a
// fake tar.gz with a *fake* SQL payload — the SQL restore step is replaced by
// pointing at an empty database URL. We only assert on the key being written,
// which happens after a successful psql restore. To avoid needing psql, this
// test feeds restoreFromBundle directly and asserts the bundle parser extracts
// the key entry (skipped if psql is not available).
//
// In CI we want a meaningful smoke test even without psql, so we exercise the
// in-memory parser (writeBundle / parseBundle) via the public CreateBackup ->
// RestoreFromReader cycle in integration tests.
func TestBundleEncodingRoundTrip(t *testing.T) {
	// Build a bundle in memory, then parse it back manually.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	dump := []byte("-- fake dump\n")
	var dumpGzBuf bytes.Buffer
	dgw := gzip.NewWriter(&dumpGzBuf)
	if _, err := dgw.Write(dump); err != nil {
		t.Fatalf("write dump: %v", err)
	}
	if err := dgw.Close(); err != nil {
		t.Fatalf("close dump gz: %v", err)
	}
	if err := writeTarEntry(tw, bundleDumpEntry, dumpGzBuf.Bytes(), 0o600, time.Now()); err != nil {
		t.Fatalf("write dump entry: %v", err)
	}
	keyPayload := []byte("0123456789abcdef\n")
	if err := writeTarEntry(tw, bundleEncryptionKeyEntry, keyPayload, 0o600, time.Now()); err != nil {
		t.Fatalf("write key entry: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gz: %v", err)
	}

	// Parse it back.
	gzr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	tr := tar.NewReader(gzr)

	seen := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("tar read %s: %v", hdr.Name, err)
		}
		seen[hdr.Name] = data
	}

	if _, ok := seen[bundleDumpEntry]; !ok {
		t.Fatalf("bundle missing %s entry", bundleDumpEntry)
	}
	if got, ok := seen[bundleEncryptionKeyEntry]; !ok {
		t.Fatalf("bundle missing %s entry", bundleEncryptionKeyEntry)
	} else if !bytes.Equal(got, keyPayload) {
		t.Fatalf("key payload mismatch: got %q, want %q", got, keyPayload)
	}
}

// createFakeGzFile creates a small valid gzip file for testing.
func createFakeGzFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	_, err = gw.Write([]byte("-- fake SQL dump\n"))
	if err != nil {
		return err
	}
	return gw.Close()
}
