//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
)

// T19ListWorkflowPresets_ReturnsRegistry verifies that ListWorkflowPresets
// returns the full preset catalog (all registered presets surfaced as views),
// and that on a fresh DB no preset is flagged as enabled.
func T19ListWorkflowPresets_ReturnsRegistry(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}

	// The registry must expose at least the five presets defined at launch.
	if len(views) < 5 {
		t.Fatalf("expected >=5 preset views, got %d", len(views))
	}

	// Every view must carry the basic metadata fields from the registry.
	for _, v := range views {
		if v.Slug == "" {
			t.Errorf("preset view has empty Slug: %+v", v)
		}
		if v.Name == "" {
			t.Errorf("preset %q has empty Name", v.Slug)
		}
		if v.Category == "" {
			t.Errorf("preset %q has empty Category", v.Slug)
		}
		if v.Icon == "" {
			t.Errorf("preset %q has empty Icon", v.Slug)
		}
		if v.Description == "" {
			t.Errorf("preset %q has empty Description", v.Slug)
		}
	}

	// On a fresh DB (nothing enabled) every view must have Enabled=false and
	// both pointer fields nil.
	for _, v := range views {
		if v.Enabled {
			t.Errorf("preset %q unexpectedly enabled on fresh DB", v.Slug)
		}
		if v.WorkflowSlug != nil {
			t.Errorf("preset %q has non-nil WorkflowSlug on fresh DB", v.Slug)
		}
		if v.WorkflowEnabled != nil {
			t.Errorf("preset %q has non-nil WorkflowEnabled on fresh DB", v.Slug)
		}
	}
}

// T19ListWorkflowPresets_EnabledPresetFlagged verifies that after enabling a
// preset, ListWorkflowPresets marks that preset as enabled and surfaces the
// instantiated workflow's slug and enabled toggle.
func T19ListWorkflowPresets_EnabledPresetFlagged(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Confirm "routine-reviewer" is not yet enabled.
	before, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets (before): %v", err)
	}
	for _, v := range before {
		if v.Slug == "routine-reviewer" && v.Enabled {
			t.Fatal("precondition: routine-reviewer already enabled on fresh DB")
		}
	}

	// Enable the preset in the paused state.
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	// Now ListWorkflowPresets must show it as enabled.
	after, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets (after): %v", err)
	}

	var found bool
	for _, v := range after {
		if v.Slug != "routine-reviewer" {
			continue
		}
		found = true

		if !v.Enabled {
			t.Error("routine-reviewer view.Enabled = false, want true")
		}
		if v.WorkflowSlug == nil {
			t.Fatal("routine-reviewer view.WorkflowSlug is nil")
		}
		if *v.WorkflowSlug != wf.Slug {
			t.Errorf("view.WorkflowSlug = %q, want %q", *v.WorkflowSlug, wf.Slug)
		}
		if v.WorkflowEnabled == nil {
			t.Fatal("routine-reviewer view.WorkflowEnabled is nil")
		}
		// We enabled it paused (Enabled=false), so WorkflowEnabled must reflect
		// the instantiated definition's own enabled toggle (false).
		if *v.WorkflowEnabled != wf.Enabled {
			t.Errorf("view.WorkflowEnabled = %v, want %v", *v.WorkflowEnabled, wf.Enabled)
		}
	}

	if !found {
		t.Fatal("routine-reviewer not present in preset views")
	}

	// Every other preset must remain NOT enabled.
	for _, v := range after {
		if v.Slug == "routine-reviewer" {
			continue
		}
		if v.Enabled {
			t.Errorf("preset %q unexpectedly enabled after enabling only routine-reviewer", v.Slug)
		}
	}
}

// T19ListWorkflowPresets_WorkflowEnabledToggle verifies that WorkflowEnabled
// tracks the instantiated workflow's own enabled toggle accurately (both true
// and false states).
func T19ListWorkflowPresets_WorkflowEnabledToggle(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Enable the preset in active state (Enabled: true).
	_, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{Enabled: true})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset (active): %v", err)
	}

	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}

	for _, v := range views {
		if v.Slug != "weekly-money-digest" {
			continue
		}
		if !v.Enabled {
			t.Error("view.Enabled = false, want true")
		}
		if v.WorkflowEnabled == nil {
			t.Fatal("view.WorkflowEnabled is nil")
		}
		if !*v.WorkflowEnabled {
			t.Error("view.WorkflowEnabled = false, want true (enabled=true was passed)")
		}
		return
	}
	t.Fatal("weekly-money-digest not found in preset views")
}

// T19ListWorkflowPresets_OptionsAreSurfaced verifies that presets which carry
// specialized options (e.g. the applyModeOption) expose those options in the
// view so callers can render a configure drawer.
func T19ListWorkflowPresets_OptionsAreSurfaced(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}

	// "routine-reviewer" is defined with the applyModeOption.
	for _, v := range views {
		if v.Slug != "routine-reviewer" {
			continue
		}
		if len(v.Options) == 0 {
			t.Fatal("routine-reviewer has no options in the view")
		}
		var foundApplyMode bool
		for _, opt := range v.Options {
			if opt.Key == "apply_mode" {
				foundApplyMode = true
				if opt.Label == "" {
					t.Error("apply_mode option has empty Label")
				}
				if len(opt.Choices) < 2 {
					t.Errorf("apply_mode option has %d choices, want >=2", len(opt.Choices))
				}
				if opt.Default == "" {
					t.Error("apply_mode option has empty Default")
				}
				// Every choice must have a non-empty Value and Label.
				for _, ch := range opt.Choices {
					if ch.Value == "" {
						t.Errorf("apply_mode choice has empty Value: %+v", ch)
					}
					if ch.Label == "" {
						t.Errorf("apply_mode choice %q has empty Label", ch.Value)
					}
				}
			}
		}
		if !foundApplyMode {
			t.Error("routine-reviewer is missing the apply_mode option")
		}
		return
	}
	t.Fatal("routine-reviewer not found in preset views")
}

// T19ListWorkflowPresets_NoOptionsPreset verifies that presets without
// configured options surface an empty (not nil) Options slice, consistent
// with the registry definition.
func T19ListWorkflowPresets_NoOptionsPreset(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}

	// "weekly-money-digest" is defined without options in the registry.
	for _, v := range views {
		if v.Slug != "weekly-money-digest" {
			continue
		}
		// Options may be nil or empty — both are acceptable; the caller must not
		// panic iterating over it. A non-zero length here would be wrong.
		if len(v.Options) != 0 {
			t.Errorf("weekly-money-digest has %d unexpected options", len(v.Options))
		}
		return
	}
	t.Fatal("weekly-money-digest not found in preset views")
}

// T19ListWorkflowPresets_OrderMatchesRegistry verifies that ListWorkflowPresets
// returns views in the same order as the underlying registry (gallery display
// order is stable, not DB-order).
func T19ListWorkflowPresets_OrderMatchesRegistry(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}

	// The first five presets in registry order are known at test time.
	want := []string{
		"routine-reviewer",
		"weekly-money-digest",
		"subscription-auditor",
		"backlog-closer",
		"monthly-close",
	}
	if len(views) < len(want) {
		t.Fatalf("expected >=%d views, got %d", len(want), len(views))
	}
	for i, slug := range want {
		if views[i].Slug != slug {
			t.Errorf("views[%d].Slug = %q, want %q (registry order)", i, views[i].Slug, slug)
		}
	}
}

// T19ListWorkflowPresets_CostEstimatesSurfaced verifies that each view carries
// a positive EstCostPerRunUSD from the registry.
func T19ListWorkflowPresets_CostEstimatesSurfaced(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}
	if len(views) == 0 {
		t.Fatal("expected at least one preset view")
	}
	for _, v := range views {
		if v.EstCostPerRunUSD <= 0 {
			t.Errorf("preset %q has non-positive EstCostPerRunUSD: %v", v.Slug, v.EstCostPerRunUSD)
		}
	}
}
