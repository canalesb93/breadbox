//go:build !lite

package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"breadbox/prompts"
)

// PromptBlock is one reusable agent-prompt building block — the parsed
// view of a single `prompts/agents/*.md` file. The v2 SPA's prompt
// builder composes these into agent prompts.
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
// Icon names are kebab-case Lucide identifiers (matching the React
// `DynamicIcon` resolver). Group classifies the block by its filename
// prefix and tells the UI how to render the picker:
//
//	strategy    — top-level approach. One selected; mutually exclusive.
//	depth       — review intensity modifier. Zero or one; mutually exclusive.
//	integration — optional add-ons (gmail, sync, account-linking). Multi-select.
//	knowledge   — domain knowledge (category-system, merchants, comments). Multi-select.
type PromptBlock struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
	Group       string `json:"group"`
	Content     string `json:"content"`
}

// Block IDs that the builder excludes from the user-facing palette.
// default-system-prompt is injected by AssembleJobSpec as the SDK
// systemPrompt, not as a user-prompt building block — surfacing it in
// the builder would be confusing (it's a different field entirely).
var hiddenPromptBlockIDs = map[string]bool{
	"default-system-prompt": true,
}

// ListPromptBlocks returns every block under prompts/agents/, parsed.
// Reads from the embed.FS in `breadbox/prompts`, so the file set is
// fixed at build time — no DB call, no I/O after init. Returned slice
// is grouped + sorted within group for stable UI rendering.
//
// ctx is unused today (the embed.FS read is synchronous) but kept on
// the signature so a future DB-backed override (user-authored custom
// blocks) doesn't change the call sites.
func (s *Service) ListPromptBlocks(_ context.Context) ([]PromptBlock, error) {
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
		block := parsePromptBlock(id, string(data))
		out = append(out, block)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return promptBlockGroupOrder(out[i].Group) < promptBlockGroupOrder(out[j].Group)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// parsePromptBlock splits a `prompts/agents/*.md` file into frontmatter
// metadata and body. Format:
//
//	---
//	title: ...
//	description: ...
//	icon: ...
//	---
//
//	<body>
//
// Frontmatter values may be quoted with single or double quotes; quotes
// are stripped if present. Unknown keys are ignored. The body is the
// composed-prompt content — what the model actually reads — with
// leading whitespace trimmed so the composer concatenates cleanly.
// If the frontmatter block is missing or malformed, falls back to the
// legacy `# Title` / `> Description` convention so a stray un-migrated
// file doesn't disappear from the picker.
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
	} else {
		block.Content = body
		legacyParsePromptHeader(body, &block)
	}
	if block.Title == "" {
		block.Title = humanizeBlockID(id)
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
	// Reached EOF without a closing fence — treat as no frontmatter.
	return nil, body, false
}

// legacyParsePromptHeader keeps the pre-frontmatter `# Title` and
// `> Description` convention working as a fallback. Removable once
// every file under prompts/agents/ has been migrated to frontmatter.
func legacyParsePromptHeader(body string, block *PromptBlock) {
	for _, line := range strings.SplitN(body, "\n", 5) {
		trimmed := strings.TrimSpace(line)
		if block.Title == "" && strings.HasPrefix(trimmed, "# ") {
			block.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if block.Description == "" && strings.HasPrefix(trimmed, "> ") {
			block.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "> "))
		}
		if block.Title != "" && block.Description != "" {
			return
		}
	}
}

// promptBlockGroupFor maps a block filename to its taxonomic group.
// Anything that doesn't match a known prefix falls into "knowledge"
// as the catch-all — better to surface an unknown block than hide it.
func promptBlockGroupFor(id string) string {
	switch {
	case strings.HasPrefix(id, "strategy-"):
		return "strategy"
	case strings.HasPrefix(id, "review-depth-"):
		return "depth"
	case strings.HasSuffix(id, "-integration"),
		strings.HasSuffix(id, "-linking"),
		strings.HasSuffix(id, "-management"):
		return "integration"
	default:
		return "knowledge"
	}
}

func promptBlockGroupOrder(group string) int {
	switch group {
	case "strategy":
		return 0
	case "depth":
		return 1
	case "integration":
		return 2
	case "knowledge":
		return 3
	default:
		return 4
	}
}

// humanizeBlockID turns "strategy-routine-review" into "Strategy Routine
// Review" for the rare case where a block file is missing a title.
func humanizeBlockID(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
