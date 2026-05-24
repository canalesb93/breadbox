//go:build !lite

package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"breadbox/internal/slugs"
	"breadbox/prompts"
)

// PromptBlockGroup is the taxonomy classification used by the admin
// prompt-builder picker.
type PromptBlockGroup string

const (
	// strategy — top-level approach. One selected; mutually exclusive.
	GroupStrategy PromptBlockGroup = "strategy"
	// depth — review intensity modifier. Zero or one; mutually exclusive.
	GroupDepth PromptBlockGroup = "depth"
	// integration — optional add-ons (gmail, sync, account-linking). Multi-select.
	GroupIntegration PromptBlockGroup = "integration"
	// knowledge — domain knowledge (categories, merchants, comments). Multi-select.
	GroupKnowledge PromptBlockGroup = "knowledge"
)

// PromptBlock is one reusable agent-prompt building block — the parsed
// view of a single `prompts/agents/*.md` file. The admin prompt builder
// composes these into agent prompts.
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
// Icon names are kebab-case Lucide identifiers.
type PromptBlock struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Icon        string           `json:"icon,omitempty"`
	Group       PromptBlockGroup `json:"group"`
	Content     string           `json:"content"`
}

// Block IDs that the builder excludes from the user-facing palette.
// default-system-prompt is injected by AssembleJobSpec as the SDK
// systemPrompt, not as a user-prompt building block — surfacing it in
// the builder would be confusing (it's a different field entirely).
var hiddenPromptBlockIDs = map[string]bool{
	"default-system-prompt": true,
}

// loadPromptBlocks reads + parses every file under prompts/agents/
// once. The embed.FS contents are immutable at runtime, so the result
// is cached forever via sync.OnceValues — the admin UI hits this on
// every page render, no reason to re-walk the FS each time.
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
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return promptBlockGroupOrder(out[i].Group) < promptBlockGroupOrder(out[j].Group)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
})

// ListPromptBlocks returns the parsed library. ctx is unused today
// (the embed.FS read is synchronous and cached) but kept on the
// signature so a future DB-backed override (user-authored custom
// blocks) doesn't change call sites.
func (s *Service) ListPromptBlocks(_ context.Context) ([]PromptBlock, error) {
	return loadPromptBlocks()
}

// parsePromptBlock splits a `prompts/agents/*.md` file into frontmatter
// metadata and body. Format is documented on PromptBlock.
func parsePromptBlock(id, body string) PromptBlock {
	block := PromptBlock{
		ID:    id,
		Group: promptBlockGroupFor(id),
	}
	if meta, content, ok := parsePromptFrontmatter(body); ok {
		block.Title = meta["title"]
		block.Description = meta["description"]
		block.Icon = meta["icon"]
		block.Content = strings.TrimLeft(content, "\n")
	}
	// Markdown sources usually end with one trailing newline (some
	// with two). Strip them so the expansion editor doesn't surface
	// phantom blank lines and the composed prompt joins cleanly.
	block.Content = strings.TrimRight(block.Content, " \t\r\n")
	if block.Title == "" {
		block.Title = slugs.TitleCase(id)
	}
	return block
}

// parsePromptFrontmatter extracts the leading YAML-ish frontmatter
// block from `body`. We deliberately don't import a YAML library — the
// shape is fixed (`key: value` per line, optional surrounding quotes)
// and a 30-line scanner is easier to audit. Returns the parsed map,
// the post-frontmatter content, and whether a frontmatter block was
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

// promptBlockGroupFor infers the group from the block's filename. A
// future migration could move this into frontmatter, but today every
// block file follows one of these prefix/suffix conventions.
func promptBlockGroupFor(id string) PromptBlockGroup {
	switch {
	case strings.HasPrefix(id, "strategy-"):
		return GroupStrategy
	case strings.HasPrefix(id, "review-depth-"):
		return GroupDepth
	case strings.HasSuffix(id, "-integration"),
		strings.HasSuffix(id, "-linking"),
		strings.HasSuffix(id, "-management"):
		return GroupIntegration
	default:
		return GroupKnowledge
	}
}

func promptBlockGroupOrder(group PromptBlockGroup) int {
	switch group {
	case GroupStrategy:
		return 0
	case GroupDepth:
		return 1
	case GroupIntegration:
		return 2
	case GroupKnowledge:
		return 3
	default:
		return 4
	}
}
