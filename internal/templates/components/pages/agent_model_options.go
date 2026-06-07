//go:build !headless && !lite

package pages

// AgentModelOption is one entry in a model <select>.
type AgentModelOption struct {
	Value string
	Label string
}

// DefaultAgentModelOptions is the canonical list of model choices
// surfaced in workflow/agent forms. Keep aligned with the
// service.DefaultAgentModel constant and the agent_definitions migration
// default.
func DefaultAgentModelOptions() []AgentModelOption {
	return []AgentModelOption{
		{Value: "claude-opus-4-7", Label: "Claude Opus 4.7 (most capable)"},
		{Value: "claude-sonnet-4-6", Label: "Claude Sonnet 4.6 (balanced)"},
		{Value: "claude-haiku-4-5", Label: "Claude Haiku 4.5 (fastest)"},
	}
}

// AgentModelShortLabel returns a compact, human-friendly label for the
// model a run executed with — e.g. "claude-opus-4-7" → "Opus 4.7". Used
// on the run-detail page where the verbose form-option labels are too
// long. Unknown/custom IDs fall back to the raw string so disclosure
// stays honest; an empty model (runs predating the snapshot column)
// returns "" so callers can omit the affordance entirely.
func AgentModelShortLabel(model string) string {
	switch model {
	case "":
		return ""
	case "claude-opus-4-7":
		return "Opus 4.7"
	case "claude-sonnet-4-6":
		return "Sonnet 4.6"
	case "claude-haiku-4-5":
		return "Haiku 4.5"
	default:
		return model
	}
}
