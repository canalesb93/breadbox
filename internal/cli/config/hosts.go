// Package config implements the CLI-side host configuration store.
//
// `breadbox auth login` writes to `~/.config/breadbox/hosts.toml`; every other
// command reads from it via Load() and resolves a single Host through
// (*Hosts).Get(name). The file lives outside the repo and never ships with
// the binary — it's purely user-state on the machine running the CLI.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// Hosts is the in-memory shape of hosts.toml.
type Hosts struct {
	Default string          `toml:"default"`
	Hosts   map[string]Host `toml:"hosts"`
}

// Host carries the credentials and base URL for one breadbox instance.
type Host struct {
	BaseURL string `toml:"base_url"`
	Token   string `toml:"token,omitempty"`
	Socket  string `toml:"socket,omitempty"` // optional unix socket override
}

// ErrNoHosts is returned by Get when the config has no hosts configured at
// all. CLI callers can branch on this to surface a helpful "run
// `breadbox auth login` first" message instead of a generic not-found.
var ErrNoHosts = errors.New("no hosts configured")

// ErrHostNotFound is returned by Get when the named host is not in the
// config. The CLI's PersistentPreRunE distinguishes this from ErrNoHosts so
// it can offer different remediation.
var ErrHostNotFound = errors.New("host not found")

// configDir resolves the directory hosts.toml lives in. Order:
//  1. BREADBOX_CONFIG_DIR (explicit override; used by tests and packaged installs)
//  2. XDG_CONFIG_HOME/breadbox
//  3. ~/.config/breadbox
func configDir() (string, error) {
	if d := os.Getenv("BREADBOX_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "breadbox"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, ".config", "breadbox"), nil
}

// Path returns the absolute path to the hosts.toml file (creating the
// containing directory is the caller's responsibility — Save() does it).
func Path() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "hosts.toml"), nil
}

// Load reads hosts.toml from disk. A missing file returns a zero-valued
// *Hosts with no error — first-run callers can immediately .Set() and
// .Save() without a stat dance.
func Load() (*Hosts, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	h := &Hosts{Hosts: map[string]Host{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return nil, fmt.Errorf("read hosts.toml: %w", err)
	}
	if err := toml.Unmarshal(data, h); err != nil {
		return nil, fmt.Errorf("parse hosts.toml: %w", err)
	}
	if h.Hosts == nil {
		h.Hosts = map[string]Host{}
	}
	return h, nil
}

// Save writes hosts.toml back to disk at 0600 (it carries tokens) with the
// containing directory created if missing.
func (h *Hosts) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := toml.Marshal(h)
	if err != nil {
		return fmt.Errorf("encode hosts.toml: %w", err)
	}
	// Write to a temp file then rename, so a half-written file never
	// replaces the existing one on crash. 0600 perms throughout.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write hosts.toml.tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename hosts.toml: %w", err)
	}
	return nil
}

// Get returns the named host. Empty name resolves to the default. Returns
// ErrNoHosts if the config has zero hosts; ErrHostNotFound otherwise.
func (h *Hosts) Get(name string) (Host, string, error) {
	if len(h.Hosts) == 0 {
		return Host{}, "", ErrNoHosts
	}
	if name == "" {
		name = h.Default
	}
	if name == "" {
		// No default set but hosts exist — pick the alphabetically-first
		// host as a deterministic fallback. CLI surfaces a warning.
		names := h.Names()
		name = names[0]
	}
	host, ok := h.Hosts[name]
	if !ok {
		return Host{}, "", ErrHostNotFound
	}
	return host, name, nil
}

// Set adds or replaces a host entry. If the config was empty, the new host
// becomes the default too (so `breadbox auth login` on a fresh machine
// doesn't leave the user without a default).
func (h *Hosts) Set(name string, host Host) error {
	if name == "" {
		return errors.New("host name is required")
	}
	if h.Hosts == nil {
		h.Hosts = map[string]Host{}
	}
	wasEmpty := len(h.Hosts) == 0
	h.Hosts[name] = host
	if wasEmpty || h.Default == "" {
		h.Default = name
	}
	return nil
}

// SetDefault swaps the default-host pointer. The named host must already
// exist or ErrHostNotFound is returned.
func (h *Hosts) SetDefault(name string) error {
	if _, ok := h.Hosts[name]; !ok {
		return ErrHostNotFound
	}
	h.Default = name
	return nil
}

// Remove drops a host entry. If the removed host was the default, the
// default is cleared (or rebound to the alphabetically-first remaining
// host, if any).
func (h *Hosts) Remove(name string) error {
	if _, ok := h.Hosts[name]; !ok {
		return ErrHostNotFound
	}
	delete(h.Hosts, name)
	if h.Default == name {
		h.Default = ""
		names := h.Names()
		if len(names) > 0 {
			h.Default = names[0]
		}
	}
	return nil
}

// Names returns host names in alphabetical order, deterministic for CLI
// output and tests.
func (h *Hosts) Names() []string {
	out := make([]string, 0, len(h.Hosts))
	for n := range h.Hosts {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
