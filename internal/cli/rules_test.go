package cli

import (
	"strings"
	"testing"
)

// TestValidateRuleFile_OK asserts a well-formed rule body passes the
// client-side sanity check.
func TestValidateRuleFile_OK(t *testing.T) {
	body := []byte(`{"name":"test","conditions":{"field":"name","op":"contains","value":"x"},"actions":[{"type":"set_category","category_slug":"food"}]}`)
	if _, err := validateRuleFile(body); err != nil {
		t.Fatalf("validateRuleFile: %v", err)
	}
}

// TestValidateRuleFile_CategoryShortcut accepts the actions shortcut
// where only category_slug is supplied (server creates the implicit
// set_category action).
func TestValidateRuleFile_CategoryShortcut(t *testing.T) {
	body := []byte(`{"name":"test","conditions":{"field":"name","op":"contains","value":"x"},"category_slug":"food"}`)
	if _, err := validateRuleFile(body); err != nil {
		t.Fatalf("validateRuleFile: %v", err)
	}
}

// TestValidateRuleFile_MissingKeys produces a usage error mentioning the
// missing key — surfaced as exit code 2 to the caller.
func TestValidateRuleFile_MissingKeys(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"no name", `{"actions":[]}`, "name"},
		{"no actions or category", `{"name":"x"}`, "actions"},
		{"not an object", `[]`, "JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateRuleFile([]byte(tc.body))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q did not mention %q", err.Error(), tc.want)
			}
		})
	}
}

// TestBoolYN spot-checks the rendering helper.
func TestBoolYN(t *testing.T) {
	if got := boolYN(true); got != "yes" {
		t.Errorf("boolYN(true) = %q, want yes", got)
	}
	if got := boolYN(false); got != "no" {
		t.Errorf("boolYN(false) = %q, want no", got)
	}
}
