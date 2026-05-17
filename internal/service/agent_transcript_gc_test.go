//go:build !lite

package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneTranscriptFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	mkFile := func(name string, mtime time.Time) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
		return p
	}

	oldNDJSON := mkFile("aaaa1111.ndjson", now.Add(-40*24*time.Hour))
	newNDJSON := mkFile("bbbb2222.ndjson", now.Add(-1*24*time.Hour))
	oldOther := mkFile("aaaa1111.log", now.Add(-40*24*time.Hour))
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cutoff := now.AddDate(0, 0, -30)
	deleted, scanned, err := pruneTranscriptFiles(dir, cutoff)
	if err != nil {
		t.Fatalf("pruneTranscriptFiles: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (only the old .ndjson)", deleted)
	}
	if scanned != 2 {
		t.Errorf("scanned = %d, want 2 (both .ndjson files)", scanned)
	}
	if _, err := os.Stat(oldNDJSON); !os.IsNotExist(err) {
		t.Errorf("expected old .ndjson removed, stat err = %v", err)
	}
	if _, err := os.Stat(newNDJSON); err != nil {
		t.Errorf("recent .ndjson should remain, stat err = %v", err)
	}
	if _, err := os.Stat(oldOther); err != nil {
		t.Errorf("non-ndjson file should be left alone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "subdir")); err != nil {
		t.Errorf("subdirectory should be left alone, stat err = %v", err)
	}
}

func TestPruneTranscriptFiles_MissingDirIsNotAnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	deleted, scanned, err := pruneTranscriptFiles(dir, time.Now())
	if err != nil {
		t.Errorf("missing dir should not error, got %v", err)
	}
	if deleted != 0 || scanned != 0 {
		t.Errorf("expected zero counts, got deleted=%d scanned=%d", deleted, scanned)
	}
}

func TestPruneTranscriptFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	deleted, scanned, err := pruneTranscriptFiles(dir, time.Now())
	if err != nil {
		t.Errorf("empty dir should not error, got %v", err)
	}
	if deleted != 0 || scanned != 0 {
		t.Errorf("expected zero counts, got deleted=%d scanned=%d", deleted, scanned)
	}
}
