package pages

// AgentsSettingsProps drives the Agents tab inside the Settings shell —
// MCP server instructions, review guidelines, report-format prompt, the
// per-tool enable toggles, and the read-only connection cheatsheet.
//
// All POST targets keep their /-/mcp-settings/* paths (they predate this
// reorg). The handler in internal/admin/settings_agents.go pre-resolves
// every default vs. saved string so the templ stays free of nil-handling.
type AgentsSettingsProps struct {
	CSRFToken string

	// Server Instructions card.
	Instructions        string
	DefaultInstructions string

	// Review Guidelines card.
	ReviewGuidelines        string
	DefaultReviewGuidelines string

	// Report Format card.
	ReportFormat        string
	DefaultReportFormat string

	// Tool toggles.
	ToolGroups         []AgentsSettingsToolGroup
	ToolsEnabledCount  int
	ToolsTotalCount    int
	ToolsDisabledCount int
}

// AgentsSettingsToolGroup represents one labelled cluster of tools (e.g.
// "Transactions", "Categorization") in the tool-enable card.
type AgentsSettingsToolGroup struct {
	Name  string
	Tools []AgentsSettingsTool
}

// AgentsSettingsTool is the per-tool toggle row. Classification is "read"
// or "write" — the templ picks a soft success/warning badge accordingly.
type AgentsSettingsTool struct {
	Name           string
	Description    string
	Classification string
	Enabled        bool
}
