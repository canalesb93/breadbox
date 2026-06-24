//go:build !lite

package service

import (
	"fmt"
	"strings"
	"sync"

	"breadbox/prompts"
)

// PromptBlock is one reusable agent-prompt building block — the parsed
// view of a single `prompts/agents/*.md` file, stripped of its YAML
// frontmatter. composePresetPrompt concatenates the blocks named by a
// workflow preset into that preset's base prompt.
//
// On-disk format is YAML frontmatter + markdown body:
//
//	---
//	title: Routine Review Strategy
//	description: Daily or weekly review of recent transactions
//	icon: calendar-check
//	---
//
//	<body markdown — sent to the model verbatim>
//
// Only the body survives into Content; the frontmatter is metadata for
// humans editing the files and is dropped at load time.
type PromptBlock struct {
	ID      string
	Content string
}

// Block IDs excluded from the composable library. default-system-prompt
// is injected by AssembleJobSpec as the SDK systemPrompt, not composed
// into a preset's user prompt — loading it as a block would let a preset
// accidentally reference it as one.
var hiddenPromptBlockIDs = map[string]bool{
	"default-system-prompt": true,
}

// loadPromptBlocks reads + parses every file under prompts/agents/
// once. The embed.FS contents are immutable at runtime, so the result
// is cached forever via sync.OnceValues.
var loadPromptBlocks = sync.OnceValues(func() ([]PromptBlock, error) {
	entries, err := prompts.FS.ReadDir("agents")
	if err != nil {
		return nil, fmt.Errorf("read prompts/agents: %w", err)
	}
	out := make([]PromptBlock, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		id := strings.TrimSuffix(name, ".md")
		if hiddenPromptBlockIDs[id] {
			continue
		}
		data, err := prompts.FS.ReadFile("agents/" + name)
		if err != nil {
			return nil, fmt.Errorf("read prompts/agents/%s: %w", name, err)
		}
		out = append(out, parsePromptBlock(id, string(data)))
	}
	return out, nil
})

// parsePromptBlock splits a `prompts/agents/*.md` file into frontmatter
// (discarded) and body (kept as Content).
func parsePromptBlock(id, body string) PromptBlock {
	content := body
	if _, rest, ok := parsePromptFrontmatter(body); ok {
		content = strings.TrimLeft(rest, "\n")
	}
	// Markdown sources usually end with one trailing newline (some
	// with two). Strip them so the composed prompt joins cleanly.
	content = strings.TrimRight(content, " \t\r\n")
	return PromptBlock{ID: id, Content: content}
}

// parsePromptFrontmatter extracts the leading YAML-ish frontmatter
// block from `body`. We deliberately don't import a YAML library — the
// shape is fixed (`key: value` per line, optional surrounding quotes)
// and a short scanner is easier to audit. Returns the parsed map, the
// post-frontmatter content, and whether a frontmatter block was
// actually present.
func parsePromptFrontmatter(body string) (map[string]string, string, bool) {
	const fence = "---"
	lines := strings.Split(body, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != fence {
		return nil, body, false
	}
	meta := map[string]string{}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == fence {
			return meta, strings.Join(lines[i+1:], "\n"), true
		}
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		sep := strings.IndexByte(line, ':')
		if sep <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:sep])
		val := strings.TrimSpace(line[sep+1:])
		// Strip a single matching pair of wrapping quotes — single OR double.
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		meta[key] = val
	}
	return nil, body, false
}
