// Package prompts is the home for editable prompt content shipped with the
// binary. Each subdirectory groups one kind of prompt:
//
//   - agents/  — composable blocks used by the prompt builder to assemble
//     agent system prompts (see internal/prompts for the composition logic).
//   - mcp/     — defaults served by the MCP server (instructions, review
//     guidelines, report format), overridable per install via app_config.
//
// Editing a .md file here and rebuilding is the entire workflow — files are
// embedded at build time, so nothing else needs to change.
package prompts

import (
	"embed"
	"fmt"
)

//go:embed agents/*.md mcp/*.md
var FS embed.FS

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
