package pages

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// inlineScriptCeiling is the maximum number of content lines allowed in a
// literal inline <script>...</script> block inside a *.templ file in this
// package. Page-level Alpine factories live in static/js/admin/components/
// per docs/design-system.md → "Alpine page components"; literal inline
// scripts in templ files are reserved for trivial DOM glue (e.g. a one-shot
// lucide.createIcons()).
const inlineScriptCeiling = 5

// xDataFactoryAntiPattern catches the regression that #827 / #828 removed:
// an x-data attribute rendered from a Go expression that calls a factory
// with interpolated arguments — e.g.
//
//	x-data={ "promptBuilder(" + p.BlocksJSON + ")" }
//
// The string-literal form `x-data="factoryName"` is the convention. The
// factory takes no arguments; data flows through @templ.JSONScript or
// data-* attributes instead.
var xDataFactoryAntiPattern = regexp.MustCompile(`x-data=\{\s*"[A-Za-z_$][A-Za-z0-9_$]*\(`)

// TestNoLargeInlineScripts walks every *.templ file under
// internal/templates/components/pages and enforces two rules:
//
//  1. No `x-data={ "factory(...)"` Go-expression form (anti-pattern).
//  2. No literal <script>...</script> block exceeds inlineScriptCeiling
//     content lines (non-blank, non-pure-comment).
//
// Excluded from the line-count rule: <script src="...">, <script
// type="application/json">, and any line rendered through Go expressions
// such as `@templ.Raw("<script>" + body + "</script>")`.
func TestNoLargeInlineScripts(t *testing.T) {
	t.Run("AntiPattern_xDataFactoryArgs", func(t *testing.T) {
		entries, err := filepath.Glob("*.templ")
		if err != nil {
			t.Fatalf("glob *.templ: %v", err)
		}
		for _, path := range entries {
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("open %s: %v", path, err)
			}
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Text()
				if !xDataFactoryAntiPattern.MatchString(line) {
					continue
				}
				t.Errorf(
					"%s:%d: x-data factory-with-arguments anti-pattern detected:\n  %s\n"+
						"Use string-literal x-data=\"factoryName\" + @templ.JSONScript(...) "+
						"or data-* attributes instead. See docs/design-system.md → "+
						"\"Alpine page components\". Port the factory to "+
						"static/js/admin/components/ instead of inlining it.",
					path, lineNo, strings.TrimSpace(line),
				)
			}
			if err := scanner.Err(); err != nil {
				t.Errorf("scan %s: %v", path, err)
			}
			f.Close()
		}
	})

	t.Run("LineCount", func(t *testing.T) {
		entries, err := filepath.Glob("*.templ")
		if err != nil {
			t.Fatalf("glob *.templ: %v", err)
		}
		for _, path := range entries {
			blocks, err := findInlineScriptBlocks(path)
			if err != nil {
				t.Fatalf("scan %s: %v", path, err)
			}
			for _, b := range blocks {
				if b.contentLines > inlineScriptCeiling {
					t.Errorf(
						"%s:%d-%d: inline <script> block has %d content lines (ceiling %d). "+
							"Extract the body into static/js/admin/components/<page>.js per "+
							"docs/design-system.md → \"Alpine page components\".",
						path, b.startLine, b.endLine, b.contentLines, inlineScriptCeiling,
					)
				}
			}
		}
	})
}

type inlineScriptBlock struct {
	startLine    int
	endLine      int
	contentLines int
}

// findInlineScriptBlocks walks a templ source file and returns every literal
// <script>...</script> block (i.e. opens with a line whose lstripped form
// starts with "<script") that does not have a src= attribute and does not
// declare type="application/json".
//
// Lines starting with "//" are skipped — Go comments in templ files contain
// markup fragments like `the inline <script>` that would otherwise confuse
// the parser. Go-expression renders such as `@templ.Raw("<script>" + ...)`
// also don't match because the line starts with `@templ.Raw`, not
// `<script`.
func findInlineScriptBlocks(path string) ([]inlineScriptBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		blocks   []inlineScriptBlock
		inBlock  bool
		startLn  int
		bodyLn   int
		bodyOpen []string
	)

	scriptOpen := regexp.MustCompile(`^<script\b([^>]*)>(.*)$`)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		stripped := strings.TrimLeft(raw, " \t")

		if !inBlock {
			if !strings.HasPrefix(stripped, "<script") {
				continue
			}
			m := scriptOpen.FindStringSubmatch(stripped)
			if m == nil {
				continue
			}
			attrs := m[1]
			rest := m[2]
			if strings.Contains(attrs, "src=") {
				continue
			}
			if strings.Contains(attrs, "application/json") {
				continue
			}
			if strings.Contains(rest, "</script>") {
				// One-liner inline script — uncommon and never large; skip.
				continue
			}
			inBlock = true
			startLn = lineNo
			bodyLn = 0
			bodyOpen = bodyOpen[:0]
			_ = bodyOpen // keep linter quiet; only counts matter
			continue
		}

		if strings.Contains(raw, "</script>") {
			blocks = append(blocks, inlineScriptBlock{
				startLine:    startLn,
				endLine:      lineNo,
				contentLines: bodyLn,
			})
			inBlock = false
			continue
		}

		if isContentLine(raw) {
			bodyLn++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return blocks, nil
}

// isContentLine reports whether a line counts toward the inline-script line
// budget. Blank lines and pure single-line comments (HTML, JS line comment,
// or block-comment continuations) don't count.
func isContentLine(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "//") {
		return false
	}
	if strings.HasPrefix(s, "/*") || strings.HasPrefix(s, "*/") || strings.HasPrefix(s, "*") {
		return false
	}
	if strings.HasPrefix(s, "<!--") && strings.HasSuffix(s, "-->") {
		return false
	}
	return true
}
