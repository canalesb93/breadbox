package pages

import "breadbox/internal/service"

// AgentsTab is one of the four tabs the /agents composite page renders.
type AgentsTab string

const (
	AgentsTabGuide    AgentsTab = "guide"
	AgentsTabWizard   AgentsTab = "wizard"
	AgentsTabSettings AgentsTab = "settings"
	AgentsTabActivity AgentsTab = "activity"
)

// AgentsSessionRow is the projection of one MCP session for the Activity tab.
// The handler in admin/agents_page.go fills these in; the templ component
// only knows about strings/ints.
type AgentsSessionRow struct {
	ShortID         string
	Purpose         string
	APIKeyName      string
	AgentName       string
	HasReport       bool
	ToolCallCount   int64
	CreatedAtRel    string
}

// AgentsProps is the typed payload for the composite Agents() templ
// component. It contains the union of data the four old html/templates
// shared (see internal/admin/agents_page.go).
type AgentsProps struct {
	Tab AgentsTab

	// Guide tab.
	Guide MCPGuideProps

	// Wizard tab.
	Wizard AgentWizardProps

	// Settings tab.
	Settings MCPSettingsProps

	// Activity tab.
	Sessions           []AgentsSessionRow
	SessionsPage       int
	SessionsTotalPages int
}

// MCPGuideProps powers the Getting Started tab.
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

// AgentWizardProps powers the Prompt Library tab.
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

// AgentsSessionFromService projects one service.MCPSessionResponse into
// the templ-friendly AgentsSessionRow shape, using a relativeTime helper
// supplied by the caller (admin package). Kept here so the handler can
// build the slice with one .Map() call.
func AgentsSessionFromService(s service.MCPSessionResponse, rel func(string) string) AgentsSessionRow {
	return AgentsSessionRow{
		ShortID:       s.ShortID,
		Purpose:       s.Purpose,
		APIKeyName:    s.APIKeyName,
		AgentName:     s.AgentName,
		HasReport:     s.ReportID != nil && *s.ReportID != "",
		ToolCallCount: s.ToolCallCount,
		CreatedAtRel:  rel(s.CreatedAt),
	}
}
