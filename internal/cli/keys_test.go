package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// TestKeysCreate_FlagValidation covers the unit-level guardrails on
// `breadbox keys create`: scope, actor, and --name presence. These run
// before any HTTP call so they can be exercised without a server.
func TestKeysCreate_FlagValidation(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		wantIn string
	}{
		{"missing name", []string{"keys", "create"}, "--name"},
		{"bad scope", []string{"keys", "create", "--name", "x", "--scope", "weird"}, "--scope must be one of"},
		{"bad actor", []string{"keys", "create", "--name", "x", "--actor", "robot"}, "--actor must be one of"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newTestRoot(t)
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantIn)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("expected error containing %q, got %q", tc.wantIn, err.Error())
			}
			if got := MapExitCode(err); got != ExitUsage {
				t.Fatalf("MapExitCode = %d, want %d", got, ExitUsage)
			}
		})
	}
}

// TestLoginsCreate_FlagValidation: --user, --email, and --role are all
// required (or strictly validated when present).
func TestLoginsCreate_FlagValidation(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		wantIn string
	}{
		{"missing user", []string{"logins", "create", "--email", "alice@example.com"}, "--user"},
		{"missing email", []string{"logins", "create", "--user", "abc"}, "--email"},
		{"bad role", []string{"logins", "create", "--user", "abc", "--email", "a@b.c", "--role", "wizard"}, "--role"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newTestRoot(t)
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantIn)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("expected error containing %q, got %q", tc.wantIn, err.Error())
			}
			if got := MapExitCode(err); got != ExitUsage {
				t.Fatalf("MapExitCode = %d, want %d", got, ExitUsage)
			}
		})
	}
}

// TestLoginsUpdate_FlagValidation: --role is required and must be valid.
func TestLoginsUpdate_FlagValidation(t *testing.T) {
	root := newTestRoot(t)
	root.SetArgs([]string{"logins", "update", "abc"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--role is required") {
		t.Fatalf("missing --role: got %v", err)
	}

	root = newTestRoot(t)
	root.SetArgs([]string{"logins", "update", "abc", "--role", "wizard"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--role must be one of") {
		t.Fatalf("bad --role: got %v", err)
	}
}

// TestUsersUpdate_NoChanges: PATCH with no fields is a usage error so
// agents don't accidentally PATCH a no-op.
func TestUsersUpdate_NoChanges(t *testing.T) {
	root := newTestRoot(t)
	root.SetArgs([]string{"users", "update", "abc"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("expected at-least-one error, got %v", err)
	}
	if got := MapExitCode(err); got != ExitUsage {
		t.Fatalf("MapExitCode = %d, want %d", got, ExitUsage)
	}
}

// TestUsersCreate_MissingName: --name is required.
func TestUsersCreate_MissingName(t *testing.T) {
	root := newTestRoot(t)
	root.SetArgs([]string{"users", "create"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("expected --name required error, got %v", err)
	}
}

// TestMapExitCode_APIErrorStatuses pins the auth / validation / upstream
// fork in MapExitCode. The flag-validation tests above lean on this.
func TestMapExitCode_APIErrorStatuses(t *testing.T) {
	cases := []struct {
		status int
		want   int
	}{
		{http.StatusUnauthorized, ExitAuth},
		{http.StatusForbidden, ExitAuth},
		{http.StatusBadRequest, ExitValidation},
		{http.StatusNotFound, ExitValidation},
		{http.StatusConflict, ExitValidation},
		{http.StatusInternalServerError, ExitUpstream},
		{http.StatusBadGateway, ExitUpstream},
	}
	for _, tc := range cases {
		err := &client.APIError{Status: tc.status, Code: "X", Message: "y"}
		if got := MapExitCode(err); got != tc.want {
			t.Errorf("status %d: MapExitCode = %d, want %d", tc.status, got, tc.want)
		}
	}
	if MapExitCode(nil) != ExitOK {
		t.Errorf("nil -> ExitOK")
	}
	if MapExitCode(errors.New("anything")) != ExitRuntime {
		t.Errorf("unknown -> ExitRuntime")
	}
}

// newTestRoot builds a root command suitable for unit tests of flag
// validation. The usage subtrees (users, logins, keys) require a host in
// production; we strip the requires-host annotation so RunE actually runs
// and the flag-validation code path is exercised.
func newTestRoot(t *testing.T) *cobra.Command {
	t.Helper()
	r := NewRootCmd("test")
	r.SetContext(context.Background())
	// Silence the help banner cobra would otherwise print on argument
	// failures so test output stays signal-only.
	r.SilenceUsage = true
	r.SilenceErrors = true
	out := &bytes.Buffer{}
	r.SetOut(out)
	r.SetErr(out)
	for _, name := range []string{"users", "logins", "keys"} {
		sub, _, err := r.Find([]string{name})
		if err == nil && sub != nil && sub.Annotations != nil {
			delete(sub.Annotations, annotRequiresHost)
		}
	}
	return r
}
