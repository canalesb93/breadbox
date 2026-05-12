//go:build integration

// Drift test: assert openapi.yaml and the live chi router agree on the
// /api/v1/* surface. Run with:
//
//	go test -tags integration -run TestOpenAPIDrift ./internal/api/...
//
// The test is integration-tagged purely for tooling consistency with the rest
// of internal/api/*_integration_test.go; it does not touch the database.
//
// Implementation note: rather than pulling in a full OpenAPI library or
// yaml.v3 as a direct dependency, we parse openapi.yaml with two simple
// regexes — one for top-level path keys, one for HTTP verb keys two-space
// indented under a path. The drift check only cares about the surface
// (which {METHOD, path} pairs exist), not response shapes, so this is
// sufficient and keeps the dependency footprint zero.
package api

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// liveRouteRegex matches r.<Method>("/path", handler) calls inside router.go
// where Method is one of the chi verbs we mount.
var liveRouteRegex = regexp.MustCompile(
	`r\.(Get|Post|Put|Patch|Delete)\(\s*"([^"]+)"`,
)

// healthVersionRoutes are the auth-free routes mounted outside /api/v1 that
// the spec still documents. The drift check stitches them into the live set
// so spec parity covers them too.
//
// `/api/v1/auth/device-code*` lives here because device-code endpoints must
// run without APIKeyAuth — there is no API key yet — so they are mounted
// at the top-level router rather than inside the `r.Route("/api/v1", ...)`
// block the regex scans.
var healthVersionRoutes = map[string]struct{}{
	"GET /health":                            {},
	"GET /health/live":                       {},
	"GET /health/ready":                      {},
	"GET /api/v1/version":                    {},
	"POST /api/v1/auth/device-code":          {},
	"POST /api/v1/auth/device-code/poll":     {},
}

// liveRoutesFromSource greps router.go for chi route declarations under
// /api/v1, plus the standalone /health* and /api/v1/version routes mounted
// at the top level. Returns a set of "METHOD /full/path" strings.
func liveRoutesFromSource(t *testing.T) map[string]struct{} {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	routerPath := filepath.Join(filepath.Dir(file), "router.go")
	body, err := os.ReadFile(routerPath)
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}

	out := make(map[string]struct{})
	for k := range healthVersionRoutes {
		out[k] = struct{}{}
	}

	// We only want routes inside the /api/v1 Route block. Slice the source
	// so the regex can't pick up admin/web/MCP routes mounted later.
	src := string(body)
	start := strings.Index(src, `r.Route("/api/v1"`)
	if start < 0 {
		t.Fatal(`could not find r.Route("/api/v1" in router.go`)
	}
	// End-of-block heuristic: the next "// MCP server" comment marks the
	// boundary between /api/v1 and the rest of the router.
	end := strings.Index(src[start:], "// MCP server")
	if end < 0 {
		end = len(src) - start
	}
	apiBlock := src[start : start+end]

	for _, m := range liveRouteRegex.FindAllStringSubmatch(apiBlock, -1) {
		method := strings.ToUpper(m[1])
		path := "/api/v1" + m[2]
		out[method+" "+path] = struct{}{}
	}
	return out
}

// pathLineRegex matches a top-level YAML path key, e.g. "  /api/v1/foo:"
// (two-space indent, value is empty/colon-terminated). Comments and other
// content lines are filtered out.
var (
	pathLineRegex = regexp.MustCompile(`^  (/[^:]*):\s*$`)
	verbLineRegex = regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
)

// specRoutes parses openapi.yaml at the repo root with simple regex (no
// YAML library). Returns a set of "METHOD /full/path" strings.
//
// The spec is hand-authored under standard 2-space indentation: paths live
// at column 2 ("  /foo:") and methods at column 4 ("    get:"). Anything
// outside that pattern is ignored.
func specRoutes(t *testing.T) map[string]struct{} {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
	specPath := filepath.Join(repoRoot, "openapi.yaml")
	body, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}

	out := make(map[string]struct{})
	inPaths := false
	currentPath := ""

	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimRight(raw, "\r")
		// Skip comment-only lines so "  # /api/v1/foo:" can't fool us.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Track when we enter the top-level "paths:" block.
		if line == "paths:" {
			inPaths = true
			continue
		}
		// Top-level keys (column 0, ending in ":") end the paths block.
		if inPaths && len(line) > 0 && line[0] != ' ' && strings.HasSuffix(line, ":") {
			inPaths = false
			continue
		}
		if !inPaths {
			continue
		}

		if m := pathLineRegex.FindStringSubmatch(line); m != nil {
			currentPath = m[1]
			continue
		}
		if currentPath == "" {
			continue
		}
		if m := verbLineRegex.FindStringSubmatch(line); m != nil {
			out[strings.ToUpper(m[1])+" "+currentPath] = struct{}{}
		}
	}
	return out
}

// TestOpenAPIDrift fails when openapi.yaml and the live router disagree.
// Adding an endpoint to router.go without a matching spec entry — or vice
// versa — fails this test. This is the single quality gate keeping the
// machine-readable contract honest.
func TestOpenAPIDrift(t *testing.T) {
	live := liveRoutesFromSource(t)
	spec := specRoutes(t)

	if len(live) == 0 {
		t.Fatal("extracted zero live routes — regex broken?")
	}
	if len(spec) == 0 {
		t.Fatal("extracted zero spec routes — openapi.yaml malformed or unreadable?")
	}

	// Endpoints intentionally not modeled in the spec. Keep this list
	// short — every entry is a documentation gap.
	specOmissions := map[string]struct{}{}

	var missingFromSpec []string
	for r := range live {
		if _, ok := spec[r]; ok {
			continue
		}
		if _, ok := specOmissions[r]; ok {
			continue
		}
		missingFromSpec = append(missingFromSpec, r)
	}

	var missingFromRouter []string
	for r := range spec {
		if _, ok := live[r]; ok {
			continue
		}
		missingFromRouter = append(missingFromRouter, r)
	}

	sort.Strings(missingFromSpec)
	sort.Strings(missingFromRouter)

	if len(missingFromSpec) > 0 {
		t.Errorf("openapi.yaml is missing %d live route(s):\n  - %s",
			len(missingFromSpec), strings.Join(missingFromSpec, "\n  - "))
	}
	if len(missingFromRouter) > 0 {
		t.Errorf("openapi.yaml documents %d route(s) that no longer exist on the router:\n  - %s",
			len(missingFromRouter), strings.Join(missingFromRouter, "\n  - "))
	}

	if t.Failed() {
		t.Logf("Live routes: %d, Spec routes: %d", len(live), len(spec))
	}
}
