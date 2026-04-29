package prompts

import (
	"testing"
)

// TestCatalogLoads guards against typos and missing block files in
// prompts/agents.yaml. The loader resolves every referenced block at
// load-time, so a successful ListAgents() implies all blocks parsed.
func TestCatalogLoads(t *testing.T) {
	sections, err := ListSections()
	if err != nil {
		t.Fatalf("ListSections: %v", err)
	}
	if len(sections) == 0 {
		t.Fatal("expected at least one section in agents.yaml")
	}
	sectionIDs := make(map[string]bool, len(sections))
	for _, s := range sections {
		if s.ID == "" || s.Title == "" {
			t.Errorf("section missing id or title: %+v", s)
		}
		sectionIDs[s.ID] = true
	}

	agents, err := ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) == 0 {
		t.Fatal("expected at least one agent in agents.yaml")
	}

	for _, a := range agents {
		if a.Slug == "" {
			t.Errorf("agent missing slug: %+v", a)
		}
		if a.Label == "" {
			t.Errorf("agent %q missing label", a.Slug)
		}
		if a.Section != "" && !sectionIDs[a.Section] {
			t.Errorf("agent %q references unknown section %q", a.Slug, a.Section)
		}
		// Blocks resolve via LoadAgentBlocks — exercises the same path the
		// builder handler uses and proves every block .md is reachable.
		blocks, err := LoadAgentBlocks(a.Slug)
		if err != nil {
			t.Errorf("LoadAgentBlocks(%q): %v", a.Slug, err)
			continue
		}
		want := len(a.Blocks.Core) + len(a.Blocks.Default) + len(a.Blocks.Optional)
		if len(blocks) != want {
			t.Errorf("agent %q: got %d blocks, want %d", a.Slug, len(blocks), want)
		}
	}
}

// TestGetAgentConfigBackcompat covers the legacy view used by the prompt
// builder handler.
func TestGetAgentConfigBackcompat(t *testing.T) {
	if _, ok := GetAgentConfig("does-not-exist"); ok {
		t.Fatal("expected GetAgentConfig to return false for unknown slug")
	}
	cfg, ok := GetAgentConfig("initial-setup")
	if !ok {
		t.Fatal("expected initial-setup to exist")
	}
	if cfg.Type != "initial-setup" {
		t.Errorf("Type: got %q, want initial-setup", cfg.Type)
	}
	if len(cfg.Core) == 0 {
		t.Errorf("expected initial-setup to have at least one core block")
	}
}

// TestLoadBlockParsesHeader checks the H1 + blockquote heuristic still
// extracts title and description from a real block file.
func TestLoadBlockParsesHeader(t *testing.T) {
	b, err := LoadBlock("strategy-initial-setup")
	if err != nil {
		t.Fatalf("LoadBlock: %v", err)
	}
	if b.Title == "" {
		t.Error("expected non-empty Title")
	}
	if b.Description == "" {
		t.Error("expected non-empty Description")
	}
	if b.Content == "" {
		t.Error("expected non-empty Content")
	}
}
