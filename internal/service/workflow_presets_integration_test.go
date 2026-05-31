//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/service"
)

func TestEnableWorkflowFromPreset(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// All presets start "available" (not enabled).
	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}
	if len(views) < 3 {
		t.Fatalf("expected >=3 presets, got %d", len(views))
	}
	for _, v := range views {
		if v.Enabled {
			t.Fatalf("preset %q unexpectedly enabled on a fresh DB", v.Slug)
		}
	}

	// Enable the flagship — it instantiates a workflow stamped with source_template.
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	if wf.SourceTemplate == nil || *wf.SourceTemplate != "routine-reviewer" {
		t.Fatalf("source_template = %v, want routine-reviewer", wf.SourceTemplate)
	}
	if wf.Slug != "routine-reviewer" {
		t.Fatalf("slug = %q, want routine-reviewer", wf.Slug)
	}
	if wf.ToolScope != "read_write" {
		t.Fatalf("tool_scope = %q, want read_write", wf.ToolScope)
	}
	if !wf.TriggerOnSyncComplete {
		t.Fatalf("routine-reviewer should trigger on sync complete")
	}
	if wf.Enabled {
		t.Fatalf("workflow should be instantiated paused (Enabled=false)")
	}
	if len(wf.Prompt) == 0 {
		t.Fatalf("composed prompt is empty")
	}

	// The gallery now reflects it as enabled.
	views, err = svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}
	var found bool
	for _, v := range views {
		if v.Slug == "routine-reviewer" {
			found = true
			if !v.Enabled || v.WorkflowSlug == nil || *v.WorkflowSlug != "routine-reviewer" {
				t.Fatalf("routine-reviewer view not marked enabled: %+v", v)
			}
		}
	}
	if !found {
		t.Fatal("routine-reviewer not in preset views")
	}

	// One instance per preset — a second enable conflicts.
	if _, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("double-enable err = %v, want ErrConflict", err)
	}

	// Unknown preset -> not found.
	if _, err := svc.EnableWorkflowFromPreset(ctx, "no-such-preset", service.EnableWorkflowFromPresetParams{}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("unknown preset err = %v, want ErrNotFound", err)
	}
}
