package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// withTempConfigDir points BREADBOX_CONFIG_DIR at t.TempDir() for the
// duration of the test so Load/Save round-trip against an isolated file.
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BREADBOX_CONFIG_DIR", dir)
	// Block XDG_CONFIG_HOME from leaking in via the user's env.
	t.Setenv("XDG_CONFIG_HOME", "")
	return dir
}

func TestLoad_MissingFileReturnsEmpty(t *testing.T) {
	withTempConfigDir(t)
	h, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(h.Hosts) != 0 {
		t.Fatalf("expected zero hosts on fresh dir, got %d", len(h.Hosts))
	}
	if h.Default != "" {
		t.Fatalf("expected empty default, got %q", h.Default)
	}
}

func TestSetSaveLoad_RoundTrip(t *testing.T) {
	dir := withTempConfigDir(t)
	h := &Hosts{Hosts: map[string]Host{}}
	if err := h.Set("local", Host{BaseURL: "http://localhost:8080", Token: "bb_secret"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if h.Default != "local" {
		t.Fatalf("first Set should bind default, got %q", h.Default)
	}
	if err := h.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file permissions (0600) — it carries tokens.
	info, err := os.Stat(filepath.Join(dir, "hosts.toml"))
	if err != nil {
		t.Fatalf("stat hosts.toml: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 perms, got %o", info.Mode().Perm())
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if got.Default != "local" {
		t.Fatalf("default not preserved: %q", got.Default)
	}
	host, name, err := got.Get("")
	if err != nil {
		t.Fatalf("Get default: %v", err)
	}
	if name != "local" {
		t.Fatalf("Get name = %q want local", name)
	}
	if host.BaseURL != "http://localhost:8080" {
		t.Fatalf("BaseURL = %q", host.BaseURL)
	}
	if host.Token != "bb_secret" {
		t.Fatalf("Token = %q", host.Token)
	}
}

func TestGet_NoHosts(t *testing.T) {
	withTempConfigDir(t)
	h, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, _, err := h.Get(""); !errors.Is(err, ErrNoHosts) {
		t.Fatalf("expected ErrNoHosts, got %v", err)
	}
}

func TestGet_HostNotFound(t *testing.T) {
	withTempConfigDir(t)
	h := &Hosts{Hosts: map[string]Host{}}
	_ = h.Set("local", Host{BaseURL: "http://localhost:8080"})
	if _, _, err := h.Get("prod"); !errors.Is(err, ErrHostNotFound) {
		t.Fatalf("expected ErrHostNotFound, got %v", err)
	}
}

func TestSetDefault_RequiresExisting(t *testing.T) {
	h := &Hosts{Hosts: map[string]Host{}}
	if err := h.SetDefault("missing"); !errors.Is(err, ErrHostNotFound) {
		t.Fatalf("expected ErrHostNotFound, got %v", err)
	}
	_ = h.Set("local", Host{BaseURL: "x"})
	_ = h.Set("prod", Host{BaseURL: "y"})
	if err := h.SetDefault("prod"); err != nil {
		t.Fatalf("SetDefault prod: %v", err)
	}
	if h.Default != "prod" {
		t.Fatalf("Default = %q", h.Default)
	}
}

func TestRemove_ReboundsDefault(t *testing.T) {
	h := &Hosts{Hosts: map[string]Host{}}
	_ = h.Set("alpha", Host{BaseURL: "x"})
	_ = h.Set("beta", Host{BaseURL: "y"})
	if err := h.SetDefault("alpha"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	if err := h.Remove("alpha"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Default rebinds to the alphabetically-first remaining host.
	if h.Default != "beta" {
		t.Fatalf("Default after remove = %q want beta", h.Default)
	}
}

func TestRemove_LastClearsDefault(t *testing.T) {
	h := &Hosts{Hosts: map[string]Host{}}
	_ = h.Set("only", Host{BaseURL: "x"})
	if err := h.Remove("only"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if h.Default != "" {
		t.Fatalf("Default not cleared: %q", h.Default)
	}
}

func TestNames_Sorted(t *testing.T) {
	h := &Hosts{Hosts: map[string]Host{
		"charlie": {BaseURL: "z"},
		"alpha":   {BaseURL: "x"},
		"bravo":   {BaseURL: "y"},
	}}
	names := h.Names()
	want := []string{"alpha", "bravo", "charlie"}
	if len(names) != 3 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("Names = %v want %v", names, want)
	}
}
