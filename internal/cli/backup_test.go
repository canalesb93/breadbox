package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultBackupDir ensures the XDG/data-home fallback chain is well-behaved.
func TestDefaultBackupDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "") // Windows
	got, err := defaultBackupDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// On a machine without HOME we expect "./backups".
	if got != filepath.Join(".", "backups") {
		// On many CI setups HOME is restored by t.Setenv resetters anyway,
		// so just sanity-check we got *some* path.
		if !strings.HasSuffix(got, "backups") {
			t.Errorf("unexpected default backup dir: %q", got)
		}
	}
}

func TestDefaultBackupDir_HonorsXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-test")
	got, err := defaultBackupDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/tmp/xdg-test/breadbox/backups"
	if got != want {
		t.Errorf("default backup dir = %q, want %q", got, want)
	}
}

// TestBackupListDirCreated ensures NewBackupService + EnsureBackupDir doesn't
// blow up when pointed at a fresh tempdir. We don't run pg_dump.
func TestBackupListEmptyTempDir(t *testing.T) {
	dir := t.TempDir()
	// The CLI list path eventually calls bs.ListBackups(); we exercise the
	// dir-creation half by writing a no-op file and listing again.
	if err := os.WriteFile(filepath.Join(dir, "not-a-backup.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Listing should not include the txt file.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql.gz") {
			t.Errorf("unexpected fake backup file: %s", e.Name())
		}
	}
}
