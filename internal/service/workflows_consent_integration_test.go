//go:build integration && !lite

package service_test

import (
	"context"
	"testing"
)

// T9ConsentInitiallyFalse verifies that on a fresh household (empty app_config
// table) WorkflowsConsentAcknowledged returns false -- the safe path.
func TestT9ConsentInitiallyFalse(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if svc.WorkflowsConsentAcknowledged(ctx) {
		t.Fatal("T9: fresh household must not have acknowledged consent")
	}
}

// T9ConsentAfterAckTrue verifies that after calling AcknowledgeWorkflowsConsent
// the gate flips to true.
func TestT9ConsentAfterAckTrue(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Precondition: not acknowledged.
	if svc.WorkflowsConsentAcknowledged(ctx) {
		t.Fatal("T9: precondition failed: consent already acknowledged on fresh DB")
	}

	// Acknowledge.
	if err := svc.AcknowledgeWorkflowsConsent(ctx); err != nil {
		t.Fatalf("T9: AcknowledgeWorkflowsConsent: %v", err)
	}

	// Postcondition: acknowledged.
	if !svc.WorkflowsConsentAcknowledged(ctx) {
		t.Fatal("T9: consent must be acknowledged after AcknowledgeWorkflowsConsent")
	}
}

// T9ConsentIdempotent verifies that calling AcknowledgeWorkflowsConsent a
// second time does not error and the gate remains true.
func TestT9ConsentIdempotent(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// First acknowledgement.
	if err := svc.AcknowledgeWorkflowsConsent(ctx); err != nil {
		t.Fatalf("T9: first AcknowledgeWorkflowsConsent: %v", err)
	}
	if !svc.WorkflowsConsentAcknowledged(ctx) {
		t.Fatal("T9: consent not acknowledged after first call")
	}

	// Second acknowledgement -- must not error (idempotent).
	if err := svc.AcknowledgeWorkflowsConsent(ctx); err != nil {
		t.Fatalf("T9: second AcknowledgeWorkflowsConsent must be idempotent, got: %v", err)
	}

	// Gate must still be true.
	if !svc.WorkflowsConsentAcknowledged(ctx) {
		t.Fatal("T9: consent must remain acknowledged after idempotent second call")
	}
}
