package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"breadbox/internal/cli/config"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// TestConnectionsLinkRequiresUser asserts that the cobra command surfaces
// a usage-tier error when `--user` is missing — the spec calls this out
// because every hosted-link session is owner-attributed.
func TestConnectionsLinkRequiresUser(t *testing.T) {
	cmd := newConnectionsLinkCmd()
	cmd.SetArgs([]string{})
	// stash a FlagBag + a dummy client so RunE doesn't panic
	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxKeyFlags, &FlagBag{})
	ctx = context.WithValue(ctx, ctxKeyClient, client.New(config.Host{BaseURL: "http://localhost"}, "test"))
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatalf("expected usage error, got nil")
	}
	var ue *usageError
	if !errors.As(err, &ue) {
		t.Fatalf("expected *usageError, got %T", err)
	}
	if !strings.Contains(err.Error(), "--user") {
		t.Errorf("expected error to mention --user, got %q", err.Error())
	}
}

// TestWaitForHostedLinkSuccess covers the happy-path poll loop: a session
// that transitions from `active` → `completed` after a couple of polls
// should be returned cleanly.
func TestWaitForHostedLinkSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if n < 2 {
			w.Write([]byte(`{"id":"abc","short_id":"sid","status":"active","result_connection_ids":[],"expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		w.Write([]byte(`{"id":"abc","short_id":"sid","status":"completed","result_connection_ids":["conn1"],"expires_at":"2030-01-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: "k"}, "test")
	// Use a tiny poll interval via a hand-rolled poller — we can't tune
	// the const directly. Instead, override deadline so the loop runs a
	// few iterations within reason.
	final, err := waitForHostedLink(context.Background(), c, "abc", 4*time.Second)
	if err != nil {
		t.Fatalf("waitForHostedLink: %v", err)
	}
	if final.Status != "completed" {
		t.Fatalf("expected completed, got %q", final.Status)
	}
}

// TestWaitForHostedLinkTimeout asserts that a session that never reaches
// a terminal status returns ErrHostedLinkTimeout (so MapExitCode emits 4).
func TestWaitForHostedLinkTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"abc","short_id":"sid","status":"active","result_connection_ids":[],"expires_at":"2030-01-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: "k"}, "test")
	// 1ns timeout means the deadline trips on the first iteration.
	_, err := waitForHostedLink(context.Background(), c, "abc", 1)
	if !errors.Is(err, ErrHostedLinkTimeout) {
		t.Fatalf("expected ErrHostedLinkTimeout, got %v", err)
	}
	// Confirm the exit-code mapper turns this into ExitUpstream (4).
	if got := MapExitCode(err); got != ExitUpstream {
		t.Errorf("MapExitCode(timeout) = %d, want %d", got, ExitUpstream)
	}
}

// _ ensures the cobra import survives.
var _ = cobra.Command{}
