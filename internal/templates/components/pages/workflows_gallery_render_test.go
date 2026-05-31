//go:build !headless && !lite

package pages

import (
	"context"
	"strings"
	"testing"
)

// T15buildGalleryProps returns a hand-built WorkflowsGalleryProps fixture
// with two categories and three presets for render-testing the gallery.
func T15buildGalleryProps() WorkflowsGalleryProps {
	return WorkflowsGalleryProps{
		CSRFToken:           "test-csrf-token",
		ConsentAcknowledged: false,
		IsAdmin:             true,
		Status: AgentSubsystemStatusProps{
			Ready:          false,
			AuthConfigured: false,
			BinaryPresent:  false,
		},
		Spend: WorkflowSpendBanner{Show: false},
		Categories: []WorkflowCategoryProps{
			{
				Name: "Categorization & Review",
				Icon: "sparkles",
				Presets: []WorkflowPresetCardProps{
					{
						Slug:             "smart-categorizer",
						Name:             "Smart Categorizer",
						Description:      "Automatically categorizes new transactions using AI.",
						Icon:             "tag",
						TriggerLabel:     "After each sync",
						TriggerOnSync:    true,
						ScheduleCron:     "",
						EstCostPerRunUSD: 0.02,
						Enabled:          false,
					},
					{
						Slug:             "uncategorized-review",
						Name:             "Uncategorized Review",
						Description:      "Flags uncategorized transactions for manual review.",
						Icon:             "search",
						TriggerLabel:     "Weekly",
						TriggerOnSync:    false,
						ScheduleCron:     "0 7 * * 1",
						EstCostPerRunUSD: 0.05,
						Enabled:          true,
						WorkflowSlug:     "uncategorized-review",
						WorkflowEnabled:  true,
					},
				},
			},
			{
				Name: "Insights & Reports",
				Icon: "bar-chart-3",
				Presets: []WorkflowPresetCardProps{
					{
						Slug:             "monthly-summary",
						Name:             "Monthly Summary",
						Description:      "Generates a monthly spending summary report.",
						Icon:             "file-text",
						TriggerLabel:     "Monthly",
						TriggerOnSync:    false,
						ScheduleCron:     "0 8 1 * *",
						EstCostPerRunUSD: 0.10,
						Enabled:          false,
					},
				},
			},
		},
	}
}

// TestT15WorkflowsGalleryRenders asserts the WorkflowsGallery templ component
// renders without error and emits the key structural strings: preset titles,
// the "Set up" affordance for disabled presets, and category section headers.
// This is a pure templ render — no DB, no HTTP server.
func TestT15WorkflowsGalleryRenders(t *testing.T) {
	props := T15buildGalleryProps()

	var buf strings.Builder
	if err := WorkflowsGallery(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowsGallery.Render returned error: %v", err)
	}

	html := buf.String()
	t.Logf("Rendered %d bytes", buf.Len())

	// Category headers (section titles). templ HTML-escapes text, so an "&"
	// in a category name renders as "&amp;" — compare against the escaped form.
	for _, cat := range props.Categories {
		want := strings.ReplaceAll(cat.Name, "&", "&amp;")
		if !strings.Contains(html, want) {
			t.Errorf("expected category header %q (escaped %q) to appear in rendered HTML", cat.Name, want)
		}
	}

	// Preset names and descriptions.
	for _, cat := range props.Categories {
		for _, preset := range cat.Presets {
			if !strings.Contains(html, preset.Name) {
				t.Errorf("expected preset name %q to appear in rendered HTML", preset.Name)
			}
			if !strings.Contains(html, preset.Description) {
				t.Errorf("expected preset description %q to appear in rendered HTML", preset.Description)
			}
		}
	}

	// "Set up" affordance: present for unenabled presets when IsAdmin=true.
	if !strings.Contains(html, "Set up") {
		t.Error(`expected "Set up" button text for unenabled presets (IsAdmin=true)`)
	}

	// Alpine component initialization.
	if !strings.Contains(html, `x-data="workflowsGallery"`) {
		t.Error(`expected Alpine x-data="workflowsGallery" attribute`)
	}

	// Gallery JS script tag.
	if !strings.Contains(html, `src="/static/js/admin/components/workflows_gallery.js"`) {
		t.Error(`expected workflows_gallery.js script tag`)
	}

	// Tab bar links.
	if !strings.Contains(html, `href="/workflows"`) {
		t.Error(`expected Gallery tab link href="/workflows"`)
	}
	if !strings.Contains(html, `href="/workflows/runs"`) {
		t.Error(`expected Runs tab link href="/workflows/runs"`)
	}

	// Runtime-not-ready warning banner (Status.Ready=false).
	if !strings.Contains(html, "Set up the agent runtime first") {
		t.Error(`expected runtime-readiness warning when Status.Ready=false`)
	}

	// CSRF token is threaded into the data attribute.
	if !strings.Contains(html, "test-csrf-token") {
		t.Error("expected CSRF token to appear in rendered HTML (data-csrf attribute)")
	}
}

// TestT15WorkflowsGalleryGridAndTiles asserts the redesigned gallery layout:
// presets flow in a 2-up grid (single column on mobile), and each card's
// leading icon tile is gray by default but green-accented once the preset is
// set up. The T15 fixture has both an enabled and a disabled preset, so both
// tile states must appear.
func TestT15WorkflowsGalleryGridAndTiles(t *testing.T) {
	props := T15buildGalleryProps()

	var buf strings.Builder
	if err := WorkflowsGallery(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowsGallery.Render returned error: %v", err)
	}
	html := buf.String()

	// 2-up grid on desktop, single column on mobile.
	if !strings.Contains(html, "grid-cols-1") || !strings.Contains(html, "lg:grid-cols-2") {
		t.Error("expected a single-column / 2-up-desktop grid wrapper (grid-cols-1 lg:grid-cols-2)")
	}

	// Green-accented tile for the enabled preset (uncategorized-review).
	if !strings.Contains(html, "bg-success/15") {
		t.Error("expected a green-accent icon tile (bg-success/15) for the enabled preset")
	}
	// Gray tile for the disabled presets.
	if !strings.Contains(html, "bg-base-200") {
		t.Error("expected a gray icon tile (bg-base-200) for disabled presets")
	}

	// The card surfaces the preset's own icon — e.g. "tag" for Smart
	// Categorizer renders as a lucide-tag SVG.
	if !strings.Contains(html, "lucide-tag") {
		t.Error("expected the preset icon (lucide-tag) to render in its card tile")
	}

	// Preview prompt now lives inside the configure/reconfigure drawer, not
	// the card ⋯ menu. The button + its handler should be present (admin view).
	if !strings.Contains(html, "Preview prompt") || !strings.Contains(html, "previewPrompt(") {
		t.Error("expected the Preview prompt affordance inside the drawer (button + handler)")
	}
}

// TestT15WorkflowsGalleryEnabledPresetRendersToggle asserts that a preset
// marked Enabled=true renders a toggle checkbox rather than the "Set up" button.
func TestT15WorkflowsGalleryEnabledPresetRendersToggle(t *testing.T) {
	props := WorkflowsGalleryProps{
		CSRFToken:           "csrf",
		ConsentAcknowledged: true,
		IsAdmin:             true,
		Status:              AgentSubsystemStatusProps{Ready: true},
		Categories: []WorkflowCategoryProps{
			{
				Name: "Categorization & Review",
				Icon: "sparkles",
				Presets: []WorkflowPresetCardProps{
					{
						Slug:            "enabled-preset",
						Name:            "Enabled Preset",
						Description:     "A preset that has already been enabled.",
						Icon:            "check",
						TriggerLabel:    "After each sync",
						TriggerOnSync:   true,
						Enabled:         true,
						WorkflowSlug:    "enabled-preset",
						WorkflowEnabled: true,
					},
				},
			},
		},
	}

	var buf strings.Builder
	if err := WorkflowsGallery(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowsGallery.Render returned error: %v", err)
	}
	html := buf.String()

	// Enabled preset renders a toggle, not a "Set up" button.
	if !strings.Contains(html, `type="checkbox"`) {
		t.Error("expected toggle checkbox for an enabled preset")
	}
	// The toggle should reference toggleWorkflow with the workflow's slug.
	if !strings.Contains(html, "toggleWorkflow") {
		t.Error("expected toggleWorkflow JS call for enabled preset control")
	}
}

// TestT15WorkflowsGalleryNonAdminDisablesSetUp asserts that when IsAdmin=false
// the "Set up" button is rendered in a disabled state with a tooltip.
func TestT15WorkflowsGalleryNonAdminDisablesSetUp(t *testing.T) {
	props := WorkflowsGalleryProps{
		CSRFToken:           "csrf",
		ConsentAcknowledged: false,
		IsAdmin:             false, // non-admin
		Status:              AgentSubsystemStatusProps{Ready: true},
		Categories: []WorkflowCategoryProps{
			{
				Name: "Hygiene & Maintenance",
				Icon: "wrench",
				Presets: []WorkflowPresetCardProps{
					{
						Slug:          "cleanup-preset",
						Name:          "Cleanup Preset",
						Description:   "Cleans up duplicate entries.",
						Icon:          "trash-2",
						TriggerLabel:  "Weekly",
						TriggerOnSync: false,
						ScheduleCron:  "0 7 * * 1",
						Enabled:       false,
					},
				},
			},
		},
	}

	var buf strings.Builder
	if err := WorkflowsGallery(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowsGallery.Render returned error: %v", err)
	}
	html := buf.String()

	// Non-admin: disabled "Set up" button with explanatory tooltip.
	if !strings.Contains(html, "disabled") {
		t.Error("expected disabled attribute on Set up button for non-admin user")
	}
	if !strings.Contains(html, "Only admins can enable workflows") {
		t.Error("expected tooltip text 'Only admins can enable workflows' for non-admin user")
	}
	// Non-admin: no configure drawer should be rendered (drawers are admin-only).
	if strings.Contains(html, "submitDrawer") {
		t.Error("expected no configure drawer (submitDrawer) for non-admin user")
	}
	// Preview prompt lives only inside the (admin-only) drawer now, so a
	// non-admin sees no Preview prompt affordance at all.
	if strings.Contains(html, "Preview prompt") {
		t.Error("expected no Preview prompt affordance for non-admin (it lives in the admin-only drawer)")
	}
}

// TestT15WorkflowsGallerySpendBannerOver asserts the "spend ceiling reached"
// error banner renders when Spend.Over=true.
func TestT15WorkflowsGallerySpendBannerOver(t *testing.T) {
	props := WorkflowsGalleryProps{
		CSRFToken:           "csrf",
		ConsentAcknowledged: true,
		IsAdmin:             true,
		Status:              AgentSubsystemStatusProps{Ready: true},
		Spend: WorkflowSpendBanner{
			Show:       true,
			Over:       true,
			SpentStr:   "$5.00",
			CeilingStr: "$5.00",
			PctStr:     "100%",
		},
		Categories: []WorkflowCategoryProps{},
	}

	var buf strings.Builder
	if err := WorkflowsGallery(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowsGallery.Render returned error: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Workflows paused") {
		t.Error("expected 'Workflows paused' error banner when spend ceiling is reached")
	}
	if !strings.Contains(html, "$5.00") {
		t.Error("expected formatted spend amount in banner")
	}
}

// TestT15WorkflowsGallerySpendBannerApproaching asserts the "approaching
// ceiling" warning renders when Spend.Show=true but Spend.Over=false.
func TestT15WorkflowsGallerySpendBannerApproaching(t *testing.T) {
	props := WorkflowsGalleryProps{
		CSRFToken:           "csrf",
		ConsentAcknowledged: true,
		IsAdmin:             true,
		Status:              AgentSubsystemStatusProps{Ready: true},
		Spend: WorkflowSpendBanner{
			Show:       true,
			Over:       false,
			SpentStr:   "$4.10",
			CeilingStr: "$5.00",
			PctStr:     "82%",
		},
		Categories: []WorkflowCategoryProps{},
	}

	var buf strings.Builder
	if err := WorkflowsGallery(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("WorkflowsGallery.Render returned error: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Approaching spend ceiling") {
		t.Error("expected 'Approaching spend ceiling' warning banner")
	}
	if !strings.Contains(html, "82%") {
		t.Error("expected percentage used in approaching-ceiling banner")
	}
}
