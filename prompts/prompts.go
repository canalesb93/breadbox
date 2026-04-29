// Package prompts is the home for editable prompt content shipped with the
// binary. The layout is:
//
//   - agents.yaml — agent catalogue (slugs, labels, card metadata, block
//     composition). Edit this to add or recompose agents.
//   - agents/*.md — composable prompt blocks referenced by agents.yaml.
//   - mcp/*.md    — defaults served by the MCP server (instructions, review
//     guidelines, report format), overridable per install via app_config.
//
// Editing a file here and rebuilding is the entire workflow — everything is
// embedded at build time. The composition logic and YAML parsing live in
// internal/prompts.
package prompts

import (
	"embed"
	"fmt"
)

//go:embed agents.yaml agents/*.md mcp/*.md
var FS embed.FS

// AgentsConfig returns the embedded agents.yaml bytes.
func AgentsConfig() ([]byte, error) {
	data, err := FS.ReadFile("agents.yaml")
	if err != nil {
		return nil, fmt.Errorf("read agents.yaml: %w", err)
	}
	return data, nil
}

// Agent reads an agent block by ID (filename without extension).
func Agent(id string) ([]byte, error) {
	data, err := FS.ReadFile("agents/" + id + ".md")
	if err != nil {
		return nil, fmt.Errorf("read agent prompt %q: %w", id, err)
	}
	return data, nil
}

// MCP reads an MCP default by name (filename without extension):
// "instructions", "review-guidelines", or "report-format".
//
// Panics if the name does not exist — these files are embedded at build
// time and missing one is a programming error, not a runtime condition.
func MCP(name string) string {
	data, err := FS.ReadFile("mcp/" + name + ".md")
	if err != nil {
		panic(fmt.Sprintf("prompts: missing embedded mcp prompt %q: %v", name, err))
	}
	return string(data)
}
