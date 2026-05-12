//go:build integration

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"breadbox/internal/service"
)

// TestDeviceCode_HappyPath exercises the full pending → approved
// lifecycle: create, pending poll, approve via service layer (skipping
// the browser), single-use token poll, and the post-consumption "no
// secret again" behavior.
func TestDeviceCode_HappyPath(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	dc, err := svc.CreateDeviceCode(ctx)
	if err != nil {
		t.Fatalf("CreateDeviceCode: %v", err)
	}
	if dc.DeviceCode == "" || len(dc.UserCode) != 8 {
		t.Fatalf("unexpected device code shape: %+v", dc)
	}
	if dc.Status != "pending" {
		t.Errorf("status = %q, want pending", dc.Status)
	}

	// Poll before approval — should report pending without leaking
	// any token material.
	pending, err := svc.PollDeviceCode(ctx, dc.DeviceCode)
	if err != nil {
		t.Fatalf("PollDeviceCode pending: %v", err)
	}
	if pending.Status != "pending" {
		t.Errorf("pending status = %q, want pending", pending.Status)
	}
	if pending.Token != "" {
		t.Errorf("pending poll leaked token: %q", pending.Token)
	}

	// Approve via the service layer (mirrors what the verification
	// page handler does on the admin's behalf).
	approved, err := svc.ApproveDeviceCode(ctx, service.ApproveDeviceCodeParams{
		UserCode:  service.FormatUserCode(dc.UserCode),
		ActorName: "test-host",
		Scope:     "read_only",
	})
	if err != nil {
		t.Fatalf("ApproveDeviceCode: %v", err)
	}
	if approved.Status != "approved" {
		t.Errorf("approved status = %q, want approved", approved.Status)
	}

	// First poll after approval — should receive the plaintext token.
	first, err := svc.PollDeviceCode(ctx, dc.DeviceCode)
	if err != nil {
		t.Fatalf("PollDeviceCode after approve: %v", err)
	}
	if first.Status != "approved" {
		t.Errorf("first.Status = %q, want approved", first.Status)
	}
	if !strings.HasPrefix(first.Token, "bb_") {
		t.Fatalf("token missing bb_ prefix: %q", first.Token)
	}

	// Replayed poll — token is already consumed; status stays
	// approved but no secret is returned.
	second, err := svc.PollDeviceCode(ctx, dc.DeviceCode)
	if err != nil {
		t.Fatalf("PollDeviceCode replay: %v", err)
	}
	if second.Status != "approved" {
		t.Errorf("second.Status = %q, want approved", second.Status)
	}
	if second.Token != "" {
		t.Errorf("replay leaked token: %q", second.Token)
	}
}

func TestDeviceCode_Denied(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	dc, err := svc.CreateDeviceCode(ctx)
	if err != nil {
		t.Fatalf("CreateDeviceCode: %v", err)
	}
	if err := svc.DenyDeviceCode(ctx, dc.UserCode, ""); err != nil {
		t.Fatalf("DenyDeviceCode: %v", err)
	}

	// A poll on a denied row maps to ErrInvalidState (which the API
	// layer renders as 400 DENIED).
	_, err = svc.PollDeviceCode(ctx, dc.DeviceCode)
	if !errors.Is(err, service.ErrInvalidState) {
		t.Fatalf("PollDeviceCode after deny: err = %v, want ErrInvalidState", err)
	}
}

func TestDeviceCode_PollUnknown(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.PollDeviceCode(ctx, "no-such-device-code-value-very-random")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("PollDeviceCode unknown: err = %v, want ErrNotFound", err)
	}
}

func TestDeviceCode_ApproveTwiceLosesRace(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	dc, err := svc.CreateDeviceCode(ctx)
	if err != nil {
		t.Fatalf("CreateDeviceCode: %v", err)
	}
	if _, err := svc.ApproveDeviceCode(ctx, service.ApproveDeviceCodeParams{
		UserCode: dc.UserCode, ActorName: "first",
	}); err != nil {
		t.Fatalf("ApproveDeviceCode first: %v", err)
	}
	// Second approve on the same row should be rejected — the row
	// is no longer pending.
	_, err = svc.ApproveDeviceCode(ctx, service.ApproveDeviceCodeParams{
		UserCode: dc.UserCode, ActorName: "second",
	})
	if !errors.Is(err, service.ErrInvalidState) {
		t.Fatalf("second approve: err = %v, want ErrInvalidState", err)
	}
}

func TestDeviceCode_NormalizationOnApproval(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	dc, err := svc.CreateDeviceCode(ctx)
	if err != nil {
		t.Fatalf("CreateDeviceCode: %v", err)
	}

	// The verification page submits the user_code with a dash; the
	// stored canonical form has none. Approval should accept either.
	formatted := service.FormatUserCode(dc.UserCode)
	if !strings.Contains(formatted, "-") {
		t.Fatalf("FormatUserCode produced no dash: %q", formatted)
	}
	if _, err := svc.ApproveDeviceCode(ctx, service.ApproveDeviceCodeParams{
		UserCode: formatted, ActorName: "dash-form",
	}); err != nil {
		t.Fatalf("ApproveDeviceCode with dash: %v", err)
	}
}
