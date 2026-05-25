//go:build !headless && !lite

package pages

// MCPGuideProps powers the standalone "Getting Started" guide page (renders
// instructions for connecting an external MCP client to Breadbox).
type MCPGuideProps struct {
	MCPServerURL    string
	HasAPIKeys      bool
	HasOAuthClients bool
}

// AgentWizardStatsProps mirrors AgentWizardStats for the templ side.
type AgentWizardStatsProps struct {
	PendingReviews int64
	TotalRules     int64
	TotalAccounts  int64
}

// AgentWizardProps powers the /agent-prompts prompt-library page.
type AgentWizardProps struct {
	Stats AgentWizardStatsProps
}

// MCPSettingsToolInfo is one tool row in the Tools Enabled card.
type MCPSettingsToolInfo struct {
	Name           string
	Description    string
	Classification string // "read" | "write"
	Enabled        bool
}

// MCPSettingsToolGroup groups MCPSettingsToolInfo rows under a heading.
type MCPSettingsToolGroup struct {
	Name  string
	Tools []MCPSettingsToolInfo
}

// MCPSettingsProps powers the MCP Settings tab.
type MCPSettingsProps struct {
	CSRFToken string

	Instructions            string
	DefaultInstructions     string
	ReviewGuidelines        string
	DefaultReviewGuidelines string
	ReportFormat            string
	DefaultReportFormat     string

	ToolGroups         []MCPSettingsToolGroup
	ToolsEnabledCount  int
	ToolsDisabledCount int
	ToolsTotalCount    int
}

// MCPSettingsClient is the embedded JSON shape used by the mcp_settings
// Alpine factory. Captured here so the handler can reuse it via
// @templ.JSONScript without crossing into the service layer.
type MCPSettingsClient struct {
	Instructions            string `json:"instructions"`
	DefaultInstructions     string `json:"defaultInstructions"`
	ReviewGuidelines        string `json:"reviewGuidelines"`
	DefaultReviewGuidelines string `json:"defaultReviewGuidelines"`
	ReportFormat            string `json:"reportFormat"`
	DefaultReportFormat     string `json:"defaultReportFormat"`
}
