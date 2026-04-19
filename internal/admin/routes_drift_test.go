package admin

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestTemplateHrefsResolveToAdminRoutes is a drift guard: every static
// `href="/..."` in the admin templates must match a GET route registered on
// the admin chi router. The bug that motivated this test (dashboard CTA
// linking to /reviews, a route that didn't exist) was silent at every layer —
// no handler, no compile error, no warning. This catches it at CI time.
//
// The test parses `router.go` via go/ast to enumerate chi route patterns
// (rather than constructing the real router, which would pull in the entire
// handler graph). Template scanning covers both html/template (`.html`) and
// templ (`.templ`) files under internal/templates/.
//
// Dynamic hrefs (containing `{{`, `{%`, or templ expression braces) are
// skipped — only literal paths are validated.
func TestTemplateHrefsResolveToAdminRoutes(t *testing.T) {
	routes := parseAdminGETRoutes(t, "router.go")
	if len(routes) == 0 {
		t.Fatal("parseAdminGETRoutes returned no routes — parser is broken")
	}
	// Prefixes served outside the admin router (main api/router.go mounts
	// /static/* for embedded assets). Listing them here keeps the test scoped
	// to admin without adding a second parser pass.
	routes = append(routes, "/static/*")

	patterns := make([]*regexp.Regexp, 0, len(routes))
	for _, r := range routes {
		patterns = append(patterns, chiPatternToRegexp(t, r))
	}

	hrefs := scanTemplateHrefs(t, "../templates")
	if len(hrefs) == 0 {
		t.Fatal("scanTemplateHrefs returned no hrefs — scanner is broken")
	}

	for _, h := range hrefs {
		if anyMatch(patterns, h.path) {
			continue
		}
		t.Errorf("%s: href %q has no matching admin GET route", h.loc, h.path)
	}
}

// hrefRef is a static href discovered in a template, with source location for
// error messages.
type hrefRef struct {
	loc  string // file:line
	path string // path-only (query + fragment stripped)
}

// parseAdminGETRoutes returns the full set of GET route patterns declared in
// the given router source. It tracks r.Route("/prefix", ...) blocks by
// pushing and popping a prefix stack.
func parseAdminGETRoutes(t *testing.T, routerFile string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, routerFile, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", routerFile, err)
	}

	var routes []string
	var prefix []string

	// recurse walks the body, pushing the Route prefix when it descends into a
	// Route block so that nested r.Get("/sub", ...) calls produce the full
	// /prefix/sub pattern.
	var recurse func(n ast.Node)
	recurse = func(n ast.Node) {
		if n == nil {
			return
		}
		switch v := n.(type) {
		case *ast.CallExpr:
			method, path, routeFn := chiMethodCall(v)
			switch {
			case method == "Route" && routeFn != nil:
				prefix = append(prefix, path)
				recurse(routeFn)
				prefix = prefix[:len(prefix)-1]
				return
			case method == "Get":
				routes = append(routes, joinPath(prefix, path))
			case method == "Mount":
				// e.g. r.Mount("/static", ...) — treat as GET-accessible prefix so
				// any href under it passes. Route to /* wildcard.
				if path != "" {
					routes = append(routes, strings.TrimRight(path, "/")+"/*")
				}
			}
		}
		// Generic walk over children.
		ast.Inspect(n, func(child ast.Node) bool {
			if child == n {
				return true
			}
			if call, ok := child.(*ast.CallExpr); ok {
				recurse(call)
				return false
			}
			return true
		})
	}
	recurse(f)
	return routes
}

// chiMethodCall returns (methodName, firstStringArg, routeFnBody) if the call
// is a chi router method registering a path. The body is non-nil only for
// Route. Returns ("", "", nil) for non-chi calls.
func chiMethodCall(call *ast.CallExpr) (string, string, ast.Node) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", nil
	}
	// Methods we care about — GET/Route/Mount register paths consumed by hrefs.
	// Post/Put/Delete register non-GET routes; skipped.
	switch sel.Sel.Name {
	case "Get", "Route", "Mount":
	default:
		return "", "", nil
	}
	if len(call.Args) < 1 {
		return "", "", nil
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", "", nil
	}
	path, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", "", nil
	}
	if sel.Sel.Name == "Route" && len(call.Args) >= 2 {
		// Second arg is a FuncLit whose body contains nested route calls.
		if fl, ok := call.Args[1].(*ast.FuncLit); ok {
			return "Route", path, fl.Body
		}
	}
	return sel.Sel.Name, path, nil
}

// joinPath merges a prefix stack with a leaf pattern. "/connections" + "/" →
// "/connections/", "/connections" + "/{id}" → "/connections/{id}".
func joinPath(prefix []string, leaf string) string {
	base := strings.Join(prefix, "")
	if leaf == "/" {
		if base == "" {
			return "/"
		}
		return base + "/"
	}
	return base + leaf
}

// chiPatternToRegexp converts a chi path pattern into an anchored regexp.
// `{id}`, `{id:[0-9]+}` → `[^/]+`. Trailing `/*` → `.*`.
func chiPatternToRegexp(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	p := pattern
	// Trailing wildcard — chi's catch-all.
	hasWild := strings.HasSuffix(p, "/*")
	if hasWild {
		p = strings.TrimSuffix(p, "/*")
	}
	// Normalize trailing slash. chi's r.Route("/x", func(r) { r.Get("/", ...) })
	// registers "/x/", but callers hit it equally as "/x" or "/x/". Treat
	// those as equivalent for matching.
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	// Escape regex metacharacters in literal segments, then substitute {param}
	// placeholders. Simpler: tokenize on `{...}` chunks.
	var sb strings.Builder
	sb.WriteString("^")
	i := 0
	for i < len(p) {
		if p[i] == '{' {
			end := strings.IndexByte(p[i:], '}')
			if end < 0 {
				t.Fatalf("malformed chi pattern %q (unclosed '{')", pattern)
			}
			sb.WriteString(`[^/]+`)
			i += end + 1
			continue
		}
		sb.WriteString(regexp.QuoteMeta(string(p[i])))
		i++
	}
	if hasWild {
		sb.WriteString(`(/.*)?`)
	}
	// Allow an optional trailing slash on exact matches — chi normalizes.
	sb.WriteString(`/?$`)
	re, err := regexp.Compile(sb.String())
	if err != nil {
		t.Fatalf("compile pattern %q -> %q: %v", pattern, sb.String(), err)
	}
	return re
}

func anyMatch(patterns []*regexp.Regexp, path string) bool {
	for _, re := range patterns {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

// hrefAttrRe captures href attribute values. Handles both `href="..."` (html)
// and `href="..."` in templ files.
var hrefAttrRe = regexp.MustCompile(`href\s*=\s*"([^"]*)"`)

// templSafeURLLiteralRe catches a common templ idiom — `templ.SafeURL("/path")`
// or `templ.SafeURL("/prefix/" + something)` — and extracts the first string
// literal so paths embedded in templ expressions aren't invisible to the test.
var templSafeURLLiteralRe = regexp.MustCompile(`templ\.SafeURL\s*\(\s*"([^"]+)"`)

// scanTemplateHrefs walks templates under root and returns every static href
// whose value begins with `/`. Dynamic hrefs (containing a template
// expression) are skipped.
func scanTemplateHrefs(t *testing.T, root string) []hrefRef {
	t.Helper()
	var refs []hrefRef
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".html" && ext != ".templ" {
			return nil
		}
		// Skip generated files — they're re-derived from .templ sources.
		if strings.HasSuffix(path, "_templ.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			matches := hrefAttrRe.FindAllStringSubmatch(line, -1)
			if ext == ".templ" {
				matches = append(matches, templSafeURLLiteralRe.FindAllStringSubmatch(line, -1)...)
			}
			for _, m := range matches {
				raw := m[1]
				if !strings.HasPrefix(raw, "/") {
					continue
				}
				// Skip dynamic values — html/template expressions.
				if strings.Contains(raw, "{{") || strings.Contains(raw, "{%") {
					continue
				}
				// Strip query + fragment.
				pathOnly := raw
				if q := strings.IndexAny(pathOnly, "?#"); q >= 0 {
					pathOnly = pathOnly[:q]
				}
				// For prefix literals like "/transactions/" + id, trim the trailing
				// slash so the match succeeds against "/transactions/{id}".
				if strings.HasSuffix(pathOnly, "/") && len(pathOnly) > 1 {
					pathOnly = pathOnly + "x" // sentinel that a {id} segment would fill
				}
				refs = append(refs, hrefRef{
					loc:  fmt.Sprintf("%s:%d", path, i+1),
					path: pathOnly,
				})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return refs
}
