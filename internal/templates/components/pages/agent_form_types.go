//go:build !headless && !lite

package pages

import (
	"strings"

	"breadbox/internal/templates/components"
)

// agentFormAvatarSrc builds the initial src for the form's live avatar
// preview tile. Edit mode seeds on the agent's slug so you see the same
// robot that appears in the list / detail / run rows; new mode (empty
// slug) uses a stable "new-agent" placeholder so the tile shows a robot
// rather than a broken image before a slug is typed. The Alpine factory
// (agentAvatarPreviewSrc) swaps this on every slug keystroke.
func agentFormAvatarSrc(slug string) string {
	seed := strings.TrimSpace(slug)
	if seed == "" {
		seed = "new-agent"
	}
	return components.AvatarURLWith(seed, "", "agent", 80)
}

// AgentFormProps is the view-model the AgentForm templ reads. Built in
// admin/agent_form_page.go from the service.AgentDefinitionResponse (edit
// mode) or empty defaults (new mode).
type AgentFormProps struct {
	// Mode is either "new" or "edit". The templ keys submit-button label,
	// header copy, and which extra mini-forms (run-now, delete) render off
	// of it.
	Mode string
	// Slug is empty in new mode; populated in edit mode and used to build
	// the action URLs (/-/agents/{slug}/update, /-/agents/{slug}/run,
	// /-/agents/{slug}/delete).
	Slug string
	// Title is the page header, e.g. "New agent" or "Edit agent <name>".
	Title string
	// Form holds the per-field string/bool/numeric values. Always populated
	// (new mode uses zero-valued + sensible defaults below). The submit
	// handler is expected to re-render with the user's typed values so the
	// form round-trips on validation errors.
	Form AgentFormFields
	// FieldErrors keys field names (matching the form name= attributes) to
	// human-readable validation messages. Rendered inline next to each
	// field.
	FieldErrors map[string]string
	// FormError is the top-of-form banner — shown when something blocks
	// submission but isn't tied to a single field (e.g. 409 duplicate
	// slug, 422 AUTH_NOT_CONFIGURED). Empty string → not shown.
	FormError string
	// ModelOptions feeds the model <select>. Order is preserved.
	ModelOptions []AgentModelOption
	// CSRFToken is rendered into every form's hidden _csrf input.
	CSRFToken string
}

// AgentFormFields is the round-trip view of the form payload. All scalar
// fields are stored as strings so the user's literal input survives a
// validation-error re-render (e.g. "abc" in a number field). The handler
// parses + validates these on submit; this struct holds the raw text.
type AgentFormFields struct {
	Name                  string
	Slug                  string
	Prompt                string
	SystemPrompt          string
	ScheduleCron          string
	ToolScope             string
	Model                 string
	AllowedTools          string // comma-separated list of MCP tool names
	QuietHoursStart       string // "HH:MM" 24-hour
	QuietHoursEnd         string
	MaxTurns              int
	MaxBudgetUSD          float64
	Enabled               bool
	TriggerOnSyncComplete bool
	// Connectors are the custom MCP servers configured on this workflow.
	// Populated in edit mode from the stored definition; empty in new mode.
	Connectors []AgentFormConnector
}

// AgentFormConnector is the form view of one custom MCP connector. The secret
// is never sent to the browser — HasSecret only signals whether one is stored,
// so the edit form can show a "leave blank to keep" affordance.
type AgentFormConnector struct {
	Name       string
	URL        string
	HeaderName string
	HasSecret  bool
}

// AgentModelOption is one entry in the model <select>.
type AgentModelOption struct {
	Value string
	Label string
}

// DefaultAgentModelOptions is the canonical list of model choices
// surfaced in the form. Keep aligned with the service.DefaultAgentModel
// constant and the agent_definitions migration default.
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

// fieldErr returns the validation message for a given field name, or empty
// string when none. Used by the templ to conditionally render an inline
// error paragraph without nil-map panics.
func (p AgentFormProps) FieldErr(name string) string {
	if p.FieldErrors == nil {
		return ""
	}
	return p.FieldErrors[name]
}
