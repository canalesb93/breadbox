//go:build !headless && !lite

package components

import (
	"context"
	"html/template"
	"strings"
	"testing"
)

// Every output of renderLucideIcon must carry these invariants — both
// load-bearing for downstream CSS and tests:
//   - `lucide` class: `svg.lucide` rules in input.css strip alpha on
//     overlapping strokes and neutralise pointer-events on icons.
//   - `lucide-{name}` class: matches what lucide.createIcons() stamps,
//     so feature tests can grep for `lucide-home` etc.
//   - `aria-hidden="true"`: icons are decorative; their accessible name
//     lives on the wrapping button/link.
func TestRenderLucideIcon_AlwaysOnInvariants(t *testing.T) {
	cases := []struct {
		name   string
		icon   string
		class  string
		style  string
		expect []string
	}{
		{
			name:   "static name, single class",
			icon:   "home",
			class:  "w-4 h-4",
			expect: []string{`class="lucide lucide-home w-4 h-4"`, `aria-hidden="true"`},
		},
		{
			name:   "kebab-case name",
			icon:   "alert-triangle",
			class:  "w-3.5 h-3.5 text-warning",
			expect: []string{`class="lucide lucide-alert-triangle w-3.5 h-3.5 text-warning"`},
		},
		{
			name:   "empty class still emits lucide + lucide-name",
			icon:   "search",
			class:  "",
			expect: []string{`class="lucide lucide-search"`, `aria-hidden="true"`},
		},
		{
			name:   "style attribute injected before class",
			icon:   "folder",
			class:  "w-4 h-4",
			style:  "color: #ff0000",
			expect: []string{`style="color: #ff0000" class="lucide lucide-folder w-4 h-4"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderLucideIcon(tc.icon, tc.class, tc.style)
			if !strings.HasPrefix(got, "<svg ") {
				t.Fatalf("expected output to start with <svg, got: %q", got[:min(60, len(got))])
			}
			if !strings.HasSuffix(got, "</svg>") {
				t.Fatalf("expected output to end with </svg>, got: ...%q", got[max(0, len(got)-60):])
			}
			for _, want := range tc.expect {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in output:\n%s", want, got)
				}
			}
		})
	}
}

// Unknown icon names return an empty string from lucide-go (the upstream
// library's safety net). Better than rendering a broken `<svg></svg>` —
// the calling page gets nothing, which is louder during review than an
// invisible placeholder. Lock the behavior in so a future lucide-go
// upgrade can't silently change it.
func TestRenderLucideIcon_UnknownNameRendersEmpty(t *testing.T) {
	got := renderLucideIcon("definitely-not-a-real-icon-xyz", "w-4 h-4", "")
	if got != "" {
		t.Errorf("expected empty string for unknown icon, got: %q", got)
	}
}

// LucideFuncMap exposes the same renderLucideIcon under the `lucide`
// template-func name. Verifies the html/template path matches the templ
// path byte-for-byte so a partial migration can never drift between the
// two surfaces.
func TestLucideFuncMap_MatchesTemplPath(t *testing.T) {
	fm := LucideFuncMap()
	fn, ok := fm["lucide"].(func(name, class string) template.HTML)
	if !ok {
		t.Fatalf("expected func(string, string) template.HTML, got %T", fm["lucide"])
	}
	got := string(fn("home", "w-4 h-4"))
	want := renderLucideIcon("home", "w-4 h-4", "")
	if got != want {
		t.Errorf("html/template func and templ-shared renderer drifted\n  got: %s\n  want: %s", got, want)
	}
}

// Smoke-test the templ Component end-to-end: it should produce the same
// bytes as renderLucideIcon. Catches accidental wrapping changes in the
// generated lucide_templ.go that would otherwise sneak through.
func TestLucideIconTemplComponent(t *testing.T) {
	var sb strings.Builder
	if err := LucideIcon("home", "w-4 h-4").Render(context.Background(), &sb); err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := renderLucideIcon("home", "w-4 h-4", "")
	if sb.String() != want {
		t.Errorf("templ output != shared renderer\n  got: %s\n  want: %s", sb.String(), want)
	}
}

func TestLucideIconWithStyleTemplComponent(t *testing.T) {
	var sb strings.Builder
	err := LucideIconWithStyle("folder", "w-3.5 h-3.5", "color: #abcdef").
		Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, `style="color: #abcdef"`) {
		t.Errorf("expected style attribute in output: %s", out)
	}
	if !strings.Contains(out, `class="lucide lucide-folder w-3.5 h-3.5"`) {
		t.Errorf("expected merged class attribute in output: %s", out)
	}
	// `style` must appear before `class` — that's where renderLucideIcon
	// splices it. Locking the order lets future readers grep for the
	// canonical shape.
	styleIdx := strings.Index(out, "style=")
	classIdx := strings.Index(out, "class=")
	if styleIdx == -1 || classIdx == -1 || styleIdx >= classIdx {
		t.Errorf("expected style before class, got: %s", out)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
