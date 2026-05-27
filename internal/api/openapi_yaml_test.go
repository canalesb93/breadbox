//go:build !lite

// Spec sanity check: parse openapi.yaml as YAML and fail loudly on syntax
// errors or duplicate keys. Catches bugs that the regex-based drift test
// in openapi_drift_test.go cannot see — duplicate `paths` entries, malformed
// scalars, embedded `: ` in unquoted descriptions, indentation breaks.
//
// Untagged with `integration` so it runs in the default `go test ./...`
// pass and not just the integration matrix.
package api

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIYAMLValid(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	specPath := filepath.Join(filepath.Dir(file), "..", "..", "openapi.yaml")
	body, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}

	// Parse as a Node tree so we can walk the AST and detect duplicate
	// mapping keys — yaml.v3's Unmarshal into map[string]any silently
	// overwrites duplicates, which is exactly the failure mode Mintlify's
	// strict parser rejected. Node-mode preserves both entries.
	var root yaml.Node
	if err := yaml.Unmarshal(body, &root); err != nil {
		t.Fatalf("openapi.yaml failed to parse as YAML: %v", err)
	}

	if dup := findDuplicateKey(&root); dup != "" {
		t.Fatalf("openapi.yaml has duplicate mapping key: %s", dup)
	}
}

// findDuplicateKey walks the YAML node tree and returns a "key (line:col)"
// description of the first duplicate mapping key it finds, or empty string
// if none.
func findDuplicateKey(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	if n.Kind == yaml.MappingNode {
		seen := make(map[string]int, len(n.Content)/2)
		for i := 0; i < len(n.Content)-1; i += 2 {
			key := n.Content[i]
			if prev, dup := seen[key.Value]; dup {
				return fmt.Sprintf("%q first at line %d, again at line %d", key.Value, prev, key.Line)
			}
			seen[key.Value] = key.Line
		}
	}
	for _, child := range n.Content {
		if dup := findDuplicateKey(child); dup != "" {
			return dup
		}
	}
	return ""
}
