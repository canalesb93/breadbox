package components

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// staticILucidePattern catches a literal `<i data-lucide="X">` placeholder.
// We then *filter* those hits against an Alpine signal — runtime
// placeholders are legitimate only when Alpine (or a `data-*` hook the
// admin JS reads) needs the `<i>` to persist across icon-name states.
// Anything else means someone hand-authored a placeholder on a fresh
// template; the SSR component `@LucideIcon` / `{{ lucide ... }}` should
// be used instead.
var staticILucidePattern = regexp.MustCompile(`<i\s+[^>]*\bdata-lucide="[a-z0-9-]+"`)

// alpineBindingMarkers is the closed set of attributes that justify a
// runtime `<i data-lucide>` placeholder. Each one means the rendered
// `<i>` is a real DOM element Alpine (or admin JS) continues to update
// after `lucide.createIcons()` swaps in the SVG — toggling visibility,
// swapping classes, or letting the JS layer find the node by attribute
// selector. Add to this list ONLY when the same affordance can't be
// expressed by re-emitting the @LucideIcon on the relevant branch.
var alpineBindingMarkers = []string{
	"x-show=",
	"x-cloak",
	"x-bind:class=",
	":class=",
	":style=",
	":data-lucide=",
	":title=",
	":aria-",
	`x-bind:data-lucide=`,
	// Custom data-attrs the admin JS reads to find icon nodes. They
	// belong with Alpine-bound cases because the JS swaps the icon
	// name at runtime by re-emitting an `<i data-lucide>` placeholder
	// inside the same anchor element.
	"data-eye",
	"data-copy-icon",
	"data-copy-done",
}

// TestNoUnboundStaticIPlaceholders walks every templ + base.html source
// and fails when a literal `<i data-lucide="X">` appears on a line that
// has no Alpine binding (and isn't inside a `<template>` Alpine clones
// at runtime). Catches the regression where someone copies an old
// snippet into new markup and bypasses `@LucideIcon`.
//
// Generated *_templ.go files are not scanned — they contain the
// rendered output and would always pass.
func TestNoUnboundStaticIPlaceholders(t *testing.T) {
	root := templatesRoot(t)

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".templ" && ext != ".html" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)

		lines := strings.Split(string(body), "\n")
		insideTemplate := false // track Alpine `<template ... >` blocks
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Track Alpine `<template>` blocks: anything inside is cloned
			// by Alpine and replays through `createIcons({ nodes: [...] })`
			// at runtime, so static placeholders inside are legit.
			if strings.Contains(line, "<template ") || strings.Contains(line, "<template>") {
				insideTemplate = true
			}
			if strings.Contains(line, "</template>") {
				insideTemplate = false
				continue
			}
			if insideTemplate {
				continue
			}

			// Skip prose mentions in comments.
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "<!--") {
				continue
			}

			if !staticILucidePattern.MatchString(line) {
				continue
			}

			// Does this `<i>` carry an Alpine / admin-JS binding that
			// makes the placeholder necessary?
			bound := false
			for _, marker := range alpineBindingMarkers {
				if strings.Contains(line, marker) {
					bound = true
					break
				}
			}
			if bound {
				continue
			}

			offenders = append(offenders, rel+":"+itoa(i+1)+": "+trimmed)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf(
			"%d static <i data-lucide=\"...\"> placeholders without an Alpine binding. "+
				"Use @LucideIcon (in-package) or @components.LucideIcon (subpackages) instead:\n  %s\n\n"+
				"If you actually need the runtime placeholder (Alpine swaps the icon name "+
				"or admin JS reads a data-* attribute on the node), add the relevant marker "+
				"to the alpineBindingMarkers list in lucide_lint_test.go.",
			len(offenders), strings.Join(offenders, "\n  "),
		)
	}
}

// templatesRoot returns the absolute path to internal/templates. The
// test package is `components`, sitting at
// internal/templates/components, so the parent is the root.
func templatesRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(wd)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
