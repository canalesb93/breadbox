package prompts

import "testing"

// TestParseBlockFile_YAMLFrontmatter pins the contract that prompt block
// files with YAML frontmatter ("---\ntitle: …\n---\n…") parse the title and
// description out and leave the body free of frontmatter noise. The previous
// implementation treated the first "---" delimiter as a markdown heading,
// putting the literal string "---" in the title and dropping the body into
// the wrong slot.
func TestParseBlockFile_YAMLFrontmatter(t *testing.T) {
	in := "---\ntitle: Initial Setup Strategy\ndescription: First-time setup\nicon: sparkles\n---\n\nYou are the agent.\nDo the thing.\n"
	title, desc, body := parseBlockFile(in)
	if title != "Initial Setup Strategy" {
		t.Errorf("title = %q, want %q", title, "Initial Setup Strategy")
	}
	if desc != "First-time setup" {
		t.Errorf("description = %q, want %q", desc, "First-time setup")
	}
	wantBody := "You are the agent.\nDo the thing."
	if body != wantBody {
		t.Errorf("body = %q, want %q", body, wantBody)
	}
}

func TestParseBlockFile_QuotedValues(t *testing.T) {
	in := "---\ntitle: \"Quoted Title\"\ndescription: 'single quoted'\n---\nbody"
	title, desc, _ := parseBlockFile(in)
	if title != "Quoted Title" {
		t.Errorf("title = %q, want %q", title, "Quoted Title")
	}
	if desc != "single quoted" {
		t.Errorf("description = %q, want %q", desc, "single quoted")
	}
}

func TestParseBlockFile_LegacyShape(t *testing.T) {
	// Files without frontmatter still load via the original
	// "# Title\n> description\n\nbody" convention.
	in := "# Legacy Title\n> legacy description\n\nbody line 1\nbody line 2"
	title, desc, body := parseBlockFile(in)
	if title != "Legacy Title" {
		t.Errorf("title = %q, want %q", title, "Legacy Title")
	}
	if desc != "legacy description" {
		t.Errorf("description = %q, want %q", desc, "legacy description")
	}
	if body != "body line 1\nbody line 2" {
		t.Errorf("body = %q, want %q", body, "body line 1\nbody line 2")
	}
}

func TestLoadBlock_StrategyInitialSetup(t *testing.T) {
	b, err := LoadBlock("strategy-initial-setup")
	if err != nil {
		t.Fatalf("LoadBlock: %v", err)
	}
	if b.Title != "Initial Setup Strategy" {
		t.Errorf("title = %q, want %q", b.Title, "Initial Setup Strategy")
	}
	if b.Description == "" || b.Description == "title: Initial Setup Strategy" {
		t.Errorf("description = %q (must not be empty or the YAML key)", b.Description)
	}
	if b.Content == "" {
		t.Errorf("content is empty")
	}
	// The composed content must start with the H1 derived from the title,
	// not the raw "---" frontmatter delimiter.
	wantPrefix := "# Initial Setup Strategy"
	if len(b.Content) < len(wantPrefix) || b.Content[:len(wantPrefix)] != wantPrefix {
		t.Errorf("content prefix = %q, want %q", b.Content[:min(len(b.Content), len(wantPrefix))], wantPrefix)
	}
}
